package automation

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// fakeHome records the Commands it is given; its Updates are whatever the
// test emits.
type fakeHome struct {
	snap     home.Snapshot
	deliver  func(home.Update)
	commands []string
	origins  []home.Origin
	refuse   map[home.Target]bool
	status   []home.AutomationStatus
	runs     []home.RunEnd
	known    chan struct{} // closed once the home is known; nil: known already
}

func (f *fakeHome) Known() <-chan struct{} {
	if f.known == nil {
		known := make(chan struct{})
		close(known)
		return known
	}
	return f.known
}

func (f *fakeHome) Waiting() []string { return nil }

func (f *fakeHome) Follow(deliver func(home.Update)) (home.Snapshot, func()) {
	f.deliver = deliver
	return f.snap, func() {}
}

func (f *fakeHome) Command(t home.Target, req home.Request) (string, error) {
	if f.refuse[t] {
		return "", fmt.Errorf("%w: refused", home.ErrInvalid)
	}
	name := string(t.Device()) + "/" + t.Function()
	switch {
	case t.Aggregate() != "":
		name = "aggregate " + string(t.Aggregate())
	case t.Flag() != "":
		name = "flag " + string(t.Flag())
	}
	f.commands = append(f.commands, fmt.Sprint(name, " ", req.Values, " ", req.Transition))
	f.origins = append(f.origins, req.Origin)
	return fmt.Sprint("c", len(f.origins)), nil
}

func (f *fakeHome) SetAutomationStatus(list []home.AutomationStatus) { f.status = list }

func (f *fakeHome) EndRun(r home.RunEnd) { f.runs = append(f.runs, r) }

// keep returns a record func for New that keeps every Trace in *traces.
func keep(traces *[]*Trace) func(*Trace) {
	return func(t *Trace) { *traces = append(*traces, t) }
}

// path sums up the Steps of t, one line each: its id, the handles it fired
// and the evidence.
func path(t *Trace) []string {
	var lines []string
	for _, r := range t.Steps {
		line := fmt.Sprint(r.Step, " ", r.Fired)
		switch {
		case r.Unknown:
			line += " unknown"
		case r.Read != nil:
			line += fmt.Sprint(" read ", r.Read)
		}
		if r.Action != "" {
			line += " " + r.Action
		}
		if !r.Until.IsZero() {
			line += " until " + r.Until.Format("15:04:05")
		}
		for _, c := range r.Commands {
			line += " " + cmp.Or(c.ID, c.Refused)
		}
		lines = append(lines, line)
	}
	return lines
}

// take returns the Commands recorded since the last call.
func (f *fakeHome) take() []string {
	c := f.commands
	f.commands = nil
	return c
}

// fixture is a home with the Devices, Aggregates and Flags of the Node-RED
// flows, and a few more Flags.
func fixture() *fakeHome {
	f := &fakeHome{snap: home.Snapshot{}}
	for _, id := range []home.AggregateID{"bedroom-alice", "veranda", "motion-veranda", "all-but-veranda"} {
		f.snap.Aggregates = append(f.snap.Aggregates, home.Aggregate{ID: id})
	}
	for _, id := range []home.FlagID{"night", "guest", "late-evening", "holiday", "yes", "no", "a", "a2", "b", "first-a", "first-b", "second-a", "second-b"} {
		f.snap.Flags = append(f.snap.Flags, home.Flag{ID: id})
	}
	for id, fn := range map[home.DeviceID]string{
		"switch-alice": "button", "button-alice": "button", "bedside-alice": "light", "lamp-alice": "light", "ceiling-alice": "light",
		"switch-bob": "button", "lamp-bob": "light", "ceiling-bob": "light", "plug-floor-lamp-bob": "switch",
		"switch-parents": "button", "bedside-dave": "light", "bedside-carol": "light",
		"plug-vacuum-charger": "switch",
		"switch-living":       "button", "light-living": "light", "light-entrance": "light", "ceiling-entrance": "light", "ceiling-living": "light",
		"button-office": "button", "office-strip": "light", "lamp-office": "light", "ceiling-office": "light",
		"veranda-1": "light", "veranda-2": "light", "veranda-3": "light", "veranda-4": "light",
		"lux-sensor": "illuminance", "alarm": "alarm", "door-entrance": "contact", "door-kitchen": "contact",
	} {
		f.snap.Devices = append(f.snap.Devices, home.Device{ID: id, Functions: []home.Function{{Key: fn, Kind: fn}}})
	}
	return f
}

