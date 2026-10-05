package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nathan-osman/go-sunrise"

	"github.com/llehouerou/oiko/internal/home"
)

var (
	sast  = time.FixedZone("SAST", 2*3600)
	place = Place{Latitude: -26.2041, Longitude: 28.0473} // Johannesburg
)

// at is a time of June 2026 in Johannesburg; 15 June is a Monday.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.June, day, hour, minute, 0, 0, sast)
}

// clock is a fake clock.
type clock struct{ t time.Time }

// timed returns a fixture and its engine, on a fake clock reading start.
func timed(start time.Time) (*fakeHome, *Engine, *clock) {
	f := fixture()
	e := New(f, nil, nil, &place, nil, nil, nil)
	c := &clock{start}
	e.now = func() time.Time { return c.t }
	return f, e, c
}

// advance moves c on to t, stopping at each deadline on the way to run what
// comes due then, as the engine goroutine would.
func (c *clock) advance(e *Engine, t time.Time) {
	for {
		e.mu.Lock()
		next := e.tick()
		e.mu.Unlock()
		if next.IsZero() || next.After(t) {
			break
		}
		c.t = next
	}
	c.t = t
}

func stateOf(t *testing.T, e *Engine, id string) []string {
	t.Helper()
	list, err := e.State(id)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, s := range list {
		line := s.Step
		if !s.Deadline.IsZero() {
			line += " due " + s.Deadline.Format("15:04:05")
		}
		var passed []string
		for _, p := range s.Passed {
			passed = append(passed, fmt.Sprintf(" passed %q at %s", p.Target.Device(), p.At.Format("15:04:05")))
		}
		line += strings.Join(passed, "")
		lines = append(lines, line)
	}
	return lines
}

// expect checks the Commands issued since the last call.
func expect(t *testing.T, f *fakeHome, when string, want ...string) {
	t.Helper()
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("%s:\n got  %q\n want %q", when, got, want)
	}
}

func TestTimeWindowWrapsMidnight(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	create(t, e, doc("window", []string{pressSingle,
		`night timeWindow {"from": "21:00", "to": "06:00"}`,
		cmd("yes", `"flag:yes"`), cmd("no", `"flag:no"`)},
		"single.out night.in", "night.true yes.in", "night.false no.in"))
	for _, x := range []struct {
		t    time.Time
		want string
	}{
		{at(15, 20, 59), "no"},
		{at(15, 21, 0), "yes"},
		{at(15, 23, 0), "yes"},
		{at(16, 5, 59), "yes"},
		{at(16, 6, 0), "no"},
	} {
		c.t = x.t
		press(e, f, "button-office", "single")
		expect(t, f, x.t.Format("15:04"), "flag "+x.want+" map[on:true] 0s")
	}
}

