// Package history keeps, in SQLite (ADR 0006, ADR 0011), the History of every
// Target indefinitely, the history of Commands indefinitely, and the Traces
// of Automation Runs for 30 days. Home and the engine hand entries over
// without ever waiting: one writer goroutine inserts them in batched
// transactions, and an entry that finds the queue full is dropped and
// counted. Where the History is incomplete, because Oiko was stopped or
// points were lost, it records a Gap. A Replace or a Delete Home announces is
// never dropped (ADR 0016): the History follows the Targets' lifecycle.
package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

const (
	retention = 30 * 24 * time.Hour
	buffered  = 8192 // entries awaiting the writer
	maxBatch  = 1024 // entries per transaction
	recent    = 100  // entries listed
	aliveEach = time.Minute
)

// schema creates a fresh database in the current format.
const schema = `
CREATE TABLE runs (
	id             TEXT PRIMARY KEY,
	automation     TEXT NOT NULL,
	time           INTEGER NOT NULL, -- Unix ns
	outcome        TEXT NOT NULL,
	trace          TEXT NOT NULL,    -- JSON
	trigger_target TEXT              -- the Target whose Value or Event triggered the Run, if any
);
CREATE INDEX runs_by_automation ON runs (automation, time);
CREATE INDEX runs_by_time ON runs (time);
CREATE INDEX runs_by_trigger ON runs (trigger_target, time);
CREATE TABLE commands (
	id      TEXT PRIMARY KEY,
	target  TEXT NOT NULL,
	time    INTEGER NOT NULL, -- Unix ns
	status  TEXT NOT NULL,
	command TEXT NOT NULL     -- JSON
);
CREATE INDEX commands_by_target ON commands (target, time);
CREATE INDEX commands_by_time ON commands (time);
CREATE TABLE series (
	id         INTEGER PRIMARY KEY,
	target     TEXT NOT NULL,
	capability TEXT NOT NULL, -- '' for the Target's Availability
	UNIQUE (target, capability)
);
CREATE TABLE points (
	series INTEGER NOT NULL,
	ts     INTEGER NOT NULL, -- Unix ns, strictly increasing per series
	value  NOT NULL,         -- REAL, 0/1, TEXT, or JSON TEXT for composite and list Values
	PRIMARY KEY (series, ts)
) WITHOUT ROWID;
CREATE TABLE gaps (
	start INTEGER NOT NULL, -- Unix ns
	"end" INTEGER NOT NULL, -- Unix ns
	lost  INTEGER NOT NULL  -- points lost, 0 while Oiko was stopped
);
CREATE TABLE alive (
	id   INTEGER PRIMARY KEY CHECK (id = 0),
	time INTEGER NOT NULL -- Unix ns, Oiko last known running
);
` + auditSchema

// auditSchema creates the Audit log (ADR 0033): append-only, each entry kept
// a year. Its actor and subject are each a kind (person, kiosk, program,
// host, oiko or unknown), an id and the Name they had then.
const auditSchema = `
CREATE TABLE audit (
	id           INTEGER PRIMARY KEY,
	time         INTEGER NOT NULL, -- Unix ns, when it happened
	event        TEXT NOT NULL,
	actor_kind   TEXT NOT NULL,
	actor_id     TEXT NOT NULL DEFAULT '',
	actor_name   TEXT NOT NULL DEFAULT '',
	subject_kind TEXT NOT NULL,
	subject_id   TEXT NOT NULL DEFAULT '',
	subject_name TEXT NOT NULL DEFAULT '',
	browser      TEXT NOT NULL DEFAULT '', -- its label, where a browser is involved
	detail       TEXT                      -- JSON: what else the event tells, if anything
);
CREATE INDEX audit_by_time ON audit (time);
CREATE INDEX audit_by_actor ON audit (actor_kind, actor_id, time);
CREATE INDEX audit_by_subject ON audit (subject_kind, subject_id, time);
CREATE TRIGGER audit_append_only BEFORE UPDATE ON audit
BEGIN
	SELECT RAISE(ABORT, 'the Audit log is append-only');
END;
`

