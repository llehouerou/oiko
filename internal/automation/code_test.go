package automation

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// coded is a Code Step "id code params", as doc takes it.
func coded(id, source string, outputs []string, bindings map[string]home.Target) string {
	p, err := json.Marshal(map[string]any{"source": source, "outputs": outputs, "bindings": bindings})
	if err != nil {
		panic(err)
	}
	return id + " code " + string(p)
}

// codeState returns the Code state of Step step of Automation id, in JSON.
func codeState(t *testing.T, e *Engine, id, step string) string {
	t.Helper()
	list, err := e.State(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.Step == step {
			return string(s.State)
		}
	}
	return ""
}

func door(d home.DeviceID) home.Ref {
	return home.TargetDevice(d, "contact").Ref("contact")
}

func TestCodeCooldownPerDoorBehavesLikeTheCooldownStep(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	opened := func(d home.DeviceID) string {
		return string(d) + ` valueTrigger {"target": "device:` + string(d) + `/contact", "capability": "contact", "op": "eq", "value": false}`
	}
	create(t, e, doc("doors", []string{opened("door-entrance"), opened("door-kitchen"),
		`each cooldown {"duration": 30, "perTarget": true}`,
		coded("code", `
def run(trigger, state):
    last = state.get(trigger.key)
    if last != None and now.unix < last + 30:
        return None
    state[trigger.key] = now.unix
    return "out"
`, []string{"out"}, nil),
		cmd("each-a", `"flag:a"`), cmd("code-b", `"flag:b"`)},
		"door-entrance.out each.in", "door-kitchen.out each.in", "door-entrance.out code.run", "door-kitchen.out code.run",
		"each.out each-a.in", "code.out code-b.in"))
	for _, d := range []home.DeviceID{"door-entrance", "door-kitchen"} {
		set(e, f, home.ValueChanged, door(d), true) // closed, a first Value
	}

	both := []string{"flag a map[on:true] 0s", "flag b map[on:true] 0s"}
	for _, x := range []struct {
		door home.DeviceID
		t    time.Time
		pass bool
	}{
		{"door-entrance", at(15, 12, 0), true},
		{"door-kitchen", at(15, 12, 0).Add(10 * time.Second), true},
		{"door-entrance", at(15, 12, 0).Add(29 * time.Second), false},
		{"door-entrance", at(15, 12, 0).Add(30 * time.Second), true},
		{"door-kitchen", at(15, 12, 0).Add(39 * time.Second), false},
		{"door-kitchen", at(15, 12, 0).Add(40 * time.Second), true},
	} {
		c.advance(e, x.t)
		set(e, f, home.ValueChanged, door(x.door), false)
		set(e, f, home.ValueChanged, door(x.door), true)
		var want []string
		if x.pass {
			want = both
		}
		expect(t, f, string(x.door)+" at "+x.t.Format("15:04:05"), want...)
	}
}

func TestCodeBuiltins(t *testing.T) {
	f, e, _ := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	id := create(t, e, doc("builtins", []string{
		`dark valueTrigger {"target": "device:lux-sensor/illuminance", "capability": "illuminance", "op": "lt", "value": 100}`,
		coded("code", `
def run(trigger, state):
    print(trigger.kind, trigger.source, trigger.capability, trigger.value, trigger.event, type(trigger.time))
    print(value("lux", "illuminance"), value("night", "on"), now.hour, time.now() == now, math.floor(2.5))
    command("lamp", 0.5, brightness=254, state=True)
    command("night", transition=2, on="toggle")
    return ["b", "a"]
`, []string{"a", "b"}, map[string]home.Target{
			"lux":   home.TargetDevice("lux-sensor", "illuminance"),
			"lamp":  home.TargetDevice("lamp-office", "light"),
			"night": home.TargetFlag("night"),
		}),
		cmd("a", `"flag:a"`), cmd("b", `"flag:b"`)},
		"dark.out code.run", "code.a a.in", "code.b b.in"))
	set(e, f, home.ValueChanged, luxRef, 150.0)
	set(e, f, home.ValueChanged, luxRef, 50.0)

	expect(t, f, "the Run", "lamp-office/light map[brightness:254 state:true] 500ms", "flag night map[on:toggle] 2s",
		"flag b map[on:true] 0s", "flag a map[on:true] 0s")
	if want := (home.Origin{Automation: id, Step: "code", Run: traces[0].Run}); f.origins[0] != want || f.origins[1] != want {
		t.Errorf("origins %+v, want %+v", f.origins[:2], want)
	}
	r := traces[0].Steps[1]
	if want := "value lux illuminance 50.0 None time.time\n50.0 None 12 True 2\n"; r.Print != want || r.Error != "" {
		t.Errorf("print %q, error %q; want print %q", r.Print, r.Error, want)
	}
	if got, want := path(traces[0]), []string{"dark [out]", "code [b a] c1 c2", "b [then] c3", "a [then] c4"}; !slices.Equal(got, want) {
		t.Errorf("path:\n got  %q\n want %q", got, want)
	}
}

