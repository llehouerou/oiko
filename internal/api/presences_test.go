package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

// presenceNames are the Names in a presences list, as JSON gives it.
func presenceNames(t *testing.T, m map[string]any) []string {
	t.Helper()
	var names []string
	for _, p := range m["presences"].([]any) {
		p := p.(map[string]any)
		if p["kind"] != "presence" || len(p["capabilities"].([]any)) != 2 {
			t.Errorf("presence %v", p)
		}
		names = append(names, p["name"].(string))
	}
	return names
}

// observers is Oiko's API with a Person and a Program at each level and a
// Guest Kiosk, each of whom does requests with the function it answers by
// who it is; Alice, the Admin, does hers as "an Admin's Session". It answers
// the home too.
func observers(t *testing.T) (*home.Home, map[string]func(method, path, body string) *http.Response) {
	t.Helper()
	dir := t.TempDir()
	cookies := withPersons(t, dir)
	h, acc, hdl := handlerIn(t, dir, &url.URL{Scheme: "https", Host: "oiko.example"}, time.Now)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := acc.Resolve(cookies[access.Admin].Value)
	do := browser(t, srv, "https://oiko.example")
	as := map[string]func(method, path, body string) *http.Response{}
	for _, l := range allLevels {
		cookie := cookies[l]
		_, token := withProgram(t, acc, alice, "Program "+string(l), l)
		as[fmt.Sprintf("a %s's Session", l)] = func(method, path, body string) *http.Response { return do(method, path, body, cookie) }
		as[fmt.Sprintf("a %s's Token", l)] = asProgram(t, srv, token)
	}
	_, screen := withKiosk(t, acc, alice, "Hall tablet", access.Guest)
	as["a Guest Kiosk"] = func(method, path, body string) *http.Response { return do(method, path, body, screen) }
	return h, as
}

func TestEveryoneSeesThePresences(t *testing.T) {
	_, as := observers(t)
	admin := as["a admin's Session"]
	streams := map[string]<-chan map[string]any{}
	for who, do := range as {
		streams[who] = stream(t, do)
		if got := presenceNames(t, next(t, streams[who], "snapshot")); !slices.Equal(got, []string{"Alice", "Bob", "Carol"}) {
			t.Errorf("%s snapshot: presences %v", who, got)
		}
	}

	dan := decodeAs[struct{ ID string }](t, admin("POST", "/api/persons", `{"name": "Dan", "level": "member"}`), http.StatusCreated).ID
	for who, s := range streams {
		if got := presenceNames(t, next(t, s, "presences")); !slices.Equal(got, []string{"Alice", "Bob", "Carol", "Dan"}) {
			t.Errorf("%s once Dan is created: %v", who, got)
		}
	}
	if resp := admin("PUT", "/api/persons/"+dan, `{"name": "Aaron", "level": "member"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("renaming Dan: %d", resp.StatusCode)
	}
	for who, s := range streams {
		if got := presenceNames(t, next(t, s, "presences")); !slices.Equal(got, []string{"Aaron", "Alice", "Bob", "Carol"}) {
			t.Errorf("%s once Dan is Aaron: %v", who, got)
		}
	}
	if resp := as["a member's Session"]("PUT", "/api/me", `{"name": "Bobby"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Bob renaming himself: %d", resp.StatusCode)
	}
	for who, s := range streams {
		if got := presenceNames(t, next(t, s, "presences")); !slices.Equal(got, []string{"Aaron", "Alice", "Bobby", "Carol"}) {
			t.Errorf("%s once Bob is Bobby: %v", who, got)
		}
	}
	if resp := admin("DELETE", "/api/persons/"+dan, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("removing Aaron: %d", resp.StatusCode)
	}
	for who, s := range streams {
		if m := next(t, s, "deleted"); m["target"] != "presence:"+dan {
			t.Errorf("%s: deleted %v", who, m)
		}
		if got := presenceNames(t, next(t, s, "presences")); !slices.Equal(got, []string{"Alice", "Bobby", "Carol"}) {
			t.Errorf("%s once Aaron is removed: %v", who, got)
		}
	}
}

func TestTheFirstAdminHasAPresenceFromSetup(t *testing.T) {
	h, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	secret, _ := acc.Setup()
	if resp := browser(t, srv, "https://oiko.example")("POST", "/api/setup", fmt.Sprintf(`{"secret": %q, "name": "Alice"}`, secret), nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("setup: %d", resp.StatusCode)
	}
	snap, _, cancel := h.Subscribe()
	cancel()
	if len(snap.Presences) != 1 || snap.Presences[0].Name != "Alice" {
		t.Errorf("presences %+v, want Alice's", snap.Presences)
	}
}

func TestAPresenceIsCommandedLikeAnyTarget(t *testing.T) {
	_, as := observers(t)
	alice := "presence:p2" // as withPersons names her
	for _, who := range []string{"a guest's Session", "a guest's Token", "a Guest Kiosk"} {
		resp := as[who]("POST", "/api/commands", fmt.Sprintf(`{"target": %q, "values": {"home": "toggle"}}`, alice))
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("a Command from %s: %d", who, resp.StatusCode)
		}
	}
	if resp := as["a guest's Session"]("POST", "/api/commands", fmt.Sprintf(`{"target": %q, "values": {"manual": false}}`, alice)); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("setting manual: %d, want refused", resp.StatusCode)
	}

	from, to := time.Now().Add(-48*time.Hour).UnixMilli(), time.Now().Add(time.Hour).UnixMilli()
	history := fmt.Sprintf(`{"from": %d, "to": %d, "points": 10, "refs": [{"target": %q, "capability": "home"}, {"target": %q, "capability": "manual"}]}`, from, to, alice, alice)
	periods := fmt.Sprintf(`{"from": %d, "to": %d, "per": "day", "items": [{"ref": {"target": %q, "capability": "home"}, "measure": "time_on"}]}`, from, to, alice)
	for path, body := range map[string]string{"/api/history": history, "/api/history/periods": periods} {
		if resp := as["a guest's Session"]("POST", path, body); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s to a Guest: %d, want refused", path, resp.StatusCode)
		}
		if resp := as["a member's Session"]("POST", path, body); resp.StatusCode != http.StatusOK {
			t.Errorf("%s to a Member: %d", path, resp.StatusCode)
		}
	}
	if resp := as["a member's Session"]("GET", "/api/commands?target="+alice, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("the Command history to a Member: %d", resp.StatusCode)
	}
}