// The format of history.db (ADR 0019), kept in its user_version: the oldest
// one migrated from, the release to go through first with an older one, and
// the migrations from there, migrations[i] taking oldest+i to oldest+i+1.
var (
	oldest     = 1
	through    = ""
	migrations = []func(*sql.Tx) error{toFormat2}
)

// toFormat2 reads the Commands from the API, before sign-in, as of unknown
// Origin (ADR 0031), and creates the Audit log (ADR 0033).
func toFormat2(tx *sql.Tx) error {
	if _, err := tx.Exec(`UPDATE commands SET command = json_set(command, '$.origin', 'unknown')
		WHERE json_extract(command, '$.origin') = 'api'`); err != nil {
		return err
	}
	_, err := tx.Exec(auditSchema)
	return err
}

// Store keeps the History, Commands and Traces.
type Store struct {
	db     *sql.DB
	ready  chan struct{} // signalled once entries are queued
	lost   atomic.Uint64
	series map[seriesKey]*series // owned by the writer
	opened map[seriesKey]series  // each series' last point as Open found it, for Recall

	mu    sync.Mutex
	queue []entry // awaiting the writer
	drops drops   // points lost since the last Gap was recorded
}

// drops are points lost from first to last.
type drops struct {
	first, last time.Time
	n           int64
}

// entry is a Trace, a Command record, a point, or a Replace or Delete,
// awaiting the writer.
type entry struct {
	trace     *automation.Trace
	command   *home.CommandRecord
	point     *point
	lifecycle func(*sql.Tx) error
}

type seriesKey struct{ target, capability string }

// series is a series' row id and its last recorded point, if any (nil value).
type series struct {
	id    int64
	ts    int64
	value any // as stored
}

// point is a Value, an Event or an Availability, awaiting the writer.
type point struct {
	seriesKey
	at    time.Time
	data  any
	event bool
}

// Open opens the database at path, creating it if need be. Entries are
// written once Run is called.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(runtime.GOMAXPROCS(0) + 1) // Points reads on every core, the writer on its own
	s := &Store{db: db, ready: make(chan struct{}, 1)}
	if err := migrate(db, path); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.load(); err != nil {
		db.Close()
		return nil, err
	}
	s.opened = map[seriesKey]series{}
	for k, x := range s.series {
		s.opened[k] = *x
	}
	if err := s.start(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

const (
	markAlive = `INSERT OR REPLACE INTO alive (id, time) VALUES (0, ?)`
	insertGap = `INSERT INTO gaps (start, "end", lost) VALUES (?, ?, ?)`
)

// migrate brings the database at path to the current format: a fresh one is
// created in it, an older one is copied to path.v<format> through VACUUM
// INTO, then migrated one transaction per format.
func migrate(db *sql.DB, path string) error {
	current := oldest + len(migrations)
	var version, tables int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables); err != nil {
		return err
	}
	if version == 0 && tables == 0 {
		return inFormat(db, current, func(tx *sql.Tx) error {
			_, err := tx.Exec(schema)
			return err
		})
	}
	version = max(version, 1) // written before formats
	if err := store.Check(path, version, oldest, current, through); err != nil {
		return err
	}
	if version == current {
		return nil
	}
	copied := fmt.Sprintf("%s.v%d", path, version)
	if err := os.Remove(copied); err != nil && !errors.Is(err, fs.ErrNotExist) { // left by a migration cut short
		return err
	}
	if _, err := db.Exec(`VACUUM INTO ?`, copied); err != nil {
		return err
	}
	for v := version; v < current; v++ {
		if err := inFormat(db, v+1, migrations[v-oldest]); err != nil {
			return fmt.Errorf("migrating %s to format %d: %w", path, v+1, err)
		}
	}
	return nil
}

// inFormat runs f in one transaction leaving the database in format version.
func inFormat(db *sql.DB, version int, f func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once committed
	if err := f(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		return err
	}
	return tx.Commit()
}

