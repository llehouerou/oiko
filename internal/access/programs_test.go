package access

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// admin is Alice, the Admin of claimed, as her Session resolves now.
func admin(t *testing.T, s *Store, session string) Identity {
	t.Helper()
	id, ok := s.Resolve(session)
	if !ok {
		t.Fatal("Alice's Session refused")
	}
	return id
}

// program is a Program named name at level, created by by, with its Token.
func program(t *testing.T, s *Store, by Identity, name string, level Level) (Program, string) {
	t.Helper()
	p, err := s.CreateProgram(by, name, level)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.GenerateToken(by, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p, token
}

// closed reports whether ch is closed.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestAProgramIsServedByItsTokenAtItsLevelNow(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	p, token := program(t, s, alice, " Node-RED ", Member)
	if p.Name != "Node-RED" || p.Level != Member || p.Creator != alice.ID || !p.Created.Equal(c.t) || p.Token != nil {
		t.Errorf("created %+v", p)
	}
	if !strings.HasPrefix(token, "oiko_") || len(token) < len("oiko_")+22 { // 128 bits in base32
		t.Errorf("Token %q: want oiko_ and 128 random bits", token)
	}
	id, ok := s.ResolveToken(token)
	if !ok || id.Kind != ProgramKind || id.ID != p.ID || id.Name != "Node-RED" || id.Level != Member || id.Fresh {
		t.Fatalf("ResolveToken = %+v, %v", id, ok)
	}
	if err := s.EditProgram(alice, p.ID, "Node-RED", Admin); err != nil {
		t.Fatal(err)
	}
	if id, _ := s.ResolveToken(token); id.Level != Admin {
		t.Errorf("level after a change: %v, want admin", id.Level)
	}
	for _, wrong := range []string{"", "oiko_", "unknown", strings.TrimPrefix(token, "oiko_")} {
		if _, ok := s.ResolveToken(wrong); ok {
			t.Errorf("Token %q accepted", wrong)
		}
	}
}

func TestRevokingOrRegeneratingEndsTheToken(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	p, first := program(t, s, alice, "Node-RED", Member)
	id, _ := s.ResolveToken(first)
	second, err := s.GenerateToken(alice, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ResolveToken(first); ok {
		t.Error("a regenerated Token accepted")
	}
	if !closed(id.Ended) {
		t.Error("regenerating left the previous Token's streams open")
	}
	id, ok := s.ResolveToken(second)
	if !ok {
		t.Fatal("the new Token refused")
	}
	if err := s.RevokeToken(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ResolveToken(second); ok {
		t.Error("a revoked Token accepted")
	}
	if !closed(id.Ended) {
		t.Error("revoking left the stream open")
	}
	ps, _ := s.Programs(alice)
	if len(ps) != 1 || ps[0].Name != "Node-RED" || ps[0].Token != nil {
		t.Errorf("after revoking: %+v, want the Program intact without a Token", ps)
	}
}

func TestChangingTheLevelOrRemovingEndsTheStreams(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	p, token := program(t, s, alice, "Node-RED", Member)
	id, _ := s.ResolveToken(token)
	if err := s.EditProgram(alice, p.ID, "Flows", Member); err != nil {
		t.Fatal(err)
	}
	if closed(id.Ended) {
		t.Error("a rename closed the stream")
	}
	if err := s.EditProgram(alice, p.ID, "Flows", Guest); err != nil {
		t.Fatal(err)
	}
	if !closed(id.Ended) {
		t.Error("a level change left the stream open")
	}
	id, _ = s.ResolveToken(token)
	if err := s.RemoveProgram(alice, p.ID); err != nil {
		t.Fatal(err)
	}
	if !closed(id.Ended) {
		t.Error("removing left the stream open")
	}
	if _, ok := s.ResolveToken(token); ok {
		t.Error("the Token of a removed Program accepted")
	}
	if ps, _ := s.Programs(alice); len(ps) != 0 {
		t.Errorf("after removing: %+v", ps)
	}
	if err := s.RemoveProgram(alice, p.ID); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("removing again: %v, want ErrNotFound", err)
	}
}

func TestOnlyAFreshAdminPersonManagesPrograms(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	alice := admin(t, s, session)
	p, token := program(t, s, alice, "Node-RED", Admin)
	bot, _ := s.ResolveToken(token)
	stale := alice
	stale.Fresh = false
	member := alice
	member.Level = Member
	for name, by := range map[string]Identity{"an Admin Program": bot, "a stale Admin": stale, "a Member": member, "anonymous": {}} {
		for action, err := range map[string]error{
			"create":   func() error { _, err := s.CreateProgram(by, "Script", Guest); return err }(),
			"edit":     s.EditProgram(by, p.ID, "Script", Guest),
			"generate": func() error { _, err := s.GenerateToken(by, p.ID); return err }(),
			"revoke":   s.RevokeToken(by, p.ID),
			"remove":   s.RemoveProgram(by, p.ID),
			"list":     func() error { _, err := s.Programs(by); return err }(),
		} {
			if name == "a stale Admin" && action == "list" {
				if err != nil {
					t.Errorf("a stale Admin lists Programs: %v", err)
				}
				continue
			}
			if !errors.Is(err, ErrRefused) {
				t.Errorf("%s, %s: %v, want ErrRefused", name, action, err)
			}
		}
	}
	if err := s.EditProgram(alice, p.ID, "Script", "owner"); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an unknown level: %v, want ErrInvalid", err)
	}
	if _, err := s.CreateProgram(alice, " ", Guest); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an empty Name: %v, want ErrInvalid", err)
	}
	if _, ok := s.ResolveToken(token); !ok {
		t.Error("refused actions changed the Token")
	}
}

