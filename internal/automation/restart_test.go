package automation

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nathan-osman/go-sunrise"

	"github.com/llehouerou/oiko/internal/home"
)

// issuing is a fakeHome that hands each Command to the test, in order.
type issuing struct {
	*fakeHome
	issued chan home.Target
}

func (h issuing) Command(t home.Target, req home.Request) (string, error) {
	h.issued <- t
	return "", nil
}

// The clock starts once the home is known: what came before is applied then.
// A home never known holds it back knownWithin at most.
func TestRunWaitsForTheHomeToBeKnown(t *testing.T) {
	for _, replayed := range []bool{true, false} {
		synctest.Test(t, func(t *testing.T) {
			f := fixture()
			f.known = make(chan struct{})
			h := issuing{f, make(chan home.Target, 1)}
			e := New(h, []Document{doc("x", []string{pressSingle, cmd("on", `"flag:a"`)}, "single.out on.in")}, nil, nil, nil, nil, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go e.Run(ctx)
			f.deliver(home.Update{Kind: home.EventOccurred,
				Ref:   &home.Ref{Target: home.TargetDevice("button-office", "button"), Capability: "action"},
				Value: &home.Value{Data: "single"}})
			time.Sleep(knownWithin - time.Millisecond)
			synctest.Wait()
			select {
			case <-h.issued:
				t.Fatalf("replayed %v: ran before the home was known", replayed)
			default:
			}
			if replayed {
				close(f.known)
			} else {
				time.Sleep(time.Millisecond)
			}
			synctest.Wait()
			select {
			case <-h.issued:
			default:
				t.Errorf("replayed %v: the press ran nothing once started", replayed)
			}
		})
	}
}

// restart stops e and starts a new engine from its Documents and Step state,
// as Oiko does on a restart, with c reading t. The home's Values are unknown
// until the test replays them; then start starts the new engine's clock.
func restart(t *testing.T, e *Engine, f *fakeHome, c *clock, at time.Time) *Engine {
	t.Helper()
	e.mu.Lock()
	docs, state := e.documents(), e.states()
	e.mu.Unlock()
	data, err := json.Marshal(state) // as automation-state.json keeps it
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]State
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	c.t = at
	r := New(f, nil, nil, &place, nil, nil, e.record)
	r.now = func() time.Time { return c.t }
	r.mu.Lock()
	r.loadAll(docs, saved)
	r.mu.Unlock()
	return r
}

func TestTimerDueWhileDownFiresOnceAtStartupAfterTheReplay(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	create(t, e, doc("delayed", []string{pressSingle,
		`delay timer {"duration": 600, "reentry": "keep"}`,
		`dark valueCondition {"target": "device:lux-sensor/illuminance", "capability": "illuminance", "op": "lt", "value": 100}`,
		cmd("act", `"flag:a"`)},
		"single.out delay.start", "delay.fired dark.in", "dark.true act.in"))
	press(e, f, "button-office", "single")

	e = restart(t, e, f, c, at(15, 12, 30))
	set(e, f, home.ValueChanged, luxRef, 50.0) // replayed
	e.start()
	c.advance(e, at(15, 12, 30))
	expect(t, f, "at startup", "flag a map[on:true] 0s")
	tr := traces[len(traces)-1]
	if got, want := path(tr), []string{"delay [fired]", "dark [true] read 50", "act [then] c1"}; !slices.Equal(got, want) {
		t.Errorf("path:\n got  %q\n want %q", got, want)
	}
	if !tr.Trigger.CatchUp || !tr.Trigger.Time.Equal(at(15, 12, 10)) || !tr.Time.Equal(at(15, 12, 30)) {
		t.Errorf("trigger %+v at %v: want a catch-up of 12:10 at 12:30", tr.Trigger, tr.Time)
	}
	if end := f.runs[len(f.runs)-1]; end.Trigger != tr.Trigger || end.Trigger.Step != "delay" || end.Commands != 1 {
		t.Errorf("run end = %+v, want a catch-up started by delay, with 1 Command", end)
	}
	if c := tr.Steps[2].Commands[0]; len(c.Values) == 0 {
		t.Errorf("command %+v, want what it asked for", c)
	}
	c.advance(e, at(15, 13, 0))
	expect(t, f, "later")
}

