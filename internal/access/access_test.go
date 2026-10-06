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

// clock is a time a test moves forward.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *clock { return &clock{time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)} }

func open(t *testing.T, dir string, c *clock) *Store {
	t.Helper()
	s, err := Open(dir, c.now)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// claimed is an Oiko claimed by Alice through its Setup link, and her
// Session's secret.
func claimed(t *testing.T, dir string, c *clock) (*Store, string) {
	t.Helper()
	s := open(t, dir, c)
	secret, ok := s.Setup()
	if !ok {
		t.Fatal("a fresh Oiko has no Setup link")
	}
	session, err := s.Claim(secret, " Alice ", "Firefox on Linux")
	if err != nil {
		t.Fatal(err)
	}
	return s, session
}

func TestTheSetupLinkMakesTheFirstAdminOnce(t *testing.T) {
	c := newClock()
	s := open(t, t.TempDir(), c)
	if s.Claimed() {
		t.Fatal("a fresh Oiko is claimed")
	}
	secret, ok := s.Setup()
	if !ok || len(secret) < 22 { // 128 bits in base32
		t.Fatalf("Setup() = %q, %v", secret, ok)
	}
	if _, err := s.Claim("wrong", "Alice", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a wrong secret: %v, want ErrRefused", err)
	}
	if _, err := s.Claim(secret, "  ", ""); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("an empty Name: %v, want ErrInvalid", err)
	}
	session, err := s.Claim(secret, " Alice ", "Firefox on Linux")
	if err != nil {
		t.Fatal(err)
	}
	id, ok := s.Resolve(session)
	if !ok || id.Person.Name != "Alice" || id.Person.Level != Admin || id.Person.ID == "" {
		t.Fatalf("Resolve = %+v, %v; want Alice, an Admin", id, ok)
	}
	if !s.Claimed() {
		t.Error("not claimed once an Admin exists")
	}
	if _, err := s.Claim(secret, "Mallory", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a second use: %v, want ErrRefused", err)
	}
	if _, ok := s.Setup(); ok {
		t.Error("a claimed Oiko offers a Setup link")
	}
}

func TestTheSetupLinkDiesWithARestart(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	old, _ := open(t, dir, c).Setup()
	s := open(t, dir, c)
	fresh, ok := s.Setup()
	if !ok || fresh == old {
		t.Fatalf("after a restart: %q, %v; want a new link", fresh, ok)
	}
	if _, err := s.Claim(old, "Mallory", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("the link of the previous start: %v, want ErrRefused", err)
	}
	if _, err := s.Claim(fresh, "Alice", ""); err != nil {
		t.Errorf("the current link: %v", err)
	}
}

func TestOnlyTheLatestSetupLinkWorks(t *testing.T) {
	s := open(t, t.TempDir(), newClock())
	first, _ := s.Setup()
	second, _ := s.Setup()
	if _, err := s.Claim(first, "Mallory", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a replaced link: %v, want ErrRefused", err)
	}
	if _, err := s.Claim(second, "Alice", ""); err != nil {
		t.Error(err)
	}
}

func TestASessionEndsAfter30DaysIdleOr1Year(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	// Used every 29 days, it lasts a year.
	for range 12 {
		c.advance(29 * 24 * time.Hour)
		if _, ok := s.Resolve(session); !ok {
			t.Fatalf("refused %v after signing in, used every 29 days", c.t.Sub(newClock().t))
		}
	}
	c.advance(18 * 24 * time.Hour) // 366 days
	if _, ok := s.Resolve(session); ok {
		t.Error("accepted after a year")
	}

	c = newClock()
	s, session = claimed(t, t.TempDir(), c)
	c.advance(30*24*time.Hour + time.Second)
	if _, ok := s.Resolve(session); ok {
		t.Error("accepted after 30 days unused")
	}
	if _, ok := s.Resolve("unknown"); ok {
		t.Error("an unknown secret accepted")
	}
}

func TestSigningOutEndsTheSession(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s, session := claimed(t, dir, c)
	if err := s.SignOut(session); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Resolve(session); ok {
		t.Error("signed out, yet accepted")
	}
	if _, ok := open(t, dir, c).Resolve(session); ok {
		t.Error("signed out, yet accepted after a restart")
	}
}

