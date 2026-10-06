package access

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// paired is a Kiosk named name at level, paired by fresh Admin by from a
// screen in browser, and its Session's secret.
func paired(t *testing.T, s *Store, by Identity, name string, level Level) (string, string) {
	t.Helper()
	approval, claim := s.RequestPairing("Chrome on Android")
	id, err := s.ApprovePairing(by, approval, "", name, level)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.ClaimPairing(claim)
	if err != nil || session == "" {
		t.Fatalf("ClaimPairing = %q, %v", session, err)
	}
	return id, session
}

// stale is Alice's Session once it no longer passes step-up.
func stale(t *testing.T, s *Store, c *clock, session string) Identity {
	t.Helper()
	c.advance(freshFor)
	id := admin(t, s, session)
	if id.Fresh {
		t.Fatal("Alice still fresh")
	}
	return id
}

func TestAPairedScreenSignsInAsItsKiosk(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	approval, claim := s.RequestPairing("Chrome on Android")
	if len(approval) < 26 || len(claim) < 26 || approval == claim {
		t.Fatalf("secrets %q, %q: want two of 128 bits", approval, claim)
	}
	if got, err := s.ClaimPairing(claim); got != "" || err != nil {
		t.Fatalf("before approval: %q, %v", got, err)
	}
	c.advance(time.Minute)
	req, err := s.PairingRequest(alice, approval)
	if err != nil || req.Browser != "Chrome on Android" || !req.Created.Equal(c.t.Add(-time.Minute)) {
		t.Fatalf("PairingRequest = %+v, %v", req, err)
	}
	id, err := s.ApprovePairing(alice, approval, "", " Hall tablet ", Guest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApprovePairing(alice, approval, "", "Again", Guest); !errors.Is(err, ErrRefused) {
		t.Errorf("approved twice: %v, want ErrRefused", err)
	}
	if _, err := s.ClaimPairing(approval); !errors.Is(err, ErrRefused) {
		t.Errorf("claimed with the approval secret: %v, want ErrRefused", err)
	}
	kiosk, err := s.ClaimPairing(claim)
	if err != nil || kiosk == "" {
		t.Fatalf("ClaimPairing = %q, %v", kiosk, err)
	}
	who, ok := s.Resolve(kiosk)
	if !ok || who.Kind != KioskKind || who.ID != id || who.Name != "Hall tablet" || who.Level != Guest || who.Fresh || who.Session == "" {
		t.Fatalf("Resolve = %+v, %v", who, ok)
	}
	if _, err := s.ClaimPairing(claim); !errors.Is(err, ErrRefused) {
		t.Errorf("claimed twice: %v, want ErrRefused", err)
	}
	ks, err := s.Kiosks(alice)
	if err != nil || len(ks) != 1 {
		t.Fatalf("Kiosks = %+v, %v", ks, err)
	}
	if k := ks[0]; k.Name != "Hall tablet" || k.Pairer != alice.ID || !k.Paired.Equal(c.t) || !k.LastUse.Equal(c.t) || !k.SignedIn {
		t.Errorf("Kiosk %+v", k)
	}
	if names := s.CurrentNames(); names[KioskKind][id] != "Hall tablet" {
		t.Errorf("names %v", names)
	}
}

func TestAPairingRequestLasts10MinutesAndTheCapDropsTheOldest(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	approval, claim := s.RequestPairing("")
	c.advance(pairingLife)
	if _, err := s.PairingRequest(alice, approval); !errors.Is(err, ErrRefused) {
		t.Errorf("expired request: %v, want ErrRefused", err)
	}
	if _, err := s.ClaimPairing(claim); !errors.Is(err, ErrRefused) {
		t.Errorf("expired claim: %v, want ErrRefused", err)
	}

	first, _ := s.RequestPairing("")
	second, _ := s.RequestPairing("")
	for range maxPairings - 2 {
		s.RequestPairing("")
	}
	s.RequestPairing("") // past the cap: the oldest goes
	if _, err := s.PairingRequest(alice, first); !errors.Is(err, ErrRefused) {
		t.Errorf("the oldest, past the cap: %v, want ErrRefused", err)
	}
	if _, err := s.PairingRequest(alice, second); err != nil {
		t.Errorf("the second oldest: %v", err)
	}
}

func TestPairingAKioskAgainEndsItsPreviousSession(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	id, old := paired(t, s, alice, "Hall tablet", Member)
	was, _ := s.Resolve(old)
	approval, claim := s.RequestPairing("Firefox on Android")
	if _, err := s.ApprovePairing(alice, approval, id, "ignored", Guest); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Resolve(old); ok || !closed(was.Ended) {
		t.Error("the previous Session lasts")
	}
	kiosk, err := s.ClaimPairing(claim)
	if err != nil {
		t.Fatal(err)
	}
	if who, ok := s.Resolve(kiosk); !ok || who.ID != id || who.Name != "Hall tablet" || who.Level != Member {
		t.Errorf("Resolve = %+v, %v; want the same Kiosk", who, ok)
	}
	if ks, _ := s.Kiosks(alice); len(ks) != 1 {
		t.Errorf("%d Kiosks, want 1", len(ks))
	}
	if _, err := s.ApprovePairing(alice, "unknown", id, "", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("an unknown request: %v, want ErrRefused", err)
	}
	approval, _ = s.RequestPairing("")
	if _, err := s.ApprovePairing(alice, approval, "unknown", "", ""); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("an unknown Kiosk: %v, want ErrNotFound", err)
	}
}

