package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	Identity  *struct{ Person string } `json:"identity"`
	Name      string                   `json:"name"`
	Level     access.Level             `json:"level"`
	Fresh     bool                     `json:"fresh"`
	Claimed   bool                     `json:"claimed"`
	PublicURL *string                  `json:"publicUrl"`
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
	if m.Identity == nil || m.Identity.Person == "" || m.Name != "Alice" || m.Level != access.Admin || !m.Fresh || !m.Claimed {
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

func TestTodaysEndpointsServeASessionAndNoSession(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	do := browser(t, srv, "http://localhost:8080")
	secret, _ := acc.Setup()
	session, err := acc.Claim(secret, "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range []*http.Cookie{nil, {Name: "__Host-oiko-session", Value: session}, {Name: "__Host-oiko-session", Value: "ended"}} {
		for _, path := range []string{"/api/automations", "/api/build", "/api/lost-entries"} {
			if resp := do("GET", path, "", cookie); resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s with %v: %d", path, cookie, resp.StatusCode)
			}
		}
	}
}
