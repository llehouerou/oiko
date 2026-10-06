package access

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/access/passkeytest"
)

const public = "https://oiko.example"

// enrol gives Person by, fresh for step-up, a Passkey made by a new
// authenticator at origin, which it answers.
func enrol(t *testing.T, s *Store, by Identity, origin string) *passkeytest.Authenticator {
	t.Helper()
	a := passkeytest.New(origin)
	options, err := s.BeginPasskey(by, origin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskey(by, origin, a.Create(t, options)); err != nil {
		t.Fatal(err)
	}
	return a
}

// signIn signs in with a's Passkey at origin, answering the Session's secret.
func signIn(t *testing.T, s *Store, a *passkeytest.Authenticator, origin string) (string, error) {
	t.Helper()
	options, err := s.BeginSignIn(origin)
	if err != nil {
		t.Fatal(err)
	}
	return s.FinishSignIn(origin, a.Get(t, options), "Safari on iPhone")
}

// withPasskey is an Oiko claimed by Alice, who added a Passkey at the Public
// URL: her identity and her authenticator.
func withPasskey(t *testing.T, dir string, c *clock) (*Store, Identity, *passkeytest.Authenticator) {
	t.Helper()
	s, session := claimed(t, dir, c)
	alice, _ := s.Resolve(session)
	return s, alice, enrol(t, s, alice, public)
}

func TestAPersonSignsInWithTheirPasskey(t *testing.T) {
	for _, origin := range []string{public, "http://localhost:5173", "http://localhost"} {
		c, dir := newClock(), t.TempDir()
		s, session := claimed(t, dir, c)
		alice, _ := s.Resolve(session)
		a := enrol(t, s, alice, origin)
		c.advance(time.Hour)
		s = open(t, dir, c) // the Passkey survives a restart

		secret, err := signIn(t, s, a, origin)
		if err != nil {
			t.Fatalf("at %s: %v", origin, err)
		}
		id, ok := s.Resolve(secret)
		if !ok || id.ID != alice.ID || id.Level != Admin || !id.Fresh {
			t.Errorf("at %s: signed in as %+v, %v; want Alice, fresh", origin, id, ok)
		}
		passkeys, err := s.Passkeys(id)
		if err != nil || len(passkeys) != 1 {
			t.Fatalf("Passkeys = %v, %v", passkeys, err)
		}
		if k := passkeys[0]; !k.LastUse.Equal(c.t) || !k.Created.Equal(newClock().t) || Provider(k.AAGUID) != "Google Password Manager" {
			t.Errorf("Passkey = created %v, last used %v, provider %q", k.Created, k.LastUse, Provider(k.AAGUID))
		}
	}
}

func TestAPasskeyIsRefused(t *testing.T) {
	refused := func(t *testing.T, err error, want string) {
		t.Helper()
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), want) {
			t.Errorf("signing in: %v; want refused saying %q", err, want)
		}
	}
	t.Run("from another origin", func(t *testing.T) {
		s, _, a := withPasskey(t, t.TempDir(), newClock())
		a.Origin = "https://evil.example"
		_, err := signIn(t, s, a, public)
		refused(t, err, "not accepted")
	})
	t.Run("without user verification", func(t *testing.T) {
		s, _, a := withPasskey(t, t.TempDir(), newClock())
		a.NoUserVerification = true
		_, err := signIn(t, s, a, public)
		refused(t, err, "not accepted")
	})
	t.Run("as a Passkey not registered at all", func(t *testing.T) {
		s, alice, _ := withPasskey(t, t.TempDir(), newClock())
		stranger := passkeytest.New(public)
		options, _ := s.BeginPasskey(alice, public)
		stranger.Create(t, options) // never finished
		_, err := signIn(t, s, stranger, public)
		refused(t, err, "not accepted")
	})
	t.Run("whose counter went backwards", func(t *testing.T) {
		c, dir := newClock(), t.TempDir()
		s, _, a := withPasskey(t, dir, c)
		a.Counter = 5
		if _, err := signIn(t, s, a, public); err != nil {
			t.Fatal(err)
		}
		a.Counter = 5 // not greater
		_, err := signIn(t, open(t, dir, c), a, public)
		refused(t, err, "use another Passkey, or ask for a Sign-in link")
		a.Counter = 6
		if _, err := signIn(t, s, a, public); err != nil {
			t.Errorf("a counter going up again: %v", err)
		}
	})
	t.Run("synced, its counter always 0", func(t *testing.T) {
		s, _, a := withPasskey(t, t.TempDir(), newClock())
		for range 2 {
			if _, err := signIn(t, s, a, public); err != nil {
				t.Error(err)
			}
		}
	})
	t.Run("of a removed Person", func(t *testing.T) {
		c, dir := newClock(), t.TempDir()
		_, _, a := withPasskey(t, dir, c)
		if err := store.Save(filepath.Join(dir, "persons.json"), PersonsFormat, []Person{}); err != nil {
			t.Fatal(err)
		}
		_, err := signIn(t, open(t, dir, c), a, public)
		refused(t, err, "belongs to no one")
	})
	t.Run("answering a request used or too old", func(t *testing.T) {
		c := newClock()
		s, _, a := withPasskey(t, t.TempDir(), c)
		options, _ := s.BeginSignIn(public)
		answer := a.Get(t, options)
		if _, err := s.FinishSignIn(public, answer, ""); err != nil {
			t.Fatal(err)
		}
		_, err := s.FinishSignIn(public, answer, "")
		refused(t, err, "already used")

		options, _ = s.BeginSignIn(public)
		c.advance(5 * time.Minute)
		_, err = s.FinishSignIn(public, a.Get(t, options), "")
		refused(t, err, "ended")
	})
	t.Run("finished at another origin than it began", func(t *testing.T) {
		s, _, a := withPasskey(t, t.TempDir(), newClock())
		options, _ := s.BeginSignIn(public)
		a.Origin = "http://localhost:8080"
		_, err := s.FinishSignIn("http://localhost:8080", a.Get(t, options), "")
		refused(t, err, "try again")
	})
}

