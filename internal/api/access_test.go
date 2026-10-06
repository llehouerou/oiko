package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// browser does requests on srv as a browser at origin would: its Origin, and
// the Host it reached (behind a proxy, the Public URL's), with cookie if any.
func browser(t *testing.T, srv *httptest.Server, origin string) func(method, path, body string, cookie *http.Cookie) *http.Response {
	return func(method, path, body string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if origin != "" {
			req.Header.Set("Origin", origin)
			req.Host = strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
}

type me struct {
	Identity  map[string]string `json:"identity"` // {"person": id} or {"program": id}
	Name      string            `json:"name"`
	Level     access.Level      `json:"level"`
	Fresh     bool              `json:"fresh"`
	Claimed   bool              `json:"claimed"`
	PublicURL *string           `json:"publicUrl"`
}

func whoAmI(t *testing.T, resp *http.Response) me {
	t.Helper()
	var m me
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me: %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestClaimingThroughTheSetupLink(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	do := browser(t, srv, "http://localhost:8080")

	if m := whoAmI(t, do("GET", "/api/me", "", nil)); m.Claimed || m.Identity != nil || m.PublicURL != nil {
		t.Errorf("a fresh Oiko: %+v", m)
	}
	if resp := do("GET", "/setup", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /setup: %d, want the web client", resp.StatusCode)
	}
	secret, _ := acc.Setup()
	resp := do("POST", "/api/setup", `{"secret":"`+secret+`","name":"Alice"}`, nil)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/setup: %d %s", resp.StatusCode, b)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "__Host-oiko-session" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
		t.Fatalf("Session cookie = %+v, want __Host-, Secure, HttpOnly, Path=/, SameSite=Strict", cookie)
	}
	m := whoAmI(t, do("GET", "/api/me", "", cookie))
	if m.Identity["person"] == "" || m.Name != "Alice" || m.Level != access.Admin || !m.Fresh || !m.Claimed {
		t.Errorf("signed in: %+v", m)
	}
	if resp := do("POST", "/api/setup", `{"secret":"`+secret+`","name":"Mallory"}`, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("the Setup link used again: %d, want 403", resp.StatusCode)
	}

	if resp := do("POST", "/api/sign-out", "", cookie); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/sign-out: %d", resp.StatusCode)
	}
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); m.Identity != nil || !m.Claimed {
		t.Errorf("signed out: %+v", m)
	}
	// Signing out again, the Session ended, still clears its cookie.
	resp = do("POST", "/api/sign-out", "", cookie)
	if cleared := resp.Cookies(); resp.StatusCode != http.StatusNoContent || len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("signing out an ended Session: %d %v, want its cookie cleared", resp.StatusCode, cleared)
	}
}

func TestSignInEndpointsRefuseAnotherOrigin(t *testing.T) {
	public := &url.URL{Scheme: "https", Host: "oiko.example"}
	for _, c := range []struct {
		public *url.URL
		origin string
		ok     bool
	}{
		{public, "https://oiko.example", true},
		{public, "http://localhost:5180", true},
		{public, "http://localhost", true},
		{nil, "http://localhost:8080", true},
		{public, "https://evil.example", false},
		{public, "http://oiko.example", false},
		{public, "https://localhost", false},
		{public, "http://192.168.1.2:8080", false},
		{public, "", false},
		{nil, "https://oiko.example", false},
	} {
		_, acc, hdl := handlerAt(t, c.public)
		srv := httptest.NewServer(hdl)
		secret, _ := acc.Setup()
		// An empty Name: refused for its origin first, else as invalid.
		resp := browser(t, srv, c.origin)("POST", "/api/setup", `{"secret":"`+secret+`","name":""}`, nil)
		want := map[bool]int{true: http.StatusBadRequest, false: http.StatusForbidden}[c.ok]
		if resp.StatusCode != want {
			t.Errorf("Public URL %v, Origin %q: %d, want %d", c.public, c.origin, resp.StatusCode, want)
		}
		srv.Close()
	}
}

func TestWhoAmITellsThePublicURL(t *testing.T) {
	_, _, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	if m := whoAmI(t, browser(t, srv, "")("GET", "/api/me", "", nil)); m.PublicURL == nil || *m.PublicURL != "https://oiko.example" {
		t.Errorf("publicUrl = %v", m.PublicURL)
	}
}