func TestCodeSeesATimeTrigger(t *testing.T) {
	_, e, c := timed(at(15, 12, 0))
	var traces []*Trace
	e.record = keep(&traces)
	create(t, e, doc("at six", []string{`six timeTrigger {"at": "06:00"}`, coded("code", `
def run(trigger, state):
    print(trigger.kind, trigger.source, trigger.key, trigger.capability, trigger.value, trigger.time == now, now.hour)
`, nil, nil)}, "six.out code.run"))
	c.advance(e, at(16, 6, 0))
	if got, want := traces[0].Steps[1].Print, "time None None None None True 6\n"; got != want {
		t.Errorf("print %q, want %q", got, want)
	}
}

func TestCodeFailuresAreAllOrNothing(t *testing.T) {
	for _, c := range []struct{ name, fail, err string }{
		{"exception", "1 // 0", "division by zero"},
		{"infinite loop", "for i in range(1000000000):\n        pass", "too many steps"},
		{"undeclared handle", `return "nope"`, `undeclared handle "nope"`},
		{"unknown alias", `value(trigger.kind, "on")`, `unknown alias "event"`},
		{"non-JSON state", "state[\"f\"] = run", "function"},
		{"state too big", "state[\"big\"] = \"x\" * 16384", "16 KiB"},
		{"non-string key", "state[1] = 1", "key"},
		{"bad command value", `command("a", on=run)`, "function"},
		{"negative transition", `command("a", -1, on=True)`, "transition"},
		{"NaN transition", `command("a", float("nan"), on=True)`, "transition"},
		{"transition twice", `command("a", 1, transition=2, on=True)`, "transition"},
		{"invalid UTF-8", `state["s"] = "é"[0]`, "UTF-8"},
	} {
		f := fixture()
		var traces []*Trace
		e := New(f, nil, nil, keep(&traces))
		a := map[string]home.Target{"a": home.TargetFlag("a"), "b": home.TargetFlag("b")}
		id := create(t, e, doc("fails", []string{pressSingle,
			coded("fails", "def run(trigger, state):\n    command(\"a\", on=True)\n    state[\"n\"] = 1\n    "+c.fail+"\n    return \"out\"", []string{"out"}, a),
			coded("next", "def run(trigger, state):\n    command(\"b\", on=True)", nil, a),
			cmd("out", `"flag:guest"`)},
			"single.out fails.run", "single.out next.run", "fails.out out.in"))
		press(e, f, "button-office", "single")

		expect(t, f, c.name, "flag b map[on:true] 0s") // the rest of the Run continues
		if s := codeState(t, e, id, "fails"); s != "" {
			t.Errorf("%s: state %s", c.name, s)
		}
		if r := traces[0].Steps[1]; !strings.Contains(r.Error, c.err) || r.Fired != nil || traces[0].Outcome != home.RunError {
			t.Errorf("%s: error %q, fired %q, outcome %s; want an error with %q", c.name, r.Error, r.Fired, traces[0].Outcome, c.err)
		}
		if s := statusOf(f, id); s.Status != home.AutomationEnabled {
			t.Errorf("%s: status %+v", c.name, s)
		}
	}
}

// counter is a Code Step counting its Runs in its state, and failing on a
// double press.
var counter = coded("count", `
def run(trigger, state):
    state["n"] = state.get("n", 0) + 1
    if trigger.event == "double":
        state["big"] = "x" * 20000
`, nil, nil)

func counting() Document {
	return doc("counter", []string{
		`press eventTrigger {"target": "device:button-office/button", "capability": "action", "events": ["single", "double"]}`,
		counter}, "press.out count.run")
}

func TestCodeStateSurvivesARestartAndASourceEdit(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, counting())
	press(e, f, "button-office", "single")
	press(e, f, "button-office", "single")
	press(e, f, "button-office", "double") // rejected: the previous state is kept
	if s := codeState(t, e, id, "count"); s != `{"n":2}` {
		t.Fatalf("state %s", s)
	}

	e = restart(t, e, f, c, at(15, 13, 0))
	press(e, f, "button-office", "single")
	if s := codeState(t, e, id, "count"); s != `{"n":3}` {
		t.Errorf("state after a restart %s", s)
	}

	d := counting()
	d.Steps[1] = Step{ID: "count", Kind: code, Params: json.RawMessage(`{"source": "def run(trigger, state):\n  state[\"n\"] += 10"}`)}
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	press(e, f, "button-office", "single")
	if s := codeState(t, e, id, "count"); s != `{"n":13}` {
		t.Errorf("state after a source edit %s", s)
	}
}

