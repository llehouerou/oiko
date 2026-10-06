// Package access keeps who may use Oiko (ADR 0022, 0032): the Persons, their
// Passkeys, their pending Sign-in links and their Sessions, in persons.json
// and sessions.json, the Programs and their Tokens, in programs.json, all
// loaded at start and held in memory, and, in memory only, the Setup link
// that claims a fresh Oiko (ADR 0026) and the WebAuthn ceremonies in
// progress. Secrets are 128 random bits or more, kept only as their SHA-256
// hash; Oiko alone decides when a Session ends (ADR 0025).
package access

import (
	"container/list"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// The formats of persons.json and sessions.json; format 1 each.
var (
	PersonsFormat  store.Format
	SessionsFormat store.Format
)

// Lifetimes of a Person's Session (ADR 0025), and how often its last use
// reaches the disk (ADR 0032).
const (
	idleLimit    = 30 * 24 * time.Hour
	SessionLimit = 365 * 24 * time.Hour // the longest a Session lasts, used or not
	freshFor     = 10 * time.Minute
	writeLastUse = time.Hour
)

// ErrRefused is a credential or an action refused; its message says why.
var ErrRefused = errors.New("refused")

// ErrStale is an action needing step-up (ADR 0025) asked from a Session that
// has not proved itself lately.
var ErrStale = fmt.Errorf("%w: sign in again to do this", ErrRefused)

// Level is an Access level: Guest < Member < Admin (ADR 0023).
type Level string

const (
	Guest  Level = "guest"
	Member Level = "member"
	Admin  Level = "admin"
)

// levels are the Access levels, each allowing what those before it do.
var levels = []Level{Guest, Member, Admin}

func (l Level) valid() bool { return slices.Contains(levels, l) }

// Allows reports whether l does what need does; no level allows anything,
// and nothing allows a level that is none.
func (l Level) Allows(need Level) bool {
	return l.valid() && need.valid() && slices.Index(levels, l) >= slices.Index(levels, need)
}

// Person is a human known to Oiko. A Guest may have an end date, when their
// access ends while they stay (ADR 0030).
type Person struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Level    Level     `json:"level"`
	Ends     time.Time `json:"ends,omitzero"`
	Ended    bool      `json:"ended,omitempty"` // Oiko noticed their end date came: their Sessions ended
	Passkeys []Passkey `json:"passkeys,omitempty"`
	Link     *Link     `json:"link,omitempty"` // their unused Sign-in link, if any
}

// session is a Session as stored: its secret's hash, the id the API names it
// by, whose it is, the browser it lives in, how and when it signed in, when
// it last confirmed a Passkey, and when it was last used.
type session struct {
	Hash      string    `json:"hash"`
	ID        string    `json:"id"`
	Person    string    `json:"person"`
	Browser   string    `json:"browser"`
	Method    string    `json:"method"`             // how it signed in: "setup", "passkey", "link", or "host": a link from Oiko's host
	Provider  string    `json:"provider,omitempty"` // the provider of the Passkey it signed in with, if known
	By        string    `json:"by,omitempty"`       // the Person who created the link it signed in with, when not its own
	SignedIn  time.Time `json:"signedIn"`
	Confirmed time.Time `json:"confirmed,omitzero"`
	LastUse   time.Time `json:"lastUse"`
}

// Session is a Session as its Person, or an Admin, sees it (ADR 0025): its
// id, the browser it lives in, how it signed in (with a Passkey from
// Provider, if known; with a link By created, if not their own), when, and
// when it was last used.
type Session struct {
	ID, Browser, Method, Provider, By string
	SignedIn, LastUse                 time.Time
}

// fresh reports whether Session x signed in, or confirmed a Passkey, lately
// enough for step-up at now (ADR 0025).
func (x *session) fresh(now time.Time) bool {
	return now.Sub(x.SignedIn) < freshFor || now.Sub(x.Confirmed) < freshFor
}

// Kind is what an identity is, or, in the Audit log, who else acts.
type Kind string

