package api

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

// homeEndpoints are the endpoints of the home, each with a body it takes and
// the Access level it needs; ids that match nothing keep the home as it is.
var homeEndpoints = []struct {
	method, path, body string
	level              access.Level
}{
	{"GET", "/api/updates", "", access.Guest},
	{"POST", "/api/commands", `{"target": "flag:unknown", "values": {"on": true}}`, access.Guest},
	{"POST", "/api/automations/unknown/steps/go/run", "", access.Guest},
	{"GET", "/api/automations/tiles", "", access.Guest},
	{"GET", "/api/automations", "", access.Member},
	{"GET", "/api/automations/unknown/state", "", access.Member},
	{"GET", "/api/automations/unknown/runs", "", access.Member},
	{"GET", "/api/runs/unknown", "", access.Member},
	{"GET", "/api/commands?target=flag:unknown", "", access.Member},
	{"POST", "/api/history", `{"from": 0, "to": 1, "points": 1, "refs": []}`, access.Member},
	{"POST", "/api/history/periods", `{"from": 0, "to": 1, "per": "day", "items": []}`, access.Member},
	{"GET", "/api/lost-entries", "", access.Member},
	{"GET", "/api/build", "", access.Admin},
	{"PATCH", "/api/devices/unknown", `{"name": "Lamp"}`, access.Admin},
	{"PUT", "/api/icon", `{"target": "flag:unknown", "icon": ""}`, access.Admin},
	{"PUT", "/api/area", `{"target": "flag:unknown", "area": ""}`, access.Admin},
	{"POST", "/api/areas", `{"name": "Office"}`, access.Admin},
	{"PUT", "/api/areas/unknown", `{"name": "Office"}`, access.Admin},
	{"DELETE", "/api/areas/unknown", "", access.Admin},
	{"PUT", "/api/areas/unknown/display", `{}`, access.Admin},
	{"PUT", "/api/areas/unknown/layout", `{"columns": 2, "layout": []}`, access.Admin},
	{"PUT", "/api/areas", `{"order": []}`, access.Admin},
	{"DELETE", "/api/devices/unknown", "", access.Admin},
	{"POST", "/api/devices/unknown/replace", `{"with": "other"}`, access.Admin},
	{"POST", "/api/aggregates", `{"name": "Lights"}`, access.Admin},
	{"PUT", "/api/aggregates/unknown", `{"name": "Lights"}`, access.Admin},
	{"DELETE", "/api/aggregates/unknown", "", access.Admin},
	{"POST", "/api/flags", `{"name": "Away"}`, access.Admin},
	{"PUT", "/api/flags/unknown", `{"name": "Away"}`, access.Admin},
	{"DELETE", "/api/flags/unknown", "", access.Admin},
	{"POST", "/api/automations", `{"name": "Night"}`, access.Admin},
	{"PUT", "/api/automations/unknown", `{"name": "Night"}`, access.Admin},
	{"DELETE", "/api/automations/unknown", "", access.Admin},
	{"DELETE", "/api/automations/unknown/runs", "", access.Admin},
	{"DELETE", "/api/runs/unknown", "", access.Admin},
}

var allLevels = []access.Level{access.Guest, access.Member, access.Admin}

// withPersons writes into dir, before its access store opens, a Person at
// each level, each signed in: Alice the Admin, Bob the Member, Carol the
// Guest. It answers their Session cookies by level.
func withPersons(t *testing.T, dir string) map[access.Level]*http.Cookie {
	t.Helper()
	type session struct {
		Hash     string    `json:"hash"`
		Person   string    `json:"person"`
		Browser  string    `json:"browser"`
		Method   string    `json:"method"`
		SignedIn time.Time `json:"signedIn"`
		LastUse  time.Time `json:"lastUse"`
	}
	var persons []access.Person
	var sessions []session
	cookies := map[access.Level]*http.Cookie{}
	for i, name := range []string{"Carol", "Bob", "Alice"} {
		p := access.Person{ID: fmt.Sprintf("p%d", i), Name: name, Level: allLevels[i]}
		secret := "secret-of-" + name
		sum := sha256.Sum256([]byte(secret))
		persons = append(persons, p)
		sessions = append(sessions, session{hex.EncodeToString(sum[:]), p.ID, "", "setup", time.Now(), time.Now()})
		cookies[p.Level] = &http.Cookie{Name: "__Host-oiko-session", Value: secret}
	}
	if err := store.Save(filepath.Join(dir, "persons.json"), access.PersonsFormat, persons); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(filepath.Join(dir, "sessions.json"), access.SessionsFormat, sessions); err != nil {
		t.Fatal(err)
	}
	return cookies
}