// emit hands u to the engine and lets it apply everything that follows.
func (f *fakeHome) emit(e *Engine, u home.Update) {
	f.deliver(u)
	settle(e)
}

func settle(e *Engine) {
	for e.drain() {
	}
}

func press(e *Engine, f *fakeHome, device home.DeviceID, event string) {
	f.emit(e, home.Update{Kind: home.EventOccurred,
		Ref:   &home.Ref{Target: home.TargetDevice(device, "button"), Capability: "action"},
		Value: &home.Value{Data: event, At: time.Now()}})
}

func set(e *Engine, f *fakeHome, kind home.UpdateKind, ref home.Ref, data any) {
	u := home.Update{Kind: kind, Ref: &ref}
	if data != nil {
		u.Value = &home.Value{Data: data, At: time.Now()}
	}
	f.emit(e, u)
}

var (
	nightRef = home.TargetFlag("night").Ref("on")
	luxRef   = home.TargetDevice("lux-sensor", "illuminance").Ref("illuminance")
)

// doc builds a Document from its Steps, given as "id kind params" with params
// in JSON, and its edges, given as "from.handle to.handle".
func doc(name string, steps []string, edges ...string) Document {
	d := Document{Name: name, Enabled: true}
	for _, s := range steps {
		f := strings.SplitN(s, " ", 3)
		d.Steps = append(d.Steps, Step{ID: f[0], Kind: f[1], Params: json.RawMessage(f[2])})
	}
	for _, e := range edges {
		from, to, _ := strings.Cut(e, " ")
		fs, fh, _ := strings.Cut(from, ".")
		ts, th, _ := strings.Cut(to, ".")
		d.Edges = append(d.Edges, Edge{From: Port{fs, fh}, To: Port{ts, th}})
	}
	return d
}