func TestRestartKeepsStepState(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, doc("stateful", []string{pressSingle,
		`delay timer {"duration": 600, "reentry": "keep"}`, `once cooldown {"duration": 3600}`, cmd("act", `"flag:a"`)},
		"single.out delay.start", "single.out once.in", "once.out act.in"))
	press(e, f, "button-office", "single")
	want := []string{"delay due 12:10:00", `once passed "" at 12:00:00`}
	if got := stateOf(t, e, id); !slices.Equal(got, want) {
		t.Fatalf("state = %q, want %q", got, want)
	}
	e = restart(t, e, f, c, at(15, 12, 5))
	if got := stateOf(t, e, id); !slices.Equal(got, want) {
		t.Errorf("state after a restart = %q, want %q", got, want)
	}
}

func TestFlow3bCatchesUpTheSunAfterARestart(t *testing.T) {
	_, dusk17 := sunrise.TimeOfElevation(place.Latitude, place.Longitude, -6, 2026, time.June, 17)
	for _, c := range []struct {
		name    string
		catchUp bool
		want    []string
	}{
		{"catch-up", true, []string{"plug-vacuum-charger/switch map[state:true] 0s", "plug-vacuum-charger/switch map[state:false] 0s"}},
		{"no catch-up", false, nil},
	} {
		f, e, clk := timed(at(15, 12, 0))
		d := decoded(t, "flow3b-vacuum.json")
		if !c.catchUp {
			for _, i := range []int{0, 2} { // dusk and dawn
				d.Steps[i].Params = []byte(`{"event": "` + d.Steps[i].ID + `"}`)
			}
		}
		create(t, e, d)
		clk.advance(e, at(15, 20, 0))
		expect(t, f, c.name+": dusk on the 15th", "plug-vacuum-charger/switch map[state:false] 0s")

		// Down from 20:00 on the 15th to after dusk on the 17th: two dawns
		// and two dusks missed, the latest of each caught up in order.
		e = restart(t, e, f, clk, dusk17.Add(time.Hour))
		e.start()
		clk.advance(e, dusk17.Add(time.Hour))
		expect(t, f, c.name+": at startup", c.want...)
		clk.advance(e, at(18, 12, 0))
		expect(t, f, c.name+": dawn on the 18th", "plug-vacuum-charger/switch map[state:true] 0s")
	}
}

func TestCatchUpJustTurnedOnStartsFromNow(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	d := doc("reset", []string{`six timeTrigger {"at": "06:00"}`, cmd("off", `"flag:a"`)}, "six.out off.in")
	id := create(t, e, d)
	e = restart(t, e, f, c, at(16, 12, 0)) // 06:00 missed, not caught up
	e.start()
	c.advance(e, at(16, 12, 0))
	expect(t, f, "without catch-up")

	d.Steps[0].Params = []byte(`{"at": "06:00", "catchUp": true}`)
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	e = restart(t, e, f, c, at(16, 12, 1))
	e.start()
	c.advance(e, at(16, 12, 1))
	expect(t, f, "catch-up just turned on")
}

func TestEditingACatchUpTriggerReplaysNothing(t *testing.T) {
	f, e, c := timed(at(15, 5, 0))
	d := doc("reset", []string{`six timeTrigger {"at": "06:00", "catchUp": true}`, cmd("off", `"flag:a"`)}, "six.out off.in")
	id := create(t, e, d)
	c.advance(e, at(15, 12, 0))
	expect(t, f, "06:00", "flag a map[on:true] 0s")

	d.Steps[0].Params = []byte(`{"at": "09:00", "catchUp": true}`) // already past today
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	c.advance(e, at(15, 12, 1))
	expect(t, f, "moved to 09:00 at 12:00")
	c.advance(e, at(16, 9, 0))
	expect(t, f, "09:00 the next day", "flag a map[on:true] 0s")
}