// start records a Gap from the alive mark, if any, to now: Oiko was
// stopped. Then it marks Oiko alive.
func (s *Store) start() error {
	now := time.Now().UnixNano()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mark int64
	switch err := tx.QueryRow(`SELECT time FROM alive`).Scan(&mark); {
	case err == nil:
		if _, err := tx.Exec(insertGap, mark, now, 0); err != nil {
			return err
		}
	case !errors.Is(err, sql.ErrNoRows): // no mark: the first start
		return err
	}
	if _, err := tx.Exec(markAlive, now); err != nil {
		return err
	}
	return tx.Commit()
}

// load reads every series with its last point.
func (s *Store) load() error {
	rows, err := s.db.Query(`SELECT s.id, s.target, s.capability, coalesce(p.ts, 0), p.value FROM series s
		LEFT JOIN points p ON p.series = s.id AND p.ts = (SELECT max(ts) FROM points WHERE series = s.id)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	all := map[seriesKey]*series{}
	for rows.Next() {
		var k seriesKey
		var x series
		if err := rows.Scan(&x.id, &k.target, &k.capability, &x.ts, &x.value); err != nil {
			return err
		}
		all[k] = &x
	}
	s.series = all
	return rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }

// Recall is when ref's Value took data before Oiko started: its last point,
// if that holds data. Home asks it for a Ref's first Value.
func (s *Store) Recall(ref home.Ref, data any) (time.Time, bool) {
	last, ok := s.opened[seriesKey{ref.Target.Key(), ref.Capability}]
	if v, err := encode(data); !ok || last.ts == 0 || err != nil || v != last.value {
		return time.Time{}, false
	}
	return time.Unix(0, last.ts), true
}

// Record hands over the Trace of a Run, without waiting.
func (s *Store) Record(t *automation.Trace) {
	if !s.hand(entry{trace: t}) {
		t.Release()
	}
}

// Command hands over a Command record, without waiting.
func (s *Store) Command(c home.CommandRecord) {
	s.hand(entry{command: &c})
}

// Follow hands over an Update of Home.Follow, without waiting. A Value, an
// Event or an Availability, at the time it is handed over, becomes a point
// of its series; the Values and Events a Replace moved do not: the Replace
// carries their points over. A Target deleted takes its History with it, a
// Device's Functions' included; a Device replaced takes over the History of
// the hardware it got, merged into its own series of the same keys. Neither
// is ever dropped, and each lands after every point handed over before it.
// Other Updates are ignored.
func (s *Store) Follow(u home.Update) {
	var p point
	switch u.Kind {
	case home.ValueChanged, home.ValueRefreshed, home.EventOccurred:
		if u.Value == nil || u.Moved { // an Aggregate lost its Value: nothing is synthesized
			return
		}
		p = point{seriesKey: seriesKey{u.Ref.Target.Key(), u.Ref.Capability}, at: u.Value.At, data: u.Value.Data, event: u.Kind == home.EventOccurred}
	case home.AvailabilityChanged:
		// Home reports no time for a transition: it happens as it is announced.
		p = point{seriesKey: seriesKey{u.Target.Key(), ""}, at: time.Now(), data: string(u.Availability)}
	case home.TargetDeleted:
		t := u.Target
		s.hand(entry{lifecycle: func(tx *sql.Tx) error { return s.delete(tx, t) }})
		return
	case home.DeviceReplaced:
		kept, fresh := u.Target.Device(), u.Replaced
		s.hand(entry{lifecycle: func(tx *sql.Tx) error { return s.replace(tx, kept, fresh) }})
		return
	default:
		return
	}
	s.hand(entry{point: &p})
}

// owned lists the series of Target t, with their Target: t's own, and its
// Functions' for a Device itself.
func (s *Store) owned(t home.Target) map[seriesKey]home.Target {
	all := map[seriesKey]home.Target{}
	for k := range s.series {
		if o, err := home.ParseTarget(k.target); err == nil && (o == t || t.Function() == "" && o.Device() != "" && o.Device() == t.Device()) {
			all[k] = o
		}
	}
	return all
}

func (s *Store) replace(tx *sql.Tx, kept, fresh home.DeviceID) error {
	for k, o := range s.owned(home.TargetDevice(fresh, "")) {
		moved := s.series[k]
		delete(s.series, k)
		k.target = home.TargetDevice(kept, o.Function()).Key()
		into := s.series[k]
		if into == nil {
			if _, err := tx.Exec(`UPDATE series SET target = ? WHERE id = ?`, k.target, moved.id); err != nil {
				return err
			}
			s.series[k] = moved
			continue
		}
		// ponytail: of two points in the same ns, the kept identity's stays and
		// the other is lost unrecorded; shift it by 1 ns if that ever matters.
		if _, err := tx.Exec(`INSERT OR IGNORE INTO points (series, ts, value) SELECT ?, ts, value FROM points WHERE series = ?`, into.id, moved.id); err != nil {
			return err
		}
		if err := deleteSeries(tx, moved.id); err != nil {
			return err
		}
		if moved.ts > into.ts {
			into.ts, into.value = moved.ts, moved.value
		}
	}
	return nil
}

func (s *Store) delete(tx *sql.Tx, t home.Target) error {
	for k := range s.owned(t) {
		if err := deleteSeries(tx, s.series[k].id); err != nil {
			return err
		}
		delete(s.series, k)
	}
	return nil
}

func deleteSeries(tx *sql.Tx, id int64) error {
	if _, err := tx.Exec(`DELETE FROM points WHERE series = ?`, id); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM series WHERE id = ?`, id)
	return err
}