func create(t *testing.T, e *Engine, d Document) string {
	t.Helper()
	id, err := e.Create(d)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func statusOf(f *fakeHome, id string) home.AutomationStatus {
	i := slices.IndexFunc(f.status, func(s home.AutomationStatus) bool { return s.ID == id })
	if i < 0 {
		return home.AutomationStatus{}
	}
	return f.status[i]
}

const (
	pressSingle = `single eventTrigger {"target": "device:button-office/button", "capability": "action", "events": ["single"]}`
	nightOn     = `night-on valueTrigger {"target": "flag:night", "capability": "on", "op": "eq", "value": true}`
	luxLow      = `lux-low valueTrigger {"target": "device:lux-sensor/illuminance", "capability": "illuminance", "op": "lt", "value": 100}`
	isNight     = `is-night valueCondition {"target": "flag:night", "capability": "on", "op": "eq", "value": true}`
)

func cmd(id, target string) string {
	return id + ` command {"targets": [` + target + `], "values": {"on": true}}`
}

// decoded decodes the Document in testdata/file, as the API decodes one.
func decoded(t testing.TB, file string) Document {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFlowsFromNodeRED(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil)
	for _, file := range []string{"flow1-bedroom-alice.json", "flow2-bedroom-bob.json", "flow3a-bedroom-parents.json", "flow4-living.json", "flow8-office.json"} {
		id := create(t, e, decoded(t, file))
		if s := statusOf(f, id); s.Status != home.AutomationEnabled {
			t.Fatalf("%s: %+v", file, s)
		}
	}
	const (
		warm     = "map[brightness:254 color_temp:500 state:true] 500ms"
		lampWarm = "map[brightness:254 color_temp:454 state:true] 500ms"
		cold     = "map[brightness:254 color_temp:200 state:true] 500ms"
		off      = "map[state:false] 500ms"
		living   = "map[brightness:254 color_temp:303 state:true] 500ms"
	)
	for _, c := range []struct {
		device home.DeviceID
		event  string
		want   []string
	}{
		// 1. Bedroom Alice
		{"switch-alice", "on_press_release", []string{"bedside-alice/light " + warm, "ceiling-alice/light " + warm, "lamp-alice/light " + lampWarm}},
		{"switch-alice", "off_press_release", []string{"aggregate bedroom-alice " + off}},
		{"switch-alice", "on_hold_release", []string{"lamp-alice/light map[brightness:3 color_temp:454 state:true] 500ms", "bedside-alice/light " + off, "ceiling-alice/light " + off}},
		{"button-alice", "long", []string{"lamp-alice/light map[brightness:3 color_temp:454 state:true] 500ms", "bedside-alice/light " + off, "ceiling-alice/light " + off}},
		{"switch-alice", "up_press_release", []string{"bedside-alice/light map[brightness:175 color_temp:200 state:toggle] 500ms"}},
		{"switch-alice", "down_press_release", []string{"lamp-alice/light map[brightness:175 color_temp:200 state:toggle] 500ms"}},
		{"switch-alice", "up_hold_release", []string{"bedside-alice/light " + warm}},
		{"switch-alice", "down_hold_release", []string{"lamp-alice/light map[brightness:254 color_temp:454 state:toggle] 500ms"}},
		{"button-alice", "single", []string{"aggregate bedroom-alice map[brightness:254 color_temp:200 state:toggle] 500ms"}},
		{"button-alice", "double", nil},
		// 2. Bedroom Bob
		{"switch-bob", "on_press_release", []string{"lamp-bob/light " + cold, "ceiling-bob/light " + cold, "plug-floor-lamp-bob/switch map[state:true] 0s"}},
		{"switch-bob", "off_press_release", []string{"lamp-bob/light " + off, "ceiling-bob/light " + off, "plug-floor-lamp-bob/switch map[state:false] 0s"}},
		// 3a. Parents' bedroom
		{"switch-parents", "on_press_release", []string{"bedside-dave/light " + warm, "bedside-carol/light " + warm}},
		{"switch-parents", "off_press_release", []string{"bedside-dave/light " + off, "bedside-carol/light " + off}},
		{"switch-parents", "up_hold_release", []string{"bedside-dave/light " + off, "bedside-carol/light map[brightness:1 color_temp:500 state:true] 500ms"}},
		{"switch-parents", "down_hold_release", []string{"bedside-carol/light " + off, "bedside-dave/light map[brightness:1 color_temp:500 state:true] 500ms"}},
		// 4. Living room
		{"switch-living", "on_press_release", []string{
			"light-living/light " + living, "light-entrance/light " + living, "ceiling-entrance/light " + living, "ceiling-living/light " + living,
			"office-strip/light map[state:true] 500ms", "lamp-office/light map[state:true] 500ms", "ceiling-office/light map[state:true] 500ms"}},
		{"switch-living", "off_press_release", []string{
			"office-strip/light " + off, "lamp-office/light " + off, "ceiling-office/light " + off,
			"light-living/light " + off, "light-entrance/light " + off, "ceiling-entrance/light " + off, "ceiling-living/light " + off}},
		{"switch-living", "on_hold_release", []string{"light-living/light map[brightness:254 color_temp:284 state:true] 500ms",
			"ceiling-office/light " + off, "ceiling-entrance/light " + off, "ceiling-living/light " + off, "light-entrance/light " + off}},
		{"switch-living", "off_hold_release", []string{"flag late-evening map[on:toggle] 0s"}},
		// 8. Office: Ceiling office stays out of the toggle
		{"button-office", "single", []string{"office-strip/light map[state:toggle] 0s", "lamp-office/light map[state:toggle] 0s"}},
		{"button-office", "long", []string{"office-strip/light map[brightness:254 color_temp:199 state:true] 500ms", "lamp-office/light map[brightness:254 color_temp:195 state:true] 500ms"}},
	} {
		press(e, f, c.device, c.event)
		if got := f.take(); !slices.Equal(got, c.want) {
			t.Errorf("%s %s:\n got  %q\n want %q", c.device, c.event, got, c.want)
		}
	}
}

func TestConvergingPathsActOnce(t *testing.T) {
	f := fixture()
	e := New(f, []Document{doc("converge",
		[]string{pressSingle, isNight, cmd("lamp", `"device:lamp-office/light"`)},
		"single.out lamp.in", "single.out is-night.in", "is-night.true lamp.in", "is-night.false lamp.in",
		"lamp.then lamp.in",
	)}, nil, nil)

	set(e, f, home.ValueChanged, nightRef, true)
	press(e, f, "button-office", "single")
	if got := f.take(); len(got) != 1 {
		t.Errorf("commands = %q, want one", got)
	}
	press(e, f, "button-office", "single") // each Run starts afresh
	if got := f.take(); len(got) != 1 {
		t.Errorf("second Run: commands = %q, want one", got)
	}
}

func TestAutomationsFireInDocumentOrderAndFanOutInEdgeOrder(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil)
	for _, name := range []string{"first", "second"} {
		create(t, e, doc(name,
			[]string{pressSingle, cmd("b", `"flag:`+name+`-b"`), cmd("a", `"flag:`+name+`-a"`)},
			"single.out b.in", "single.out a.in"))
	}
	press(e, f, "button-office", "single")
	want := []string{"flag first-b map[on:true] 0s", "flag first-a map[on:true] 0s", "flag second-b map[on:true] 0s", "flag second-a map[on:true] 0s"}
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("commands = %q, want %q", got, want)
	}
}

