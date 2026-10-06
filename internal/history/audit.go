package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// The Audit log keeps each entry a year (ADR 0033), and writes at most
// anonymousBudget anonymous refusals an hour individually (ADR 0034).
const (
	auditRetention  = 365 * 24 * time.Hour
	anonymousBudget = 100
	auditPage       = 100 // entries read at once
)

// budget counts the anonymous refusals of the hour from hour: those written,
// and, past anonymousBudget, those only counted, by event.
type budget struct {
	hour    time.Time
	written int
	counted map[access.Event]int
}

// Audit hands an Audit log entry over, without waiting; it is never dropped
// (ADR 0033). Past anonymousBudget anonymous refusals in an hour, each is only
// counted, and the hour's count of each event follows as one entry once a
// later hour begins, or when the writer stops.
func (s *Store) Audit(e access.Entry) {
	s.mu.Lock()
	var counts []access.Entry
	write := true
	if e.Anonymous() {
		if hour := e.Time.Truncate(time.Hour); hour.After(s.budget.hour) {
			counts = s.counts()
			s.budget = budget{hour: hour, counted: map[access.Event]int{}}
		}
		if write = s.budget.written < anonymousBudget; write {
			s.budget.written++
		} else {
			s.budget.counted[e.Event]++
		}
	}
	s.mu.Unlock()
	if write {
		counts = append(counts, e)
	}
	for _, e := range counts {
		s.hand(entry{audit: &e})
	}
}

// counts are the entries counting the anonymous refusals of the budget's
// hour past it, which it then forgets. Callers hold s.mu.
func (s *Store) counts() []access.Entry {
	var es []access.Entry
	unknown := access.Party{Kind: access.UnknownKind}
	for event, n := range s.budget.counted {
		es = append(es, access.Entry{Time: s.budget.hour, Event: event, Actor: unknown, Subject: unknown, Detail: map[string]any{"count": n}})
	}
	clear(s.budget.counted)
	return es
}

// count hands over the counts of the budget's hour if it is over at now, or
// whatever its time when ending.
func (s *Store) count(now time.Time, ending bool) {
	s.mu.Lock()
	var counts []access.Entry
	if ending || !now.Before(s.budget.hour.Add(time.Hour)) {
		counts = s.counts()
	}
	s.mu.Unlock()
	for _, e := range counts {
		s.hand(entry{audit: &e})
	}
}

func insertAudit(tx *sql.Tx, e *access.Entry) error {
	var detail any
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("audit entry %s: %w", e.Event, err)
		}
		detail = string(b)
	}
	_, err := tx.Exec(`INSERT INTO audit (time, event, actor_kind, actor_id, actor_name, subject_kind, subject_id, subject_name, browser, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.Time.UnixNano(), e.Event, e.Actor.Kind, e.Actor.ID, e.Actor.Name,
		e.Subject.Kind, e.Subject.ID, e.Subject.Name, e.Browser, detail)
	return err
}

// AuditEntry is an entry of the Audit log as read, with its id, which pages
// through it.
type AuditEntry struct {
	ID int64 `json:"id"`
	access.Entry
}

// AuditQuery reads the Audit log: the entries where Party, unless its Kind is
// "", acts or is acted upon, newest first, from before the entry of id
// Before, if not 0.
type AuditQuery struct {
	Party  access.Party
	Before int64
}

// AuditLog answers a page of the Audit log, as q asks.
func (s *Store) AuditLog(q AuditQuery) ([]AuditEntry, error) {
	k, id := q.Party.Kind, q.Party.ID
	rows, err := s.db.Query(`SELECT id, time, event, actor_kind, actor_id, actor_name, subject_kind, subject_id, subject_name, browser, coalesce(detail, '')
		FROM audit
		WHERE (? = '' OR actor_kind = ? AND actor_id = ? OR subject_kind = ? AND subject_id = ?)
			AND (? = 0 OR (time, id) < (SELECT time, id FROM audit WHERE id = ?))
		ORDER BY time DESC, id DESC LIMIT ?`, k, k, id, k, id, q.Before, q.Before, auditPage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	es := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var at int64
		var detail string
		if err := rows.Scan(&e.ID, &at, &e.Event, &e.Actor.Kind, &e.Actor.ID, &e.Actor.Name,
			&e.Subject.Kind, &e.Subject.ID, &e.Subject.Name, &e.Browser, &detail); err != nil {
			return nil, err
		}
		e.Time = time.Unix(0, at)
		if detail != "" {
			if err := json.Unmarshal([]byte(detail), &e.Detail); err != nil {
				return nil, err
			}
		}
		es = append(es, e)
	}
	return es, rows.Err()
}
