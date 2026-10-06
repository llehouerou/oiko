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

// person is a Person named name at level, created by Alice, the Admin of
// claimed.
func person(t *testing.T, s *Store, alice Identity, name string, level Level) Person {
	t.Helper()
	p, err := s.CreatePerson(alice, name, level)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// link creates a Sign-in link for Person id, by by, answering its secret.
func link(t *testing.T, s *Store, by Identity, id string) string {
	t.Helper()
	secret, _, err := s.CreateLink(by, id)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// linkOf is the pending Sign-in link of Person id, as an Admin sees it.
func linkOf(t *testing.T, s *Store, alice Identity, id string) *Link {
	t.Helper()
	ps, err := s.Persons(alice)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ID == id {
			return p.Link
		}
	}
	t.Fatalf("no Person %s", id)
	return nil
}

func TestAnInvitedPersonSignsInWithTheirLink(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, " Bob ", Member)
	if bob.Name != "Bob" || bob.Level != Member || bob.ID == "" {
		t.Fatalf("created %+v", bob)
	}
	secret, expires, err := s.CreateLink(alice, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) < 22 || !expires.Equal(c.t.Add(24*time.Hour)) { // 128 bits in base32
		t.Errorf("CreateLink = %q, %v; want 128 bits, for 24 hours", secret, expires)
	}
	if name, err := s.LinkedName(secret); err != nil || name != "Bob" {
		t.Errorf("LinkedName = %q, %v; want Bob", name, err)
	}
	bobs, err := s.SignInWithLink(secret, "Safari on iPhone")
	if err != nil {
		t.Fatal(err)
	}
	id, ok := s.Resolve(bobs)
	if !ok || id.ID != bob.ID || id.Level != Member || !id.Fresh {
		t.Errorf("Resolve = %+v, %v; want Bob, fresh", id, ok)
	}
	if x := s.sessions[hash(bobs)]; x.Method != "link" || x.By != alice.ID {
		t.Errorf("Bob's Session: %+v; want signed in by link, labelled with Alice", x)
	}
	// Single use.
	if _, err := s.SignInWithLink(secret, ""); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "ask whoever sent it") {
		t.Errorf("used again: %v, want refused, telling to ask whoever sent it", err)
	}
	if _, err := s.LinkedName(secret); !errors.Is(err, ErrRefused) {
		t.Error("a used link still names its Person")
	}
	if l := linkOf(t, s, alice, bob.ID); l != nil {
		t.Errorf("a used link is still pending: %+v", l)
	}
}

func TestALinkLasts24HoursFromAnAdminAnd15MinutesForOneself(t *testing.T) {
	for _, c := range []struct {
		name  string
		life  time.Duration
		level Level
		self  bool
	}{
		{"an Admin's for another", 24 * time.Hour, Member, false},
		{"an Admin's for another Admin", 24 * time.Hour, Admin, false},
		{"a Member's for themself", 15 * time.Minute, Member, true},
		{"a Guest's for themself", 15 * time.Minute, Guest, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			clk := newClock()
			s, session := claimed(t, t.TempDir(), clk)
			alice := admin(t, s, session)
			bob := person(t, s, alice, "Bob", c.level)
			by := alice
			if c.self {
				by = signedInAs(t, s, alice, bob.ID)
			}
			secret, expires, err := s.CreateLink(by, bob.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !expires.Equal(clk.t.Add(c.life)) {
				t.Errorf("expires %v after creation, want %v", expires.Sub(clk.t), c.life)
			}
			clk.advance(c.life)
			if _, err := s.LinkedName(secret); err != nil {
				t.Error("refused at its last instant")
			}
			clk.advance(time.Nanosecond)
			if _, err := s.LinkedName(secret); err == nil {
				t.Error("named its Person once expired")
			}
			if _, err := s.SignInWithLink(secret, ""); !errors.Is(err, ErrRefused) {
				t.Errorf("expired: %v, want ErrRefused", err)
			}
			if l := linkOf(t, s, admin(t, s, session), bob.ID); l != nil {
				t.Errorf("an expired link is pending: %+v", l)
			}
		})
	}
}

// signedInAs signs Person id in through a link Alice creates, answering
// their identity, fresh.
func signedInAs(t *testing.T, s *Store, alice Identity, id string) Identity {
	t.Helper()
	session, err := s.SignInWithLink(link(t, s, alice, id), "")
	if err != nil {
		t.Fatal(err)
	}
	who, _ := s.Resolve(session)
	return who
}

