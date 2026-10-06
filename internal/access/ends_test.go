package access

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access/passkeytest"
	"github.com/llehouerou/oiko/internal/home"
)

// guest is a Guest named Bob whose access ends at ends, created by Alice,
// signed in by link with a Passkey of his own: his identity, the secret of
// his Session, and his authenticator.
func guest(t *testing.T, s *Store, alice Identity, ends time.Time) (Identity, string, *passkeytest.Authenticator) {
	t.Helper()
	bob, err := s.CreatePerson(alice, "Bob", Guest, ends)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.SignInWithLink(link(t, s, alice, bob.ID), "Chrome on Android")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := s.Resolve(session)
	return id, session, enrol(t, s, id, public)
}

func TestAGuestsAccessEndsAtTheirEndDate(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	ends := c.t.Add(2 * time.Hour)
	bob, bobs, phone := guest(t, s, alice, ends)
	unused := link(t, s, alice, bob.ID)
	c.take()

	c.advance(2*time.Hour - time.Second)
	if _, ok := s.Resolve(bobs); !ok {
		t.Fatal("refused before his end date")
	}
	c.advance(time.Second)
	if _, ok := s.Resolve(bobs); ok {
		t.Error("his Session outlived his end date")
	}
	if !closed(bob.Ended) {
		t.Error("his stream outlived his end date")
	}
	bobParty := Party{PersonKind, bob.ID, "Bob"}
	got := c.take()
	recorded(t, got,
		Entry{Event: EndDateReached, Actor: oiko, Subject: bobParty},
		Entry{Event: SessionEnded, Actor: oiko, Subject: bobParty, Browser: "Chrome on Android", Detail: map[string]any{"reason": "access ended"}})
	if !got[0].Time.Equal(ends) || !got[1].Time.Equal(ends) {
		t.Errorf("recorded at %v and %v, want at his end date %v", got[0].Time, got[1].Time, ends)
	}

	// Signing in is refused, saying why; no link is created for him.
	ended := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "access to Oiko has ended") {
			t.Errorf("%s: %v; want refused, his access ended", what, err)
		}
	}
	_, err := signIn(t, s, phone, public)
	ended("his Passkey", err)
	_, err = s.SignInWithLink(unused, "")
	ended("a link created before", err)
	recorded(t, c.take(),
		Entry{Event: PasskeyRefused, Actor: nobody, Subject: bobParty, Browser: "Safari on iPhone"},
		Entry{Event: LinkRefused, Actor: nobody, Subject: bobParty})
	alice = admin(t, s, session)
	alice.Fresh = true
	if _, _, err := s.CreateLink(alice, bob.ID); !errors.Is(err, ErrRefused) {
		t.Errorf("a link for him: %v, want ErrRefused", err)
	}

	// A new end date lets the same Passkey in again.
	later := c.t.Add(24 * time.Hour)
	if err := s.EditPerson(alice, bob.ID, "Bob", Guest, later); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: EndDateChanged, Actor: party(alice), Subject: bobParty, Detail: map[string]any{"from": ends, "to": later}})
	if _, err := signIn(t, s, phone, public); err != nil {
		t.Errorf("his Passkey with a new end date: %v", err)
	}
}

func TestAnEndDateThatCameWhileOikoWasStoppedEndsTheSessionsOnLoad(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	_, bobs, _ := guest(t, s, admin(t, s, session), c.t.Add(time.Hour))
	c.take()
	c.advance(2 * time.Hour)
	s = open(t, dir, c)
	if _, ok := s.Resolve(bobs); ok {
		t.Error("his Session outlived his end date")
	}
	if got := c.take(); len(got) != 2 || got[0].Event != EndDateReached {
		t.Errorf("recorded on load: %+v; want his end date reached and his Session ended", got)
	}
	open(t, dir, c)
	if got := c.take(); len(got) != 0 {
		t.Errorf("recorded again on the next load: %+v", got)
	}
}

func TestOnlyAGuestHasAnEndDate(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	ends := c.t.Add(time.Hour)
	for _, level := range []Level{Member, Admin} {
		if _, err := s.CreatePerson(alice, "Carol", level, ends); !errors.Is(err, home.ErrInvalid) {
			t.Errorf("a %s with an end date: %v, want ErrInvalid", level, err)
		}
	}
	bob, err := s.CreatePerson(alice, "Bob", Guest, ends)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EditPerson(alice, bob.ID, "Bob", Member, ends); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("promoting him, keeping his end date: %v, want ErrInvalid", err)
	}
	// Promoted, his end date is cleared.
	c.take()
	if err := s.EditPerson(alice, bob.ID, "Bob", Member, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if ps, _ := s.Persons(alice); !ps[1].Ends.IsZero() {
		t.Errorf("promoted, his end date stays: %v", ps[1].Ends)
	}
	bobParty := personParty(bob)
	recorded(t, c.take(),
		Entry{Event: LevelChanged, Actor: party(alice), Subject: bobParty},
		Entry{Event: EndDateChanged, Actor: party(alice), Subject: bobParty, Detail: map[string]any{"from": ends, "to": nil}})
	// Stale, an Admin changes no end date.
	alice.Fresh = false
	if err := s.EditPerson(alice, bob.ID, "Bob", Guest, ends); !errors.Is(err, ErrStale) {
		t.Errorf("a stale Admin setting an end date: %v, want ErrStale", err)
	}
}