// hand queues e for the writer, never waiting. Past buffered entries it
// drops a point, a Trace or a Command, but never a Replace or a Delete.
func (s *Store) hand(e entry) bool {
	s.mu.Lock()
	if e.lifecycle == nil && len(s.queue) >= buffered {
		s.mu.Unlock()
		s.lose(e)
		return false
	}
	s.queue = append(s.queue, e)
	s.mu.Unlock()
	s.wake()
	return true
}

// wake tells the writer entries are queued.
func (s *Store) wake() {
	select {
	case s.ready <- struct{}{}:
	default: // already told
	}
}

// lose counts e as lost. A lost point belongs to the next Gap; a lost Trace
// or Command does not.
func (s *Store) lose(e entry) {
	s.lost.Add(1)
	if e.point != nil {
		now := time.Now()
		s.drop(drops{now, now, 1})
	}
}

func (s *Store) drop(d drops) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.drops.n == 0 || d.first.Before(s.drops.first) {
		s.drops.first = d.first
	}
	if d.last.After(s.drops.last) {
		s.drops.last = d.last
	}
	s.drops.n += d.n
}

// recordDrops records the points lost so far as one Gap, once the writer
// has caught up with the hand-off, so that a burst is one Gap. It reports
// whether every lost point is recorded.
func (s *Store) recordDrops() bool {
	s.mu.Lock()
	d := s.drops
	if d.n == 0 || len(s.queue) >= maxBatch {
		s.mu.Unlock()
		return d.n == 0
	}
	s.drops = drops{}
	s.mu.Unlock()
	if _, err := s.db.Exec(insertGap, d.first.UnixNano(), d.last.UnixNano(), d.n); err != nil {
		log.Printf("history: recording a gap of %d points: %v", d.n, err)
		s.drop(d) // retried with the next drops
		return false
	}
	return true
}

// alive marks Oiko alive now, once every lost point before is recorded: a
// crash meanwhile leaves them in the Gap from the previous mark.
func (s *Store) alive() {
	if !s.recordDrops() {
		return
	}
	if _, err := s.db.Exec(markAlive, time.Now().UnixNano()); err != nil {
		log.Printf("history: marking alive: %v", err)
	}
}

// Lost is the number of entries dropped: the queue was full, or writing
// them failed.
func (s *Store) Lost() uint64 { return s.lost.Load() }