const (
	PersonKind  Kind = "person"
	ProgramKind Kind = "program"
	HostKind    Kind = "host"    // Oiko's host: oiko sign-in-link
	OikoKind    Kind = "oiko"    // Oiko itself: an expiry
	UnknownKind Kind = "unknown" // no identity found
)

// Identity is who a request is, as they are now: a Person by their Session,
// or a Program by its Token.
type Identity struct {
	Kind    Kind
	ID      string
	Name    string
	Level   Level
	Session string          // the id of the Session it holds, if any
	Fresh   bool            // a Session that proved itself lately enough for step-up
	Ended   <-chan struct{} // closed once this Session or Token ends, or the identity's access changes
}

// StepUp refuses an action needing step-up unless the Session is fresh.
func (id Identity) StepUp() error {
	if !id.Fresh {
		return ErrStale
	}
	return nil
}

// Store holds the Persons and their Sessions, and the Programs, and writes
// every refusal and change to access to the Audit log.
type Store struct {
	personsFile, sessionsFile, programsFile string
	now                                     func() time.Time
	audit                                   func(Entry)
	mu                                      sync.Mutex
	persons                                 []Person
	sessions                                map[string]*session // by hash
	programs                                []Program
	ends                                    map[string]chan struct{} // closed when an access ends or changes: a Session's by its hash, a Program's by its id
	setup                                   string                   // the Setup link's hash; "" when there is none
	sessionUses                             pending
	programUses                             pending
	ceremonies                              ceremonies
}

// pending tells when a document was last written, and whether last uses held
// in memory are not yet (ADR 0032).
type pending struct {
	written time.Time
	unsaved bool
}

// Open loads the Persons, their Sessions and the Programs from dir, dropping
// the Sessions and Sign-in links that ended, and the Sessions whose Person
// is gone. now is the clock; audit writes an Entry to the Audit log, never
// waiting.
func Open(dir string, now func() time.Time, audit func(Entry)) (*Store, error) {
	s := &Store{
		personsFile:  filepath.Join(dir, "persons.json"),
		sessionsFile: filepath.Join(dir, "sessions.json"),
		programsFile: filepath.Join(dir, "programs.json"),
		now:          now,
		audit:        audit,
		sessions:     map[string]*session{},
		ends:         map[string]chan struct{}{},
		sessionUses:  pending{written: now()},
		programUses:  pending{written: now()},
		ceremonies:   ceremonies{by: map[string]*list.Element{}},
	}
	var sessions []*session
	if err := store.Load(s.personsFile, PersonsFormat, &s.persons); err != nil {
		return nil, err
	}
	if err := store.Load(s.sessionsFile, SessionsFormat, &sessions); err != nil {
		return nil, err
	}
	if err := store.Load(s.programsFile, ProgramsFormat, &s.programs); err != nil {
		return nil, err
	}
	for _, x := range sessions {
		if s.person(x.Person) >= 0 { // else recorded with its Person's removal
			s.sessions[x.Hash] = x
		}
	}
	// Notices, and records, what ended while Oiko was stopped.
	if err := s.saveSessions(); err != nil {
		return nil, err
	}
	if slices.ContainsFunc(s.persons, func(p Person) bool { return p.Link != nil && !live(p.Link, now()) }) {
		if err := s.savePersons(slices.Clone(s.persons)); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Setup draws a new Setup link's secret while no Admin exists, replacing the
// previous one; false once Oiko is claimed. It lives in memory only.
func (s *Store) Setup() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed() {
		return "", false
	}
	secret := rand.Text()
	s.setup = hash(secret)
	return secret, true
}

// Claimed reports whether an Admin exists.
func (s *Store) Claimed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimed()
}

func (s *Store) claimed() bool {
	return slices.ContainsFunc(s.persons, func(p Person) bool { return p.Level == Admin })
}