func TestNextOccurrence(t *testing.T) {
	sunset := func(day int) time.Time {
		_, s := sunrise.SunriseSunset(place.Latitude, place.Longitude, 2026, time.June, day)
		return s
	}
	for _, c := range []struct {
		name, kind, params string
		from, want         time.Time
	}{
		{"later today", timeTrigger, `{"at": "06:00"}`, at(15, 5, 0), at(15, 6, 0)},
		{"tomorrow", timeTrigger, `{"at": "06:00"}`, at(15, 6, 0), at(16, 6, 0)},
		{"next Monday", timeTrigger, `{"at": "06:00:30", "weekdays": ["mon"]}`, at(15, 7, 0), at(22, 6, 0).Add(30 * time.Second)},
		{"sunset minus 30 min on Saturdays", sunTrigger, `{"event": "sunset", "offset": -1800, "weekdays": ["sat"]}`, at(15, 12, 0), sunset(20).Add(-30 * time.Minute)},
		{"yesterday's sunset plus 8 h", sunTrigger, `{"event": "sunset", "offset": 28800}`, at(16, 1, 0), sunset(15).Add(8 * time.Hour)},
	} {
		var p dailyParams
		if err := json.Unmarshal([]byte(c.params), &p); err != nil {
			t.Fatal(err)
		}
		var d daily
		if err := d.parse(c.kind, p); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := d.next(c.from, &place); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFlow3bFollowsTheSun(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	create(t, e, decoded(t, "flow3b-vacuum.json"))
	_, dusk := sunrise.TimeOfElevation(place.Latitude, place.Longitude, -6, 2026, time.June, 15)
	dawn, _ := sunrise.TimeOfElevation(place.Latitude, place.Longitude, -6, 2026, time.June, 16)
	if dusk.Before(at(15, 17, 30)) || dusk.After(at(15, 19, 0)) || dawn.Before(at(16, 5, 30)) || dawn.After(at(16, 7, 0)) {
		t.Fatalf("dusk %v, dawn %v", dusk.In(sast), dawn.In(sast))
	}

	c.advance(e, dusk.Add(-time.Second))
	expect(t, f, "before dusk")
	c.advance(e, dusk)
	expect(t, f, "dusk", "plug-vacuum-charger/switch map[state:false] 0s")
	c.advance(e, dawn)
	expect(t, f, "dawn", "plug-vacuum-charger/switch map[state:true] 0s")
	if tr := traces[0].Trigger; tr.Kind != sunTrigger || !tr.Time.Equal(dusk) {
		t.Errorf("trigger = %+v", tr)
	}
}

func TestFlow5bEndsLateEveningEveryMorning(t *testing.T) {
	f, e, c := timed(at(15, 22, 0))
	create(t, e, decoded(t, "flow5b-late-evening.json"))
	c.advance(e, at(16, 5, 59))
	expect(t, f, "05:59")
	c.advance(e, at(16, 6, 0))
	expect(t, f, "06:00", "flag late-evening map[on:false] 0s")
	c.advance(e, at(17, 6, 0))
	expect(t, f, "the next day", "flag late-evening map[on:false] 0s")
	c.t = at(20, 12, 0) // the host slept, or its clock jumped
	c.advance(e, at(20, 12, 0))
	expect(t, f, "three missed mornings, not caught up", "flag late-evening map[on:false] 0s")
	c.advance(e, at(21, 6, 0))
	expect(t, f, "the next morning", "flag late-evening map[on:false] 0s")
}

func TestFlow6bSirenHeldFor5Minutes(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, decoded(t, "flow6b-siren.json"))
	siren := home.TargetDevice("alarm", "alarm").Ref("alarm")
	set(e, f, home.ValueChanged, siren, true) // replayed: starts nothing
	c.advance(e, at(15, 12, 10))
	expect(t, f, "replayed on")

	set(e, f, home.ValueChanged, siren, false)
	set(e, f, home.ValueChanged, siren, true)
	if got := stateOf(t, e, id); !slices.Equal(got, []string{"ringing due 12:15:00"}) {
		t.Errorf("state = %q", got)
	}
	c.advance(e, at(15, 12, 15).Add(-time.Second))
	expect(t, f, "held 4:59")
	c.advance(e, at(15, 12, 15))
	expect(t, f, "held 5 min", "alarm/alarm map[alarm:false] 0s")

	set(e, f, home.ValueChanged, siren, false)
	c.advance(e, at(15, 13, 0))
	set(e, f, home.ValueChanged, siren, true)
	c.advance(e, at(15, 13, 2))
	set(e, f, home.ValueChanged, siren, false) // a change away
	if got := stateOf(t, e, id); got != nil {
		t.Errorf("state after a change away = %q", got)
	}
	c.advance(e, at(15, 13, 10))
	expect(t, f, "cancelled by a change away")
	set(e, f, home.ValueChanged, siren, true)
	set(e, f, home.ValueChanged, siren, nil) // unknown
	c.advance(e, at(15, 13, 30))
	expect(t, f, "cancelled by unknown")
}

func TestFlow6cNightMode(t *testing.T) {
	f, e, c := timed(at(15, 20, 0))
	var traces []*Trace
	e.record = keep(&traces)
	create(t, e, decoded(t, "flow6c-night-mode.json"))
	lights := home.TargetAggregate("all-but-veranda").Ref("state")
	guest := home.TargetFlag("guest").Ref("on")
	set(e, f, home.ValueChanged, guest, false)
	set(e, f, home.ValueChanged, lights, true)
	lightsAt := func(t time.Time, on bool) {
		c.advance(e, t)
		set(e, f, home.ValueChanged, lights, on)
	}

	lightsAt(at(15, 20, 30), false) // outside the window
	c.advance(e, at(15, 21, 30))
	expect(t, f, "off at 20:30")

	lightsAt(at(15, 22, 0), true)
	expect(t, f, "on at 22:00", "flag night map[on:false] 0s")
	lightsAt(at(15, 23, 0), false)
	c.advance(e, at(15, 23, 15))
	expect(t, f, "off at 23:00", "flag night map[on:true] 0s")

	lightsAt(at(15, 23, 20), true)
	lightsAt(at(15, 23, 30), false)
	lightsAt(at(15, 23, 40), true) // cancels the timer
	c.advance(e, at(15, 23, 50))
	expect(t, f, "off for 10 min", "flag night map[on:false] 0s", "flag night map[on:false] 0s")

	lightsAt(at(16, 5, 50), false)
	c.advance(e, at(16, 6, 30))
	expect(t, f, "off at 05:50: 06:00 resets, and 06:05 is not night", "flag night map[on:false] 0s")
	last := traces[len(traces)-1]
	if got, want := path(last), []string{"delay [fired]", "still-night [false]"}; !slices.Equal(got, want) {
		t.Errorf("06:05 path = %q, want %q", got, want)
	}
}

func TestFlow5aVerandaLatestMotionWins(t *testing.T) {
	f, e, c := timed(at(15, 20, 0))
	var traces []*Trace
	e.record = keep(&traces)
	id := create(t, e, decoded(t, "flow5a-veranda.json"))
	motion := home.TargetAggregate("motion-veranda").Ref("occupancy")
	lit := home.TargetAggregate("veranda").Ref("state")
	for ref, v := range map[home.Ref]any{
		motion: false, lit: false, luxRef: 50.0,
		home.TargetFlag("late-evening").Ref("on"): false, home.TargetFlag("guest").Ref("on"): false,
	} {
		set(e, f, home.ValueChanged, ref, v)
	}
	moveAt := func(t time.Time) {
		c.advance(e, t)
		set(e, f, home.ValueChanged, motion, true)
		set(e, f, home.ValueChanged, motion, false)
	}
	const (
		on  = "map[brightness:254 color_temp:200 state:true] 500ms"
		all = "aggregate veranda "
	)

	moveAt(at(15, 20, 59))
	expect(t, f, "20:59: all", all+on)
	set(e, f, home.ValueChanged, lit, true)
	set(e, f, home.ValueChanged, luxRef, 150.0) // the lamps' own light
	moveAt(at(15, 21, 5))
	expect(t, f, "21:05, lit: partial despite the lux", "veranda-1/light "+on, "veranda-2/light "+on)
	if got, want := path(traces[len(traces)-1]), []string{
		"motion [out]", "lit [true] read true", "late-evening [false] read false", "guest [false] read false", "day [false]",
		"partial-on [then] c2 c3", "short [] start until 21:10:00", "long [] cancel",
	}; !slices.Equal(got, want) {
		t.Errorf("21:05 path:\n got  %q\n want %q", got, want)
	}
	if got := stateOf(t, e, id); !slices.Equal(got, []string{"short due 21:10:00"}) {
		t.Errorf("state = %q", got)
	}
	c.advance(e, at(15, 21, 30))
	expect(t, f, "5 min later, the 15-min timer cancelled", all+"map[state:false] 1m0s")

	set(e, f, home.ValueChanged, lit, false)
	moveAt(at(15, 21, 40))
	expect(t, f, "unlit and bright")

	set(e, f, home.ValueChanged, luxRef, 50.0)
	moveAt(at(15, 22, 0))
	moveAt(at(15, 22, 3)) // restarts the timer
	expect(t, f, "night, dark", "veranda-1/light "+on, "veranda-2/light "+on, "veranda-1/light "+on, "veranda-2/light "+on)
	set(e, f, home.ValueChanged, home.TargetFlag("guest").Ref("on"), true)
	moveAt(at(15, 22, 6))
	expect(t, f, "a guest: all", all+on)
	c.advance(e, at(15, 22, 20))
	expect(t, f, "22:20")
	c.advance(e, at(15, 22, 21))
	expect(t, f, "15 min after the last motion", all+"map[state:false] 15m0s")
}

// delayed is a press that starts a timer, which turns Flag a on when fired.
func delayed(duration int) Document {
	return doc("delayed", []string{pressSingle,
		fmt.Sprintf(`delay timer {"duration": %d, "reentry": "keep"}`, duration),
		cmd("act", `"flag:a"`)},
		"single.out delay.start", "delay.fired act.in")
}

func TestTimerTrace(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	create(t, e, doc("timer", []string{pressSingle,
		`long eventTrigger {"target": "device:button-office/button", "capability": "action", "events": ["long"]}`,
		`keep timer {"duration": 600, "reentry": "keep"}`,
		`restart timer {"duration": 600, "reentry": "restart"}`},
		"single.out keep.start", "single.out restart.start", "long.out keep.cancel", "long.out restart.cancel"))
	press(e, f, "button-office", "single")
	c.advance(e, at(15, 12, 5))
	press(e, f, "button-office", "single")
	press(e, f, "button-office", "long")
	press(e, f, "button-office", "long")
	for i, want := range [][]string{
		{"single [out]", "keep [] start until 12:10:00", "restart [] start until 12:10:00"},
		{"single [out]", "keep [] keep until 12:10:00", "restart [] restart until 12:15:00"},
		{"long [out]", "keep [] cancel", "restart [] cancel"},
		{"long [out]", "keep [] cancel", "restart [] cancel"},
	} {
		if got := path(traces[i]); !slices.Equal(got, want) {
			t.Errorf("Run %d:\n got  %q\n want %q", i, got, want)
		}
	}
	c.advance(e, at(15, 13, 0))
	if len(traces) != 4 {
		t.Errorf("%d Runs after cancelling", len(traces))
	}
}

func TestHotReloadKeepsATimerOfTheSameIdAndKind(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, delayed(900))
	press(e, f, "button-office", "single")

	if err := e.Replace(id, delayed(60)); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, e, id); !slices.Equal(got, []string{"delay due 12:15:00"}) {
		t.Errorf("state after editing the duration = %q", got)
	}
	c.advance(e, at(15, 12, 14))
	expect(t, f, "the new duration from the old start")
	c.advance(e, at(15, 12, 15))
	expect(t, f, "the old deadline", "flag a map[on:true] 0s")
	press(e, f, "button-office", "single")
	if got := stateOf(t, e, id); !slices.Equal(got, []string{"delay due 12:16:00"}) {
		t.Errorf("state after the next start = %q", got)
	}

	cooled := doc("delayed", []string{pressSingle, `delay cooldown {"duration": 60}`, cmd("act", `"flag:a"`)},
		"single.out delay.in", "delay.out act.in")
	if err := e.Replace(id, cooled); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, e, id); got != nil {
		t.Errorf("state after changing the kind = %q", got)
	}
	c.advance(e, at(15, 12, 30))
	expect(t, f, "the timer dropped")
}

