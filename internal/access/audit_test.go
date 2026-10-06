package access

import (
	"reflect"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access/passkeytest"
)

// take answers what c's Audit log recorded since the last take.
func (c *clock) take() []Entry {
	log := c.log
	c.log = nil
	return log
}

// recorded checks that got are the Entries want, in order, by their event,
// actor, subject and browser, and, where want has one, their detail.
func recorded(t *testing.T, got []Entry, want ...Entry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("recorded %d entries, want %d:\n%+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Event != w.Event || g.Actor != w.Actor || g.Subject != w.Subject || g.Browser != w.Browser {
			t.Errorf("entry %d: %s by %+v of %+v in %q, want %s by %+v of %+v in %q", i, g.Event, g.Actor, g.Subject, g.Browser, w.Event, w.Actor, w.Subject, w.Browser)
		}
		if w.Detail != nil && !reflect.DeepEqual(g.Detail, w.Detail) {
			t.Errorf("entry %d, %s: detail %v, want %v", i, g.Event, g.Detail, w.Detail)
		}
		if g.Time.IsZero() {
			t.Errorf("entry %d, %s: no time", i, g.Event)
		}
	}
}

func TestSigningInAndOutIsRecorded(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s := open(t, dir, c)
	s.Setup()
	if _, err := s.Claim("wrong", "Mallory", "Firefox on Linux"); err == nil {
		t.Fatal("a wrong Setup link accepted")
	}
	recorded(t, c.take(), Entry{Event: SetupRefused, Actor: nobody, Subject: nobody, Browser: "Firefox on Linux"})

	secret, _ := s.Setup()
	session, _ := s.Claim(secret, "Alice", "Firefox on Linux")
	alice, _ := s.Resolve(session)
	me := party(alice)
	recorded(t, c.take(),
		Entry{Event: PersonCreated, Actor: me, Subject: me, Detail: map[string]any{"level": Admin}},
		Entry{Event: SignedIn, Actor: me, Subject: me, Browser: "Firefox on Linux", Detail: map[string]any{"method": "setup"}})

	bob := person(t, s, alice, "Bob", Member)
	bobs := personParty(bob)
	secret = link(t, s, alice, bob.ID)
	c.take()
	bobsSession, err := s.SignInWithLink(secret, "Chrome on Android")
	if err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: SignedIn, Actor: bobs, Subject: bobs, Browser: "Chrome on Android", Detail: map[string]any{"method": "link", "by": me}})

	if err := s.SignOut(bobsSession); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: SessionEnded, Actor: bobs, Subject: bobs, Browser: "Chrome on Android", Detail: map[string]any{"reason": "signed out"}})

	// A Session that ended by itself is recorded when noticed, dated when it ended.
	lastUse := c.t
	c.advance(31 * 24 * time.Hour)
	if _, ok := s.Resolve(session); ok {
		t.Fatal("a Session idle 31 days accepted")
	}
	got := c.take()
	recorded(t, got, Entry{Event: SessionEnded, Actor: oiko, Subject: me, Browser: "Firefox on Linux", Detail: map[string]any{"reason": "expired"}})
	if want := lastUse.Add(30 * 24 * time.Hour); !got[0].Time.Equal(want) {
		t.Errorf("expiry dated %v, want %v", got[0].Time, want)
	}
}

func TestAnExpiryNoticedOnLoadIsRecorded(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	_, session := claimed(t, dir, c)
	s := open(t, dir, c)
	alice, _ := s.Resolve(session)
	c.take()
	c.advance(31 * 24 * time.Hour)
	open(t, dir, c)
	recorded(t, c.take(), Entry{Event: SessionEnded, Actor: oiko, Subject: party(alice), Browser: "Firefox on Linux"})
}

func TestChangesToPersonsAndLinksAreRecorded(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	me := party(alice)
	c.take()

	bob := person(t, s, alice, "Bob", Member)
	bobs := personParty(bob)
	recorded(t, c.take(), Entry{Event: PersonCreated, Actor: me, Subject: bobs, Detail: map[string]any{"level": Member}})

	if err := s.EditPerson(alice, bob.ID, "Robert", Guest); err != nil {
		t.Fatal(err)
	}
	robert := Party{PersonKind, bob.ID, "Robert"}
	recorded(t, c.take(),
		Entry{Event: PersonRenamed, Actor: me, Subject: robert, Detail: map[string]any{"from": "Bob"}},
		Entry{Event: LevelChanged, Actor: me, Subject: robert, Detail: map[string]any{"from": Member, "to": Guest}})

	link(t, s, alice, bob.ID)
	recorded(t, c.take(), Entry{Event: LinkCreated, Actor: me, Subject: robert, Detail: map[string]any{"expires": c.t.Add(24 * time.Hour)}})
	if err := s.RevokeLink(alice, bob.ID); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: LinkRevoked, Actor: me, Subject: robert})

	robertID := signedInAs(t, s, alice, bob.ID)
	c.take()
	if err := s.Rename(robertID, "Rob"); err != nil {
		t.Fatal(err)
	}
	rob := Party{PersonKind, bob.ID, "Rob"}
	recorded(t, c.take(), Entry{Event: PersonRenamed, Actor: rob, Subject: rob, Detail: map[string]any{"from": "Robert"}})

	// An unused link expires, recorded when noticed, dated when it did.
	link(t, s, alice, bob.ID)
	expires := c.t.Add(24 * time.Hour)
	c.take()
	c.advance(25 * time.Hour)
	alice = admin(t, s, session)
	alice.Fresh = true
	person(t, s, alice, "Carol", Guest)
	got := c.take()
	recorded(t, got[:1], Entry{Event: LinkExpired, Actor: oiko, Subject: rob})
	if !got[0].Time.Equal(expires) {
		t.Errorf("expiry dated %v, want %v", got[0].Time, expires)
	}

	if err := s.RemovePerson(alice, bob.ID); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(),
		Entry{Event: PersonRemoved, Actor: me, Subject: rob},
		Entry{Event: SessionEnded, Actor: me, Subject: rob, Detail: map[string]any{"reason": "removed"}})
}

