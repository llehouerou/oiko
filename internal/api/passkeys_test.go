package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/access/passkeytest"
)

// read is resp's body, which must have status.
func read(t *testing.T, resp *http.Response, status int) []byte {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s: %d %s, want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, b, status)
	}
	return b
}

// session is the Session cookie resp sets.
func session(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatalf("%s %s set no Session", resp.Request.Method, resp.Request.URL.Path)
	return nil
}

func TestPasskeysEndToEnd(t *testing.T) {
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); defer mu.Unlock(); now = now.Add(d) }
	const origin = "https://oiko.example"
	_, acc, hdl := handlerWith(t, &url.URL{Scheme: "https", Host: "oiko.example"}, clock)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	do := browser(t, srv, origin)
	setup, _ := acc.Setup()
	cookie := session(t, do("POST", "/api/setup", `{"secret":"`+setup+`","name":"Alice"}`, nil))

	// Right after the Setup link, the offered Passkey is added.
	phone := passkeytest.New(origin)
	options := read(t, do("POST", "/api/me/passkeys/options", "", cookie), http.StatusOK)
	read(t, do("POST", "/api/me/passkeys", string(phone.Create(t, options)), cookie), http.StatusCreated)
	keys := decodeAs[[]struct {
		ID, Provider string
		Created      time.Time
	}](t, do("GET", "/api/me/passkeys", "", cookie), http.StatusOK)
	if len(keys) != 1 || keys[0].Provider != "Google Password Manager" || keys[0].Created.IsZero() {
		t.Fatalf("own Passkeys: %+v", keys)
	}
	read(t, do("POST", "/api/sign-out", "", cookie), http.StatusNoContent)

	// Signing in with it, from no Session.
	options = read(t, do("POST", "/api/sign-in/passkey/options", "", nil), http.StatusOK)
	cookie = session(t, do("POST", "/api/sign-in/passkey", string(phone.Get(t, options)), nil))
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); m.Name != "Alice" || !m.Fresh {
		t.Errorf("signed in with a Passkey: %+v", m)
	}

	// 10 minutes on, step-up asks to confirm it, after which a Program is created.
	advance(10 * time.Minute)
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); m.Fresh {
		t.Fatal("fresh 10 minutes after signing in")
	}
	read(t, do("POST", "/api/programs", `{"name":"Node-RED","level":"member"}`, cookie), http.StatusForbidden)
	options = read(t, do("POST", "/api/step-up/options", "", cookie), http.StatusOK)
	read(t, do("POST", "/api/step-up", string(phone.Get(t, options)), cookie), http.StatusNoContent)
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); !m.Fresh {
		t.Error("not fresh after confirming a Passkey")
	}
	read(t, do("POST", "/api/programs", `{"name":"Node-RED","level":"member"}`, cookie), http.StatusCreated)

	// Removing it, still fresh: it then signs in no one.
	read(t, do("DELETE", "/api/me/passkeys/"+keys[0].ID, "", cookie), http.StatusNoContent)
	options = read(t, do("POST", "/api/sign-in/passkey/options", "", nil), http.StatusOK)
	if b := read(t, do("POST", "/api/sign-in/passkey", string(phone.Get(t, options)), nil), http.StatusForbidden); !strings.Contains(string(b), "not accepted") {
		t.Errorf("a removed Passkey: %s", b)
	}
}

func TestPasskeyCeremoniesRunWhereSignInWorks(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, _ := signedIn(t, acc)
	for _, origin := range []string{"https://evil.example", "http://192.168.1.2:8080", ""} {
		do := browser(t, srv, origin)
		for path, cookie := range map[string]*http.Cookie{
			"/api/sign-in/passkey/options": nil,
			"/api/step-up/options":         cookie,
			"/api/me/passkeys/options":     cookie,
		} {
			if resp := do("POST", path, "", cookie); resp.StatusCode != http.StatusForbidden {
				t.Errorf("POST %s from %q: %d, want 403", path, origin, resp.StatusCode)
			}
		}
	}
	// A browser at the Public URL whose answer names another origin.
	do := browser(t, srv, "https://oiko.example")
	options := read(t, do("POST", "/api/me/passkeys/options", "", cookie), http.StatusOK)
	phishing := passkeytest.New("https://oiko.example.evil")
	if b := read(t, do("POST", "/api/me/passkeys", string(phishing.Create(t, options)), cookie), http.StatusForbidden); !strings.Contains(string(b), "not accepted") {
		t.Errorf("an answer for another origin: %s", b)
	}
}

func TestAProgramHasNoPasskeys(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	_, token := withProgram(t, acc, alice, "Node-RED", access.Admin)
	bot := asProgram(t, srv, token)
	if resp := bot("GET", "/api/me/passkeys", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a Program's Passkeys: %d, want 403", resp.StatusCode)
	}
}