// Run writes entries as they arrive until ctx is done, then writes those
// still queued. It marks Oiko alive every minute and when it stops.
func (s *Store) Run(ctx context.Context) {
	batch := make([]entry, 0, maxBatch)
	tick := time.NewTicker(aliveEach)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			for batch = s.take(batch[:0]); len(batch) > 0; batch = s.take(batch[:0]) {
				s.write(batch)
			}
			s.alive()
			return
		case <-tick.C:
			s.alive()
		case <-s.ready:
			s.write(s.take(batch[:0]))
		}
	}
}

// take appends to batch the entries queued, up to maxBatch. If more are
// left, the writer comes back for them.
func (s *Store) take(batch []entry) []entry {
	s.mu.Lock()
	n := min(maxBatch-len(batch), len(s.queue))
	batch = append(batch, s.queue[:n]...)
	clear(s.queue[:n]) // the Traces they hold go with the batch
	s.queue = s.queue[n:]
	left := len(s.queue) > 0
	s.mu.Unlock()
	if left {
		s.wake()
	}
	return batch
}

// write inserts batch in one transaction, then purges Traces past
// retention. Entries it fails to write count as lost, and lost points are
// recorded as a Gap. If the batch fails as a whole, each Replace or Delete
// in it is retried alone: it is not lost for the others' sake.
func (s *Store) write(batch []entry) {
	failed, err := s.insert(batch)
	if err != nil {
		log.Printf("history: writing %d entries: %v", len(batch), err)
		s.reload()
		failed = nil
		for _, e := range batch {
			if e.lifecycle == nil {
				failed = append(failed, e)
			} else if _, err := s.insert([]entry{e}); err != nil {
				log.Printf("history: replacing or deleting: %v", err)
				s.reload()
				failed = append(failed, e)
			}
		}
	}
	for _, e := range failed {
		s.lose(e)
	}
	s.recordDrops()
	for _, e := range batch {
		if e.trace != nil {
			e.trace.Release()
		}
	}
	clear(batch)
}

// reload reads the series as committed, after a failed transaction.
func (s *Store) reload() {
	if err := s.load(); err != nil {
		log.Printf("history: reading series: %v", err)
	}
}

