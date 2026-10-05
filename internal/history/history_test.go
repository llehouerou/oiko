package history

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// written runs s's writer until everything recorded so far is written.
func written(s *Store) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Run(ctx)
}

func trace(automationID string, at time.Time, outcome home.RunOutcome) *automation.Trace {
	return &automation.Trace{Run: uuid.NewV7(), Automation: automationID, Time: at, Outcome: outcome,
		Trigger: home.Trigger{Step: "single", Kind: "eventTrigger", Value: "single", Time: at},
		Steps:   []automation.Reached{{Step: "single", Fired: []string{"out"}}, {Step: "cond", Unknown: true}}}
}

func command(id string, status home.CommandStatus, at time.Time, origin home.Origin) home.CommandRecord {
	c := home.CommandRecord{CommandState: home.CommandState{ID: id, Target: home.TargetDevice("lamp", "light"), Status: status, Origin: origin}, Time: at}
	if status == home.Pending {
		c.Values, c.Transition = map[string]any{"state": true}, 1.5
	}
	return c
}

func TestTracesAreKeptByRun(t *testing.T) {
	s := open(t)
	now := time.Now()
	first, second := trace("a", now.Add(-time.Minute), home.RunNothing), trace("a", now, home.RunActed)
	second.Trigger.CatchUp = true
	second.Steps = append(second.Steps, automation.Reached{Step: "lamp", Commands: []automation.Issued{{ID: "c1"}}})
	s.Record(first)
	s.Record(second)
	s.Record(trace("b", now, home.RunError))
	written(s)

	runs, err := s.Runs("a")
	if err != nil {
		t.Fatal(err)
	}
	want := []home.RunEnd{
		{Automation: "a", Run: second.Run, Time: now, Outcome: home.RunActed, Trigger: second.Trigger, Commands: 1},
		{Automation: "a", Run: first.Run, Time: now.Add(-time.Minute), Outcome: home.RunNothing, Trigger: first.Trigger},
	}
	same := func(a, b home.RunEnd) bool {
		return a.Automation == b.Automation && a.Run == b.Run && a.Time.Equal(b.Time) && a.Outcome == b.Outcome &&
			a.Trigger.Step == b.Trigger.Step && a.Trigger.Value == b.Trigger.Value && a.Trigger.CatchUp == b.Trigger.CatchUp && a.Commands == b.Commands
	}
	if len(runs) != 2 || !same(runs[0], want[0]) || !same(runs[1], want[1]) {
		t.Errorf("runs = %+v, want the latest first: %+v", runs, want)
	}

	data, err := s.Trace(first.Run.String())
	if err != nil {
		t.Fatal(err)
	}
	var got automation.Trace
	if err := json.Unmarshal(data, &got); err != nil || got.Run != first.Run || len(got.Steps) != 2 || !got.Steps[1].Unknown {
		t.Errorf("trace = %s, %v", data, err)
	}
	if _, err := s.Trace(uuid.NewV7().String()); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("unknown trace: %v", err)
	}
}

func TestRunsAreDeletedOneByOneOrAll(t *testing.T) {
	s := open(t)
	now := time.Now()
	a1, a2, b := trace("a", now, home.RunActed), trace("a", now, home.RunActed), trace("b", now, home.RunActed)
	s.Record(a1)
	s.Record(a2)
	s.Record(b)
	written(s)

	if err := s.DeleteRun(a1.Run.String()); err != nil {
		t.Fatal(err)
	}
	if runs, _ := s.Runs("a"); len(runs) != 1 || runs[0].Run != a2.Run {
		t.Errorf("runs of a = %+v, want only the other one", runs)
	}
	if err := s.ClearRuns("a"); err != nil {
		t.Fatal(err)
	}
	if runs, _ := s.Runs("a"); len(runs) != 0 {
		t.Errorf("runs of a = %+v, want none", runs)
	}
	if runs, _ := s.Runs("b"); len(runs) != 1 {
		t.Errorf("runs of b = %+v, want untouched", runs)
	}
}

