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
// Person who created it, by id, or "" for Oiko's host, when, and until when
// it signs them in.
type Link struct {
	Hash    string    `json:"hash"`
	Creator string    `json:"creator,omitempty"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

// errLinkEnded refuses a Sign-in link that expired, was used or revoked, or
// never existed, naming no one: the visitor may not be whom it was meant for
// (ADR 0030).
var errLinkEnded = fmt.Errorf("%w: this Sign-in link has expired or was already used; ask whoever sent it for a new one", ErrRefused)

// errAccessEnded refuses a Guest past their end date (ADR 0030).
var errAccessEnded = fmt.Errorf("%w: your access to Oiko has ended; ask an Admin if it should go on", ErrRefused)

// live reports whether l is a link that still signs in at now.
func live(l *Link, now time.Time) bool { return l != nil && !now.After(l.Expires) }

// over reports whether p is a Guest whose end date has come at now.
func over(p Person, now time.Time) bool { return !p.Ends.IsZero() && !now.Before(p.Ends) }

// validEnds checks the end date of a Person at level, zero for none: only a
// Guest has one (ADR 0030).
func validEnds(level Level, ends time.Time) error {
	if !ends.IsZero() && level != Guest {
		return fmt.Errorf("%w: only a Guest has an end date", home.ErrInvalid)
	}
	return nil
}

// endsDetail is how the Audit log tells end date t: null for none.
func endsDetail(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// notice ends the access of each Guest whose end date came unnoticed: their
// Sessions end, recorded as of their end date. Callers hold s.mu, and write
// the Sessions.
func (s *Store) notice() {
	var ps []Person
	for i, p := range s.persons {
		if p.Ended || !over(p, s.now()) {
			continue
		}
		if ps == nil {
			ps = slices.Clone(s.persons)
		}
		ps[i].Ended = true
		s.record(Entry{Time: p.Ends, Event: EndDateReached, Actor: oiko, Subject: personParty(p)})
		for _, x := range s.sessions {
			if x.Person == p.ID {
				s.endSession(x, oiko, personParty(p), "access ended", p.Ends)
			}
		}
	}
	if ps != nil {
		if err := s.savePersons(ps); err != nil {
			slog.Error("access: writing that a Guest's access ended", "err", err)
		}
	}
}

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
// Admin Person (ADR 0030); a Guest may have an end date, zero for none.
func (s *Store) CreatePerson(by Identity, name string, level Level, ends time.Time) (Person, error) {
	if err := mayManage(by, true); err != nil {
		return Person{}, err
	}
	name, err := validDefinition(name, level)
	if err == nil {
		err = validEnds(level, ends)
	}
	if err != nil {
		return Person{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Person{ID: uuid.NewV7().String(), Name: name, Level: level, Ends: ends}
	if err := s.savePersons(append(slices.Clone(s.persons), p)); err != nil {
		return Person{}, err
	}
	s.record(Entry{Event: PersonCreated, Actor: party(by), Subject: personParty(p), Detail: map[string]any{"level": level}})
	if !ends.IsZero() {
		s.record(Entry{Event: EndDateChanged, Actor: party(by), Subject: personParty(p), Detail: map[string]any{"from": nil, "to": ends}})
	}
	return p, nil
}

// EditPerson renames Person id and sets their Access level and end date,
// for a fresh Admin Person, never their own level. A new level ends their
// open event streams, not their Sessions; an end date that has come ends
// their Sessions.
func (s *Store) EditPerson(by Identity, id, name string, level Level, ends time.Time) error {
	if err := mayManage(by, true); err != nil { // refused before told what is invalid
		return err
	}
	name, err := validDefinition(name, level)
	if err == nil {
		err = validEnds(level, ends)
	}
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
	if !ends.Equal(was.Ends) {
		ps[i].Ends, ps[i].Ended = ends, false
	}
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
	if !ends.Equal(was.Ends) {
		s.record(Entry{Event: EndDateChanged, Actor: party(by), Subject: personParty(ps[i]), Detail: map[string]any{"from": endsDetail(was.Ends), "to": endsDetail(ends)}})
		if err := s.saveSessions(); err != nil { // notices an end date already come
			return err
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
	for _, x := range s.sessions {
		if x.Person == id {
			s.endSession(x, party(by), personParty(p), "removed", time.Time{})
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
	return s.newLink(id, party(by), by.ID, life)
}

// HostLink answers the secret of a new Sign-in link for Person id, and when
// it expires, for oiko sign-in-link on Oiko's host (ADR 0026): for 15
// minutes, whatever their Access level. It revokes their previous unused
// link, whoever created it.
func (s *Store) HostLink(id string) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newLink(id, host, "", linkForSelf)
}

// newLink creates a Sign-in link for Person id, by Party by, the Person
// creator unless "" for the host, that lasts life. Callers hold s.mu.
func (s *Store) newLink(id string, by Party, creator string, life time.Duration) (string, time.Time, error) {
	i, err := s.found(id)
	if err != nil {
		return "", time.Time{}, err
	}
	if over(s.persons[i], s.now()) {
		return "", time.Time{}, fmt.Errorf("%w: the access of %s has ended; give them a new end date first", ErrRefused, s.persons[i].Name)
	}
	secret, now := rand.Text(), s.now()
	ps := slices.Clone(s.persons)
	ps[i].Link = &Link{Hash: hash(secret), Creator: creator, Created: now, Expires: now.Add(life)}
	if err := s.savePersons(ps); err != nil {
		return "", time.Time{}, err
	}
	s.record(Entry{Event: LinkCreated, Actor: by, Subject: personParty(ps[i]), Detail: map[string]any{"expires": now.Add(life)}})
	return secret, now.Add(life), nil
}

// HostPersons answers every Person, oldest first, to Oiko itself: for oiko
// sign-in-link on Oiko's host to pick whom it signs in, and for the
// Dashboards to know whose lists to keep.
func (s *Store) HostPersons() []Person {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.persons)
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

// LinkedPerson answers the Person the Sign-in link of secret signs in, for
// its welcome page in browser, without spending it.
func (s *Store) LinkedPerson(secret, browser string) (Person, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.linked(secret, browser)
	if err != nil {
		return Person{}, err
	}
	return s.persons[i], nil
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
	x := session{Person: p.ID, Browser: browser, Method: "link", By: p.Link.Creator}
	switch p.Link.Creator {
	case p.ID:
		x.By = ""
	case "":
		x.Method = "host"
	}
	session, err := s.signIn(&x)
	if err == nil {
		s.recordSignIn(personParty(p), x)
	}
	return session, err
}

// linked is the index of the Person whom the Sign-in link of secret signs
// in now; refused, and recorded, if none, or their access has ended. Callers
// hold s.mu.
func (s *Store) linked(secret, browser string) (int, error) {
	h := hash(secret)
	i := slices.IndexFunc(s.persons, func(p Person) bool { return live(p.Link, s.now()) && p.Link.Hash == h })
	switch {
	case i < 0:
		s.record(Entry{Event: LinkRefused, Actor: nobody, Subject: nobody, Browser: browser})
		return 0, errLinkEnded
	case over(s.persons[i], s.now()):
		s.record(Entry{Event: LinkRefused, Actor: nobody, Subject: personParty(s.persons[i]), Browser: browser, Detail: map[string]any{"reason": "access ended"}})
		return 0, errAccessEnded
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