func TestPasskeysAreRecorded(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, alice, a := withPasskey(t, dir, c)
	me := party(alice)
	got := c.take()
	recorded(t, got[len(got)-1:], Entry{Event: PasskeyAdded, Actor: me, Subject: me, Detail: map[string]any{"provider": "Google Password Manager"}})

	session, err := signIn(t, s, a, public)
	if err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: SignedIn, Actor: me, Subject: me, Browser: "Safari on iPhone", Detail: map[string]any{"method": "passkey"}})

	// Another Oiko's Passkey is anonymous; a counter going backwards is its
	// Person's, as is a Passkey never added that says it is theirs.
	_, _, stranger := withPasskey(t, t.TempDir(), newClock())
	if _, err := signIn(t, s, stranger, public); err == nil {
		t.Fatal("an unknown Passkey signed in")
	}
	recorded(t, c.take(), Entry{Event: PasskeyRefused, Actor: nobody, Subject: nobody, Browser: "Safari on iPhone"})
	impostor := passkeytest.New(public)
	creation, _ := s.BeginPasskey(alice, public)
	impostor.Create(t, creation)
	signIn(t, s, impostor, public)
	recorded(t, c.take(), Entry{Event: PasskeyRefused, Actor: nobody, Subject: me, Browser: "Safari on iPhone"})
	a.Counter = 5
	signIn(t, s, a, public)
	c.take()
	if _, err := signIn(t, s, a, public); err == nil {
		t.Fatal("a counter going backwards signed in")
	}
	recorded(t, c.take(), Entry{Event: PasskeyRefused, Actor: nobody, Subject: me, Browser: "Safari on iPhone"})

	// A step-up that fails is the Person's; one that succeeds is not recorded.
	c.advance(time.Hour)
	options, err := s.BeginStepUp(session, public)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishStepUp(session, public, a.Get(t, options)); err == nil {
		t.Fatal("a step-up with a counter going backwards accepted")
	}
	recorded(t, c.take(), Entry{Event: StepUpRefused, Actor: me, Subject: me, Browser: "Safari on iPhone"})
	a.Counter = 10
	options, _ = s.BeginStepUp(session, public)
	if err := s.FinishStepUp(session, public, a.Get(t, options)); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take())

	alice, _ = s.Resolve(session)
	passkeys, _ := s.Passkeys(alice)
	if err := s.RemovePasskey(alice, passkeys[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: PasskeyRemoved, Actor: me, Subject: me, Detail: map[string]any{"provider": "Google Password Manager"}})
}

func TestProgramsAndTokensAreRecorded(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	me := party(alice)
	c.take()

	p, err := s.CreateProgram(alice, "Node-RED", Member)
	if err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: ProgramCreated, Actor: me, Subject: programParty(p), Detail: map[string]any{"level": Member}})
	if err := s.EditProgram(alice, p.ID, "Scripts", Admin); err != nil {
		t.Fatal(err)
	}
	scripts := Party{ProgramKind, p.ID, "Scripts"}
	recorded(t, c.take(),
		Entry{Event: ProgramRenamed, Actor: me, Subject: scripts, Detail: map[string]any{"from": "Node-RED"}},
		Entry{Event: LevelChanged, Actor: me, Subject: scripts, Detail: map[string]any{"from": Member, "to": Admin}})
	if _, err := s.GenerateToken(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: TokenGenerated, Actor: me, Subject: scripts})
	if err := s.RevokeToken(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: TokenRevoked, Actor: me, Subject: scripts})
	if _, ok := s.ResolveToken("oiko_bogus"); ok {
		t.Fatal("a bogus Token accepted")
	}
	recorded(t, c.take(), Entry{Event: TokenRefused, Actor: nobody, Subject: nobody})
	if err := s.RemoveProgram(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	recorded(t, c.take(), Entry{Event: ProgramRemoved, Actor: me, Subject: scripts})
}

func TestARefusedLinkIsRecordedAnonymously(t *testing.T) {
	c := newClock()
	s, _ := claimed(t, t.TempDir(), c)
	c.take()
	s.LinkedName("bogus", "Chrome on Android")
	s.SignInWithLink("bogus", "Chrome on Android")
	refused := Entry{Event: LinkRefused, Actor: nobody, Subject: nobody, Browser: "Chrome on Android"}
	recorded(t, c.take(), refused, refused)
}