// Claim spends the Setup link's secret: it creates the first Person, named
// name, as an Admin, and answers the secret of their new Session in browser.
func (s *Store) Claim(secret, name, browser string) (string, error) {
	name, err := home.ValidName(name)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setup == "" || s.claimed() || subtle.ConstantTimeCompare([]byte(hash(secret)), []byte(s.setup)) != 1 {
		s.record(Entry{Event: SetupRefused, Actor: nobody, Subject: nobody, Browser: browser})
		return "", fmt.Errorf("%w: this Setup link is no longer valid; Oiko's log has the current one, if Oiko has no Admin yet", ErrRefused)
	}
	// The Session first: written without its Person, it is dropped on load.
	p := Person{ID: uuid.NewV7().String(), Name: name, Level: Admin}
	x := session{Person: p.ID, Browser: browser, Method: "setup"}
	session, err := s.signIn(&x)
	if err != nil {
		return "", err
	}
	if err := s.savePersons(append(slices.Clone(s.persons), p)); err != nil {
		delete(s.sessions, hash(session))
		return "", err
	}
	s.setup = ""
	s.record(Entry{Event: PersonCreated, Actor: personParty(p), Subject: personParty(p), Detail: map[string]any{"level": p.Level}})
	s.recordSignIn(p, x)
	return session, nil
}

// signIn opens Session x, of its Person, browser and how it signed in, and
// answers its secret. Callers hold s.mu.
func (s *Store) signIn(x *session) (string, error) {
	secret := rand.Text()
	x.Hash, x.ID = hash(secret), uuid.NewV7().String()
	x.SignedIn = s.now()
	x.LastUse = x.SignedIn
	s.sessions[x.Hash] = x
	if err := s.saveSessions(); err != nil {
		delete(s.sessions, x.Hash)
		return "", err
	}
	return secret, nil
}

// recordSignIn records that Person p signed in, opening Session x. Callers
// hold s.mu.
func (s *Store) recordSignIn(p Person, x session) {
	detail := map[string]any{"method": x.Method}
	if x.By != "" {
		detail["by"] = s.personParty(x.By)
	}
	if x.Provider != "" {
		detail["provider"] = x.Provider
	}
	s.record(Entry{Event: SignedIn, Actor: personParty(p), Subject: personParty(p), Browser: x.Browser, Detail: detail})
}

// personParty is Person p as the Audit log names them.
func personParty(p Person) Party { return Party{PersonKind, p.ID, p.Name} }

// personParty is Person id as the Audit log names them: by id alone once
// they are removed. Callers hold s.mu.
func (s *Store) personParty(id string) Party {
	if i := s.person(id); i >= 0 {
		return personParty(s.persons[i])
	}
	return Party{Kind: PersonKind, ID: id}
}

// Resolve answers who holds the Session of secret, counting it as a use; false
// if there is no such Session, or it has ended.
func (s *Store) Resolve(secret string) (Identity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.sessions[hash(secret)]
	if x == nil {
		return Identity{}, false
	}
	i := s.person(x.Person)
	switch {
	case i < 0: // recorded with its Person's removal
		s.end(x)
		return Identity{}, false
	case s.ended(x):
		s.saveSessions() // records it ended
		return Identity{}, false
	}
	now := s.now()
	x.LastUse = now
	s.used(&s.sessionUses, s.saveSessions)
	p := s.persons[i]
	return Identity{Kind: PersonKind, ID: p.ID, Name: p.Name, Level: p.Level, Session: x.ID, Fresh: x.fresh(now), Ended: s.ending(x.Hash)}, true
}

// ending is what closes when the access of key ends or changes, made on
// first use. Callers hold s.mu.
func (s *Store) ending(key string) <-chan struct{} {
	ch, ok := s.ends[key]
	if !ok {
		ch = make(chan struct{})
		s.ends[key] = ch
	}
	return ch
}

// finish ends the access of key: it closes what ending made for it. Callers
// hold s.mu.
func (s *Store) finish(key string) {
	if ch, ok := s.ends[key]; ok {
		close(ch)
		delete(s.ends, key)
	}
}