// levelled is Oiko's API with a Person and a Program at each level: it
// answers the home and, by level, how to do a request as each.
func levelled(t *testing.T) (*home.Home, map[access.Level]map[string]func(method, path, body string) *http.Response) {
	t.Helper()
	dir := t.TempDir()
	cookies := withPersons(t, dir)
	h, acc, hdl := handlerIn(t, dir, &url.URL{Scheme: "https", Host: "oiko.example"}, time.Now)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := acc.Resolve(cookies[access.Admin].Value)
	do := browser(t, srv, "https://oiko.example")
	as := map[access.Level]map[string]func(method, path, body string) *http.Response{}
	for _, l := range allLevels {
		_, token := withProgram(t, acc, alice, "Program "+string(l), l)
		cookie := cookies[l]
		as[l] = map[string]func(method, path, body string) *http.Response{
			"Session": func(method, path, body string) *http.Response { return do(method, path, body, cookie) },
			"Token":   asProgram(t, srv, token),
		}
	}
	return h, as
}

func TestEachEndpointServesTheLevelsItNeeds(t *testing.T) {
	_, as := levelled(t)
	for _, e := range homeEndpoints {
		for _, l := range allLevels {
			for by, do := range as[l] {
				resp := do(e.method, e.path, e.body)
				refused := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized
				if want := !l.Allows(e.level); refused != want || (want && resp.StatusCode != http.StatusForbidden) {
					t.Errorf("%s %s by a %s's %s: %d, want refused %v", e.method, e.path, l, by, resp.StatusCode, want)
				}
			}
		}
	}
}

func TestSettingAConfigurationCapabilityIsAnAdmins(t *testing.T) {
	h, as := levelled(t)
	port := h.Attach("z2m", nopBridge{})
	port.SetOnline(true)
	settable := home.Access{Observable: true, Settable: true}
	port.SyncDevices([]bridge.Device{{NativeAddress: "0x1", Name: "Lamp", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []home.Capability{
		{Key: "state", Label: "State", Type: home.Binary, Category: home.Primary, Access: settable},
		{Key: "power_on_behavior", Label: "Power-on behaviour", Type: home.Enum, Options: []string{"on", "off", "previous"}, Category: home.Config, Access: settable},
	}}}}})
	snap, _, cancel := h.Subscribe()
	cancel()
	lamp := home.TargetDevice(snap.Devices[0].ID, "light")
	for _, l := range allLevels {
		for by, do := range as[l] {
			for values, want := range map[string]int{
				`{"state": true}`:                   http.StatusAccepted,
				`{"power_on_behavior": "previous"}`: map[bool]int{true: http.StatusAccepted, false: http.StatusForbidden}[l == access.Admin],
			} {
				resp := do("POST", "/api/commands", fmt.Sprintf(`{"target": %q, "values": %s}`, lamp, values))
				if resp.StatusCode != want {
					b, _ := io.ReadAll(resp.Body)
					t.Errorf("%s by a %s's %s: %d %s, want %d", values, l, by, resp.StatusCode, b, want)
				}
			}
		}
	}
}