// insert writes batch, reporting the entries that failed on their own. A
// Replace or Delete that fails fails the whole batch.
func (s *Store) insert(batch []entry) (failed []entry, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, e := range batch {
		var err error
		switch c := e.command; {
		case e.lifecycle != nil:
			// Half applied, it would leave series that are no Target's.
			if err := e.lifecycle(tx); err != nil {
				return nil, err
			}
		case e.point != nil:
			err = s.insertPoint(tx, e.point)
		case e.trace != nil:
			err = insertTrace(tx, e.trace)
		case c.Status == home.Pending:
			err = insertCommand(tx, c)
		default:
			_, err = tx.Exec(`UPDATE commands SET status = ? WHERE id = ?`, c.Status, c.ID)
		}
		if err != nil {
			failed = append(failed, e)
			log.Printf("history: %v", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM runs WHERE time < ?`, time.Now().Add(-retention).UnixNano()); err != nil {
		return nil, err
	}
	return failed, tx.Commit()
}

// insertPoint records p if it differs from its series' last point, or is
// an Event, never earlier than 1 ns after that point.
func (s *Store) insertPoint(tx *sql.Tx, p *point) error {
	v, err := encode(p.data)
	if err != nil {
		return fmt.Errorf("point of %s %q: %w", p.target, p.capability, err)
	}
	last := s.series[p.seriesKey]
	if last == nil {
		last = &series{}
		if err := tx.QueryRow(`INSERT INTO series (target, capability) VALUES (?, ?) RETURNING id`, p.target, p.capability).Scan(&last.id); err != nil {
			return err
		}
		s.series[p.seriesKey] = last
	} else if !p.event && last.value == v {
		return nil
	}
	ts := max(p.at.UnixNano(), last.ts+1)
	if _, err := tx.Exec(`INSERT INTO points (series, ts, value) VALUES (?, ?, ?)`, last.id, ts, v); err != nil {
		return err
	}
	last.ts, last.value = ts, v
	return nil
}

// encode is how a Value's data is stored: a REAL, 0/1, TEXT, or JSON TEXT
// for composite and list Values.
func encode(data any) (any, error) {
	switch d := data.(type) {
	case bool:
		if d {
			return int64(1), nil
		}
		return int64(0), nil
	case float64, string:
		return d, nil
	}
	b, err := json.Marshal(data)
	return string(b), err
}

func insertTrace(tx *sql.Tx, t *automation.Trace) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("trace of run %s: %w", t.Run, err)
	}
	var trigger any // a Value or an Event triggered it: one of its Target's
	if t.Trigger.Capability != "" {
		trigger = t.Trigger.Target.Key()
	}
	_, err = tx.Exec(`INSERT INTO runs (id, automation, time, outcome, trace, trigger_target) VALUES (?, ?, ?, ?, ?, ?)`,
		t.Run.String(), t.Automation, t.Time.UnixNano(), t.Outcome, string(data), trigger)
	return err
}

func insertCommand(tx *sql.Tx, c *home.CommandRecord) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("command %s: %w", c.ID, err)
	}
	_, err = tx.Exec(`INSERT INTO commands (id, target, time, status, command) VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Target.Key(), c.Time.UnixNano(), c.Status, string(data))
	return err
}

// Runs lists the latest Runs of an Automation, the latest first.
func (s *Store) Runs(automationID string) ([]home.RunEnd, error) {
	return runs(s.db.Query(`SELECT trace FROM runs WHERE automation = ? ORDER BY time DESC LIMIT ?`, automationID, recent))
}

// runs reads the ends of the Runs whose Traces rows hold.
func runs(rows *sql.Rows, err error) ([]home.RunEnd, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []home.RunEnd{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var t automation.Trace
		if err := json.Unmarshal([]byte(data), &t); err != nil {
			return nil, err
		}
		runs = append(runs, t.End())
	}
	return runs, rows.Err()
}

// DeleteRun forgets the Trace of a Run.
func (s *Store) DeleteRun(run string) error {
	_, err := s.db.Exec(`DELETE FROM runs WHERE id = ?`, run)
	return err
}

// ClearRuns forgets the Traces of every Run of an Automation.
func (s *Store) ClearRuns(automationID string) error {
	_, err := s.db.Exec(`DELETE FROM runs WHERE automation = ?`, automationID)
	return err
}

// Trace returns the Trace of a Run, in JSON, with the final status of the
// Commands it issued.
func (s *Store) Trace(run string) (json.RawMessage, error) {
	var data string
	err := s.db.QueryRow(`SELECT trace FROM runs WHERE id = ?`, run).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, home.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t automation.Trace
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return nil, err
	}
	for _, r := range t.Steps {
		for i := range r.Commands {
			c := &r.Commands[i]
			if c.ID == "" {
				continue
			}
			err := s.db.QueryRow(`SELECT status FROM commands WHERE id = ?`, c.ID).Scan(&c.Status)
			if err != nil && !errors.Is(err, sql.ErrNoRows) { // a lost Command entry leaves it without status
				return nil, err
			}
		}
	}
	return json.Marshal(t)
}

// Commands lists the latest Commands on a target, the latest first, each
// with its final status.
func (s *Store) Commands(t home.Target) ([]home.CommandRecord, error) {
	return commands(s.db.Query(`SELECT command, status FROM commands WHERE target = ? ORDER BY time DESC LIMIT ?`, t.Key(), recent))
}

// commands reads the Commands rows hold, each with its final status.
func commands(rows *sql.Rows, err error) ([]home.CommandRecord, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cs := []home.CommandRecord{}
	for rows.Next() {
		var data string
		var status home.CommandStatus
		if err := rows.Scan(&data, &status); err != nil {
			return nil, err
		}
		var c home.CommandRecord
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		c.Status = status
		cs = append(cs, c)
	}
	return cs, rows.Err()
}