// SignOut ends the Session of secret, if there is one.
func (s *Store) SignOut(secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.sessions[hash(secret)]; x != nil {
		s.record(Entry{Event: SessionEnded, Actor: s.personParty(x.Person), Subject: s.personParty(x.Person), Browser: x.Browser, Detail: map[string]any{"reason": "signed out"}})
		return s.end(x)
	}
	return nil
}

// Flush writes last uses not yet written, on shutdown.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	if s.sessionUses.unsaved {
		errs = append(errs, s.saveSessions())
	}
	if s.programUses.unsaved {
		errs = append(errs, s.savePrograms(s.programs))
	}
	return errors.Join(errs...)
}

// used notes a last use held in memory for the document p tracks, and writes
// it with save an hour after its last write. Callers hold s.mu.
func (s *Store) used(p *pending, save func() error) {
	p.unsaved = true
	if s.now().Sub(p.written) >= writeLastUse {
		if err := save(); err != nil {
			slog.Error("access: writing last use", "err", err)
		}
	}
}

// end deletes Session x, ending its streams. Callers hold s.mu.
func (s *Store) end(x *session) error {
	delete(s.sessions, x.Hash)
	s.finish(x.Hash)
	return s.saveSessions()
}

// ended reports whether Session x is past a lifetime, or its Person's access
// has ended.
func (s *Store) ended(x *session) bool {
	if i := s.person(x.Person); i >= 0 && over(s.persons[i], s.now()) {
		return true
	}
	return s.now().After(s.endOf(x))
}

// endOf is when Session x ends unless used again.
func (s *Store) endOf(x *session) time.Time {
	idle, limit := x.LastUse.Add(idleLimit), x.SignedIn.Add(SessionLimit)
	if idle.Before(limit) {
		return idle
	}
	return limit
}

// recordExpiry records that Session x ended by itself, dated when it did.
// Callers hold s.mu.
func (s *Store) recordExpiry(x *session) {
	s.record(Entry{Time: s.endOf(x), Event: SessionEnded, Actor: oiko, Subject: s.personParty(x.Person), Browser: x.Browser, Detail: map[string]any{"reason": "expired"}})
}

// person is the index of Person id; -1 if there is none.
func (s *Store) person(id string) int {
	return slices.IndexFunc(s.persons, func(p Person) bool { return p.ID == id })
}

// saveSessions writes every Session, deleting, and recording, those that
// ended unnoticed, a Guest's whose end date came among them. Callers hold
// s.mu.
func (s *Store) saveSessions() error {
	s.notice()
	all := make([]*session, 0, len(s.sessions))
	for h, x := range s.sessions {
		if s.ended(x) {
			delete(s.sessions, h)
			s.finish(h)
			s.recordExpiry(x)
		} else {
			all = append(all, x)
		}
	}
	slices.SortFunc(all, func(a, b *session) int { return a.SignedIn.Compare(b.SignedIn) })
	if err := store.Save(s.sessionsFile, SessionsFormat, all); err != nil {
		return err
	}
	s.sessionUses = pending{written: s.now()}
	return nil
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Browser labels the browser of userAgent for a list of Sessions, e.g.
// "Firefox on Linux".
func Browser(userAgent string) string {
	pick := func(names [][2]string) string {
		for _, n := range names {
			if strings.Contains(userAgent, n[0]) {
				return n[1]
			}
		}
		return ""
	}
	// Most specific first: Edge's says Chrome too, Chrome's Safari.
	browser := pick([][2]string{
		{"Edg", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"FxiOS/", "Firefox"},
		{"CriOS/", "Chrome"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"},
	})
	system := pick([][2]string{
		{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"CrOS", "ChromeOS"},
		{"Mac OS X", "macOS"}, {"Windows", "Windows"}, {"Linux", "Linux"},
	})
	switch {
	case browser == "":
		return "Unknown browser"
	case system == "":
		return browser
	}
	return browser + " on " + system
}
