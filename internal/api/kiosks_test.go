package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// withKiosk pairs a Kiosk named name at level, approved by fresh Admin by,
// and answers its id and its Session's cookie.
func withKiosk(t *testing.T, acc *access.Store, by access.Identity, name string, level access.Level) (string, *http.Cookie) {
	t.Helper()
	approval, claim := acc.RequestPairing("")
	id, err := acc.ApprovePairing(by, approval, "", name, level)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := acc.ClaimPairing(claim)
	if err != nil {
		t.Fatal(err)
	}
	return id, &http.Cookie{Name: sessionCookie, Value: secret}
}

// cookie is the cookie named name resp sets; nil if none.
func cookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestPairingAKioskEndToEnd(t *testing.T) {
	_, acc, hdl := handlerAt(t, &url.URL{Scheme: "https", Host: "oiko.example"})
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, _ := signedIn(t, acc)
	do := browser(t, srv, "https://oiko.example")

	if resp := browser(t, srv, "https://elsewhere.example")("POST", "/api/kiosk-pairing", "", nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a pairing request from another origin: %d, want 403", resp.StatusCode)
	}
	resp := do("POST", "/api/kiosk-pairing", "", nil)
	req := decodeAs[struct{ Secret string }](t, resp, http.StatusCreated)
	claim := cookie(resp, pairingCookie)
	if claim == nil || !claim.HttpOnly || !claim.Secure || claim.SameSite != http.SameSiteStrictMode || claim.Value == req.Secret {
		t.Fatalf("claim cookie %+v", claim)
	}
	read(t, do("POST", "/api/kiosk-pairing/claim", "", claim), http.StatusAccepted)
	// The approval secret alone, as a cookie or not, claims nothing.
	read(t, do("POST", "/api/kiosk-pairing/claim", "", nil), http.StatusForbidden)
	read(t, do("POST", "/api/kiosk-pairing/claim", "", &http.Cookie{Name: pairingCookie, Value: req.Secret}), http.StatusForbidden)

	seen := decodeAs[struct{ Browser string }](t, do("POST", "/api/kiosk-pairing/request", `{"secret":"`+req.Secret+`"}`, alice), http.StatusOK)
	if seen.Browser != "Unknown browser" {
		t.Errorf("request %+v", seen)
	}
	id := decodeAs[struct{ ID string }](t, do("POST", "/api/kiosk-pairing/approve", `{"secret":"`+req.Secret+`","name":"Hall tablet","level":"guest"}`, alice), http.StatusCreated).ID
	resp = do("POST", "/api/kiosk-pairing/claim", "", claim)
	read(t, resp, http.StatusNoContent)
	kiosk := session(t, resp)
	if gone := cookie(resp, pairingCookie); gone == nil || gone.MaxAge >= 0 {
		t.Errorf("the claim cookie stays: %+v", gone)
	}
	if m := whoAmI(t, do("GET", "/api/me", "", kiosk)); m.Identity["kiosk"] != id || m.Name != "Hall tablet" || m.Level != access.Guest {
		t.Errorf("who am I, as the Kiosk: %+v", m)
	}

	// A Kiosk manages nothing, signs no one in on top, and signs itself out never.
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/sign-out", ""},
		{"POST", "/api/sign-in/passkey/options", ""},
		{"POST", "/api/sign-in/link", `{"secret":"x"}`},
		{"POST", "/api/kiosk-pairing", ""},
		{"GET", "/api/kiosks", ""},
		{"GET", "/api/audit", ""},
		{"GET", "/api/me/sessions", ""},
	} {
		if resp := do(c.method, c.path, c.body, kiosk); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as a Kiosk: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	ks := decodeAs[[]struct {
		ID       string
		Name     string
		Level    access.Level
		PairedBy struct{ Name string }
		SignedIn bool
		LastUse  time.Time
	}](t, do("GET", "/api/kiosks", "", alice), http.StatusOK)
	if len(ks) != 1 || ks[0].ID != id || ks[0].PairedBy.Name != "Alice" || !ks[0].SignedIn || ks[0].LastUse.IsZero() {
		t.Errorf("Kiosks %+v", ks)
	}
	read(t, do("DELETE", "/api/kiosks/"+id+"/session", "", alice), http.StatusNoContent)
	if m := whoAmI(t, do("GET", "/api/me", "", kiosk)); m.Identity != nil {
		t.Errorf("signed out, the Kiosk is still %+v", m)
	}
}

func TestAKiosksStreamClosesWhenItsLevelChanges(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	_, alice := signedIn(t, acc)
	id, kiosk := withKiosk(t, acc, alice, "Hall tablet", access.Member)
	do := browser(t, srv, "")
	msgs := stream(t, func(method, path, body string) *http.Response { return do(method, path, body, kiosk) })
	next(t, msgs, "snapshot")
	if err := acc.EditKiosk(alice, id, "Hall tablet", access.Guest); err != nil {
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
			t.Fatal("the stream outlived the Kiosk's level")
		}
	}
}
