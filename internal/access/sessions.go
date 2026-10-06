package access

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/llehouerou/oiko/internal/home"
)

// whose is the index of Person person, whose Sessions and Passkeys by
// manages: their own, fresh for step-up when stepUp, or, for a fresh Admin
// Person, anyone's (ADR 0025). Callers hold s.mu.
func (s *Store) whose(by Identity, person string, stepUp bool) (int, error) {
	if by.Kind == PersonKind && by.ID == person {
		return s.self(by, stepUp)
	}
	if err := mayManage(by, true); err != nil {
		return 0, err
	}
	return s.found(person)
}

// Sessions answers the Sessions of Person person, oldest first, to them, or
// to a fresh Admin Person.
func (s *Store) Sessions(by Identity, person string) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.whose(by, person, false); err != nil {
		return nil, err
	}
	var list []Session
	for _, x := range s.sessions {
		if x.Person == person && !s.ended(x) {
			list = append(list, Session{x.ID, x.Browser, x.Method, x.Provider, x.By, x.SignedIn, x.LastUse})
		}
	}
	slices.SortFunc(list, func(a, b Session) int { return cmp.Or(a.SignedIn.Compare(b.SignedIn), strings.Compare(a.ID, b.ID)) }) // ids of version 7 follow time
	return list, nil
}

// EndSession ends Session id of Person person, as they or a fresh Admin
// Person revoke it: its event streams close, and it is refused from then on.
func (s *Store) EndSession(by Identity, person, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.whose(by, person, false); err != nil {
		return err
	}
	for _, x := range s.sessions {
		if x.Person == person && x.ID == id {
			s.endSession(x, party(by), s.holder(x), "revoked")
			return s.saveSessions()
		}
	}
	return fmt.Errorf("session %q: %w", id, home.ErrNotFound)
}

// EndOtherSessions ends every Session of Person by but the one by holds:
// "sign out all other devices".
func (s *Store) EndOtherSessions(by Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.self(by, false); err != nil {
		return err
	}
	for _, x := range s.sessions {
		if x.Person == by.ID && x.ID != by.Session {
			s.endSession(x, party(by), s.holder(x), "revoked")
		}
	}
	return s.saveSessions()
}

// endSession ends Session x, of subject, as actor ends it for reason, and
// records it. Callers hold s.mu, and write the Sessions.
func (s *Store) endSession(x *session, actor, subject Party, reason string) {
	delete(s.sessions, x.Hash)
	s.finish(x.Hash)
	s.record(Entry{Event: SessionEnded, Actor: actor, Subject: subject, Browser: x.Browser, Detail: map[string]any{"reason": reason}})
}