func TestConfirmingAPasskeyMakesTheSessionFresh(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice, _ := s.Resolve(session)
	a := enrol(t, s, alice, public)
	c.advance(10 * time.Minute)
	if id, _ := s.Resolve(session); id.Fresh {
		t.Fatal("fresh 10 minutes after signing in")
	}
	options, err := s.BeginStepUp(session, public)
	if err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if err := s.FinishStepUp(session, public, a.Get(t, options)); err != nil {
		t.Fatal(err)
	}
	c.advance(10*time.Minute - time.Second)
	if id, _ := s.Resolve(session); !id.Fresh {
		t.Error("not fresh just under 10 minutes after confirming a Passkey")
	}
	c.advance(time.Second)
	if id, _ := s.Resolve(session); id.Fresh {
		t.Error("fresh 10 minutes after confirming a Passkey")
	}

	// A step-up begun for one Session makes no other fresh.
	other, _ := signIn(t, s, a, public)
	c.advance(10 * time.Minute)
	options, _ = s.BeginStepUp(session, public)
	if err := s.FinishStepUp(other, public, a.Get(t, options)); !errors.Is(err, ErrRefused) {
		t.Errorf("another Session's step-up: %v, want ErrRefused", err)
	}
	// Nor does a sign-in's answer.
	options, _ = s.BeginSignIn(public)
	if err := s.FinishStepUp(session, public, a.Get(t, options)); !errors.Is(err, ErrRefused) {
		t.Errorf("a sign-in's answer as a step-up: %v, want ErrRefused", err)
	}
	if id, _ := s.Resolve(session); id.Fresh {
		t.Error("fresh after refused step-ups")
	}
}

func TestAPersonWithoutAPasskeyIsToldToSignInByLinkAgain(t *testing.T) {
	s, session := claimed(t, t.TempDir(), newClock())
	_, err := s.BeginStepUp(session, public)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Sign-in link") {
		t.Errorf("BeginStepUp without a Passkey: %v, want refused pointing to a Sign-in link", err)
	}
	if _, err := s.BeginStepUp("ended", public); !errors.Is(err, ErrRefused) {
		t.Errorf("BeginStepUp without a Session: %v, want ErrRefused", err)
	}
}

