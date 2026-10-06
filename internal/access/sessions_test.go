package access

import (
	"errors"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

func TestAPersonSeesAndEndsTheirSessions(t *testing.T) {
	c := newClock()
	s, laptop := claimed(t, t.TempDir(), c)
	alice, _ := s.Resolve(laptop)
	phone := enrol(t, s, alice, public)
	c.advance(time.Hour)
	phones, _ := signIn(t, s, phone, public)
	tablets := s.sessionOf(t, alice.ID)
	alice, _ = s.Resolve(laptop) // no longer fresh: none of this needs step-up
	onPhone, _ := s.Resolve(phones)
	onTablet, _ := s.Resolve(tablets)

	list, err := s.Sessions(alice, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The tablet's, as sessionOf opens it, signed in before the phone's.
	if len(list) != 3 || list[0].ID != alice.Session || list[1].ID != onTablet.Session || list[2].ID != onPhone.Session {
		t.Fatalf("Sessions = %+v; want the laptop's, the tablet's and the phone's", list)
	}
	if l, p := list[0], list[2]; l.Method != "setup" || l.Browser != "Firefox on Linux" || p.Method != "passkey" || p.Provider != "Google Password Manager" || !p.LastUse.Equal(c.t) {
		t.Errorf("how they signed in: %+v, %+v", l, p)
	}

	c.take()
	if err := s.EndSession(alice, alice.ID, onPhone.Session); err != nil {
		t.Fatal(err)
	}
	if !closed(onPhone.Ended) || closed(onTablet.Ended) || closed(alice.Ended) {
		t.Error("ending the phone's Session closed other streams than its own")
	}
	if _, ok := s.Resolve(phones); ok {
		t.Error("an ended Session accepted")
	}
	me := party(alice)
	recorded(t, c.take(), Entry{Event: SessionEnded, Actor: me, Subject: me, Browser: "Safari on iPhone", Detail: map[string]any{"reason": "revoked"}})
	if err := s.EndSession(alice, alice.ID, onPhone.Session); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("ending it again: %v, want ErrNotFound", err)
	}

	// All the others: this one stays.
	if err := s.EndOtherSessions(alice); err != nil {
		t.Fatal(err)
	}
	if !closed(onTablet.Ended) || closed(alice.Ended) {
		t.Error("signing out the other devices closed the wrong streams")
	}
	if list, _ := s.Sessions(alice, alice.ID); len(list) != 1 || list[0].ID != alice.Session {
		t.Errorf("after signing out the others: %+v", list)
	}
}

func TestAnAdminEndsAnotherPersonsSessionsAndPasskeysAfterStepUp(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Admin)
	bobID := signedInAs(t, s, alice, bob.ID)
	enrol(t, s, bobID, public)
	keys, _ := s.Passkeys(bobID, bob.ID)
	carol := person(t, s, alice, "Carol", Member)
	carolID := signedInAs(t, s, alice, carol.ID)
	_, token := program(t, s, alice, "Node-RED", Admin)
	bot, _ := s.ResolveToken(token)
	stale := alice
	stale.Fresh = false

	for name, by := range map[string]Identity{"a stale Admin": stale, "a Member": carolID, "an Admin Program": bot} {
		if _, err := s.Sessions(by, bob.ID); !errors.Is(err, ErrRefused) {
			t.Errorf("%s reading Bob's Sessions: %v, want ErrRefused", name, err)
		}
		if _, err := s.Passkeys(by, bob.ID); !errors.Is(err, ErrRefused) {
			t.Errorf("%s reading Bob's Passkeys: %v, want ErrRefused", name, err)
		}
		if err := s.EndSession(by, bob.ID, bobID.Session); !errors.Is(err, ErrRefused) {
			t.Errorf("%s ending Bob's Session: %v, want ErrRefused", name, err)
		}
		if err := s.RemovePasskey(by, bob.ID, keys[0].ID.String()); !errors.Is(err, ErrRefused) {
			t.Errorf("%s removing Bob's Passkey: %v, want ErrRefused", name, err)
		}
	}

	list, err := s.Sessions(alice, bob.ID)
	if err != nil || len(list) != 1 || list[0].Method != "link" || list[0].By != alice.ID {
		t.Fatalf("Bob's Sessions, as Alice reads them: %+v, %v; want one, signed in by her link", list, err)
	}
	c.take()
	if err := s.EndSession(alice, bob.ID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if !closed(bobID.Ended) {
		t.Error("Bob's stream outlived his Session")
	}
	if err := s.RemovePasskey(alice, bob.ID, keys[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.Passkeys(alice, bob.ID); len(keys) != 0 {
		t.Errorf("Bob keeps %d Passkeys", len(keys))
	}
	me, bobs := party(alice), personParty(bob)
	recorded(t, c.take(),
		Entry{Event: SessionEnded, Actor: me, Subject: bobs, Detail: map[string]any{"reason": "revoked"}},
		Entry{Event: PasskeyRemoved, Actor: me, Subject: bobs})
}