// stream opens the event stream with do, answering its messages as they come.
func stream(t *testing.T, do func(method, path, body string) *http.Response) <-chan map[string]any {
	t.Helper()
	resp := do("GET", "/api/updates", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/updates: %d", resp.StatusCode)
	}
	msgs := make(chan map[string]any, 100)
	go func() {
		defer close(msgs)
		lines := bufio.NewScanner(resp.Body)
		lines.Buffer(nil, 1<<20)
		for lines.Scan() {
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				var m map[string]any
				json.Unmarshal([]byte(data), &m)
				msgs <- m
			}
		}
	}()
	return msgs
}

// next is the next message of msgs of kind, or of any kind with "".
func next(t *testing.T, msgs <-chan map[string]any, kind string) map[string]any {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-msgs:
			if !ok {
				t.Fatalf("the stream ended before a %q", kind)
			}
			if kind == "" || m["kind"] == kind {
				return m
			}
		case <-timeout:
			t.Fatalf("no %q message", kind)
		}
	}
}

func TestAGuestSeesNeitherOriginsNorWhoStartedARun(t *testing.T) {
	h, as := levelled(t)
	flag, _ := h.CreateFlag("Away")
	admin, member, guest := as[access.Admin]["Session"], as[access.Member]["Token"], as[access.Guest]
	auto := decodeAs[struct{ ID string }](t, admin("POST", "/api/automations", fmt.Sprintf(`{"name": "Leave", "enabled": true, "steps": [
		{"id": "go", "kind": "manualTrigger", "name": "Go", "params": {}},
		{"id": "set", "kind": "command", "params": {"targets": ["flag:%s"], "values": {"on": true}}}
	], "edges": [{"from": {"step": "go", "handle": "out"}, "to": {"step": "set", "handle": "in"}}]}`, flag)), http.StatusCreated)
	streams := map[string]<-chan map[string]any{
		"a Member's":        stream(t, member),
		"a Guest's":         stream(t, guest["Session"]),
		"a Guest Program's": stream(t, guest["Token"]),
	}
	for _, s := range streams {
		next(t, s, "snapshot")
	}

	// A Guest starts the Run: its answer does not say who did.
	resp := guest["Session"]("POST", "/api/automations/"+auto.ID+"/steps/go/run", "")
	if end := decodeAs[map[string]any](t, resp, http.StatusOK); end["trigger"].(map[string]any)["by"] != nil {
		t.Errorf("the Run's end, to the Guest who started it: %v", end)
	}
	for who, s := range streams {
		guestly := strings.Contains(who, "Guest")
		for seen := map[string]bool{}; !seen["run"] || !seen["command"]; {
			m := next(t, s, "")
			switch m["kind"] {
			case "run":
				if by := m["run"].(map[string]any)["trigger"].(map[string]any)["by"]; (by == nil) != guestly {
					t.Errorf("%s stream: the Run started by %v", who, by)
				}
			case "command":
				if origin, has := m["command"].(map[string]any)["origin"]; has == guestly {
					t.Errorf("%s stream: the Run's Command from %v", who, origin)
				}
			}
			seen[m["kind"].(string)] = true
		}
	}
	// The Guests' Tiles show the Manual trigger, not how the Automation is built.
	tiles := decodeAs[[]map[string]any](t, guest["Session"]("GET", "/api/automations/tiles", ""), http.StatusOK)
	want := `[{"id":"` + auto.ID + `","manualTriggers":[{"name":"Go","step":"go"}],"name":"Leave","status":"enabled"}]`
	if got, _ := json.Marshal(tiles); string(got) != want {
		t.Errorf("tiles = %s, want %s", got, want)
	}
}

func TestAProgramsStreamClosesWhenItsLevelChanges(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	p, token := withProgram(t, acc, alice, "Node-RED", access.Member)
	msgs := stream(t, asProgram(t, srv, token))
	next(t, msgs, "snapshot")
	if err := acc.EditProgram(alice, p.ID, "Node-RED", access.Guest); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-msgs:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("the stream outlived the Program's level")
		}
	}
}