// sourcesIn are Person id's Presence sources in a presences list, as JSON
// gives it.
func sourcesIn(m map[string]any, id string) []any {
	for _, p := range m["presences"].([]any) {
		if p := p.(map[string]any); p["id"] == id {
			return p["sources"].([]any)
		}
	}
	return nil
}

func TestAnAdminBindsPresenceSources(t *testing.T) {
	h, as := observers(t)
	admin := as["a admin's Session"]
	f, err := h.CreateFlag("Alice home")
	if err != nil {
		t.Fatal(err)
	}
	flag := home.TargetFlag(f).Key()
	guest := stream(t, as["a guest's Session"])
	next(t, guest, "snapshot")

	body := fmt.Sprintf(`{"sources": [{"target": %q, "departureDelay": 60}, {"target": %q}]}`, flag, flag)
	for who, do := range as {
		want := http.StatusForbidden
		if who == "a admin's Session" || who == "a admin's Token" {
			want = http.StatusNoContent
		}
		if resp := do("PUT", "/api/presences/p2", body); resp.StatusCode != want {
			t.Errorf("%s binding Alice's sources: %d, want %d", who, resp.StatusCode, want)
		}
	}
	if got := sourcesIn(next(t, guest, "presences"), "p2"); len(got) != 1 || got[0].(map[string]any)["target"] != flag || got[0].(map[string]any)["departureDelay"] != 60.0 {
		t.Errorf("once bound: sources %v", got)
	}

	for body, want := range map[string]int{
		`{"sources": [{"target": "flag:unknown"}]}`:                              http.StatusNotFound,
		fmt.Sprintf(`{"sources": [{"target": %q, "departureDelay": -1}]}`, flag): http.StatusBadRequest,
		`{"sources": [{"target": "presence:p1"}]}`:                               http.StatusBadRequest,
		`{"sources": [{"target": "nonsense"}]}`:                                  http.StatusBadRequest,
		`{"sources": [{"target": "flag:x", "departureDelay": 1.5}]}`:             http.StatusBadRequest,
	} {
		if resp := admin("PUT", "/api/presences/p2", body); resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", body, resp.StatusCode, want)
		}
	}
	if resp := admin("PUT", "/api/presences/nobody", `{"sources": []}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("no such Person: %d", resp.StatusCode)
	}
	for _, method := range []string{"POST", "DELETE"} {
		if resp := admin(method, "/api/presences/p2", ""); resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d, want none", method, resp.StatusCode)
		}
	}

	if resp := admin("DELETE", "/api/flags/"+string(f), ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("deleting the Flag: %d", resp.StatusCode)
	}
	next(t, guest, "deleted") // past the second binding's list
	if got := sourcesIn(next(t, guest, "presences"), "p2"); len(got) != 0 {
		t.Errorf("once the Flag is deleted: sources %v", got)
	}
}
