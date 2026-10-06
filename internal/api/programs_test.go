package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// signedIn claims the Oiko of acc for Alice, an Admin, answering her
// Session's cookie and her identity.
func signedIn(t *testing.T, acc *access.Store) (*http.Cookie, access.Identity) {
	t.Helper()
	secret, _ := acc.Setup()
	session, err := acc.Claim(secret, "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := acc.Resolve(session)
	return &http.Cookie{Name: "__Host-oiko-session", Value: session}, id
}

// asProgram does requests on srv as a Program holding token would, from the
// home network.
func asProgram(t *testing.T, srv *httptest.Server, token string) func(method, path, body string) *http.Response {
	return func(method, path, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Host = "192.168.1.2:8080"
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
}

func decodeAs[T any](t *testing.T, resp *http.Response, status int) T {
	t.Helper()
	var v T
	if resp.StatusCode != status {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: %d %s, want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, b, status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAnAdminManagesAProgramAndItsToken(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	do := browser(t, srv, "https://oiko.example")

	p := decodeAs[struct{ ID string }](t, do("POST", "/api/programs", `{"name":"Node-RED","level":"member"}`, cookie), http.StatusCreated)
	token := decodeAs[struct{ Token string }](t, do("POST", "/api/programs/"+p.ID+"/token", "", cookie), http.StatusCreated).Token
	if !strings.HasPrefix(token, "oiko_") {
		t.Fatalf("Token %q", token)
	}

	// Served as the Program, on any address, by its Token.
	bot := asProgram(t, srv, token)
	if m := whoAmI(t, bot("GET", "/api/me", "")); len(m.Identity) != 1 || m.Identity["program"] != p.ID || m.Name != "Node-RED" || m.Level != access.Member {
		t.Errorf("who am I, by Token: %+v", m)
	}

	resp := do("GET", "/api/programs", "", cookie)
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "hash") || strings.Contains(string(body), strings.TrimPrefix(token, "oiko_")) {
		t.Errorf("GET /api/programs shows the Token: %s", body)
	}
	var list []struct {
		ID, Name string
		Level    access.Level
		Creator  struct{ ID, Name string }
		Created  time.Time
		Token    *struct{ Generated, LastUse time.Time }
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Node-RED" || list[0].Level != access.Member || list[0].Creator.ID != alice.ID || list[0].Creator.Name != "Alice" ||
		list[0].Created.IsZero() || list[0].Token == nil || list[0].Token.Generated.IsZero() || list[0].Token.LastUse.IsZero() {
		t.Errorf("GET /api/programs: %s", body)
	}

	if resp := do("PUT", "/api/programs/"+p.ID, `{"name":"Flows","level":"guest"}`, cookie); resp.StatusCode != http.StatusNoContent {
		t.Errorf("PUT: %d", resp.StatusCode)
	}
	if m := whoAmI(t, bot("GET", "/api/me", "")); m.Name != "Flows" || m.Level != access.Guest {
		t.Errorf("after a change: %+v", m)
	}
	if resp := do("DELETE", "/api/programs/"+p.ID+"/token", "", cookie); resp.StatusCode != http.StatusNoContent {
		t.Errorf("revoking: %d", resp.StatusCode)
	}
	if resp := bot("GET", "/api/me", ""); resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("a revoked Token: %d %q, want 401 with WWW-Authenticate", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	if resp := do("DELETE", "/api/programs/"+p.ID, "", cookie); resp.StatusCode != http.StatusNoContent {
		t.Errorf("removing: %d", resp.StatusCode)
	}
	if list := decodeAs[[]any](t, do("GET", "/api/programs", "", cookie), http.StatusOK); len(list) != 0 {
		t.Errorf("after removing: %v", list)
	}
}

func TestTheAuthorizationHeaderIsReadAsTheRFCSays(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	p, _ := acc.CreateProgram(alice, "Node-RED", access.Member)
	token, _ := acc.GenerateToken(alice, p.ID)
	for header, want := range map[string]int{
		"Bearer " + token:        http.StatusOK,
		"bearer " + token:        http.StatusOK, // the scheme is case-insensitive
		"Bearer\t " + token:      http.StatusOK,
		"Bearer":                 http.StatusUnauthorized, // a Token attempted, never anonymous
		"Bearer " + token + " x": http.StatusUnauthorized,
		"Basic " + token:         http.StatusOK, // not a Token: anonymous, until #40
	} {
		req, _ := http.NewRequest("GET", srv.URL+"/api/me", nil)
		req.Header.Set("Authorization", header)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Authorization: %q: %d, want %d", header, resp.StatusCode, want)
		}
	}
}

func TestATokenInAQueryStringIsNotAccepted(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	p, _ := acc.CreateProgram(alice, "Node-RED", access.Admin)
	token, _ := acc.GenerateToken(alice, p.ID)
	for _, q := range []string{"access_token", "token", "bearer"} {
		resp, err := http.Get(srv.URL + "/api/me?" + q + "=" + token)
		if err != nil {
			t.Fatal(err)
		}
		if m := whoAmI(t, resp); m.Identity != nil {
			t.Errorf("?%s=: served as %+v", q, m)
		}
		resp.Body.Close()
	}
}

func TestAnAdminProgramManagesNoAccess(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	p, _ := acc.CreateProgram(alice, "Node-RED", access.Admin)
	token, _ := acc.GenerateToken(alice, p.ID)
	bot := asProgram(t, srv, token)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/programs", ""},
		{"POST", "/api/programs", `{"name":"Script","level":"admin"}`},
		{"PUT", "/api/programs/" + p.ID, `{"name":"Script","level":"admin"}`},
		{"DELETE", "/api/programs/" + p.ID, ""},
		{"POST", "/api/programs/" + p.ID + "/token", ""},
		{"DELETE", "/api/programs/" + p.ID + "/token", ""},
	} {
		if resp := bot(r.method, r.path, r.body); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s by an Admin Program: %d, want 403", r.method, r.path, resp.StatusCode)
		}
	}
	if _, ok := acc.ResolveToken(token); !ok {
		t.Error("the Program changed itself")
	}
	// Today's endpoints serve it.
	if resp := bot("GET", "/api/build", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/build: %d", resp.StatusCode)
	}
}

func TestManagingProgramsNeedsStepUp(t *testing.T) {
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	_, acc, hdl := handlerWith(t, nil, clock)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, alice := signedIn(t, acc)
	p, _ := acc.CreateProgram(alice, "Node-RED", access.Member)
	mu.Lock()
	now = now.Add(11 * time.Minute)
	mu.Unlock()
	do := browser(t, srv, "http://localhost:8080")
	for _, r := range []struct{ method, path, body string }{
		{"POST", "/api/programs", `{"name":"Script","level":"guest"}`},
		{"POST", "/api/programs/" + p.ID + "/token", ""},
	} {
		resp := do(r.method, r.path, r.body, cookie)
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(b), "sign in again") {
			t.Errorf("%s %s 11 minutes after signing in: %d %s, want 403 asking for step-up", r.method, r.path, resp.StatusCode, b)
		}
	}
	if resp := do("GET", "/api/programs", "", cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("listing needs no step-up: %d", resp.StatusCode)
	}
}

func TestAProgramsStreamClosesWhenItsTokenIsRevoked(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	p, _ := acc.CreateProgram(alice, "Node-RED", access.Member)
	token, _ := acc.GenerateToken(alice, p.ID)
	bot := asProgram(t, srv, token)
	resp := bot("GET", "/api/updates", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/updates: %d", resp.StatusCode)
	}
	lines := bufio.NewReader(resp.Body)
	if line, err := lines.ReadString('\n'); err != nil || !strings.HasPrefix(line, "data: ") {
		t.Fatalf("snapshot: %q, %v", line, err)
	}
	if err := acc.RevokeToken(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(lines); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the stream ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream outlived its Token")
	}
	if resp := bot("GET", "/api/updates", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a reconnect with a revoked Token: %d, want 401", resp.StatusCode)
	}
}
