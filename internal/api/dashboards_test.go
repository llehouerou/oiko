package api

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// requester does a request as one identity.
type requester = func(method, path, body string) *http.Response

// identities is Oiko's API with a Person at each level, signed in, and a
// Kiosk and a Program: it answers how to do a request as each, by name.
func identities(t *testing.T) map[string]requester {
	t.Helper()
	dir := t.TempDir()
	cookies := withPersons(t, dir)
	_, acc, hdl := handlerIn(t, dir, &url.URL{Scheme: "https", Host: "oiko.example"}, time.Now)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := acc.Resolve(cookies[access.Admin].Value)
	_, kiosk := withKiosk(t, acc, alice, "Hall tablet", access.Member)
	_, token := withProgram(t, acc, alice, "Script", access.Admin)
	do := browser(t, srv, "https://oiko.example")
	as := func(c *http.Cookie) requester {
		return func(method, path, body string) *http.Response { return do(method, path, body, c) }
	}
	return map[string]requester{
		"Carol": as(cookies[access.Guest]), "Bob": as(cookies[access.Member]), "Alice": as(cookies[access.Admin]),
		"Kiosk": as(kiosk), "Program": asProgram(t, srv, token),
	}
}

// names are the Names of the Dashboards in a snapshot or a dashboards message.
func names(m map[string]any) []string {
	var ns []string
	ds, _ := m["dashboards"].([]any)
	for _, d := range ds {
		ns = append(ns, d.(map[string]any)["name"].(string))
	}
	return ns
}

func TestEachPersonKeepsTheirOwnDashboardsAndNothingElseDoes(t *testing.T) {
	as := identities(t)
	for _, who := range []string{"Carol", "Bob", "Alice"} {
		do := as[who]
		created := decodeAs[struct{ ID string }](t, do("POST", "/api/dashboards", `{"name": "`+who+`'s"}`), http.StatusCreated)
		read(t, do("PUT", "/api/dashboards/"+created.ID, `{"name": "`+who+`'s evening"}`), http.StatusNoContent)
		for _, other := range []string{"Carol", "Bob", "Alice"} {
			if other != who {
				read(t, as[other]("PUT", "/api/dashboards/"+created.ID, `{"name": "Mine"}`), http.StatusForbidden)
				read(t, as[other]("DELETE", "/api/dashboards/"+created.ID, ""), http.StatusForbidden)
			}
		}
		for _, other := range []string{"Kiosk", "Program"} {
			read(t, as[other]("POST", "/api/dashboards", `{"name": "Wall"}`), http.StatusForbidden)
			read(t, as[other]("PUT", "/api/dashboards/"+created.ID, `{"name": "Wall"}`), http.StatusForbidden)
			read(t, as[other]("DELETE", "/api/dashboards/"+created.ID, ""), http.StatusForbidden)
		}
		read(t, do("DELETE", "/api/dashboards/"+created.ID, ""), http.StatusNoContent)
	}
	read(t, as["Bob"]("POST", "/api/dashboards", `{"name": " "}`), http.StatusBadRequest)
	read(t, as["Bob"]("PUT", "/api/dashboards/unknown", `{"name": "Evening"}`), http.StatusNotFound)
}

