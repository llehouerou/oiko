package api

import (
	"bytes"
	"log"
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

// invite has Alice, by her cookie, create a Person named name at level and
// a Sign-in link for them, answering their id and the link's secret.
func invite(t *testing.T, do func(method, path, body string, cookie *http.Cookie) *http.Response, alice *http.Cookie, name string, level access.Level) (string, string) {
	t.Helper()
	p := decodeAs[struct{ ID string }](t, do("POST", "/api/persons", `{"name":"`+name+`","level":"`+string(level)+`"}`, alice), http.StatusCreated)
	link := decodeAs[struct{ Secret string }](t, do("POST", "/api/persons/"+p.ID+"/sign-in-link", "", alice), http.StatusCreated)
	return p.ID, link.Secret
}

// invited is the Session cookie of a Person named name at level, invited by
// Alice and signed in by their link.
func invited(t *testing.T, do func(method, path, body string, cookie *http.Cookie) *http.Response, alice *http.Cookie, name string, level access.Level) (string, *http.Cookie) {
	t.Helper()
	id, secret := invite(t, do, alice, name, level)
	return id, session(t, do("POST", "/api/sign-in/link", `{"secret":"`+secret+`"}`, nil))
}

func TestInvitingAPersonEndToEnd(t *testing.T) {
	var logs bytes.Buffer // slog's default writes there too
	defer log.SetOutput(log.Writer())
	log.SetOutput(&logs)

	const origin = "https://oiko.example"
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	var mu sync.Mutex
	var urls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		urls = append(urls, r.URL.String())
		mu.Unlock()
		hdl.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	alice, aliceID := signedIn(t, acc)
	do := browser(t, srv, origin)

	// Alice creates Bob, then a Sign-in link for him.
	bob, secret := invite(t, do, alice, "Bob", access.Member)
	persons := decodeAs[[]struct {
		ID, Name string
		Level    access.Level
		Link     *struct {
			Creator          struct{ ID, Name string }
			Created, Expires time.Time
		}
	}](t, do("GET", "/api/persons", "", alice), http.StatusOK)
	if len(persons) != 2 || persons[1].ID != bob || persons[1].Name != "Bob" || persons[1].Level != access.Member {
		t.Fatalf("GET /api/persons: %+v", persons)
	}
	if l := persons[1].Link; l == nil || l.Creator.ID != aliceID.ID || l.Creator.Name != "Alice" || l.Expires.Sub(l.Created) != 24*time.Hour {
		t.Errorf("Bob's pending link: %+v; want Alice's, for 24 hours", l)
	}
	if persons[0].Link != nil {
		t.Errorf("Alice's pending link: %+v, want none", persons[0].Link)
	}

	// Bob opens it: its page, whom it is for, Continue, then a Passkey.
	read(t, do("GET", "/sign-in", "", nil), http.StatusOK)
	if b := read(t, do("POST", "/api/sign-in/link/person", `{"secret":"`+secret+`"}`, nil), http.StatusOK); !strings.Contains(string(b), `"name":"Bob"`) {
		t.Errorf("whom the link is for: %s", b)
	}
	cookie := session(t, do("POST", "/api/sign-in/link", `{"secret":"`+secret+`"}`, nil))
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); m.Identity["person"] != bob || m.Level != access.Member || !m.Fresh {
		t.Errorf("signed in by link: %+v", m)
	}
	phone := passkeytest.New(origin)
	options := read(t, do("POST", "/api/me/passkeys/options", "", cookie), http.StatusOK)
	read(t, do("POST", "/api/me/passkeys", string(phone.Create(t, options)), cookie), http.StatusCreated)

	// Spent: it says so, naming no one.
	for _, path := range []string{"/api/sign-in/link/person", "/api/sign-in/link"} {
		b := read(t, do("POST", path, `{"secret":"`+secret+`"}`, nil), http.StatusForbidden)
		if !strings.Contains(string(b), "ask whoever sent it") || strings.Contains(string(b), "Alice") || strings.Contains(string(b), "Bob") {
			t.Errorf("POST %s with a used link: %s", path, b)
		}
	}

	for _, u := range urls {
		if strings.Contains(u, secret) {
			t.Errorf("the link's secret in a request URL: %s", u)
		}
	}
	if strings.Contains(logs.String(), secret) {
		t.Errorf("the link's secret in the log: %s", logs.String())
	}
}

func TestSigningInByLinkRunsWhereSignInWorks(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := signedIn(t, acc)
	_, secret := invite(t, browser(t, srv, "https://oiko.example"), alice, "Bob", access.Member)
	for _, origin := range []string{"https://evil.example", "http://192.168.1.2:8080", ""} {
		for _, path := range []string{"/api/sign-in/link/person", "/api/sign-in/link"} {
			if resp := browser(t, srv, origin)("POST", path, `{"secret":"`+secret+`"}`, nil); resp.StatusCode != http.StatusForbidden {
				t.Errorf("POST %s from %q: %d, want 403", path, origin, resp.StatusCode)
			}
		}
	}
	read(t, browser(t, srv, "http://localhost:8080")("POST", "/api/sign-in/link", `{"secret":"`+secret+`"}`, nil), http.StatusNoContent)
}