func TestSessionsSurviveARestartAndTheDocumentsHoldNoSecret(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	s := open(t, dir, c)
	setup, _ := s.Setup()
	session, err := s.Claim(setup, "Alice", "Safari on iPhone")
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := open(t, dir, c).Resolve(session); !ok || id.Person.Name != "Alice" {
		t.Errorf("after a restart: %+v, %v", id, ok)
	}
	for _, name := range []string{"persons.json", "sessions.json"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want 0600", name, info.Mode().Perm())
		}
		data, _ := os.ReadFile(path)
		for _, secret := range []string{session, setup} {
			if strings.Contains(string(data), secret) {
				t.Errorf("%s holds a secret", name)
			}
		}
	}
}

func TestLastUseIsWrittenAtMostHourlyAndOnFlush(t *testing.T) {
	const idle = 30 * 24 * time.Hour
	// After a restart at start+idle+10m, the Session lives only if a use
	// after start+10m reached the disk.
	restart := func(t *testing.T, dir string, c *clock, session string) bool {
		c.t = newClock().t.Add(idle + 10*time.Minute)
		_, ok := open(t, dir, c).Resolve(session)
		return ok
	}
	t.Run("within the hour", func(t *testing.T) {
		c, dir := newClock(), t.TempDir()
		s, session := claimed(t, dir, c)
		c.advance(30 * time.Minute)
		s.Resolve(session)
		if restart(t, dir, c, session) {
			t.Error("a use 30 minutes after the last write reached the disk")
		}
	})
	t.Run("an hour on", func(t *testing.T) {
		c, dir := newClock(), t.TempDir()
		s, session := claimed(t, dir, c)
		c.advance(time.Hour)
		s.Resolve(session)
		if !restart(t, dir, c, session) {
			t.Error("a use an hour after the last write did not reach the disk")
		}
	})
	t.Run("flushed", func(t *testing.T) {
		c, dir := newClock(), t.TempDir()
		s, session := claimed(t, dir, c)
		c.advance(30 * time.Minute)
		s.Resolve(session)
		if err := s.Flush(); err != nil {
			t.Fatal(err)
		}
		if !restart(t, dir, c, session) {
			t.Error("a use flushed did not reach the disk")
		}
	})
}

func TestRecordsOfARemovedPersonAreDroppedOnLoad(t *testing.T) {
	c, dir := newClock(), t.TempDir()
	_, session := claimed(t, dir, c)
	if err := store.Save(filepath.Join(dir, "persons.json"), PersonsFormat, []Person{}); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir, c)
	if _, ok := s.Resolve(session); ok {
		t.Error("a Session of a removed Person accepted")
	}
	var saved []any
	if err := store.Load(filepath.Join(dir, "sessions.json"), SessionsFormat, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Errorf("sessions.json keeps %d Sessions of a removed Person", len(saved))
	}
	if _, ok := s.Setup(); !ok {
		t.Error("without an Admin, no Setup link")
	}
}

func TestASessionIsFreshFor10MinutesAfterSigningIn(t *testing.T) {
	c := newClock()
	s, session := claimed(t, t.TempDir(), c)
	c.advance(10*time.Minute - time.Second)
	id, _ := s.Resolve(session)
	if !id.Fresh || id.StepUp() != nil {
		t.Errorf("%v after signing in: fresh %v, StepUp %v", 10*time.Minute-time.Second, id.Fresh, id.StepUp())
	}
	c.advance(time.Second)
	id, _ = s.Resolve(session)
	if err := id.StepUp(); id.Fresh || !errors.Is(err, ErrRefused) || err.Error() == ErrRefused.Error() {
		t.Errorf("10 minutes after signing in: fresh %v, StepUp %v; want refused with a reason", id.Fresh, err)
	}
}

func TestBrowserLabels(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0":                                                                  "Firefox on Linux",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1": "Safari on iPhone",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Mobile Safari/537.36":                         "Chrome on Android",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36 Edg/138.0.0.0":           "Edge on Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15":                   "Safari on macOS",
		"Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/138.0.0.0 Mobile/15E148 Safari/604.1":       "Chrome on iPad",
		"curl/8.7.1": "Unknown browser",
		"":           "Unknown browser",
	} {
		if got := Browser(ua); got != want {
			t.Errorf("Browser(%q) = %q, want %q", ua, got, want)
		}
	}
}