func TestAPersonsDashboardsReachEachOfTheirStreamsAndOnlyTheirs(t *testing.T) {
	as := identities(t)
	bob, again, carol, kiosk := stream(t, as["Bob"]), stream(t, as["Bob"]), stream(t, as["Carol"]), stream(t, as["Kiosk"])
	// Before any, a Person's snapshot has none, and a Kiosk's no place for them.
	for name, msgs := range map[string]<-chan map[string]any{"Bob's": bob, "Bob's other": again, "Carol's": carol, "the Kiosk's": kiosk} {
		ds, has := next(t, msgs, "snapshot")["dashboards"]
		if want := name != "the Kiosk's"; has != want || has && len(ds.([]any)) != 0 {
			t.Fatalf("%s snapshot before any: %v %v", name, has, ds)
		}
	}

	created := decodeAs[struct{ ID string }](t, as["Bob"]("POST", "/api/dashboards", `{"name": "Evening"}`), http.StatusCreated)
	for _, msgs := range []<-chan map[string]any{bob, again} {
		if ns := names(next(t, msgs, "dashboards")); len(ns) != 1 || ns[0] != "Evening" {
			t.Errorf("Bob's stream after creating one: %v", ns)
		}
	}
	read(t, as["Bob"]("PUT", "/api/dashboards/"+created.ID, `{"name": "Night"}`), http.StatusNoContent)
	if ns := names(next(t, again, "dashboards")); len(ns) != 1 || ns[0] != "Night" {
		t.Errorf("Bob's other stream after a save: %v", ns)
	}
	if ns := names(next(t, stream(t, as["Bob"]), "snapshot")); len(ns) != 1 || ns[0] != "Night" {
		t.Errorf("Bob's snapshot: %v", ns)
	}
	read(t, as["Carol"]("POST", "/api/dashboards", `{"name": "Carol's"}`), http.StatusCreated)
	if ns := names(next(t, carol, "dashboards")); len(ns) != 1 || ns[0] != "Carol's" {
		t.Errorf("Carol's stream: %v", ns)
	}
	if ns := names(next(t, stream(t, as["Carol"]), "snapshot")); len(ns) != 1 || ns[0] != "Carol's" {
		t.Errorf("Carol's snapshot: %v", ns)
	}
	if ns := names(next(t, stream(t, as["Kiosk"]), "snapshot")); ns != nil {
		t.Errorf("a Kiosk's snapshot: %v", ns)
	}
	// Bob's streams heard of Carol's no more than the Kiosk's did of either.
	next(t, bob, "dashboards") // Night
	for name, msgs := range map[string]<-chan map[string]any{"Bob's": bob, "the Kiosk's": kiosk} {
		timeout := time.After(100 * time.Millisecond)
	quiet:
		for {
			select {
			case m := <-msgs:
				if m["kind"] == "dashboards" {
					t.Errorf("%s stream got %v", name, m)
				}
			case <-timeout:
				break quiet
			}
		}
	}
}

func TestAnAdminKeepsSharedDashboardsForEveryPerson(t *testing.T) {
	as := identities(t)
	persons := []string{"Carol", "Bob", "Alice"}
	streams := map[string]<-chan map[string]any{}
	for _, who := range persons {
		streams[who] = stream(t, as[who])
		next(t, streams[who], "snapshot")
	}
	everyoneGets := func(what string, want ...string) {
		t.Helper()
		for _, who := range persons {
			if ns := names(next(t, streams[who], "dashboards")); !slices.Equal(ns, want) {
				t.Errorf("%s's stream after %s: %v, want %v", who, what, ns, want)
			}
		}
	}

	// Only an Admin creates one; shared or personal, a document decides once.
	for _, who := range []string{"Carol", "Bob", "Kiosk", "Program"} {
		read(t, as[who]("POST", "/api/dashboards", `{"shared": true, "name": "Evening"}`), http.StatusForbidden)
	}
	created := decodeAs[struct{ ID string }](t, as["Alice"]("POST", "/api/dashboards", `{"shared": true, "name": "Evening"}`), http.StatusCreated)
	everyoneGets("creating one", "Evening")

	// Only an Admin saves or deletes it.
	for _, who := range []string{"Carol", "Bob", "Kiosk", "Program"} {
		read(t, as[who]("PUT", "/api/dashboards/"+created.ID, `{"name": "Mine"}`), http.StatusForbidden)
		read(t, as[who]("DELETE", "/api/dashboards/"+created.ID, ""), http.StatusForbidden)
	}
	read(t, as["Alice"]("PUT", "/api/dashboards/"+created.ID, `{"shared": false, "name": "Night"}`), http.StatusNoContent)
	everyoneGets("saving it", "Night")
	for _, who := range persons {
		snap := next(t, stream(t, as[who]), "snapshot")
		if d := snap["dashboards"].([]any)[0].(map[string]any); d["shared"] != true || d["owner"] != nil {
			t.Errorf("%s's snapshot: %v", who, d)
		}
	}
	read(t, as["Alice"]("DELETE", "/api/dashboards/"+created.ID, ""), http.StatusNoContent)
	everyoneGets("deleting it")
}