func TestProgramsSurviveARestartAndHoldNoToken(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	p, token := program(t, s, admin(t, s, session), "Node-RED", Member)
	if id, ok := open(t, dir, c).ResolveToken(token); !ok || id.ID != p.ID {
		t.Errorf("after a restart: %+v, %v", id, ok)
	}
	path := filepath.Join(dir, "programs.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), strings.TrimPrefix(token, "oiko_")) {
		t.Error("programs.json holds the Token")
	}
}

func TestATokensLastUseIsWrittenAtMostHourly(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	alice := admin(t, s, session)
	_, token := program(t, s, alice, "Node-RED", Member)
	lastUse := func() time.Time {
		ps, err := open(t, dir, c).Programs(alice)
		if err != nil || len(ps) != 1 || ps[0].Token == nil {
			t.Fatalf("Programs = %+v, %v", ps, err)
		}
		return ps[0].Token.LastUse
	}
	c.advance(30 * time.Minute)
	s.ResolveToken(token)
	if ps, _ := s.Programs(alice); !ps[0].Token.LastUse.Equal(c.t) {
		t.Errorf("last use in memory: %v, want %v", ps[0].Token.LastUse, c.t)
	}
	if !lastUse().IsZero() {
		t.Error("a use 30 minutes after the last write reached the disk")
	}
	c.advance(30 * time.Minute)
	s.ResolveToken(token)
	if !lastUse().Equal(c.t) {
		t.Error("a use an hour after the last write did not reach the disk")
	}
	c.advance(time.Minute)
	s.ResolveToken(token)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if !lastUse().Equal(c.t) {
		t.Error("a use flushed did not reach the disk")
	}
}

func TestRemovingAPersonLeavesTheirProgramsWorking(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	alice := admin(t, s, session)
	_, token := program(t, s, alice, "Node-RED", Member)
	if err := store.Save(filepath.Join(dir, "persons.json"), PersonsFormat, []Person{}); err != nil {
		t.Fatal(err)
	}
	s = open(t, dir, c)
	if _, ok := s.ResolveToken(token); !ok {
		t.Error("the Program of a removed Person refused")
	}
	if name := s.PersonName(alice.ID); name != "" {
		t.Errorf("a removed Person's Name: %q", name)
	}
}