func TestRefusedCommandStillFiresThen(t *testing.T) {
	f := fixture()
	f.refuse = map[home.Target]bool{home.TargetDevice("office-strip", "light"): true}
	e := New(f, []Document{doc("refused",
		[]string{pressSingle,
			cmd("both", `"device:office-strip/light", "device:lamp-office/light"`),
			cmd("after", `"flag:guest"`)},
		"single.out both.in", "both.then after.in",
	)}, nil, nil)

	press(e, f, "button-office", "single")
	want := []string{"lamp-office/light map[on:true] 0s", "flag guest map[on:true] 0s"}
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("commands = %q, want %q", got, want)
	}
}

func TestValueTriggerFiresOnRealChangesOnly(t *testing.T) {
	f := fixture()
	e := New(f, []Document{
		doc("night", []string{nightOn, cmd("act", `"flag:guest"`)}, "night-on.out act.in"),
		doc("dark", []string{luxLow, cmd("act", `"flag:guest"`)}, "lux-low.out act.in"),
	}, nil, nil)

	for i, c := range []struct {
		kind  home.UpdateKind
		ref   home.Ref
		data  any
		fires bool
	}{
		{home.ValueChanged, nightRef, true, false}, // a first Value, after unknown
		{home.ValueRefreshed, nightRef, true, false},
		{home.ValueChanged, nightRef, false, false},
		{home.ValueChanged, nightRef, true, true},
		{home.ValueChanged, luxRef, 50.0, false}, // a first Value
		{home.ValueChanged, luxRef, 150.0, false},
		{home.ValueChanged, luxRef, 80.0, true},
		{home.ValueChanged, luxRef, 60.0, false}, // already below
		{home.ValueChanged, luxRef, nil, false},  // unknown again
		{home.ValueChanged, luxRef, 40.0, false}, // a first Value after unknown
		{home.ValueChanged, luxRef, 120.0, false},
		{home.ValueChanged, luxRef, 99.0, true},
	} {
		set(e, f, c.kind, c.ref, c.data)
		if got := len(f.take()); got != map[bool]int{true: 1}[c.fires] {
			t.Errorf("%d: %s %v fired %d Runs", i, c.kind, c.data, got)
		}
	}
}

func TestValueConditionReadsTheLastKnownValue(t *testing.T) {
	f := fixture()
	e := New(f, []Document{doc("cond",
		[]string{pressSingle, isNight, cmd("yes", `"flag:yes"`), cmd("no", `"flag:no"`)},
		"single.out is-night.in", "is-night.true yes.in", "is-night.false no.in",
	)}, nil, nil)

	for _, c := range []struct {
		night any
		want  []string
	}{
		{nil, nil}, // unknown fires neither handle
		{true, []string{"flag yes map[on:true] 0s"}},
		{false, []string{"flag no map[on:true] 0s"}},
	} {
		if c.night != nil {
			set(e, f, home.ValueChanged, nightRef, c.night)
		}
		press(e, f, "button-office", "single")
		if got := f.take(); !slices.Equal(got, c.want) {
			t.Errorf("night %v: commands = %q, want %q", c.night, got, c.want)
		}
	}
}