func TestAPersonHasOnePendingLinkWhichAnAdminRevokes(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Member)
	first := link(t, s, alice, bob.ID)
	c.advance(time.Minute)
	bobID := signedInAs(t, s, alice, bob.ID) // Bob, signed in by a second link
	if _, err := s.LinkedName(first); err == nil {
		t.Error("a second link left the first one pending")
	}
	// Bob's own link replaces Alice's, whoever created it.
	byAlice := link(t, s, alice, bob.ID)
	byBob := link(t, s, bobID, bob.ID)
	if _, err := s.LinkedName(byAlice); err == nil {
		t.Error("Bob's link left Alice's pending")
	}
	l := linkOf(t, s, alice, bob.ID)
	if l == nil || l.Creator != bob.ID || !l.Created.Equal(c.t) || !l.Expires.Equal(c.t.Add(15*time.Minute)) {
		t.Errorf("pending: %+v; want Bob's, for 15 minutes", l)
	}
	if err := s.RevokeLink(alice, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignInWithLink(byBob, ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a revoked link: %v, want ErrRefused", err)
	}
	if err := s.RevokeLink(alice, bob.ID); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("revoking no link: %v, want ErrNotFound", err)
	}
}

func TestOnlyAFreshAdminPersonManagesPersonsAndOthersLinks(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Member)
	_, token := program(t, s, alice, "Node-RED", Admin)
	bot, _ := s.ResolveToken(token)
	stale := alice
	stale.Fresh = false
	bobID := signedInAs(t, s, alice, bob.ID)
	for name, by := range map[string]Identity{"an Admin Program": bot, "a stale Admin": stale, "a Member": bobID, "anonymous": {}} {
		for action, err := range map[string]error{
			"create":      func() error { _, err := s.CreatePerson(by, "Carol", Guest); return err }(),
			"edit":        s.EditPerson(by, alice.ID, "Alice", Guest),
			"remove":      s.RemovePerson(by, alice.ID),
			"link":        func() error { _, _, err := s.CreateLink(by, alice.ID); return err }(),
			"revoke link": s.RevokeLink(by, bob.ID),
			"list":        func() error { _, err := s.Persons(by); return err }(),
		} {
			if name == "a stale Admin" && action == "list" {
				if err != nil {
					t.Errorf("a stale Admin lists Persons: %v", err)
				}
				continue
			}
			if !errors.Is(err, ErrRefused) {
				t.Errorf("%s, %s: %v, want ErrRefused", name, action, err)
			}
		}
	}
	// A link for oneself needs step-up too.
	c.advance(freshFor)
	bobID, _ = s.Resolve(s.sessionOf(t, bob.ID))
	if _, _, err := s.CreateLink(bobID, bob.ID); !errors.Is(err, ErrStale) {
		t.Errorf("a stale Session's own link: %v, want ErrStale", err)
	}
	if _, err := s.CreatePerson(alice, " ", Guest); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an empty Name: %v, want ErrInvalid", err)
	}
	if _, err := s.CreatePerson(alice, "Carol", "owner"); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an unknown level: %v, want ErrInvalid", err)
	}
	if _, _, err := s.CreateLink(alice, "unknown"); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("a link for no one: %v, want ErrNotFound", err)
	}
}

// sessionOf is the secret of a Session of Person id, for a test that needs
// it again: it opens a new one, as old as the clock.
func (s *Store) sessionOf(t *testing.T, id string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, err := s.signIn(id, "", "link", "")
	if err != nil {
		t.Fatal(err)
	}
	s.sessions[hash(secret)].SignedIn = s.now().Add(-freshFor)
	return secret
}

func TestTheLastAdminPersonIsNeitherDemotedNorRemoved(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	_, token := program(t, s, alice, "Node-RED", Admin) // an Admin, but no Person
	for action, err := range map[string]error{
		"demoting": s.EditPerson(alice, alice.ID, "Alice", Member),
		"removing": s.RemovePerson(alice, alice.ID),
	} {
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "last Admin") {
			t.Errorf("%s the last Admin: %v, want refused saying why", action, err)
		}
	}
	if id, ok := s.Resolve(session); !ok || id.Level != Admin {
		t.Error("refused changes changed Alice")
	}
	// With a second Admin, either may go.
	bob := person(t, s, alice, "Bob", Admin)
	if err := s.EditPerson(alice, bob.ID, "Bob", Member); err != nil {
		t.Fatal(err)
	}
	if err := s.EditPerson(alice, alice.ID, "Alice", Member); !errors.Is(err, ErrRefused) {
		t.Errorf("demoting the last Admin again: %v, want ErrRefused", err)
	}
	if err := s.EditPerson(alice, bob.ID, "Bob", Admin); err != nil {
		t.Fatal(err)
	}
	if err := s.RemovePerson(alice, bob.ID); err != nil {
		t.Errorf("removing one of two Admins: %v", err)
	}
	if _, ok := s.ResolveToken(token); !ok {
		t.Error("the Admin Program refused")
	}
}