// beforeSignIn are the endpoints Oiko served before sign-in existed, each
// with a body it takes; ids that match nothing keep the home as it is.
var beforeSignIn = []struct{ method, path, body string }{
	{"GET", "/api/updates", ""},
	{"GET", "/api/build", ""},
	{"POST", "/api/commands", `{"target": "flag:unknown", "values": {"on": true}}`},
	{"PATCH", "/api/devices/unknown", `{"name": "Lamp"}`},
	{"PUT", "/api/icon", `{"target": "flag:unknown", "icon": ""}`},
	{"PUT", "/api/area", `{"target": "flag:unknown", "area": ""}`},
	{"POST", "/api/areas", `{"name": "Office"}`},
	{"PUT", "/api/areas/unknown", `{"name": "Office"}`},
	{"DELETE", "/api/areas/unknown", ""},
	{"PUT", "/api/areas/unknown/display", `{}`},
	{"PUT", "/api/areas/unknown/layout", `{"columns": 2, "layout": []}`},
	{"PUT", "/api/areas", `{"order": []}`},
	{"DELETE", "/api/devices/unknown", ""},
	{"POST", "/api/devices/unknown/replace", `{"with": "other"}`},
	{"POST", "/api/aggregates", `{"name": "Lights"}`},
	{"PUT", "/api/aggregates/unknown", `{"name": "Lights"}`},
	{"DELETE", "/api/aggregates/unknown", ""},
	{"POST", "/api/flags", `{"name": "Away"}`},
	{"PUT", "/api/flags/unknown", `{"name": "Away"}`},
	{"DELETE", "/api/flags/unknown", ""},
	{"GET", "/api/automations", ""},
	{"POST", "/api/automations", `{"name": "Night"}`},
	{"PUT", "/api/automations/unknown", `{"name": "Night"}`},
	{"DELETE", "/api/automations/unknown", ""},
	{"POST", "/api/automations/unknown/steps/go/run", ""},
	{"GET", "/api/automations/unknown/runs", ""},
	{"DELETE", "/api/automations/unknown/runs", ""},
	{"GET", "/api/automations/unknown/state", ""},
	{"GET", "/api/runs/unknown", ""},
	{"DELETE", "/api/runs/unknown", ""},
	{"GET", "/api/commands?target=flag:unknown", ""},
	{"POST", "/api/history", `{"from": 0, "to": 1, "points": 1, "refs": []}`},
	{"POST", "/api/history/periods", `{"from": 0, "to": 1, "per": "day", "items": []}`},
	{"GET", "/api/lost-entries", ""},
}

func TestTodaysEndpointsNeedASessionOrAToken(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	_, token := withProgram(t, acc, alice, "Node-RED", access.Admin)
	do, bot := browser(t, srv, "https://oiko.example"), asProgram(t, srv, token)
	ended := &http.Cookie{Name: "__Host-oiko-session", Value: "ended"}
	for _, e := range beforeSignIn {
		for who, cookie := range map[string]*http.Cookie{"no credentials": nil, "an ended Session": ended} {
			resp := do(e.method, e.path, e.body, cookie)
			b, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(b), "sign in") {
				t.Errorf("%s %s with %s: %d %s, want 401 asking to sign in", e.method, e.path, who, resp.StatusCode, b)
			}
		}
		for who, resp := range map[string]*http.Response{
			"a Session": do(e.method, e.path, e.body, cookie),
			"a Token":   bot(e.method, e.path, e.body),
		} {
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				t.Errorf("%s %s with %s: %d, want served", e.method, e.path, who, resp.StatusCode)
			}
		}
	}
	// The web client is anyone's.
	for _, path := range []string{"/", "/setup", "/assets/index-abc.js"} {
		if resp := do("GET", path, "", nil); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with no credentials: %d", path, resp.StatusCode)
		}
	}
}

func TestWithoutAPublicURLOnlyLocalhostIsToldToSignIn(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	_, token := withProgram(t, acc, alice, "Node-RED", access.Member)
	for origin, want := range map[string]string{
		"http://localhost:8080":   "sign in first",
		"http://192.168.1.2:8080": "a Public URL must be configured",
		"http://127.0.0.1:8080":   "a Public URL must be configured",
		"https://oiko.example":    "a Public URL must be configured",
	} {
		do := browser(t, srv, origin)
		resp := do("GET", "/api/automations", "", nil)
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(b), want) {
			t.Errorf("from %s: %d %s, want 401 saying %q", origin, resp.StatusCode, b, want)
		}
		// Who am I still tells the web client which sign-in page to show.
		if m := whoAmI(t, do("GET", "/api/me", "", nil)); m.Identity != nil || m.PublicURL != nil {
			t.Errorf("who am I from %s: %+v", origin, m)
		}
	}
	if resp := asProgram(t, srv, token)("GET", "/api/automations", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("a Token from the home network: %d, want served", resp.StatusCode)
	}
}

func TestSigningOutClosesTheSessionsStream(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, _ := signedIn(t, acc)
	do := browser(t, srv, "http://localhost:8080")
	resp := do("GET", "/api/updates", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/updates: %d", resp.StatusCode)
	}
	lines := bufio.NewReader(resp.Body)
	if line, err := lines.ReadString('\n'); err != nil || !strings.HasPrefix(line, "data: ") {
		t.Fatalf("snapshot: %q, %v", line, err)
	}
	if resp := do("POST", "/api/sign-out", "", cookie); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/sign-out: %d", resp.StatusCode)
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(lines); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the stream ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream outlived its Session")
	}
	if resp := do("GET", "/api/updates", "", cookie); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a reconnect with the ended Session: %d, want 401", resp.StatusCode)
	}
}
