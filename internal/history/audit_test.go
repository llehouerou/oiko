package history

import (
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

var (
	unknown = access.Party{Kind: access.UnknownKind}
	alice   = access.Party{Kind: access.PersonKind, ID: "alice", Name: "Alice"}
	bob     = access.Party{Kind: access.PersonKind, ID: "bob", Name: "Bob"}
)

func auditLog(t *testing.T, s *Store, q AuditQuery) []AuditEntry {
	t.Helper()
	es, err := s.AuditLog(q)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestTheAuditLogIsReadNewestFirstByIdentityAndPaged(t *testing.T) {
	s := open(t)
	at := time.Now().Add(-time.Hour)
	for i := range 150 {
		s.Audit(access.Entry{Time: at.Add(time.Duration(i) * time.Second), Event: access.SignedIn, Actor: alice, Subject: alice, Browser: "Firefox on Linux", Detail: map[string]any{"method": "passkey"}})
	}
	s.Audit(access.Entry{Time: at.Add(-time.Minute), Event: access.LevelChanged, Actor: alice, Subject: bob})
	s.Audit(access.Entry{Time: at.Add(time.Hour - time.Minute), Event: access.SessionEnded, Actor: access.Party{Kind: access.OikoKind}, Subject: bob})
	written(s)

	page := auditLog(t, s, AuditQuery{})
	if len(page) != 100 || page[0].Event != access.SessionEnded || page[0].Subject != bob {
		t.Fatalf("first page: %d entries, the first %+v; want 100, Bob's Session ended", len(page), page[0])
	}
	first := page[1]
	if !first.Time.Equal(at.Add(149*time.Second)) || first.Browser != "Firefox on Linux" || first.Detail["method"] != "passkey" || first.Actor != alice {
		t.Errorf("as read: %+v", first)
	}
	rest := auditLog(t, s, AuditQuery{Before: page[99].ID})
	if len(rest) != 52 || rest[51].Event != access.LevelChanged {
		t.Errorf("second page: %d entries, want 52, ending with the oldest", len(rest))
	}
	bobs := auditLog(t, s, AuditQuery{Party: bob})
	if len(bobs) != 2 || bobs[0].Event != access.SessionEnded || bobs[1].Event != access.LevelChanged {
		t.Errorf("Bob's entries: %+v", bobs)
	}
}

func TestPast100AnHourAnonymousRefusalsAreCounted(t *testing.T) {
	s := open(t)
	hour := time.Now().Truncate(time.Hour).Add(-2 * time.Hour)
	for i := range 101 {
		s.Audit(access.Entry{Time: hour.Add(time.Duration(i) * time.Second), Event: access.TokenRefused, Actor: unknown, Subject: unknown})
	}
	// Tied to an identity, a refusal is written whatever the budget.
	s.Audit(access.Entry{Time: hour.Add(time.Minute), Event: access.PasskeyRefused, Actor: unknown, Subject: alice})
	s.Audit(access.Entry{Time: hour.Add(time.Minute), Event: access.LinkRefused, Actor: unknown, Subject: unknown})
	// The next hour, the budget starts again, after the counts of the last.
	s.Audit(access.Entry{Time: hour.Add(time.Hour), Event: access.TokenRefused, Actor: unknown, Subject: unknown})
	written(s)
	es := auditLog(t, s, AuditQuery{})
	es = append(es, auditLog(t, s, AuditQuery{Before: es[len(es)-1].ID})...)
	counts := map[access.Event]any{}
	for _, e := range es {
		if n, ok := e.Detail["count"]; ok {
			counts[e.Event] = n
			if !e.Time.Equal(hour) || e.Actor != unknown || e.Subject != unknown {
				t.Errorf("a count: %+v; want the hour's, anonymous", e)
			}
		}
	}
	// 100 anonymous refusals, Alice's, the counts of the 101st and the link's,
	// and the next hour's first.
	if len(es) != 104 || counts[access.TokenRefused] != float64(1) || counts[access.LinkRefused] != float64(1) {
		t.Errorf("%d entries, counts %v; want 104, one Token and one link counted", len(es), counts)
	}
}

func TestAuditEntriesAreKeptAYear(t *testing.T) {
	s := open(t)
	s.Audit(access.Entry{Time: time.Now().Add(-366 * 24 * time.Hour), Event: access.SignedIn, Actor: alice, Subject: alice})
	s.Audit(access.Entry{Time: time.Now().Add(-364 * 24 * time.Hour), Event: access.SignedIn, Actor: bob, Subject: bob})
	written(s)
	if es := auditLog(t, s, AuditQuery{}); len(es) != 1 || es[0].Actor != bob {
		t.Errorf("after a year: %+v; want Bob's entry only", es)
	}
}

func TestAuditEntriesAreNeverDropped(t *testing.T) {
	s := open(t)
	for range buffered {
		s.Command(command("c", home.Pending, time.Now(), home.Origin{}))
	}
	s.Audit(access.Entry{Time: time.Now(), Event: access.SignedIn, Actor: alice, Subject: alice})
	written(s)
	if es := auditLog(t, s, AuditQuery{}); len(es) != 1 {
		t.Errorf("an entry handed over a full queue: %d written, want 1", len(es))
	}
}