func TestBrokenDocuments(t *testing.T) {
	for reason, d := range map[string]Document{
		`unknown kind "presence"`:     doc("x", []string{pressSingle, `t presence {}`}, "single.out t.in"),
		`no output "fired"`:           doc("x", []string{pressSingle, cmd("c", `"flag:night"`)}, "single.fired c.in"),
		`no input "start"`:            doc("x", []string{pressSingle, cmd("c", `"flag:night"`)}, "single.out c.start"),
		`edge into trigger`:           doc("x", []string{pressSingle, nightOn}, "single.out night-on.in"),
		`unknown step`:                doc("x", []string{pressSingle}, "single.out nowhere.in"),
		`duplicate id`:                doc("x", []string{pressSingle, pressSingle}),
		`unknown comparison`:          doc("x", []string{`v valueTrigger {"target": "flag:night", "capability": "on", "op": "is", "value": true}`}),
		`bad target "room:r1"`:        doc("x", []string{`c command {"targets": ["room:r1"], "values": {"on": true}}`}),
		`no targets`:                  doc("x", []string{`c command {"targets": [], "values": {"on": true}}`}),
		`target device:gone is`:       doc("x", []string{pressSingle, cmd("c", `"device:gone"`)}, "single.out c.in"),
		`bad time of day "25:00"`:     doc("x", []string{`t timeTrigger {"at": "25:00"}`}),
		`unknown weekday "lun"`:       doc("x", []string{`t timeTrigger {"at": "06:00", "weekdays": ["lun"]}`}),
		`unknown sun event`:           doc("x", []string{`s sunTrigger {"event": "noon"}`}),
		`no home location`:            doc("x", []string{`s sunTrigger {"event": "dusk"}`}),
		`an empty window`:             doc("x", []string{`w timeWindow {"from": "06:00", "to": "06:00"}`}),
		`reentry must be`:             doc("x", []string{`t timer {"duration": 60}`}),
		`duration must be`:            doc("x", []string{`c cooldown {"duration": 0}`}),
		`heldFor must be`:             doc("x", []string{`v valueTrigger {"target": "flag:night", "capability": "on", "op": "eq", "value": true, "heldFor": -1}`}),
		`maxBlocks at least`:          doc("x", []string{`p presenceSimulation {"from": "18:00", "to": "21:00", "minBlocks": 4, "maxBlocks": 3, "minDuration": 60, "maxDuration": 60}`}),
		`don't fit the window`:        doc("x", []string{`p presenceSimulation {"from": "23:00", "to": "01:00", "minBlocks": 3, "maxBlocks": 3, "minDuration": 2401, "maxDuration": 2401}`}),
		`got newline, want ':'`:       doc("x", []string{coded("c", "def run(trigger, state)\n  pass", nil, nil)}),
		`no def run(trigger, state)`:  doc("x", []string{coded("c", "def handle(trigger, state):\n  pass", nil, nil)}),
		`unknown alias "door"`:        doc("x", []string{coded("c", "def run(trigger, state):\n  value(\"door\", \"contact\")", nil, nil)}),
		`target device:gone-door/con`: doc("x", []string{coded("c", "def run(trigger, state):\n  pass", nil, map[string]home.Target{"door": home.TargetDevice("gone-door", "contact")})}),
		`load not implemented`:        doc("x", []string{coded("c", "load(\"x.star\", \"y\")\ndef run(trigger, state):\n  pass", nil, nil)}),
		`output "out": empty or dup`:  doc("x", []string{coded("c", "def run(trigger, state):\n  pass", []string{"out", "out"}, nil)}),
		`too many steps`:              doc("x", []string{coded("c", "def spin():\n  for i in range(1000000000):\n    pass\nx = spin()\ndef run(trigger, state):\n  pass", nil, nil)}),
	} {
		f := fixture()
		e := New(f, []Document{d}, nil, nil)
		s := f.status[0]
		if s.Status != home.AutomationBroken || !strings.Contains(s.Reason, reason) {
			t.Errorf("%s: status %+v", reason, s)
		}
		// The offending Step, but for an edge to nowhere.
		if named := slices.ContainsFunc(d.Steps, func(st Step) bool { return st.ID == s.Step }); named == (reason == "unknown step") {
			t.Errorf("%s: step %q", reason, s.Step)
		}
		press(e, f, "button-office", "single")
		if got := f.take(); got != nil {
			t.Errorf("%s: a broken Automation ran: %q", reason, got)
		}
	}
}