func TestCodeStateKeepsIntsAndFloats(t *testing.T) {
	f, e, c := timed(at(15, 12, 0))
	id := create(t, e, doc("types", []string{pressSingle, coded("c", `
def run(trigger, state):
    if not state:
        state.update(i=1, f=1.0, big=10000000000000000000000, l=[None, True, "é"], d={"x": 0.5})
    else:
        print(type(state["i"]), type(state["f"]), state["big"], state["l"], state["d"])
`, nil, nil)}, "single.out c.run"))
	press(e, f, "button-office", "single")
	want := `{"big":10000000000000000000000,"d":{"x":0.5},"f":1.0,"i":1,"l":[null,true,"é"]}`
	if s := codeState(t, e, id, "c"); s != want {
		t.Errorf("state %s, want %s", s, want)
	}
	var traces []*Trace
	e = restart(t, e, f, c, at(15, 13, 0))
	e.record = keep(&traces)
	press(e, f, "button-office", "single")
	if got, want := traces[0].Steps[1].Print, "int float 10000000000000000000000 [None, True, \"é\"] {\"x\": 0.5}\n"; got != want {
		t.Errorf("print %q, want %q", got, want)
	}
}

func TestCodePrintIsCapped(t *testing.T) {
	f := fixture()
	var traces []*Trace
	e := New(f, nil, nil, keep(&traces))
	create(t, e, doc("chatty", []string{pressSingle, coded("c", `
def run(trigger, state):
    for i in range(1000):
        print("0123456789")
`, nil, nil)}, "single.out c.run"))
	press(e, f, "button-office", "single")
	if n := len(traces[0].Steps[1].Print); n < maxPrint || n > maxPrint+16 {
		t.Errorf("%d bytes printed into the Trace", n)
	}
}

// handler is the representative handler of the runtime benchmark: it reads a
// Value, computes a state and a brightness, and issues a Command.
const handler = `
def run(trigger, state):
    lux = value("lux", "illuminance")
    on = trigger.value == True and lux < 100
    command("light", transition=2, state=on, brightness=min(254, int((100 - lux) * 2.54 + 0.5)) if on else 0)
`

// handled is a quiet home whose engine runs handler on motion, and a report of
// motion turning on or off by turns.
func handled(t testing.TB) (*quietHome, func()) {
	t.Helper()
	q := &quietHome{fakeHome: *fixture()}
	e := New(q, []Document{doc("handler", []string{
		`moving valueTrigger {"target": "aggregate:motion-veranda", "capability": "occupancy", "op": "eq", "value": true}`,
		`still valueTrigger {"target": "aggregate:motion-veranda", "capability": "occupancy", "op": "eq", "value": false}`,
		coded("handler", handler, nil, map[string]home.Target{
			"lux":   home.TargetDevice("lux-sensor", "illuminance"),
			"light": home.TargetAggregate("veranda"),
		})}, "moving.out handler.run", "still.out handler.run")}, nil, nil)

	motion := home.TargetAggregate("motion-veranda").Ref("occupancy")
	q.deliver(home.Update{Kind: home.ValueChanged, Ref: &luxRef, Value: &home.Value{Data: 42.0}})
	q.deliver(home.Update{Kind: home.ValueChanged, Ref: &motion, Value: &home.Value{Data: false}})
	settle(e)
	e.now = spaced()
	moves := []home.Update{
		{Kind: home.ValueChanged, Ref: &motion, Value: &home.Value{Data: true}},
		{Kind: home.ValueChanged, Ref: &motion, Value: &home.Value{Data: false}},
	}
	i := 0
	return q, func() {
		q.deliver(moves[i%2])
		e.drain()
		i++
	}
}

func TestCodeStepCallAllocations(t *testing.T) {
	q, move := handled(t)
	move()
	if allocs := testing.AllocsPerRun(1000, move); allocs > 60 {
		t.Errorf("%v allocations per call, want at most 60", allocs)
	}
	if q.n != 1002 {
		t.Errorf("%d Commands for 1002 calls", q.n)
	}
}

func BenchmarkCodeStep(b *testing.B) {
	_, move := handled(b)
	move()
	b.ReportAllocs()
	for b.Loop() {
		move()
	}
}
