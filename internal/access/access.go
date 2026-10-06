// Package access keeps who may use Oiko (ADR 0022, 0032): the Persons and
// their Sessions, in persons.json and sessions.json, loaded at start and held
// in memory, and the Setup link that claims a fresh Oiko (ADR 0026). Secrets
// are 128 random bits or more, kept only as their SHA-256 hash; Oiko alone
// decides when a Session ends (ADR 0025).
package access

import (
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

// Person is a human known to Oiko.
type Person struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level Level  `json:"level"`
}

// session is a Session as stored: its secret's hash, whose it is, the
// browser it lives in, how and when it signed in, and when it was last used.
type session struct {
	Hash     string    `json:"hash"`
	Person   string    `json:"person"`
	Browser  string    `json:"browser"`
	Method   string    `json:"method"` // how it signed in: "setup"
	SignedIn time.Time `json:"signedIn"`
	LastUse  time.Time `json:"lastUse"`
}

// Identity is who a request is: its Person as they are now, and whether its
// Session is fresh enough for step-up.
type Identity struct {
	Person Person
	Fresh  bool
}

// StepUp refuses an action needing step-up unless the Session is fresh.
func (id Identity) StepUp() error {
	if !id.Fresh {
		return ErrStale
	}
	return nil
}

// Store holds the Persons and their Sessions.
type Store struct {
	personsFile, sessionsFile string
	now                       func() time.Time

	mu       sync.Mutex
	persons  []Person
	sessions map[string]*session // by hash
	setup    string              // the Setup link's hash; "" when there is none
	written  time.Time           // when sessions.json was last written
	unsaved  bool                // a last use not yet written
}

// Open loads the Persons and their Sessions from dir, dropping the Sessions
// that ended or whose Person is gone. now is the clock.
func Open(dir string, now func() time.Time) (*Store, error) {
	s := &Store{
		personsFile:  filepath.Join(dir, "persons.json"),
		sessionsFile: filepath.Join(dir, "sessions.json"),
		now:          now,
		sessions:     map[string]*session{},
		written:      now(),
	}
	var sessions []*session
	if err := store.Load(s.personsFile, PersonsFormat, &s.persons); err != nil {
		return nil, err
	}
	if err := store.Load(s.sessionsFile, SessionsFormat, &sessions); err != nil {
		return nil, err
	}
	for _, x := range sessions {
		if s.person(x.Person) >= 0 && !s.ended(x) {
			s.sessions[x.Hash] = x
		}
	}
	if len(s.sessions) < len(sessions) {
		if err := s.saveSessions(); err != nil {
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
		return "", fmt.Errorf("%w: this Setup link is no longer valid; Oiko's log has the current one, if Oiko has no Admin yet", ErrRefused)
	}
	// The Session first: written without its Person, it is dropped on load.
	p := Person{ID: uuid.NewV7().String(), Name: name, Level: Admin}
	session, err := s.signIn(p.ID, browser, "setup")
	if err != nil {
		return "", err
	}
	if err := store.Save(s.personsFile, PersonsFormat, append(s.persons, p)); err != nil {
		delete(s.sessions, hash(session))
		return "", err
	}
	s.persons = append(s.persons, p)
	s.setup = ""
	return session, nil
}

// signIn opens a Session for Person person. Callers hold s.mu.
func (s *Store) signIn(person, browser, method string) (string, error) {
	secret := rand.Text()
	now := s.now()
	x := &session{Hash: hash(secret), Person: person, Browser: browser, Method: method, SignedIn: now, LastUse: now}
	s.sessions[x.Hash] = x
	if err := s.saveSessions(); err != nil {
		delete(s.sessions, x.Hash)
		return "", err
	}
	return secret, nil
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
	if i < 0 || s.ended(x) {
		s.end(x)
		return Identity{}, false
	}
	now := s.now()
	x.LastUse = now
	s.unsaved = true
	if now.Sub(s.written) >= writeLastUse {
		if err := s.saveSessions(); err != nil {
			slog.Error("access: writing last use", "err", err)
		}
	}
	return Identity{Person: s.persons[i], Fresh: now.Sub(x.SignedIn) < freshFor}, true
}

// SignOut ends the Session of secret, if there is one.
func (s *Store) SignOut(secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.sessions[hash(secret)]; x != nil {
		return s.end(x)
	}
	return nil
}

// Flush writes last uses not yet written, on shutdown.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.unsaved {
		return nil
	}
	return s.saveSessions()
}

// end deletes Session x. Callers hold s.mu.
func (s *Store) end(x *session) error {
	delete(s.sessions, x.Hash)
	return s.saveSessions()
}

// ended reports whether Session x is past a lifetime.
func (s *Store) ended(x *session) bool {
	now := s.now()
	return now.Sub(x.LastUse) > idleLimit || now.Sub(x.SignedIn) > SessionLimit
}

// person is the index of Person id; -1 if there is none.
func (s *Store) person(id string) int {
	return slices.IndexFunc(s.persons, func(p Person) bool { return p.ID == id })
}

// saveSessions writes every Session, deleting those that ended unnoticed.
// Callers hold s.mu.
func (s *Store) saveSessions() error {
	all := make([]*session, 0, len(s.sessions))
	for h, x := range s.sessions {
		if s.ended(x) {
			delete(s.sessions, h)
		} else {
			all = append(all, x)
		}
	}
	slices.SortFunc(all, func(a, b *session) int { return a.SignedIn.Compare(b.SignedIn) })
	if err := store.Save(s.sessionsFile, SessionsFormat, all); err != nil {
		return err
	}
	s.written, s.unsaved = s.now(), false
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