func TestDeletedTargetBreaksUntilTheDocumentIsFixed(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil)
	d := doc("guest", []string{pressSingle, cmd("c", `"flag:guest"`)}, "single.out c.in")
	id := create(t, e, d)

	f.emit(e, home.Update{Kind: home.FlagsChanged, Flags: []home.Flag{{ID: "night"}}})
	if s := statusOf(f, id); s.Status != home.AutomationBroken || !strings.Contains(s.Reason, `step "c"`) || s.Step != "c" {
		t.Fatalf("status = %+v", s)
	}
	press(e, f, "button-office", "single")
	if got := f.take(); got != nil {
		t.Errorf("broken Automation ran: %q", got)
	}

	d.Steps[1] = Step{ID: "c", Kind: command, Params: json.RawMessage(`{"targets": ["flag:night"], "values": {"on": true}}`)}
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(f, id); s.Status != home.AutomationEnabled {
		t.Fatalf("status after fix = %+v", s)
	}
	press(e, f, "button-office", "single")
	if got := f.take(); len(got) != 1 {
		t.Errorf("fixed Automation: commands = %q", got)
	}

	f.emit(e, home.Update{Kind: home.DevicesChanged}) // every Device gone
	if s := statusOf(f, id); s.Status != home.AutomationBroken || !strings.Contains(s.Reason, `step "single"`) || s.Step != "single" {
		t.Errorf("status after devices deleted = %+v", s)
	}
}

func TestHotReloadSwapsOneAutomationAlone(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil)
	a := create(t, e, doc("a", []string{pressSingle, cmd("c", `"flag:a"`)}, "single.out c.in"))
	create(t, e, doc("b", []string{pressSingle, cmd("c", `"flag:b"`)}, "single.out c.in"))
	b := e.autos[1]

	if err := e.Replace(a, doc("a2", []string{nightOn, pressSingle, cmd("c", `"flag:a2"`)}, "single.out c.in")); err != nil {
		t.Fatal(err)
	}
	if e.autos[1] != b {
		t.Error("replacing a reloaded b")
	}
	press(e, f, "button-office", "single")
	want := []string{"flag a2 map[on:true] 0s", "flag b map[on:true] 0s"} // a2 keeps a's place
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("commands = %q, want %q", got, want)
	}

	d := e.Documents()[0]
	d.Enabled = false
	if err := e.Replace(a, d); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(f, a); s.Status != home.AutomationDisabled {
		t.Errorf("status = %+v", s)
	}
	press(e, f, "button-office", "single")
	if got := f.take(); !slices.Equal(got, want[1:]) {
		t.Errorf("disabled: commands = %q", got)
	}

	if err := e.Delete(a); err != nil {
		t.Fatal(err)
	}
	if len(f.status) != 1 || len(e.onValue) != 0 || len(e.onEvent) != 1 {
		t.Errorf("after delete: status %+v, %d value and %d event keys", f.status, len(e.onValue), len(e.onEvent))
	}
	if err := e.Delete(a); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if err := e.Replace(a, d); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("replace of a deleted one: %v", err)
	}
}

