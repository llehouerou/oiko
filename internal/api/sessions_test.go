package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

type sessionJSON struct {
	ID, Browser, Method string
	By                  *struct{ ID, Name string }
	Current             bool
}

func TestSessionsAreSeenAndEnded(t *testing.T) {
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); defer mu.Unlock(); now = now.Add(d) }
	_, acc, hdl := handlerWith(t, nil, clock)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	do := browser(t, srv, "http://localhost:8080")
	laptop, alice := signedIn(t, acc)
	// Alice signs in her phone with a link for herself.
	link := decodeAs[struct{ Secret string }](t, do("POST", "/api/persons/"+alice.ID+"/sign-in-link", "", laptop), http.StatusCreated)
	phone := session(t, do("POST", "/api/sign-in/link", `{"secret":"`+link.Secret+`"}`, nil))
	bob, bobs := invited(t, do, laptop, "Bob", access.Member)

	mine := decodeAs[[]sessionJSON](t, do("GET", "/api/me/sessions", "", laptop), http.StatusOK)
	if len(mine) != 2 || !mine[0].Current || mine[0].Method != "setup" || mine[1].Current || mine[1].Method != "link" || mine[1].By != nil {
		t.Fatalf("Alice's Sessions: %+v; want this one, by the Setup link, and her phone's, by her own link", mine)
	}

	// Ten minutes on, she ends her phone's Session without step-up: its stream closes.
	advance(11 * time.Minute)
	msgs := stream(t, func(method, path, body string) *http.Response { return do(method, path, body, phone) })
	next(t, msgs, "snapshot")
	read(t, do("DELETE", "/api/me/sessions/"+mine[1].ID, "", laptop), http.StatusNoContent)
	timeout := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-msgs:
		case <-timeout:
			t.Fatal("the phone's stream outlived its Session")
		}
	}
	if resp := do("GET", "/api/updates", "", phone); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the phone reconnecting: %d, want 401", resp.StatusCode)
	}

	// Bob's Sessions are hers to see and end only after step-up; never his to see hers.
	for _, path := range []string{"/api/persons/" + bob + "/sessions", "/api/persons/" + bob + "/passkeys"} {
		read(t, do("GET", path, "", laptop), http.StatusForbidden)
	}
	read(t, do("GET", "/api/persons/"+alice.ID+"/sessions", "", bobs), http.StatusForbidden)
	fresh := freshAdmin(t, acc, alice)
	his := decodeAs[[]sessionJSON](t, do("GET", "/api/persons/"+bob+"/sessions", "", fresh), http.StatusOK)
	if len(his) != 1 || his[0].Current || his[0].By == nil || his[0].By.Name != "Alice" {
		t.Fatalf("Bob's Sessions, as Alice reads them: %+v; want one, by Alice's link", his)
	}
	read(t, do("DELETE", "/api/persons/"+bob+"/sessions/"+his[0].ID, "", fresh), http.StatusNoContent)
	if resp := do("GET", "/api/me", "", bobs); whoAmI(t, resp).Identity != nil {
		t.Error("Bob is still signed in")
	}

	// Signing out every other device keeps this one.
	read(t, do("DELETE", "/api/me/sessions", "", fresh), http.StatusNoContent)
	if mine := decodeAs[[]sessionJSON](t, do("GET", "/api/me/sessions", "", fresh), http.StatusOK); len(mine) != 1 || !mine[0].Current {
		t.Errorf("after signing out the others: %+v", mine)
	}
	if whoAmI(t, do("GET", "/api/me", "", laptop)).Identity != nil {
		t.Error("the laptop is still signed in")
	}
}

// freshAdmin is the Session cookie of a new Session of Admin alice, which
// signed in just now: fresh for step-up.
func freshAdmin(t *testing.T, acc *access.Store, alice access.Identity) *http.Cookie {
	t.Helper()
	alice.Fresh = true
	secret, _, err := acc.CreateLink(alice, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := acc.SignInWithLink(secret, "")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookie, Value: s}
}