func TestAKioskIsNeverAnAdminAndManagesNothing(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	approval, _ := s.RequestPairing("")
	if _, err := s.ApprovePairing(alice, approval, "", "Hall tablet", Admin); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an Admin Kiosk: %v, want ErrInvalid", err)
	}
	id, secret := paired(t, s, alice, "Hall tablet", Member)
	if err := s.EditKiosk(alice, id, "Hall tablet", Admin); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("made an Admin: %v, want ErrInvalid", err)
	}
	kiosk, _ := s.Resolve(secret)
	if _, err := s.Persons(kiosk); !errors.Is(err, ErrRefused) {
		t.Errorf("Persons: %v", err)
	}
	if _, err := s.Kiosks(kiosk); !errors.Is(err, ErrRefused) {
		t.Errorf("Kiosks: %v", err)
	}
	if err := s.Rename(kiosk, "Mine"); !errors.Is(err, ErrRefused) {
		t.Errorf("Rename: %v", err)
	}
	if err := s.SignOut(secret); !errors.Is(err, ErrRefused) {
		t.Errorf("SignOut: %v, want ErrRefused", err)
	}
	approval, _ = s.RequestPairing("")
	if _, err := s.PairingRequest(kiosk, approval); !errors.Is(err, ErrRefused) {
		t.Errorf("PairingRequest: %v", err)
	}
}

func TestAKioskSessionEndsAfter30DaysIdleNeverByAge(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	_, secret := paired(t, s, admin(t, s, session), "Hall tablet", Guest)
	for range 15 { // past a Person's year
		c.advance(idleLimit - time.Second)
		if _, ok := s.Resolve(secret); !ok {
			t.Fatalf("ended at %v, used within 30 days", c.t)
		}
	}
	c.advance(idleLimit + time.Second)
	if _, ok := s.Resolve(secret); ok {
		t.Error("lasted 30 days idle")
	}
}

func TestAnAdminManagesKiosksStepUpOnlyToRenameOrChangeTheirLevel(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	id, secret := paired(t, s, alice, "Hall tablet", Guest)
	kiosk, _ := s.Resolve(secret)
	if err := s.EditKiosk(alice, id, "Hall tablet", Member); err != nil {
		t.Fatal(err)
	}
	if !closed(kiosk.Ended) {
		t.Error("a changed level left the stream open")
	}
	if now, ok := s.Resolve(secret); !ok || now.Level != Member {
		t.Errorf("after a change: %+v, %v", now, ok)
	}
	alice = stale(t, s, c, session)
	if err := s.EditKiosk(alice, id, "Hall", Member); !errors.Is(err, ErrStale) {
		t.Errorf("renamed without step-up: %v, want ErrStale", err)
	}
	kiosk, _ = s.Resolve(secret)
	if err := s.SignOutKiosk(alice, id); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Resolve(secret); ok || !closed(kiosk.Ended) {
		t.Error("signed out, the Session lasts")
	}
	if ks, _ := s.Kiosks(alice); len(ks) != 1 || ks[0].SignedIn {
		t.Errorf("signed out, the Kiosk: %+v", ks)
	}
	approval, _ := s.RequestPairing("")
	if _, err := s.ApprovePairing(alice, approval, id, "", ""); !errors.Is(err, ErrStale) {
		t.Errorf("paired without step-up: %v, want ErrStale", err)
	}
	if err := s.RemoveKiosk(alice, id); err != nil {
		t.Fatal(err)
	}
	if ks, _ := s.Kiosks(alice); len(ks) != 0 {
		t.Errorf("removed, %+v", ks)
	}
}

