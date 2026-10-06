package access

import (
	"crypto/rand"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// ProgramsFormat is programs.json's; format 1.
var ProgramsFormat store.Format

// Program is external software calling Oiko under its own identity, by its
// Token (ADR 0028).
type Program struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Level   Level     `json:"level"`
	Creator string    `json:"creator"` // the Person who created it, by id; they may since be gone
	Created time.Time `json:"created"`
	Token   *Token    `json:"token,omitempty"` // nil while it has none
}

// Token is a Program's credential as stored: the hash of "oiko_" and 128
// random bits or more, when it was generated, and when it was last used.
type Token struct {
	Hash      string    `json:"hash"`
	Generated time.Time `json:"generated"`
	LastUse   time.Time `json:"lastUse,omitzero"`
}

// mayManage refuses by unless it is a signed-in Admin Person, fresh for
// step-up when stepUp: a Program never manages access, whatever its level
// (ADR 0023).
func mayManage(by Identity, stepUp bool) error {
	if by.Kind != PersonKind || by.Level != Admin {
		return fmt.Errorf("%w: only a signed-in Admin manages access", ErrRefused)
	}
	if stepUp {
		return by.StepUp()
	}
	return nil
}

// validProgram checks a Program's Name and Access level, answering the Name
// trimmed.
func validProgram(name string, level Level) (string, error) {
	if !level.valid() {
		return "", fmt.Errorf("%w: access level %q: want guest, member or admin", home.ErrInvalid, level)
	}
	return home.ValidName(name)
}

// Programs answers every Program, oldest first, to an Admin Person.
func (s *Store) Programs(by Identity) ([]Program, error) {
	if err := mayManage(by, false); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := slices.Clone(s.programs)
	for i, p := range ps {
		if p.Token != nil {
			t := *p.Token // its last use changes under s.mu
			ps[i].Token = &t
		}
	}
	return ps, nil
}

// CreateProgram creates a Program, without a Token, for a fresh Admin Person.
func (s *Store) CreateProgram(by Identity, name string, level Level) (Program, error) {
	if err := mayManage(by, true); err != nil {
		return Program{}, err
	}
	name, err := validProgram(name, level)
	if err != nil {
		return Program{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Program{ID: uuid.NewV7().String(), Name: name, Level: level, Creator: by.ID, Created: s.now()}
	ps := append(slices.Clone(s.programs), p)
	if err := s.savePrograms(ps); err != nil {
		return Program{}, err
	}
	s.programs = ps
	return p, nil
}

// EditProgram renames Program id and sets its Access level; a new level ends
// its open event streams.
func (s *Store) EditProgram(by Identity, id, name string, level Level) error {
	if err := mayManage(by, true); err != nil { // refused before told what is invalid
		return err
	}
	name, err := validProgram(name, level)
	if err != nil {
		return err
	}
	return s.changeProgram(by, id, func(ps []Program, i int) ([]Program, bool) {
		end := ps[i].Level != level
		ps[i].Name, ps[i].Level = name, level
		return ps, end
	})
}

// RemoveProgram removes Program id, ending its Token.
func (s *Store) RemoveProgram(by Identity, id string) error {
	return s.changeProgram(by, id, func(ps []Program, i int) ([]Program, bool) {
		return slices.Delete(ps, i, i+1), true
	})
}

// GenerateToken answers a new Token for Program id, shown this once; it
// revokes the previous one.
func (s *Store) GenerateToken(by Identity, id string) (string, error) {
	token := "oiko_" + rand.Text()
	err := s.changeProgram(by, id, func(ps []Program, i int) ([]Program, bool) {
		ps[i].Token = &Token{Hash: hash(token), Generated: s.now()}
		return ps, true
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// RevokeToken leaves Program id without a Token.
func (s *Store) RevokeToken(by Identity, id string) error {
	return s.changeProgram(by, id, func(ps []Program, i int) ([]Program, bool) {
		ps[i].Token = nil
		return ps, true
	})
}

// changeProgram has a fresh Admin Person change Program id: change edits a
// copy of the Programs, where it is at i, and says whether the Program's open
// event streams end.
func (s *Store) changeProgram(by Identity, id string, change func(ps []Program, i int) ([]Program, bool)) error {
	if err := mayManage(by, true); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.programs, func(p Program) bool { return p.ID == id })
	if i < 0 {
		return fmt.Errorf("program %q: %w", id, home.ErrNotFound)
	}
	ps, end := change(slices.Clone(s.programs), i)
	if err := s.savePrograms(ps); err != nil {
		return err
	}
	s.programs = ps
	if ch, ok := s.ends[id]; ok && end {
		close(ch)
		delete(s.ends, id)
	}
	return nil
}

// ResolveToken answers which Program holds token, counting it as a use; false
// if none does.
func (s *Store) ResolveToken(token string) (Identity, bool) {
	h := hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.programs, func(p Program) bool { return p.Token != nil && p.Token.Hash == h })
	if i < 0 {
		return Identity{}, false
	}
	p := s.programs[i]
	s.programs[i].Token.LastUse = s.now()
	s.used(&s.programUses, func() error { return s.savePrograms(s.programs) })
	ended, ok := s.ends[p.ID]
	if !ok {
		ended = make(chan struct{})
		s.ends[p.ID] = ended
	}
	return Identity{Kind: ProgramKind, ID: p.ID, Name: p.Name, Level: p.Level, Ended: ended}, true
}

// PersonName is the Name of Person id; "" once they are removed.
func (s *Store) PersonName(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.person(id); i >= 0 {
		return s.persons[i].Name
	}
	return ""
}

// Names answers the Name of every Person and Program, by kind then id, to a
// Member or an Admin: who reads Origins (ADR 0031).
func (s *Store) Names(by Identity) (map[Kind]map[string]string, error) {
	if by.Level != Member && by.Level != Admin {
		return nil, fmt.Errorf("%w: only a Member or an Admin reads who did what", ErrRefused)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	names := map[Kind]map[string]string{PersonKind: {}, ProgramKind: {}}
	for _, p := range s.persons {
		names[PersonKind][p.ID] = p.Name
	}
	for _, p := range s.programs {
		names[ProgramKind][p.ID] = p.Name
	}
	return names, nil
}

// savePrograms writes ps, last uses included. Callers hold s.mu.
func (s *Store) savePrograms(ps []Program) error {
	if err := store.Save(s.programsFile, ProgramsFormat, ps); err != nil {
		return err
	}
	s.programUses = pending{written: s.now()}
	return nil
}