func TestCommandsAreKeptWithTheirFinalStatus(t *testing.T) {
	s := open(t)
	now := time.Now()
	run := home.Origin{Automation: "a", Step: "s", Run: uuid.NewV7()}
	s.Command(command("c1", home.Pending, now.Add(-time.Second), home.Origin{}))
	s.Command(command("c2", home.Pending, now, run))
	s.Command(command("c1", home.Superseded, now, home.Origin{}))
	s.Command(command("c2", home.Confirmed, now, run))
	s.Command(home.CommandRecord{CommandState: home.CommandState{ID: "other", Target: home.TargetFlag("night"), Status: home.Pending}, Time: now})
	written(s)

	got, err := s.Commands(home.TargetDevice("lamp", "light"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("commands = %+v", got)
	}
	c2, c1 := got[0], got[1]
	if c2.ID != "c2" || c2.Status != home.Confirmed || c2.Origin != run || !c2.Time.Equal(now) || c2.Values["state"] != true || c2.Transition != 1.5 {
		t.Errorf("c2 = %+v", c2)
	}
	if c1.ID != "c1" || c1.Status != home.Superseded || c1.Origin != (home.Origin{}) {
		t.Errorf("c1 = %+v", c1)
	}
	if got, _ := s.Commands(home.TargetFlag("night")); len(got) != 1 || got[0].ID != "other" {
		t.Errorf("flag commands = %+v", got)
	}
}

func TestATraceReadsWithItsCommandsFinalStatus(t *testing.T) {
	s := open(t)
	tr := trace("a", time.Now(), home.RunActed)
	tr.Steps = append(tr.Steps, automation.Reached{Step: "lamp", Commands: []automation.Issued{
		{Target: home.TargetDevice("lamp", "light"), ID: "c1", Values: map[string]any{"effect": "colorloop"}},
		{Target: home.TargetFlag("night"), Refused: "offline"},
	}})
	s.Record(tr)
	s.Command(command("c1", home.Pending, tr.Time, home.Origin{}))
	s.Command(command("c1", home.Confirmed, tr.Time, home.Origin{}))
	written(s)

	data, err := s.Trace(tr.Run.String())
	var got automation.Trace
	if err == nil {
		err = json.Unmarshal(data, &got)
	}
	if err != nil {
		t.Fatal(err)
	}
	cs := got.Steps[2].Commands
	if cs[0].Status != home.Confirmed || cs[0].Values["effect"] != "colorloop" || cs[1].Status != "" {
		t.Errorf("commands = %+v, want the first confirmed with its values, the refused one without status", cs)
	}
}

func TestTracesOlderThan30DaysArePurgedButCommandsKept(t *testing.T) {
	s := open(t)
	old, now := time.Now().Add(-retention-time.Minute), time.Now()
	s.Record(trace("a", old, home.RunNothing))
	s.Command(command("old", home.Pending, old, home.Origin{}))
	written(s)
	s.Record(trace("a", now, home.RunNothing))
	s.Command(command("new", home.Pending, now, home.Origin{}))
	written(s)

	if runs, _ := s.Runs("a"); len(runs) != 1 || !runs[0].Time.Equal(now) {
		t.Errorf("runs = %+v", runs)
	}
	if cs, _ := s.Commands(home.TargetDevice("lamp", "light")); len(cs) != 2 {
		t.Errorf("commands = %+v, want both", cs)
	}
}

func TestAFullBufferDropsEntriesAndCountsThem(t *testing.T) {
	s := open(t)
	for i := range buffered + 3 { // with no writer running: no hand-off waits
		switch i % 3 {
		case 0:
			s.Record(trace("a", time.Now(), home.RunNothing))
		case 1:
			s.Command(command(uuid.NewV7().String(), home.Pending, time.Now(), home.Origin{}))
		default:
			ref := home.TargetDevice("lamp", "light").Ref("brightness")
			s.Follow(home.Update{Kind: home.ValueChanged, Ref: &ref, Value: &home.Value{Data: float64(i), At: time.Now()}})
		}
	}
	if n := s.Lost(); n != 3 {
		t.Errorf("lost = %d, want 3", n)
	}
	written(s)
	s.Record(trace("a", time.Now(), home.RunNothing))
	if n := s.Lost(); n != 3 {
		t.Errorf("lost after draining = %d, want still 3", n)
	}
}

func TestAnEntryThatCannotBeWrittenIsCounted(t *testing.T) {
	s := open(t)
	bad := trace("a", time.Now(), home.RunNothing)
	bad.Trigger.Value = math.Inf(1) // not JSON
	s.Record(bad)
	s.Record(trace("a", time.Now(), home.RunNothing))
	written(s)
	if runs, _ := s.Runs("a"); len(runs) != 1 || s.Lost() != 1 {
		t.Errorf("runs = %+v, lost = %d; want the good one kept, the bad one counted", runs, s.Lost())
	}
}