func TestDisablingCancelsTimersWithoutACommand(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, delayed(60))
	press(e, f, "button-office", "single")
	d := delayed(60)
	d.Enabled = false
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, e, id); got != nil {
		t.Errorf("state when disabled = %q", got)
	}
	c.advance(e, at(15, 12, 5))
	expect(t, f, "disabled")

	if err := e.Replace(id, delayed(60)); err != nil {
		t.Fatal(err)
	}
	press(e, f, "button-office", "single")
	f.emit(e, home.Update{Kind: home.FlagsChanged, Flags: []home.Flag{{ID: "night"}}}) // a is deleted
	f.emit(e, home.Update{Kind: home.FlagsChanged, Flags: f.snap.Flags})
	if got := stateOf(t, e, id); got != nil {
		t.Errorf("state once broken = %q", got)
	}
	c.advance(e, at(15, 12, 10))
	expect(t, f, "broken for a while")
	if len(e.pending) != 0 {
		t.Errorf("%d deadlines pending", len(e.pending))
	}
}

func TestCooldown(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	id := create(t, e, doc("cooldown", []string{pressSingle,
		`alice eventTrigger {"target": "device:button-alice/button", "capability": "action", "events": ["single"]}`,
		`once cooldown {"duration": 30}`, `each cooldown {"duration": 30, "perTarget": true}`,
		cmd("once-a", `"flag:a"`), cmd("each-b", `"flag:b"`)},
		"single.out once.in", "alice.out once.in", "single.out each.in", "alice.out each.in",
		"once.out once-a.in", "each.out each-b.in"))

	press(e, f, "button-office", "single")
	press(e, f, "button-alice", "single")
	expect(t, f, "12:00", "flag a map[on:true] 0s", "flag b map[on:true] 0s", "flag b map[on:true] 0s")
	if got, want := path(traces[1]), []string{"alice [out]", "once [] block until 12:00:30", "each [out] pass until 12:00:30", "each-b [then] c3"}; !slices.Equal(got, want) {
		t.Errorf("path:\n got  %q\n want %q", got, want)
	}
	c.advance(e, at(15, 12, 0).Add(29*time.Second))
	press(e, f, "button-office", "single")
	expect(t, f, "12:00:29")
	if got, want := stateOf(t, e, id), []string{
		`once passed "" at 12:00:00`,
		`each passed "button-alice" at 12:00:00 passed "button-office" at 12:00:00`,
	}; !slices.Equal(got, want) {
		t.Errorf("state:\n got  %q\n want %q", got, want)
	}
	c.advance(e, at(15, 12, 0).Add(30*time.Second))
	press(e, f, "button-office", "single")
	expect(t, f, "12:00:30", "flag a map[on:true] 0s", "flag b map[on:true] 0s")
}

func TestIdleEngineNeverWakesAndATimeTriggerWakesIt(t *testing.T) {
	c, lamp, _ := realHome(t)
	var reads atomic.Int64
	e := New(c, copies(t, "flow1-bedroom-alice.json", 10), nil, nil, nil, nil, nil)
	e.now = func() time.Time { reads.Add(1); return time.Now() }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	if n := reads.Load(); n != 1 { // when the clock started
		t.Errorf("idle engine read the clock %d times", n-1)
	}
	soon := time.Now().Add(1500 * time.Millisecond).Truncate(time.Second)
	create(t, e, doc("soon", []string{fmt.Sprintf(`soon timeTrigger {"at": %q}`, soon.Format("15:04:05")), lampOn(lamp)}, "soon.out act.in"))
	for deadline := time.Now().Add(5 * time.Second); c.n.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if c.n.Load() != 1 {
		t.Fatalf("%d Commands at %s", c.n.Load(), soon.Format("15:04:05"))
	}
	time.Sleep(50 * time.Millisecond)
	n := reads.Load()
	time.Sleep(200 * time.Millisecond)
	if reads.Load() != n {
		t.Errorf("the engine woke, waiting for tomorrow")
	}
}