func TestAGuestSeesOfASharedDashboardWhatTheyMayPress(t *testing.T) {
	as := identities(t)
	alice, carol := as["Alice"], as["Carol"]
	steps := func(trigger string) string {
		return `[{"id": "go", "kind": "` + trigger + `", "name": "Go", "params": {}}]`
	}
	leave := decodeAs[struct{ ID string }](t, alice("POST", "/api/automations", `{"name": "Leave", "steps": `+steps("manualTrigger")+`}`), http.StatusCreated).ID
	night := decodeAs[struct{ ID string }](t, alice("POST", "/api/automations", `{"name": "Night"}`), http.StatusCreated).ID
	watch := stream(t, alice) // once both reach a stream, the Dashboards know them too
	for m := next(t, watch, "snapshot"); len(m["automations"].([]any)) < 2; m = next(t, watch, "automations") {
	}
	section := `{"name": "E", "sections": [
		{"id": "both", "columns": 2, "col": 0, "row": 0, "width": 1, "tiles": [
			{"automation": "` + leave + `", "col": 0, "row": 0, "width": 1},
			{"automation": "` + night + `", "col": 1, "row": 0, "width": 1}]},
		{"id": "night", "columns": 1, "col": 1, "row": 0, "width": 1, "tiles": [
			{"automation": "` + night + `", "col": 0, "row": 0, "width": 1}]}]}`
	shared := strings.Replace(section, `"name": "E"`, `"shared": true, "name": "Shared"`, 1)
	read(t, alice("POST", "/api/dashboards", shared), http.StatusCreated)
	read(t, carol("POST", "/api/dashboards", strings.Replace(section, `"E"`, `"Carol's"`, 1)), http.StatusCreated)

	// tiles are the Automations of each Section of each Dashboard, by Name.
	tiles := func(m map[string]any) map[string]string {
		byName := map[string]string{}
		for _, d := range m["dashboards"].([]any) {
			var s []string
			for _, sec := range d.(map[string]any)["sections"].([]any) {
				sec := sec.(map[string]any)
				s = append(s, sec["id"].(string)+":")
				ts, _ := sec["tiles"].([]any)
				for _, tile := range ts {
					s = append(s, map[string]string{leave: "leave", night: "night"}[tile.(map[string]any)["automation"].(string)])
				}
			}
			byName[d.(map[string]any)["name"].(string)] = strings.Join(s, " ")
		}
		return byName
	}
	whole := "both: leave night night: night"
	if got := tiles(next(t, stream(t, carol), "snapshot")); !maps.Equal(got, map[string]string{"Shared": "both: leave night:", "Carol's": whole}) {
		t.Errorf("a Guest's snapshot: %v", got)
	}
	for _, who := range []string{"Bob", "Alice"} {
		if got := tiles(next(t, stream(t, as[who]), "snapshot")); !maps.Equal(got, map[string]string{"Shared": whole}) {
			t.Errorf("%s's snapshot: %v", who, got)
		}
	}

	// An Automation given a Manual trigger shows to a Guest at once.
	guest := stream(t, carol)
	next(t, guest, "snapshot")
	read(t, alice("PUT", "/api/automations/"+night, `{"name": "Night", "steps": `+steps("manualTrigger")+`}`), http.StatusNoContent)
	if got := tiles(next(t, guest, "dashboards")); got["Shared"] != whole {
		t.Errorf("a Guest's stream once Night has a Manual trigger: %v", got)
	}
}