// opened is the Engine saved in dir, following f: open it again on the same
// dir to restart it.
func opened(t *testing.T, dir string, f Home) *Engine {
	t.Helper()
	e, err := Open(dir, f, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestDocumentsSurviveARestart(t *testing.T) {
	dir, f := t.TempDir(), fixture()
	e := opened(t, dir, f)
	if _, err := e.Create(Document{Name: "  "}); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("blank name: %v", err)
	}
	id := create(t, e, Document{Name: " Empty "})
	if saved := opened(t, dir, f).Documents(); len(saved) != 1 || saved[0].ID != id || saved[0].Name != "Empty" || saved[0].Steps == nil || saved[0].Edges == nil {
		t.Errorf("after a restart: %+v", saved)
	}
	if err := e.Delete(id); err != nil {
		t.Fatal(err)
	}
	if saved := opened(t, dir, f).Documents(); len(saved) != 0 {
		t.Errorf("after delete and a restart: %+v", saved)
	}
}

func TestTraceShowsWhatAConditionRead(t *testing.T) {
	f := fixture()
	var traces []*Trace
	e := New(f, nil, nil, keep(&traces))
	id := create(t, e, doc("cond",
		[]string{pressSingle, isNight, cmd("yes", `"flag:yes"`)},
		"single.out is-night.in", "is-night.true yes.in"))

	press(e, f, "button-office", "single") // night is unknown
	set(e, f, home.ValueChanged, nightRef, false)
	press(e, f, "button-office", "single")
	if len(traces) != 2 {
		t.Fatalf("%d Traces for 2 Runs", len(traces))
	}
	for i, want := range [][]string{
		{"single [out]", "is-night [] unknown"},
		{"single [out]", "is-night [false] read false"},
	} {
		tr := traces[i]
		if got := path(tr); !slices.Equal(got, want) {
			t.Errorf("Run %d: path = %q, want %q", i, got, want)
		}
		if tr.Automation != id || tr.Outcome != home.RunNothing || tr.Trigger.Kind != eventTrigger ||
			tr.Trigger.Target != home.TargetDevice("button-office", "button") || tr.Trigger.Capability != "action" ||
			tr.Trigger.Value != "single" || tr.Trigger.Time.IsZero() || tr.Trigger.Step != "single" {
			t.Errorf("Run %d: trace = %+v", i, tr)
		}
		if r := f.runs[i]; r != (home.RunEnd{Automation: id, Run: tr.Run, Time: tr.Time, Outcome: home.RunNothing, Trigger: tr.Trigger}) {
			t.Errorf("Run %d: end = %+v, trace %+v", i, r, tr)
		}
	}
	if read := traces[1].Steps[1]; read.At.IsZero() || read.At.After(traces[1].Time) {
		t.Errorf("read at %v, want when the Value read was reported, before the Run at %v", read.At, traces[1].Time)
	}
	if read := traces[0].Steps[1]; !read.At.IsZero() {
		t.Errorf("unknown read at %v, want none", read.At)
	}
	if r := traces[0].Run; r == traces[1].Run || r[6]>>4 != 7 || r[8]>>6 != 0b10 {
		t.Errorf("Run ids %s and %s: want distinct UUIDv7s", r, traces[1].Run)
	}
}

func TestTraceShowsCommandsAndRefusals(t *testing.T) {
	f := fixture()
	f.refuse = map[home.Target]bool{home.TargetDevice("office-strip", "light"): true}
	var traces []*Trace
	e := New(f, nil, nil, keep(&traces))
	id := create(t, e, doc("both",
		[]string{pressSingle, cmd("both", `"device:office-strip/light", "device:lamp-office/light"`)},
		"single.out both.in"))

	press(e, f, "button-office", "single")
	f.refuse = nil
	press(e, f, "button-office", "single")
	for i, c := range []struct {
		path    []string
		outcome home.RunOutcome
	}{
		{[]string{"single [out]", "both [then] invalid request: refused c1"}, home.RunError},
		{[]string{"single [out]", "both [then] c2 c3"}, home.RunActed},
	} {
		if got := path(traces[i]); !slices.Equal(got, c.path) || traces[i].Outcome != c.outcome || f.runs[i].Outcome != c.outcome {
			t.Errorf("Run %d: path %q, %s; want %q, %s", i, got, traces[i].Outcome, c.path, c.outcome)
		}
	}
	for i, run := range []int{0, 1, 1} {
		if want := (home.Origin{Automation: id, Step: "both", Run: traces[run].Run}); f.origins[i] != want {
			t.Errorf("origin %d = %+v, want %+v", i, f.origins[i], want)
		}
	}
}

// A watching Step may watch a Device's own Capability, such as its battery.
func TestValueTriggerWatchesADeviceItself(t *testing.T) {
	f := fixture()
	e := New(f, []Document{doc("battery",
		[]string{`low valueTrigger {"target": "device:lux-sensor", "capability": "battery", "op": "lt", "value": 10}`, cmd("act", `"flag:guest"`)},
		"low.out act.in")}, nil, nil)

	if s := f.status[0]; s.Status != home.AutomationEnabled {
		t.Fatalf("status %+v", s)
	}
	battery := home.TargetDevice("lux-sensor", "").Ref("battery")
	set(e, f, home.ValueChanged, battery, 50.0)
	set(e, f, home.ValueChanged, battery, 5.0)
	if got, want := f.take(), []string{"flag guest map[on:true] 0s"}; !slices.Equal(got, want) {
		t.Errorf("commands = %q, want %q", got, want)
	}
}
