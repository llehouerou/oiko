package automation

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

var holidayRef = home.TargetFlag("holiday").Ref("on")

const (
	lightsOn  = "flag a map[on:true] 0s"
	lightsOff = "flag a map[on:false] 0s"
)

// simulated is a presence simulation within [from, to), enabled and disabled
// by the Holiday Flag, that turns Flag a on and off.
func simulated(from, to string) Document {
	return doc("simulated", []string{
		`holiday-on valueTrigger {"target": "flag:holiday", "capability": "on", "op": "eq", "value": true}`,
		`holiday-off valueTrigger {"target": "flag:holiday", "capability": "on", "op": "eq", "value": false}`,
		fmt.Sprintf(`sim presenceSimulation {"from": %q, "to": %q, "minBlocks": 4, "maxBlocks": 6, "minDuration": 1800, "maxDuration": 2700}`, from, to),
		`lights-on command {"targets": ["flag:a"], "values": {"on": true}}`,
		`lights-off command {"targets": ["flag:a"], "values": {"on": false}}`},
		"holiday-on.out sim.enable", "holiday-off.out sim.disable", "sim.on lights-on.in", "sim.off lights-off.in")
}

// holidayStarts turns the Holiday Flag from off to on: a real change.
func holidayStarts(e *Engine, f *fakeHome) {
	set(e, f, home.ValueChanged, holidayRef, false)
	set(e, f, home.ValueChanged, holidayRef, true)
}

func presenceOf(t *testing.T, e *Engine, id, step string) StepState {
	t.Helper()
	list, err := e.State(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.Step == step {
			return s
		}
	}
	return StepState{}
}

// checkBlocks checks that s switches on and off by turns, in order, in
// between blocks and blocks of 30 to 45 min, all within [from, to].
func checkBlocks(t *testing.T, name string, s []Switch, from, to time.Time, minBlocks, maxBlocks int) {
	t.Helper()
	if len(s)%2 != 0 || len(s)/2 < minBlocks || len(s)/2 > maxBlocks {
		t.Fatalf("%s: %d switches, want %d to %d blocks", name, len(s), minBlocks, maxBlocks)
	}
	for i, sw := range s {
		if sw.On != (i%2 == 0) {
			t.Errorf("%s: switch %d on %v", name, i, sw.On)
		}
		if sw.At.Before(from) || sw.At.After(to) || i > 0 && !sw.At.After(s[i-1].At) {
			t.Errorf("%s: switch %d at %v, out of order or out of [%v, %v]", name, i, sw.At, from, to)
		}
		if d := sw.At.Sub(s[max(i-1, 0)].At); !sw.On && (d < 30*time.Minute || d > 45*time.Minute) {
			t.Errorf("%s: block %d lasts %v", name, i/2, d)
		}
	}
}

func TestFlow7aDrawsBlocksInsideItsWindow(t *testing.T) {
	for seed := range uint64(50) {
		f, e, c := timed(at(15, 12, 0))
		e.rand = rand.New(rand.NewPCG(seed, seed))
		id := create(t, e, decoded(t, "flow7a-presence-simulation.json"))
		holidayStarts(e, f)
		expect(t, f, "enabled")
		name := fmt.Sprint("seed ", seed)
		veranda, living := presenceOf(t, e, id, "veranda"), presenceOf(t, e, id, "living")
		checkBlocks(t, name+": veranda", veranda.Schedule, at(15, 18, 0), at(15, 21, 0), 4, 6)
		checkBlocks(t, name+": living", living.Schedule, at(15, 18, 0), at(15, 22, 0), 4, 6)
		if !veranda.Enabled || !veranda.WindowEnd.Equal(at(15, 21, 0)) || !veranda.Deadline.Equal(veranda.Schedule[0].At) {
			t.Fatalf("%s: veranda %+v", name, veranda)
		}

		c.advance(e, at(15, 22, 0))
		count := map[string]int{}
		for _, cmd := range f.take() {
			count[cmd]++
		}
		const on, off = "map[brightness:254 state:true] 1s", "map[state:false] 15s"
		for cmd, want := range map[string]int{
			"aggregate veranda " + on: len(veranda.Schedule) / 2, "aggregate veranda " + off: len(veranda.Schedule) / 2,
			"ceiling-living/light " + on: len(living.Schedule) / 2, "ceiling-living/light " + off: len(living.Schedule) / 2,
		} {
			if count[cmd] != want {
				t.Errorf("%s: %d × %s, want %d", name, count[cmd], cmd, want)
			}
		}
		next := presenceOf(t, e, id, "veranda")
		checkBlocks(t, name+": the next day", next.Schedule, at(16, 18, 0), at(16, 21, 0), 4, 6)
	}
}

func TestPresenceWindowWrapsMidnight(t *testing.T) {
	for _, c := range []struct {
		name     string
		enabled  time.Time
		from, to time.Time
	}{
		{"before it", at(15, 12, 0), at(15, 22, 0), at(16, 2, 0)},
		{"after midnight, inside it", at(16, 1, 0), at(16, 1, 0), at(16, 2, 0)},
	} {
		f, e, _ := timed(c.enabled)
		id := create(t, e, simulated("22:00", "02:00"))
		holidayStarts(e, f)
		s := presenceOf(t, e, id, "sim")
		checkBlocks(t, c.name, s.Schedule, c.from, c.to, 1, 6)
		if !s.WindowEnd.Equal(c.to) {
			t.Errorf("%s: window ends %v", c.name, s.WindowEnd)
		}
	}
}

