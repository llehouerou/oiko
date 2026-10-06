package history

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

func TestMarkersAreTheCommandsAndTriggeredRunsOfTheTargetsInvolved(t *testing.T) {
	s := open(t)
	now := time.Now().Truncate(time.Millisecond)
	auto := home.Origin{Automation: "a", Step: "s", Run: uuid.NewV7()}
	s.Command(command("app", home.Pending, now.Add(-3*time.Minute), home.Origin{}))
	s.Command(command("app", home.Confirmed, now, home.Origin{}))
	s.Command(command("auto", home.Pending, now.Add(-2*time.Minute), auto))
	s.Command(command("auto", home.TimedOut, now, auto))
	s.Command(command("before", home.Pending, now.Add(-2*time.Hour), home.Origin{}))
	s.Command(home.CommandRecord{CommandState: home.CommandState{ID: "other", Target: home.TargetFlag("night"), Status: home.Pending}, Time: now})

	triggered := func(target home.Target, capability string, at time.Time) *automation.Trace {
		tr := trace("a", at, home.RunActed)
		tr.Trigger.Target, tr.Trigger.Capability = target, capability
		return tr
	}
	byState := triggered(lamp, "state", now.Add(-time.Minute))
	s.Record(byState)
	s.Record(triggered(home.TargetFlag("night"), "on", now))  // another Target
	s.Record(triggered(lamp, "", now))                        // its Availability, not a Value or an Event
	s.Record(triggered(lamp, "state", now.Add(-2*time.Hour))) // out of range

	q := Query{From: now.Add(-time.Hour), To: now.Add(time.Hour), Points: 100, Series: []Series{state, brightness}}
	written(s)
	a, err := s.Points(q)
	if err != nil {
		t.Fatal(err)
	}
	if a.Commands != nil || a.Runs != nil {
		t.Errorf("unasked: commands %+v, runs %+v", a.Commands, a.Runs)
	}
	q.Markers = true
	if a, err = s.Points(q); err != nil {
		t.Fatal(err)
	}
	if len(a.Commands) != 2 || a.Commands[0].ID != "app" || a.Commands[0].Status != home.Confirmed ||
		a.Commands[1].ID != "auto" || a.Commands[1].Status != home.TimedOut || a.Commands[1].Origin != auto {
		t.Errorf("commands = %+v, want the app's confirmed, then the Automation's timed out", a.Commands)
	}
	if len(a.Runs) != 1 || a.Runs[0].Run != byState.Run {
		t.Errorf("runs = %+v, want only the one its state triggered", a.Runs)
	}
}

func TestLiveViewsShowInTheHistoryOfTheirDevicesFunctions(t *testing.T) {
	s := open(t)
	now := time.Now().Truncate(time.Millisecond)
	garden := home.TargetDevice("garden", "camera")
	alice := home.Origin{Person: "alice"}
	s.LiveView(LiveView{Target: garden, Origin: alice, Start: now.Add(-2 * time.Minute), End: now.Add(-time.Minute)})
	s.LiveView(LiveView{Target: garden, Origin: home.Origin{Kiosk: "hall"}, Start: now.Add(-3 * time.Hour), End: now.Add(-2 * time.Hour)}) // out of range
	s.LiveView(LiveView{Target: home.TargetDevice("porch", "camera"), Origin: alice, Start: now, End: now})                                // another Device
	written(s)

	motion := Series{Ref: home.TargetDevice("garden", "occupancy").Ref("occupancy"), Type: home.Binary}
	a, err := s.Points(Query{From: now.Add(-time.Hour), To: now.Add(time.Hour), Points: 100, Series: []Series{motion}, Markers: true})
	if err != nil {
		t.Fatal(err)
	}
	want := LiveView{Target: garden, Origin: alice, Start: now.Add(-2 * time.Minute), End: now.Add(-time.Minute)}
	if len(a.LiveViews) != 1 || a.LiveViews[0].Target != want.Target || a.LiveViews[0].Origin != want.Origin ||
		!a.LiveViews[0].Start.Equal(want.Start) || !a.LiveViews[0].End.Equal(want.End) {
		t.Errorf("live views = %+v, want %+v", a.LiveViews, want)
	}
}

func TestFormat3AddsLiveViews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	create(t, path)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE live_views; PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.LiveView(LiveView{Target: home.TargetDevice("garden", "camera"), Start: time.Now(), End: time.Now()})
	written(s)
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM live_views`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("live views: %d, %v; want the one recorded", n, err)
	}
}