func TestRemovingAKioskEndsItsSession(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	id, secret := paired(t, s, alice, "Hall tablet", Guest)
	other, _ := paired(t, s, alice, "Kitchen tablet", Guest)
	approval, claim := s.RequestPairing("")
	if _, err := s.ApprovePairing(alice, approval, other, "", ""); err != nil {
		t.Fatal(err)
	}
	kiosk, _ := s.Resolve(secret)
	alice = stale(t, s, c, session)
	if err := s.RemoveKiosk(alice, id); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Resolve(secret); ok || !closed(kiosk.Ended) {
		t.Error("removed, the Session lasts")
	}
	if err := s.RemoveKiosk(alice, other); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimPairing(claim); !errors.Is(err, ErrRefused) {
		t.Errorf("claimed for a removed Kiosk: %v, want ErrRefused", err)
	}
}

func TestKiosksSurviveARestartAndKiosksJSONHoldsNoSecret(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	alice := admin(t, s, session)
	id, secret := paired(t, s, alice, "Hall tablet", Member)
	_, other := paired(t, s, alice, "Kitchen tablet", Guest)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "kiosks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []string{secret, other, hash(secret)} {
		if strings.Contains(string(raw), x) {
			t.Error("kiosks.json holds a Session")
		}
	}
	s = open(t, dir, c)
	if who, ok := s.Resolve(secret); !ok || who.ID != id || who.Level != Member {
		t.Errorf("after a restart: %+v, %v", who, ok)
	}
	alice = admin(t, s, session)
	ks, _ := s.Kiosks(alice)
	if err := s.RemoveKiosk(alice, ks[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := open(t, dir, c).Resolve(other); ok {
		t.Error("a removed Kiosk's Session came back")
	}
}

func TestKioskPairingsAreRecorded(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	a := party(alice)
	c.take()
	id, _ := paired(t, s, alice, "Hall tablet", Guest)
	k := Party{KioskKind, id, "Hall tablet"}
	recorded(t, c.take(),
		Entry{Event: KioskCreated, Actor: a, Subject: k, Detail: map[string]any{"level": Guest}},
		Entry{Event: KioskPaired, Actor: a, Subject: k, Browser: "Chrome on Android"},
		Entry{Event: SignedIn, Actor: k, Subject: k, Browser: "Chrome on Android", Detail: map[string]any{"method": "pairing", "by": a}},
	)
	approval, _ := s.RequestPairing("Firefox on Android")
	s.ApprovePairing(alice, approval, id, "", "")
	recorded(t, c.take(),
		Entry{Event: KioskPaired, Actor: a, Subject: k, Browser: "Firefox on Android"},
		Entry{Event: SessionEnded, Actor: a, Subject: k, Browser: "Chrome on Android", Detail: map[string]any{"reason": "paired again"}},
	)
	s.ClaimPairing("unknown")
	s.PairingRequest(alice, "unknown")
	recorded(t, c.take(),
		Entry{Event: PairingRefused, Actor: nobody, Subject: nobody, Detail: map[string]any{"reason": "expired"}},
		Entry{Event: PairingRefused, Actor: a, Subject: nobody, Detail: map[string]any{"reason": "expired"}},
	)
	// An Admin refuses a screen: it is told so, and asks again.
	approval, claim := s.RequestPairing("Firefox on Android")
	if err := s.RefusePairing(alice, approval); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimPairing(claim); !errors.Is(err, ErrRefused) {
		t.Errorf("claimed a refused pairing: %v", err)
	}
	recorded(t, c.take(),
		Entry{Event: PairingRefused, Actor: a, Subject: nobody, Browser: "Firefox on Android", Detail: map[string]any{"reason": "refused"}},
		Entry{Event: PairingRefused, Actor: nobody, Subject: nobody, Detail: map[string]any{"reason": "expired"}},
	)
	s.EditKiosk(alice, id, "Hall", Member)
	k2 := Party{KioskKind, id, "Hall"}
	recorded(t, c.take(),
		Entry{Event: KioskRenamed, Actor: a, Subject: k2, Detail: map[string]any{"from": "Hall tablet"}},
		Entry{Event: LevelChanged, Actor: a, Subject: k2, Detail: map[string]any{"from": Guest, "to": Member}},
	)
	s.RemoveKiosk(alice, id)
	recorded(t, c.take(), Entry{Event: KioskRemoved, Actor: a, Subject: k2})
}