func TestDisablingMidBlockFiresOffOnce(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	id := create(t, e, simulated("18:00", "21:00"))
	holidayStarts(e, f)
	first := presenceOf(t, e, id, "sim").Schedule[0].At
	c.advance(e, first.Add(time.Minute))
	expect(t, f, "the first block", lightsOn)

	set(e, f, home.ValueChanged, holidayRef, false)
	expect(t, f, "disabled mid-block", lightsOff)
	if got, want := path(traces[len(traces)-1]), []string{"holiday-off [out]", "sim [off] disable", "lights-off [then] c2"}; !slices.Equal(got, want) {
		t.Errorf("path:\n got  %q\n want %q", got, want)
	}
	if s := presenceOf(t, e, id, "sim"); s.Step != "" {
		t.Errorf("state when disabled: %+v", s)
	}
	c.advance(e, at(16, 23, 0))
	expect(t, f, "disabled")

	set(e, f, home.ValueChanged, holidayRef, true)
	set(e, f, home.ValueChanged, holidayRef, false)
	expect(t, f, "disabled outside the window")
}

func TestDisablingMidBlockAfterMovingTheWindowFiresOff(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, simulated("18:00", "21:00"))
	holidayStarts(e, f)
	first := presenceOf(t, e, id, "sim").Schedule[0].At
	c.advance(e, first.Add(time.Minute))
	expect(t, f, "the first block", lightsOn)

	if err := e.Replace(id, simulated("22:00", "02:00")); err != nil { // keeps the schedule drawn
		t.Fatal(err)
	}
	set(e, f, home.ValueChanged, holidayRef, false)
	expect(t, f, "disabled mid-block", lightsOff)
}

func TestRestartMidBlockFiresTheLatestOverdueSwitchOnly(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	id := create(t, e, simulated("18:00", "21:00"))
	holidayStarts(e, f)
	s := presenceOf(t, e, id, "sim").Schedule

	// Down from 12:00 to the middle of the second block.
	back := s[2].At.Add(s[3].At.Sub(s[2].At) / 2)
	e = restart(t, e, f, c, back)
	if d := presenceOf(t, e, id, "sim").Deadline; !d.Equal(s[2].At) {
		t.Errorf("due at %v, want at the latest overdue switch %v, in order with other overdue work", d, s[2].At)
	}
	e.record = keep(&traces)
	e.start()
	c.advance(e, back)
	expect(t, f, "at startup", lightsOn)
	tr := traces[len(traces)-1]
	if !tr.Trigger.CatchUp || tr.Trigger.Skipped != 2 || !tr.Trigger.Time.Equal(s[2].At) || !tr.Time.Equal(back) {
		t.Errorf("trigger %+v at %v: want a catch-up of %v at %v, 2 skipped", tr.Trigger, tr.Time, s[2].At, back)
	}
	if got := presenceOf(t, e, id, "sim").Schedule; !slices.EqualFunc(got, s[3:], func(x, y Switch) bool { return x.On == y.On && x.At.Equal(y.At) }) {
		t.Errorf("schedule after the restart:\n got  %v\n want %v", got, s[3:])
	}

	c.advance(e, at(15, 21, 0))
	var want []string
	for _, sw := range s[3:] {
		want = append(want, map[bool]string{true: lightsOn, false: lightsOff}[sw.On])
	}
	expect(t, f, "the rest of the window", want...)
}

func TestRestartAfterAMissedDrawDrawsForTheRestOfTheWindow(t *testing.T) {
	for _, c := range []struct {
		name     string
		back     time.Time
		from, to time.Time // the window drawn at startup
	}{
		{"back inside the window", at(16, 19, 0), at(16, 19, 0), at(16, 21, 0)},
		{"back before the window", at(16, 10, 0), at(16, 18, 0), at(16, 21, 0)},
		{"back after the window", at(16, 23, 0), at(17, 18, 0), at(17, 21, 0)},
	} {
		f, e, clk := timed(at(15, 12, 0))
		var traces []*Trace
		id := create(t, e, simulated("18:00", "21:00"))
		holidayStarts(e, f)
		n := len(presenceOf(t, e, id, "sim").Schedule)

		e = restart(t, e, f, clk, c.back)
		e.record = keep(&traces)
		e.start()
		clk.advance(e, c.back)
		expect(t, f, c.name+": at startup, the 15th's last switch", lightsOff)
		if len(traces) != 1 || traces[0].Trigger.Skipped != n-1 {
			t.Errorf("%s: %d Runs, want one skipping %d switches", c.name, len(traces), n-1)
		}
		s := presenceOf(t, e, id, "sim")
		checkBlocks(t, c.name, s.Schedule, c.from, c.to, 1, 6)
		if !s.WindowEnd.Equal(c.to) {
			t.Errorf("%s: window ends %v", c.name, s.WindowEnd)
		}
	}
}