func TestAddingOrRemovingAPasskeyNeedsStepUp(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice, _ := s.Resolve(session)
	first := enrol(t, s, alice, public)
	second := enrol(t, s, alice, public)
	passkeys, _ := s.Passkeys(alice)
	if len(passkeys) != 2 {
		t.Fatalf("%d Passkeys, want 2", len(passkeys))
	}
	// Begun while fresh, finished once not.
	c.advance(9 * time.Minute)
	options, _ := s.BeginPasskey(alice, public)
	c.advance(time.Minute)
	alice, _ = s.Resolve(session)
	if _, err := s.FinishPasskey(alice, public, passkeytest.New(public).Create(t, options)); !errors.Is(err, ErrStale) {
		t.Errorf("finishing a Passkey, no longer fresh: %v, want ErrStale", err)
	}
	if _, err := s.BeginPasskey(alice, public); !errors.Is(err, ErrStale) {
		t.Errorf("adding a Passkey, not fresh: %v, want ErrStale", err)
	}
	if err := s.RemovePasskey(alice, passkeys[0].ID.String()); !errors.Is(err, ErrStale) {
		t.Errorf("removing a Passkey, not fresh: %v, want ErrStale", err)
	}

	confirm, _ := s.BeginStepUp(session, public)
	if err := s.FinishStepUp(session, public, second.Get(t, confirm)); err != nil {
		t.Fatal(err)
	}
	alice, _ = s.Resolve(session)
	if err := s.RemovePasskey(alice, passkeys[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := s.RemovePasskey(alice, passkeys[0].ID.String()); err == nil {
		t.Error("removing a removed Passkey: no error")
	}
	if _, err := signIn(t, s, first, public); !errors.Is(err, ErrRefused) {
		t.Errorf("signing in with a removed Passkey: %v, want ErrRefused", err)
	}
	if _, err := signIn(t, s, second, public); err != nil {
		t.Errorf("the Passkey kept: %v", err)
	}
}

func TestOnlyAPersonHasPasskeys(t *testing.T) {
	s, _, _ := withPasskey(t, t.TempDir(), newClock())
	program := Identity{Kind: ProgramKind, ID: "p1", Level: Admin, Fresh: true}
	if _, err := s.Passkeys(program); !errors.Is(err, ErrRefused) {
		t.Errorf("a Program's Passkeys: %v, want ErrRefused", err)
	}
	if _, err := s.BeginPasskey(program, public); !errors.Is(err, ErrRefused) {
		t.Errorf("a Program adding a Passkey: %v, want ErrRefused", err)
	}
}

func TestAPasskeyRequestIsSomeonesOwn(t *testing.T) {
	s, alice, _ := withPasskey(t, t.TempDir(), newClock())
	options, _ := s.BeginPasskey(alice, public)
	bob := Identity{Kind: PersonKind, ID: "bob", Fresh: true}
	if _, err := s.FinishPasskey(bob, public, passkeytest.New(public).Create(t, options)); !errors.Is(err, ErrRefused) {
		t.Errorf("finishing someone else's: %v, want ErrRefused", err)
	}
}

func TestTheCeremonyCapDropsTheOldest(t *testing.T) {
	s, _, a := withPasskey(t, t.TempDir(), newClock())
	first, _ := s.BeginSignIn(public)
	second, _ := s.BeginSignIn(public)
	for range maxCeremonies - 2 {
		if _, err := s.BeginSignIn(public); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.FinishSignIn(public, a.Get(t, second), ""); err != nil {
		t.Fatalf("at the cap, the second oldest: %v", err)
	}
	s.BeginSignIn(public) // at the cap again
	s.BeginSignIn(public) // past it: the oldest goes
	if _, err := s.FinishSignIn(public, a.Get(t, first), ""); !errors.Is(err, ErrRefused) {
		t.Errorf("the oldest, past the cap: %v, want ErrRefused", err)
	}
}
