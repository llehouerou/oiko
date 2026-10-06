package access

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// Lifetimes of a Sign-in link (ADR 0030): one an Admin creates for another
// Person is usually sent through a messenger and opened hours later.
const (
	linkFromAdmin = 24 * time.Hour
	linkForSelf   = 15 * time.Minute
)

// Link is a Person's pending Sign-in link as stored: its secret's hash, the
// Person who created it, by id, when, and until when it signs them in.
type Link struct {
	Hash    string    `json:"hash"`
	Creator string    `json:"creator"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

// errLinkEnded refuses a Sign-in link that expired, was used or revoked, or
// never existed, naming no one: the visitor may not be whom it was meant for
// (ADR 0030).
var errLinkEnded = fmt.Errorf("%w: this Sign-in link has expired or was already used; ask whoever sent it for a new one", ErrRefused)

// live reports whether l is a link that still signs in at now.
func live(l *Link, now time.Time) bool { return l != nil && !now.After(l.Expires) }

// Persons answers every Person, oldest first, each with their pending
// Sign-in link if any, to an Admin Person.
func (s *Store) Persons(by Identity) ([]Person, error) {
	if err := mayManage(by, false); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := slices.Clone(s.persons) // a Link and the Passkeys are replaced, never changed in place
	for i := range ps {
		if !live(ps[i].Link, s.now()) {
			ps[i].Link = nil
		}
	}
	return ps, nil
}

// CreatePerson creates a Person, who has not signed in yet, for a fresh
// Admin Person (ADR 0030).
func (s *Store) CreatePerson(by Identity, name string, level Level) (Person, error) {
	if err := mayManage(by, true); err != nil {
		return Person{}, err
	}
	name, err := validDefinition(name, level)
	if err != nil {
		return Person{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Person{ID: uuid.NewV7().String(), Name: name, Level: level}
	if err := s.savePersons(append(slices.Clone(s.persons), p)); err != nil {
		return Person{}, err
	}
	s.record(Entry{Event: PersonCreated, Actor: party(by), Subject: personParty(p), Detail: map[string]any{"level": level}})
	return p, nil
}

// EditPerson renames Person id and sets their Access level, for a fresh
// Admin Person, never their own level; a new level ends their open event
// streams, not their Sessions.
func (s *Store) EditPerson(by Identity, id, name string, level Level) error {
	if err := mayManage(by, true); err != nil { // refused before told what is invalid
		return err
	}
	name, err := validDefinition(name, level)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.found(id)
	if err != nil {
		return err
	}
	changed := s.persons[i].Level != level
	if changed {
		if err := s.lastAdmin(i); err != nil {
			return err
		}
		if id == by.ID {
			return fmt.Errorf("%w: your own Access level is for another Admin to change", ErrRefused)
		}
	}
	was := s.persons[i]
	ps := slices.Clone(s.persons)
	ps[i].Name, ps[i].Level = name, level
	if err := s.savePersons(ps); err != nil {
		return err
	}
	if name != was.Name {
		s.record(Entry{Event: PersonRenamed, Actor: party(by), Subject: personParty(ps[i]), Detail: map[string]any{"from": was.Name}})
	}
	if changed {
		s.record(Entry{Event: LevelChanged, Actor: party(by), Subject: personParty(ps[i]), Detail: map[string]any{"from": was.Level, "to": level}})
		for h, x := range s.sessions {
			if x.Person == id {
				s.finish(h)
			}
		}
	}
	return nil
}

// RemovePerson removes Person id, for a fresh Admin Person: their Sessions
// end, and what they created, Programs among them, keeps working.
func (s *Store) RemovePerson(by Identity, id string) error {
	if err := mayManage(by, true); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.found(id)
	if err != nil {
		return err
	}
	if err := s.lastAdmin(i); err != nil {
		return err
	}
	p := s.persons[i]
	if err := s.savePersons(slices.Delete(slices.Clone(s.persons), i, i+1)); err != nil {
		return err
	}
	s.record(Entry{Event: PersonRemoved, Actor: party(by), Subject: personParty(p)})
	for h, x := range s.sessions {
		if x.Person == id {
			delete(s.sessions, h)
			s.finish(h)
			s.record(Entry{Event: SessionEnded, Actor: party(by), Subject: personParty(p), Browser: x.Browser, Detail: map[string]any{"reason": "removed"}})
		}
	}
	// The Person is gone: Sessions left on disk without them are dropped on load.
	if err := s.saveSessions(); err != nil {
		slog.Error("access: writing the Sessions of a removed Person", "err", err)
	}
	return nil
}

// Rename renames Person by, who manages their own Name.
func (s *Store) Rename(by Identity, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.self(by, false)
	if err != nil {
		return err
	}
	if name, err = home.ValidName(name); err != nil {
		return err
	}
	was := s.persons[i].Name
	ps := slices.Clone(s.persons)
	ps[i].Name = name
	if err := s.savePersons(ps); err != nil {
		return err
	}
	if name != was {
		s.record(Entry{Event: PersonRenamed, Actor: personParty(ps[i]), Subject: personParty(ps[i]), Detail: map[string]any{"from": was}})
	}
	return nil
}

// CreateLink answers the secret of a new Sign-in link for Person id, and
// when it expires: created by a fresh Admin Person, for 24 hours, or by the
// fresh Person themself, for 15 minutes (ADR 0030). It revokes their
// previous unused link, whoever created it.
func (s *Store) CreateLink(by Identity, id string) (string, time.Time, error) {
	life := linkFromAdmin
	if by.Kind == PersonKind && by.ID == id {
		life = linkForSelf
	} else if err := mayManage(by, false); err != nil {
		return "", time.Time{}, err
	}
	if err := by.StepUp(); err != nil {
		return "", time.Time{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.found(id)
	if err != nil {
		return "", time.Time{}, err
	}
	secret, now := rand.Text(), s.now()
	ps := slices.Clone(s.persons)
	ps[i].Link = &Link{Hash: hash(secret), Creator: by.ID, Created: now, Expires: now.Add(life)}
	if err := s.savePersons(ps); err != nil {
		return "", time.Time{}, err
	}
	s.record(Entry{Event: LinkCreated, Actor: party(by), Subject: personParty(ps[i]), Detail: map[string]any{"expires": now.Add(life)}})
	return secret, now.Add(life), nil
}

// RevokeLink revokes the pending Sign-in link of Person id, for a fresh
// Admin Person.
func (s *Store) RevokeLink(by Identity, id string) error {
	if err := mayManage(by, true); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.found(id)
	if err != nil {
		return err
	}
	if !live(s.persons[i].Link, s.now()) {
		return fmt.Errorf("a pending Sign-in link for %s: %w", s.persons[i].Name, home.ErrNotFound)
	}
	ps := slices.Clone(s.persons)
	ps[i].Link = nil
	if err := s.savePersons(ps); err != nil {
		return err
	}
	s.record(Entry{Event: LinkRevoked, Actor: party(by), Subject: personParty(ps[i])})
	return nil
}

// LinkedName answers the Name of the Person the Sign-in link of secret signs
// in, for its welcome page in browser, without spending it.
func (s *Store) LinkedName(secret, browser string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.linked(secret, browser)
	if err != nil {
		return "", err
	}
	return s.persons[i].Name, nil
}

// SignInWithLink spends the Sign-in link of secret: it answers the secret of
// a new Session in browser for its Person, labelled with whoever created the
// link if not them (ADR 0026).
func (s *Store) SignInWithLink(secret, browser string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.linked(secret, browser)
	if err != nil {
		return "", err
	}
	p := s.persons[i]
	// Spent first: a failure after it costs a new link, never a second use.
	ps := slices.Clone(s.persons)
	ps[i].Link = nil
	if err := s.savePersons(ps); err != nil {
		return "", err
	}
	by := p.Link.Creator
	if by == p.ID {
		by = ""
	}
	session, err := s.signIn(p.ID, browser, "link", by)
	if err == nil {
		s.recordSignIn(p, browser, "link", by)
	}
	return session, err
}

// linked is the index of the Person whom the Sign-in link of secret signs
// in now; refused, and recorded, if none. Callers hold s.mu.
func (s *Store) linked(secret, browser string) (int, error) {
	h := hash(secret)
	i := slices.IndexFunc(s.persons, func(p Person) bool { return live(p.Link, s.now()) && p.Link.Hash == h })
	if i < 0 {
		s.record(Entry{Event: LinkRefused, Actor: nobody, Subject: nobody, Browser: browser})
		return 0, errLinkEnded
	}
	return i, nil
}

// found is the index of Person id. Callers hold s.mu.
func (s *Store) found(id string) (int, error) {
	i := s.person(id)
	if i < 0 {
		return 0, fmt.Errorf("person %q: %w", id, home.ErrNotFound)
	}
	return i, nil
}

// lastAdmin refuses to demote or remove Person i if they are the last Admin
// Person: Oiko keeps one to sign in as (ADR 0023). Callers hold s.mu.
func (s *Store) lastAdmin(i int) error {
	admins := 0
	for _, p := range s.persons {
		if p.Level == Admin {
			admins++
		}
	}
	if s.persons[i].Level == Admin && admins == 1 {
		return fmt.Errorf("%w: %s is the last Admin Person; make another Person an Admin first", ErrRefused, s.persons[i].Name)
	}
	return nil
}

// savePersons writes ps, without the links that expired, which are then the
// Persons; it records those links expired. Callers hold s.mu, and pass a
// copy of s.persons they may change.
func (s *Store) savePersons(ps []Person) error {
	var expired []Entry
	for i := range ps {
		if l := ps[i].Link; l != nil && !live(l, s.now()) {
			expired = append(expired, Entry{Time: l.Expires, Event: LinkExpired, Actor: oiko, Subject: personParty(ps[i])})
			ps[i].Link = nil
		}
	}
	if err := store.Save(s.personsFile, PersonsFormat, ps); err != nil {
		return err
	}
	s.persons = ps
	for _, e := range expired {
		s.record(e)
	}
	return nil
}