func TestHeldForAcrossARestart(t *testing.T) {
	siren := home.TargetDevice("alarm", "alarm").Ref("alarm")
	const off = "alarm/alarm map[alarm:false] 0s"
	for _, c := range []struct {
		name     string
		back     time.Time // when Oiko is back
		replayed any       // the siren's Value replayed then
		want     []string
	}{
		{"still on", at(15, 12, 3), true, []string{off}},
		{"still on, overdue", at(15, 12, 10), true, []string{off}},
		{"changed during the downtime", at(15, 12, 3), false, nil},
		{"still unknown at the deadline", at(15, 12, 3), nil, nil},
	} {
		f, e, clk := timed(at(15, 12, 0))
		create(t, e, decoded(t, "flow6b-siren.json"))
		set(e, f, home.ValueChanged, siren, false)
		set(e, f, home.ValueChanged, siren, true) // held from 12:00

		e = restart(t, e, f, clk, c.back)
		if c.replayed != nil {
			set(e, f, home.ValueChanged, siren, c.replayed)
		}
		e.start()
		clk.advance(e, at(15, 12, 30))
		expect(t, f, c.name, c.want...)
	}
}

func TestRunawayStaysRunawayAcrossARestart(t *testing.T) {
	f, e, c := timed(at(15, 12, 0)) // every Run at the same instant
	d := doc("press", []string{pressSingle, cmd("act", `"flag:a"`)}, "single.out act.in")
	id := create(t, e, d)
	for range runawayRuns + 1 {
		press(e, f, "button-office", "single")
	}
	f.take()
	e = restart(t, e, f, c, at(15, 13, 0))
	if s := statusOf(f, id); s.Status != home.AutomationRunaway || !s.Since.Equal(at(15, 12, 0)) {
		t.Errorf("status after a restart = %+v", s)
	}
	press(e, f, "button-office", "single")
	expect(t, f, "runaway")

	if err := e.Replace(id, d); err != nil { // re-enabled
		t.Fatal(err)
	}
	e = restart(t, e, f, c, at(15, 14, 0))
	press(e, f, "button-office", "single")
	expect(t, f, "re-enabled", "flag a map[on:true] 0s")
}

func TestStateWritesAreCoalesced(t *testing.T) {
	f := fixture()
	var (
		mu     sync.Mutex
		writes []map[string]State
	)
	write := func(s map[string]State) error {
		mu.Lock()
		defer mu.Unlock()
		writes = append(writes, s)
		return nil
	}
	written := func() []map[string]State {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(writes)
	}
	e := New(f, nil, nil, nil, nil, write, nil)
	e.now = spaced()
	d := doc("restarted", []string{pressSingle, `delay timer {"duration": 3600, "reentry": "restart"}`}, "single.out delay.start")
	id := create(t, e, d)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(stopped)
	}()

	for range 100 { // each restarts the timer
		f.deliver(home.Update{Kind: home.EventOccurred,
			Ref:   &home.Ref{Target: home.TargetDevice("button-office", "button"), Capability: "action"},
			Value: &home.Value{Data: "single"}})
	}
	time.Sleep(writeDelay + 500*time.Millisecond)
	w := written()
	if len(w) != 1 || len(w[0][id].Steps) != 1 || w[0][id].Steps[0].Step != "delay" {
		t.Fatalf("writes after a burst = %+v, want one with the timer", w)
	}
	time.Sleep(writeDelay + 200*time.Millisecond)
	if n := len(written()); n != 1 {
		t.Errorf("%d writes while idle, want none", n-1)
	}
	cancel()
	<-stopped
	e.Flush()
	if n := len(written()); n != 1 {
		t.Errorf("a shutdown without a change wrote")
	}

	e = New(f, e.Documents(), w[0], nil, nil, write, nil) // a restart
	e.Flush()
	if n := len(written()); n != 1 {
		t.Errorf("a restart that changed nothing wrote")
	}
	d.Enabled = false
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	e.Flush()
	if w := written(); len(w) != 2 || len(w[1]) != 0 {
		t.Errorf("shutdown after disabling: writes %+v, want the cleared state flushed", w[1:])
	}
}
