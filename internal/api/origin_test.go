package api

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

// withProgram creates a Program named name at level for Admin by, and
// answers it with its Token.
func withProgram(t *testing.T, acc *access.Store, by access.Identity, name string, level access.Level) (access.Program, string) {
	t.Helper()
	p, err := acc.CreateProgram(by, name, level)
	if err != nil {
		t.Fatal(err)
	}
	token, err := acc.GenerateToken(by, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p, token
}

func TestACommandRecordsWhoIssuedIt(t *testing.T) {
	h, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	p, token := withProgram(t, acc, alice, "Node-RED", access.Member)
	kiosk, screen := withKiosk(t, acc, alice, "Hall tablet", access.Guest)
	do, bot := browser(t, srv, "https://oiko.example"), asProgram(t, srv, token)
	flag, _ := h.CreateFlag("Away")
	_, updates, cancel := h.Subscribe()
	defer cancel()

	body := fmt.Sprintf(`{"target": %q, "values": {"on": true}}`, home.TargetFlag(flag))
	for _, c := range []struct {
		who  string
		resp *http.Response
		want home.Origin
	}{
		{"a Session", do("POST", "/api/commands", body, cookie), home.Origin{Person: alice.ID}},
		{"a Token", bot("POST", "/api/commands", body), home.Origin{Program: p.ID}},
		{"a Kiosk", do("POST", "/api/commands", body, screen), home.Origin{Kiosk: kiosk}},
	} {
		id := decodeAs[struct{ ID string }](t, c.resp, http.StatusAccepted).ID
		if got := originOf(t, updates, id); got != c.want {
			t.Errorf("from %s: origin %+v, want %+v", c.who, got, c.want)
		}
	}
}

// originOf reads updates up to Command id's first, answering its Origin.
func originOf(t *testing.T, updates <-chan home.Update, id string) home.Origin {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case u := <-updates:
			if u.Kind == home.CommandChanged && u.Command.ID == id {
				return u.Command.Origin
			}
		case <-timeout:
			t.Fatalf("no Update of Command %s", id)
		}
	}
}

func TestAManualTriggerRecordsWhoStartedIt(t *testing.T) {
	h, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	do := browser(t, srv, "https://oiko.example")
	flag, _ := h.CreateFlag("Away")
	auto := decodeAs[struct{ ID string }](t, do("POST", "/api/automations", fmt.Sprintf(`{"name": "Leave", "enabled": true, "steps": [
		{"id": "go", "kind": "manualTrigger", "params": {}},
		{"id": "set", "kind": "command", "params": {"targets": ["flag:%s"], "values": {"on": true}}}
	], "edges": [{"from": {"step": "go", "handle": "out"}, "to": {"step": "set", "handle": "in"}}]}`, flag), cookie), http.StatusCreated)
	enabled(t, h, auto.ID)

	end := decodeAs[home.RunEnd](t, do("POST", "/api/automations/"+auto.ID+"/steps/go/run", "", cookie), http.StatusOK)
	alices := home.Origin{Person: alice.ID}
	if end.Trigger.By == nil || *end.Trigger.By != alices {
		t.Errorf("the Run's end: started by %v, want %v", end.Trigger.By, alices)
	}
	trace := eventually(t, "the Trace", func() (automation.Trace, bool) {
		resp := do("GET", "/api/runs/"+end.Run.String(), "", cookie)
		if resp.StatusCode != http.StatusOK {
			return automation.Trace{}, false
		}
		return decodeAs[automation.Trace](t, resp, http.StatusOK), true
	})
	if trace.Trigger.By == nil || *trace.Trigger.By != alices {
		t.Errorf("the Trace: started by %v, want %v", trace.Trigger.By, alices)
	}
}

func TestNamesAreReadFromMemberUp(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	member, memberToken := withProgram(t, acc, alice, "Node-RED", access.Member)
	guest, guestToken := withProgram(t, acc, alice, "Doorbell", access.Guest)
	do := browser(t, srv, "https://oiko.example")

	want := map[string]map[string]string{
		"person":  {alice.ID: "Alice"},
		"kiosk":   {},
		"program": {member.ID: "Node-RED", guest.ID: "Doorbell"},
	}
	for who, resp := range map[string]*http.Response{
		"an Admin Person":  do("GET", "/api/names", "", cookie),
		"a Member Program": asProgram(t, srv, memberToken)("GET", "/api/names", ""),
	} {
		got := decodeAs[map[string]map[string]string](t, resp, http.StatusOK)
		if !maps.EqualFunc(got, want, maps.Equal) {
			t.Errorf("names for %s = %v, want %v", who, got, want)
		}
	}
	if resp := asProgram(t, srv, guestToken)("GET", "/api/names", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("names for a Guest Program: %d, want 403", resp.StatusCode)
	}
}
