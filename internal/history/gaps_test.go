package history

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

type gap struct{ start, end, lost int64 }

func gaps(t *testing.T, s *Store) []gap {
	t.Helper()
	rows, err := s.db.Query(`SELECT start, "end", lost FROM gaps ORDER BY start`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var gs []gap
	for rows.Next() {
		var g gap
		if err := rows.Scan(&g.start, &g.end, &g.lost); err != nil {
			t.Fatal(err)
		}
		gs = append(gs, g)
	}
	return gs
}

func aliveMark(t *testing.T, s *Store) int64 {
	t.Helper()
	var mark int64
	if err := s.db.QueryRow(`SELECT time FROM alive`).Scan(&mark); err != nil {
		t.Fatal(err)
	}
	return mark
}

// restart closes s, if any, then opens path as Oiko does when it starts.
func restart(t *testing.T, s *Store, path string) (*Store, time.Time) {
	t.Helper()
	if s != nil {
		s.Close()
	}
	at := time.Now()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, at
}

func TestAFirstStartRecordsNoGap(t *testing.T) {
	if gs := gaps(t, open(t)); len(gs) != 0 {
		t.Errorf("gaps = %+v, want none", gs)
	}
}

func TestARestartRecordsAGapFromTheLastAliveMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, _ := restart(t, nil, path)
	written(s) // a clean stop
	mark := aliveMark(t, s)
	s, restarted := restart(t, s, path)
	if gs := gaps(t, s); len(gs) != 1 || gs[0].start != mark || gs[0].end < restarted.UnixNano() || gs[0].lost != 0 {
		t.Errorf("gaps = %+v, want one from %d to the restart at %d", gs, mark, restarted.UnixNano())
	}
}

func TestARestoredVacuumCopyRecordsAGapFromTheCopy(t *testing.T) {
	dir := t.TempDir()
	s := open(t)
	written(s)
	backup := filepath.Join(dir, "backup.db")
	if _, err := s.db.Exec(`VACUUM INTO ?`, backup); err != nil {
		t.Fatal(err)
	}
	mark := aliveMark(t, s)
	written(s) // Oiko lives on after the copy
	s, restarted := restart(t, s, backup)
	if gs := gaps(t, s); len(gs) != 1 || gs[0].start != mark || gs[0].end < restarted.UnixNano() {
		t.Errorf("gaps = %+v, want one from the copy's mark %d to the restart", gs, mark)
	}
}

// queued is the number of entries awaiting s's writer.
func (s *Store) queued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// fill fills s's queue with Commands, so that the next point's hand-off drops.
func fill(s *Store) {
	for s.queued() < buffered {
		s.Command(command("c", home.Confirmed, time.Now(), home.Origin{}))
	}
}

func TestPointsDroppedByAFullBufferRecordOneGap(t *testing.T) {
	s := open(t)
	fill(s)
	before := time.Now()
	for i := range 3 {
		s.Follow(value(home.ValueChanged, lamp.Ref("brightness"), float64(i), time.Now()))
	}
	s.write(s.take(nil)) // the writer catches up, the buffer still full
	fill(s)
	s.Follow(value(home.ValueChanged, lamp.Ref("brightness"), 3.0, time.Now()))
	after := time.Now()
	written(s)
	if gs := gaps(t, s); len(gs) != 1 || gs[0].start < before.UnixNano() || gs[0].end > after.UnixNano() || gs[0].start > gs[0].end || gs[0].lost != 4 {
		t.Errorf("gaps = %+v, want one of 4 points within [%d, %d]", gs, before.UnixNano(), after.UnixNano())
	}
}

func TestAPointThatCannotBeWrittenRecordsAGap(t *testing.T) {
	s := open(t)
	s.Follow(value(home.ValueChanged, lamp.Ref("color"), map[string]any{"x": math.Inf(1)}, time.Now())) // not JSON
	written(s)
	if gs := gaps(t, s); len(gs) != 1 || gs[0].lost != 1 {
		t.Errorf("gaps = %+v, want one of 1 point", gs)
	}
}

func TestALostTraceOrCommandRecordsNoGap(t *testing.T) {
	s := open(t)
	fill(s)
	s.Record(trace("a", time.Now(), home.RunNothing))
	s.Command(command("d", home.Confirmed, time.Now(), home.Origin{}))
	written(s)
	bad := trace("a", time.Now(), home.RunNothing)
	bad.Trigger.Value = math.Inf(1) // not JSON
	s.Record(bad)
	written(s)
	if gs := gaps(t, s); s.Lost() != 3 || len(gs) != 0 {
		t.Errorf("lost = %d, gaps = %+v; want 3 lost and no gap", s.Lost(), gs)
	}
}