func TestChangingAPersonsLevelEndsTheirStreamsNotTheirSessions(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Member)
	phone, laptop := signedInAs(t, s, alice, bob.ID), signedInAs(t, s, alice, bob.ID)
	if err := s.EditPerson(alice, bob.ID, "Robert", Member); err != nil {
		t.Fatal(err)
	}
	if closed(phone.Ended) || closed(laptop.Ended) {
		t.Error("a rename closed the streams")
	}
	if err := s.EditPerson(alice, bob.ID, "Robert", Guest); err != nil {
		t.Fatal(err)
	}
	if !closed(phone.Ended) || !closed(laptop.Ended) {
		t.Error("a level change left a stream open")
	}
	if closed(admin(t, s, session).Ended) {
		t.Error("Bob's level closed Alice's stream")
	}
	ps, _ := s.Persons(alice)
	if n := len(s.sessions); n != 3 {
		t.Errorf("%d Sessions after a level change, want 3: they stay signed in", n)
	}
	if ps[1].Name != "Robert" || ps[1].Level != Guest {
		t.Errorf("after the change: %+v", ps[1])
	}
}

func TestRemovingAPersonEndsTheirSessionsAndLeavesWhatTheyCreated(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Admin)
	bobID := signedInAs(t, s, alice, bob.ID)
	bobs := s.sessionOf(t, bob.ID)
	_, token := program(t, s, bobID, "Node-RED", Member)
	pendingLink := link(t, s, alice, bob.ID)
	if err := s.RemovePerson(alice, bob.ID); err != nil {
		t.Fatal(err)
	}
	if !closed(bobID.Ended) {
		t.Error("removing left Bob's stream open")
	}
	for _, s := range []*Store{s, open(t, dir, c)} {
		if _, ok := s.Resolve(bobs); ok {
			t.Error("a Session of a removed Person accepted")
		}
		if _, err := s.SignInWithLink(pendingLink, ""); !errors.Is(err, ErrRefused) {
			t.Errorf("the link of a removed Person: %v, want ErrRefused", err)
		}
		if _, ok := s.ResolveToken(token); !ok {
			t.Error("the Program of a removed Person refused")
		}
	}
	if err := s.RemovePerson(alice, bob.ID); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("removing again: %v, want ErrNotFound", err)
	}
}

func TestAPersonRenamesThemselfButNeverChangesTheirLevel(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Guest)
	bobID := signedInAs(t, s, alice, bob.ID)
	c.advance(freshFor) // no step-up for one's own Name
	if err := s.Rename(bobID, " Robert "); err != nil {
		t.Fatal(err)
	}
	if name := s.PersonName(bob.ID); name != "Robert" {
		t.Errorf("Name %q, want Robert", name)
	}
	if err := s.Rename(bobID, ""); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an empty Name: %v, want ErrInvalid", err)
	}
	_, token := program(t, s, alice, "Node-RED", Admin)
	bot, _ := s.ResolveToken(token)
	if err := s.Rename(bot, "Mallory"); !errors.Is(err, ErrRefused) {
		t.Errorf("a Program renaming itself: %v, want ErrRefused", err)
	}
	// An Admin with another Admin still does not change their own level.
	person(t, s, alice, "Carol", Admin)
	alice = admin(t, s, session)
	alice.Fresh = true
	if err := s.EditPerson(alice, alice.ID, "Alice", Member); !errors.Is(err, ErrRefused) {
		t.Errorf("an Admin demoting themself: %v, want ErrRefused", err)
	}
}

func TestLinksSurviveARestartAndPersonsJSONHoldsNoSecret(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	alice := admin(t, s, session)
	bob := person(t, s, alice, "Bob", Member)
	secret := link(t, s, alice, bob.ID)
	data, err := os.ReadFile(filepath.Join(dir, "persons.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Error("persons.json holds the link's secret")
	}
	if name, err := open(t, dir, c).LinkedName(secret); err != nil || name != "Bob" {
		t.Errorf("after a restart: %q, %v", name, err)
	}
	// An expired link is dropped from the document.
	c.advance(25 * time.Hour)
	open(t, dir, c)
	if data, _ := os.ReadFile(filepath.Join(dir, "persons.json")); strings.Contains(string(data), hash(secret)) {
		t.Error("persons.json keeps an expired link")
	}
}