func TestChangingAPersonsLevelClosesTheirStreams(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := signedIn(t, acc)
	do := browser(t, srv, "http://localhost:8080")
	bob, cookie := invited(t, do, alice, "Bob", access.Member)
	msgs := stream(t, func(method, path, body string) *http.Response { return do(method, path, body, cookie) })
	next(t, msgs, "snapshot")
	read(t, do("PUT", "/api/persons/"+bob, `{"name":"Bob","level":"guest"}`, alice), http.StatusNoContent)
	timeout := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-msgs:
		case <-timeout:
			t.Fatal("the stream outlived Bob's level")
		}
	}
	// Still signed in, now a Guest.
	if m := whoAmI(t, do("GET", "/api/me", "", cookie)); m.Level != access.Guest {
		t.Errorf("after the change: %+v", m)
	}
	next(t, stream(t, func(method, path, body string) *http.Response { return do(method, path, body, cookie) }), "snapshot")
}

func TestAGuestsEndDate(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := signedIn(t, acc)
	do := browser(t, srv, "http://localhost:8080")
	ends := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	if resp := do("POST", "/api/persons", `{"name":"Carol","level":"member","ends":"`+ends.Format(time.RFC3339)+`"}`, alice); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a Member with an end date: %d, want 400", resp.StatusCode)
	}
	bob := decodeAs[struct{ ID string }](t, do("POST", "/api/persons", `{"name":"Bob","level":"guest","ends":"`+ends.Format(time.RFC3339)+`"}`, alice), http.StatusCreated).ID
	secret := decodeAs[struct{ Secret string }](t, do("POST", "/api/persons/"+bob+"/sign-in-link", "", alice), http.StatusCreated).Secret

	// His welcome page tells his end date.
	welcome := decodeAs[struct {
		Name string
		Ends time.Time
	}](t, do("POST", "/api/sign-in/link/person", `{"secret":"`+secret+`"}`, nil), http.StatusOK)
	if welcome.Name != "Bob" || !welcome.Ends.Equal(ends) {
		t.Errorf("his welcome: %+v, want Bob until %v", welcome, ends)
	}
	cookie := session(t, do("POST", "/api/sign-in/link", `{"secret":"`+secret+`"}`, nil))
	persons := decodeAs[[]struct{ Ends *time.Time }](t, do("GET", "/api/persons", "", alice), http.StatusOK)
	if persons[0].Ends != nil || persons[1].Ends == nil || !persons[1].Ends.Equal(ends) {
		t.Errorf("end dates listed: Alice's %v, Bob's %v", persons[0].Ends, persons[1].Ends)
	}

	// An end date already come ends his Session and closes his stream at once.
	msgs := stream(t, func(method, path, body string) *http.Response { return do(method, path, body, cookie) })
	next(t, msgs, "snapshot")
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	read(t, do("PUT", "/api/persons/"+bob, `{"name":"Bob","level":"guest","ends":"`+past+`"}`, alice), http.StatusNoContent)
	timeout := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-msgs:
		case <-timeout:
			t.Fatal("his stream outlived his end date")
		}
	}
	if resp := do("GET", "/api/updates", "", cookie); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("his Session after his end date: %d, want 401", resp.StatusCode)
	}
	if b := read(t, do("POST", "/api/persons/"+bob+"/sign-in-link", "", alice), http.StatusForbidden); !strings.Contains(string(b), "has ended") {
		t.Errorf("a link for him: %s", b)
	}
	// Promoted, he has no end date, and is back.
	read(t, do("PUT", "/api/persons/"+bob, `{"name":"Bob","level":"member","ends":null}`, alice), http.StatusNoContent)
	read(t, do("POST", "/api/persons/"+bob+"/sign-in-link", "", alice), http.StatusCreated)
}

func TestOnlyAnAdminPersonManagesPersons(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, aliceID := signedIn(t, acc)
	do := browser(t, srv, "http://localhost:8080")
	carol, _ := invite(t, do, alice, "Carol", access.Guest)
	bob, member := invited(t, do, alice, "Bob", access.Member)
	_, token := withProgram(t, acc, aliceID, "Node-RED", access.Admin)
	bot := asProgram(t, srv, token)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/persons", ""},
		{"POST", "/api/persons", `{"name":"Mallory","level":"admin"}`},
		{"PUT", "/api/persons/" + carol, `{"name":"Carol","level":"admin"}`},
		{"DELETE", "/api/persons/" + carol, ""},
		{"POST", "/api/persons/" + carol + "/sign-in-link", ""},
		{"DELETE", "/api/persons/" + carol + "/sign-in-link", ""},
	} {
		if resp := do(r.method, r.path, r.body, member); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s by a Member: %d, want 403", r.method, r.path, resp.StatusCode)
		}
		if resp := bot(r.method, r.path, r.body); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s by an Admin Program: %d, want 403", r.method, r.path, resp.StatusCode)
		}
	}
	// Bob renames himself, and creates a link for himself.
	read(t, do("PUT", "/api/me", `{"name":"Robert"}`, member), http.StatusNoContent)
	if m := whoAmI(t, do("GET", "/api/me", "", member)); m.Name != "Robert" {
		t.Errorf("after renaming himself: %+v", m)
	}
	link := decodeAs[struct{ Expires time.Time }](t, do("POST", "/api/persons/"+bob+"/sign-in-link", "", member), http.StatusCreated)
	if d := time.Until(link.Expires); d > 15*time.Minute || d < 14*time.Minute {
		t.Errorf("his own link expires in %v, want 15 minutes", d)
	}
	// The last Admin stays; removing Bob ends his Session.
	if b := read(t, do("DELETE", "/api/persons/"+aliceID.ID, "", alice), http.StatusForbidden); !strings.Contains(string(b), "last Admin") {
		t.Errorf("removing the last Admin: %s", b)
	}
	read(t, do("DELETE", "/api/persons/"+bob, "", alice), http.StatusNoContent)
	if resp := do("GET", "/api/updates", "", member); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a removed Person's Session: %d, want 401", resp.StatusCode)
	}
}
