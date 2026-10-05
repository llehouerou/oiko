package home

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

var occupancy = Capability{Key: "occupancy", Type: Binary, Category: Primary, Access: Access{Observable: true}}

func motion(address string, caps ...Capability) bridge.Device {
	if caps == nil {
		caps = []Capability{occupancy}
	}
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "occupancy", Kind: "occupancy", Capabilities: caps}}}
}

// aggregates is a Home with a bulb and three motion sensors, whose saved
// Devices and Aggregates are captured as they would be on disk.
func aggregates(t *testing.T) (h *Home, updates <-chan Update, devices *[]Device, saved *[]Aggregate) {
	t.Helper()
	devices, saved = &[]Device{}, &[]Aggregate{}
	h = newHome(&fakeBridge{}, nil, nil, nil,
		func(d []Device) error { *devices = d; return nil },
		func(a []Aggregate) error { *saved = a; return nil }, nil, nil)
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2"), motion("0xm3")})
	_, updates, cancel := h.Subscribe()
	t.Cleanup(cancel)
	return h, updates, devices, saved
}

func members(t *testing.T, h *Home, addresses ...string) []Target {
	t.Helper()
	var ms []Target
	for _, a := range addresses {
		ms = append(ms, TargetDevice(idOf(t, h, a), "occupancy"))
	}
	return ms
}

func snapshot(h *Home) Snapshot {
	s, _, cancel := h.Subscribe()
	cancel()
	return s
}

// aggregateValues returns the Updates about Aggregate Values.
func aggregateValues(us []Update) []Update {
	var got []Update
	for _, u := range us {
		if u.Ref != nil && u.Ref.Target.Aggregate() != "" {
			got = append(got, u)
		}
	}
	return got
}

func TestCreateAggregateValidation(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	m1 := members(t, h, "0xm1")[0]
	for name, tc := range map[string]struct {
		name    string
		members []Target
		binary  BinaryRule
		numeric NumericRule
		want    error
	}{
		"no member":            {"Doors", nil, "", "", ErrInvalid},
		"unknown device":       {"Doors", []Target{TargetDevice("nope", "occupancy")}, "", "", ErrNotFound},
		"unknown function":     {"Doors", []Target{TargetDevice(m1.Device(), "contact")}, "", "", ErrNotFound},
		"mixed kinds":          {"Doors", []Target{m1, TargetDevice(idOf(t, h, "0xbulb"), "light")}, "", "", ErrInvalid},
		"blank name":           {"   ", []Target{m1}, "", "", ErrInvalid},
		"long name":            {strings.Repeat("x", 101), []Target{m1}, "", "", ErrInvalid},
		"unknown binary rule":  {"Doors", []Target{m1}, "most", "", ErrInvalid},
		"unknown numeric rule": {"Doors", []Target{m1}, "", "median", ErrInvalid},
	} {
		if _, err := h.CreateAggregate(tc.name, tc.members, tc.binary, tc.numeric); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if len(*saved) != 0 || len(drain(updates)) != 0 {
		t.Fatal("a refused Aggregate was saved or announced")
	}
}

func TestAggregateCapabilitiesAreSharedOnesWithoutStateless(t *testing.T) {
	sensitivity := Capability{Key: "sensitivity", Type: Numeric, Access: Access{Observable: true, Settable: true}}
	action := Capability{Key: "action", Type: Enum, Stateless: true, Access: Access{Observable: true}}
	h, _, _, _ := aggregates(t)
	port(h).SyncDevices([]bridge.Device{
		motion("0xm1", occupancy, Capability{Key: "tamper", Type: Binary}, action, sensitivity),
		motion("0xm2", occupancy, Capability{Key: "tamper", Type: Numeric}, action, sensitivity),
	})
	if _, err := h.CreateAggregate("Veranda", members(t, h, "0xm1", "0xm2"), "", ""); err != nil {
		t.Fatal(err)
	}
	a := snapshot(h).Aggregates[0]
	var keys []string
	for _, c := range a.Capabilities {
		keys = append(keys, c.Key)
	}
	if a.Kind != "occupancy" || a.Binary != Any || a.Numeric != Mean || !slices.Equal(keys, []string{"occupancy", "sensitivity"}) {
		t.Fatalf("aggregate = %+v, want kind occupancy, rules any and mean, capabilities occupancy and sensitivity", a)
	}
}

func TestAnAggregateCountsOnlyWhatEveryMemberCountsAndResolvesItsMembers(t *testing.T) {
	energy := Capability{Key: "energy", Type: Numeric, Counter: true}
	level := Capability{Key: "level", Type: Numeric, Counter: true}
	h, _, _, _ := aggregates(t)
	port(h).SyncDevices([]bridge.Device{
		motion("0xm1", occupancy, energy, level),
		motion("0xm2", occupancy, energy, Capability{Key: "level", Type: Numeric}),
		motion("0xm3", occupancy, energy, level),
	})
	inner, err := h.CreateAggregate("Inner", members(t, h, "0xm1", "0xm2"), "", Sum)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := h.CreateAggregate("Outer", append([]Target{TargetAggregate(inner)}, members(t, h, "0xm3", "0xm1")...), "", Sum)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"energy": true, "level": false} {
		if c, err := h.Capability(TargetAggregate(outer).Ref(key)); err != nil || c.Counter != want {
			t.Errorf("%s: counter %v (%v), want %v", key, c.Counter, err, want)
		}
	}
	if got, want := h.Members(TargetAggregate(outer)), members(t, h, "0xm1", "0xm2", "0xm3"); !reflect.DeepEqual(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if got := h.Members(members(t, h, "0xm1")[0]); got != nil {
		t.Errorf("a Function's members = %v, want none", got)
	}
}

func TestAnyAndAllIgnoreMembersWithoutValue(t *testing.T) {
	h, updates, _, _ := aggregates(t)
	all3 := members(t, h, "0xm1", "0xm2", "0xm3")
	anyID, _ := h.CreateAggregate("Any", all3, Any, "")
	allID, _ := h.CreateAggregate("All", all3, All, "")
	if len(snapshot(h).Values) != 0 {
		t.Fatal("an Aggregate whose members have no Value has a Value")
	}
	t1 := time.Now()
	t2 := t1.Add(time.Second)
	port(h).Report("0xm1", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: true}}, t1)
	port(h).Report("0xm2", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: false}}, t2)

	type got struct {
		id   AggregateID
		kind UpdateKind
		data any
		at   time.Time
	}
	var gots []got
	for _, u := range aggregateValues(drain(updates)) {
		gots = append(gots, got{u.Ref.Target.Aggregate(), u.Kind, u.Value.Data, u.Value.At})
	}
	// 0xm3 never reported: it counts neither for any nor for all.
	want := []got{
		{allID, ValueChanged, true, t1}, // within a report, Aggregates come in Name order
		{anyID, ValueChanged, true, t1},
		{allID, ValueChanged, false, t2},
		{anyID, ValueRefreshed, true, t2},
	}
	if len(gots) != len(want) {
		t.Fatalf("updates = %+v, want %+v", gots, want)
	}
	for i := range want {
		if gots[i].id != want[i].id || gots[i].kind != want[i].kind || gots[i].data != want[i].data || !gots[i].at.Equal(want[i].at) {
			t.Errorf("update %d = %+v, want %+v", i, gots[i], want[i])
		}
	}
}

func TestAStateHeldSinceItTookItsDataRecalledAcrossARestartAndAnAggregateFromItsMembers(t *testing.T) {
	h, _, _, _ := aggregates(t)
	t0 := time.Now().Add(-48 * time.Hour)
	m1 := members(t, h, "0xm1")[0]
	// m1 was on before Oiko started; its History tells since when.
	h.Recall(func(ref Ref, data any) (time.Time, bool) { return t0, ref.Target == m1 && data == true })
	all3 := members(t, h, "0xm1", "0xm2", "0xm3")
	anyID, _ := h.CreateAggregate("Any", all3, Any, "")
	allID, _ := h.CreateAggregate("All", all3, All, "")
	report := func(address string, on bool, at time.Time) {
		port(h).Report(address, []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: on}}, at)
	}
	since := func(t2 Target) time.Time {
		for _, v := range snapshot(h).Values {
			if v.Ref == t2.Ref("occupancy") {
				return v.Value.Since
			}
		}
		return time.Time{}
	}
	check := func(step string, want map[Target]time.Time) {
		t.Helper()
		for tg, w := range want {
			if got := since(tg); !got.Equal(w) {
				t.Errorf("%s: %s since %v, want %v", step, tg, got, w)
			}
		}
	}
	any, all := TargetAggregate(anyID), TargetAggregate(allID)
	t1, t2, t3, t4 := t0.Add(47*time.Hour), t0.Add(47*time.Hour+time.Minute), t0.Add(47*time.Hour+2*time.Minute), t0.Add(47*time.Hour+3*time.Minute)
	report("0xm1", true, t1)
	report("0xm2", true, t1)
	report("0xm3", false, t1)
	check("started", map[Target]time.Time{m1: t0, any: t0, all: t1}) // any on since the first on; all off since the last off
	report("0xm1", true, t2)
	check("refreshed", map[Target]time.Time{m1: t0, any: t0})
	report("0xm3", true, t3)
	check("all on", map[Target]time.Time{all: t3}) // since the last turned on
	report("0xm1", false, t4)
	check("one off", map[Target]time.Time{m1: t4, any: t1, all: t4}) // any: m2 is the first still on
}

func TestAggregateLifecycleInSnapshotAndUpdates(t *testing.T) {
	h, updates, _, _ := aggregates(t)
	at := time.Now()
	port(h).Report("0xm1", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: true}}, at)
	drain(updates)

	id, err := h.CreateAggregate("Motion Veranda", members(t, h, "0xm1", "0xm2"), Any, "")
	if err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if ks := kinds(us); !slices.Equal(ks, []UpdateKind{AggregatesChanged, ValueChanged}) {
		t.Fatalf("creation kinds = %v", ks)
	}
	if v := us[1].Value; us[1].Ref.Target.Aggregate() != id || v.Data != true || !v.At.Equal(at) {
		t.Errorf("initial value = %+v at %+v, want true from the member's report", v, us[1].Ref)
	}
	s := snapshot(h)
	want := TargetAggregate(id).Ref("occupancy")
	if len(s.Aggregates) != 1 || !slices.ContainsFunc(s.Values, func(rv RefValue) bool { return rv.Ref == want }) {
		t.Fatalf("snapshot aggregates %+v, values %+v", s.Aggregates, s.Values)
	}

	if err := h.DeleteAggregate(id); err != nil {
		t.Fatal(err)
	}
	if us := drain(updates); len(us) != 2 || us[0].Kind != TargetDeleted || us[0].Target != TargetAggregate(id) ||
		us[1].Kind != AggregatesChanged || len(us[1].Aggregates) != 0 {
		t.Fatalf("deletion updates = %+v", us)
	}
	s = snapshot(h)
	if len(s.Aggregates) != 0 || slices.ContainsFunc(s.Values, func(rv RefValue) bool { return rv.Ref == want }) {
		t.Fatalf("after deletion: aggregates %+v, values %+v", s.Aggregates, s.Values)
	}
	if err := h.DeleteAggregate(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice: got %v, want ErrNotFound", err)
	}
}

func TestAggregatesSurviveRestart(t *testing.T) {
	h, _, devices, saved := aggregates(t)
	id, err := h.CreateAggregate("Doors", members(t, h, "0xm1", "0xm2"), All, Sum)
	if err != nil {
		t.Fatal(err)
	}
	if len(*saved) != 1 || (*saved)[0].Capabilities != nil || (*saved)[0].Kind != "" {
		t.Fatalf("saved = %+v, want one definition without derived fields", *saved)
	}

	restarted := newHome(&fakeBridge{}, *devices, *saved, nil, nil, nil, nil, nil)
	if av, ok := snapshot(restarted).Availability[TargetAggregate(id)]; !ok || av != Unknown {
		t.Fatalf("availability before the Bridge is online = %q, %v; want unknown", av, ok)
	}
	port(restarted).SetOnline(true)
	port(restarted).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2"), motion("0xm3")})
	port(restarted).Report("0xm1", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: true}}, time.Now())
	s := snapshot(restarted)
	if a := s.Aggregates; len(a) != 1 || a[0].ID != id || a[0].Kind != "occupancy" || a[0].Binary != All || a[0].Numeric != Sum {
		t.Fatalf("restored aggregates = %+v", s.Aggregates)
	}
	want := TargetAggregate(id).Ref("occupancy")
	if !slices.ContainsFunc(s.Values, func(rv RefValue) bool { return rv.Ref == want && rv.Value.Data == true }) {
		t.Fatalf("the Value was not recomputed from the replayed member Value: %+v", s.Values)
	}
}

// thermometer measures temperature and also exposes Capabilities of every
// type that gets no Aggregate Value.
func thermometer(address string) bridge.Device {
	readable := Access{Observable: true}
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "temperature", Kind: "temperature", Capabilities: []Capability{
		{Key: "temperature", Type: Numeric, Category: Primary, Access: readable},
		{Key: "mode", Type: Enum, Options: []string{"a", "b"}, Access: readable},
		{Key: "label", Type: Text, Access: readable},
		{Key: "calibration", Type: Composite, Access: readable},
		{Key: "history", Type: List, Access: readable},
	}}}}
}

func TestNumericRulesIgnoreMembersWithoutValue(t *testing.T) {
	h, updates, _, _ := aggregates(t)
	port(h).SyncDevices([]bridge.Device{thermometer("0xt1"), thermometer("0xt2"), thermometer("0xt3")})
	var ms []Target
	for _, a := range []string{"0xt1", "0xt2", "0xt3"} {
		ms = append(ms, TargetDevice(idOf(t, h, a), "temperature"))
	}
	ids := map[AggregateID]NumericRule{}
	for _, rule := range []NumericRule{"", Min, Max, Sum} {
		id, err := h.CreateAggregate("Veranda "+string(rule), ms, "", rule)
		if err != nil {
			t.Fatal(err)
		}
		ids[id] = cmp.Or(rule, Mean)
	}
	if len(snapshot(h).Values) != 0 {
		t.Fatal("an Aggregate whose members have no Value has a Value")
	}
	drain(updates)

	// 0xt3 never reports: it counts for no rule.
	t1 := time.Now()
	t2 := t1.Add(time.Second)
	other := []bridge.Reading{
		{Function: "temperature", Capability: "mode", Data: "a"},
		{Function: "temperature", Capability: "label", Data: "x"},
		{Function: "temperature", Capability: "calibration", Data: map[string]any{"offset": 1.0}},
		{Function: "temperature", Capability: "history", Data: []any{1.0}},
	}
	port(h).Report("0xt1", append([]bridge.Reading{{Function: "temperature", Capability: "temperature", Data: 20.0}}, other...), t1)
	port(h).Report("0xt2", append([]bridge.Reading{{Function: "temperature", Capability: "temperature", Data: 22.0}}, other...), t2)

	want := map[NumericRule]float64{Mean: 21, Min: 20, Max: 22, Sum: 42}
	last := map[AggregateID]Update{}
	for _, u := range aggregateValues(drain(updates)) {
		if u.Ref.Capability != "temperature" {
			t.Fatalf("%s got an Aggregate Value", u.Ref.Capability)
		}
		last[u.Ref.Target.Aggregate()] = u
	}
	for id, rule := range ids {
		wantKind := ValueChanged
		if rule == Min { // 22 leaves the min at 20
			wantKind = ValueRefreshed
		}
		u := last[id]
		if u.Value == nil || u.Kind != wantKind || u.Value.Data != want[rule] || !u.Value.At.Equal(t2) {
			t.Errorf("%s: last update %+v, want %s to %v at t2", rule, u, wantKind, want[rule])
		}
	}
}

func TestABrightnessCountsOnlyTheLightsOnAllOfThemIfNone(t *testing.T) {
	h, _, _, _ := aggregates(t)
	var ms []Target
	var bulbs []bridge.Device
	for _, a := range []string{"0xb1", "0xb2", "0xb3"} {
		b := bulb
		b.NativeAddress, b.Name = a, a
		bulbs = append(bulbs, b)
	}
	port(h).SyncDevices(bulbs)
	for _, b := range bulbs {
		ms = append(ms, TargetDevice(idOf(t, h, b.NativeAddress), "light"))
	}
	id, err := h.CreateAggregate("Bedroom", ms, "", "")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	report := func(address string, rs ...bridge.Reading) {
		port(h).Report(address, rs, at)
	}
	brightness := func(step string, want float64) {
		t.Helper()
		for _, v := range snapshot(h).Values {
			if v.Ref == TargetAggregate(id).Ref("brightness") {
				if v.Value.Data != want {
					t.Errorf("%s: brightness %v, want %v", step, v.Value.Data, want)
				}
				return
			}
		}
		t.Errorf("%s: no brightness", step)
	}
	on := func(b bool) bridge.Reading { return bridge.Reading{Function: "light", Capability: "state", Data: b} }
	bri := func(f float64) bridge.Reading {
		return bridge.Reading{Function: "light", Capability: "brightness", Data: f}
	}
	report("0xb1", on(true), bri(3))
	report("0xb2", on(false), bri(254))
	report("0xb3", on(false), bri(254))
	brightness("one on", 3)
	report("0xb1", on(false)) // a state alone moves the brightness
	brightness("all off", 511.0/3)
	report("0xb2", on(true))
	brightness("another on", 254)
}

func TestEditAggregate(t *testing.T) {
	h, updates, devices, saved := aggregates(t)
	at := time.Now()
	port(h).Report("0xm1", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: false}}, at)
	port(h).Report("0xm3", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: true}}, at)
	id, _ := h.CreateAggregate("Veranda", members(t, h, "0xm1", "0xm2"), Any, "")
	drain(updates)

	if err := h.EditAggregate(id, "  Motion Veranda ", members(t, h, "0xm1", "0xm3"), All, Max); err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	// any over {false} was false; all over {false, true} is still false: no Value Update.
	if ks := kinds(us); !slices.Equal(ks, []UpdateKind{AggregatesChanged}) {
		t.Fatalf("edit kinds = %v", ks)
	}
	if a := us[0].Aggregates; len(a) != 1 || a[0].ID != id || a[0].Name != "Motion Veranda" || a[0].Binary != All || a[0].Numeric != Max || len(a[0].Members) != 2 {
		t.Fatalf("announced %+v", us[0].Aggregates)
	}
	if err := h.EditAggregate(id, "Motion Veranda", members(t, h, "0xm3"), All, Max); err != nil {
		t.Fatal(err)
	}
	vs := aggregateValues(drain(updates))
	if len(vs) != 1 || vs[0].Kind != ValueChanged || vs[0].Value.Data != true {
		t.Fatalf("after removing 0xm1: %+v, want a change to true", vs)
	}
	if err := h.EditAggregate(id, "Motion Veranda", members(t, h, "0xm2"), All, Max); err != nil {
		t.Fatal(err)
	}
	vs = aggregateValues(drain(updates))
	if len(vs) != 1 || vs[0].Kind != ValueChanged || vs[0].Value != nil {
		t.Fatalf("after keeping only 0xm2, which never reported: %+v, want a change to no Value", vs)
	}
	if err := h.EditAggregate(id, "Motion Veranda", members(t, h, "0xm3"), All, Max); err != nil {
		t.Fatal(err)
	}
	drain(updates)

	restarted := newHome(&fakeBridge{}, *devices, *saved, nil, nil, nil, nil, nil)
	if a := snapshot(restarted).Aggregates; len(a) != 1 || a[0].Name != "Motion Veranda" || !slices.Equal(a[0].Members, members(t, h, "0xm3")) || a[0].Binary != All || a[0].Numeric != Max {
		t.Fatalf("restored %+v", a)
	}
}

func TestRefusedEditLeavesAggregateUnchanged(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	id, _ := h.CreateAggregate("Veranda", members(t, h, "0xm1"), "", "")
	before, savedBefore := snapshot(h).Aggregates, *saved
	drain(updates)
	m1 := members(t, h, "0xm1")[0]
	for name, tc := range map[string]struct {
		id      AggregateID
		name    string
		members []Target
		want    error
	}{
		"unknown aggregate": {"nope", "Veranda", []Target{m1}, ErrNotFound},
		"no member":         {id, "Veranda", nil, ErrInvalid},
		"unknown member":    {id, "Veranda", []Target{m1, TargetDevice("nope", "occupancy")}, ErrNotFound},
		"mixed kinds":       {id, "Veranda", []Target{m1, TargetDevice(idOf(t, h, "0xbulb"), "light")}, ErrInvalid},
		"blank name":        {id, " ", []Target{m1}, ErrInvalid},
		"long name":         {id, strings.Repeat("x", 101), []Target{m1}, ErrInvalid},
	} {
		if err := h.EditAggregate(tc.id, tc.name, tc.members, "", ""); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if len(drain(updates)) != 0 || !reflect.DeepEqual(*saved, savedBefore) || !reflect.DeepEqual(snapshot(h).Aggregates, before) {
		t.Fatal("a refused edit changed, saved or announced the Aggregate")
	}
}

// about describes the Updates about Aggregate id, e.g. "value occupancy=true"
// or "availability online".
func about(us []Update, id AggregateID) []string {
	var got []string
	for _, u := range us {
		switch {
		case u.Ref != nil && u.Ref.Target.Aggregate() == id && u.Value != nil:
			got = append(got, fmt.Sprintf("%s %s=%v", u.Kind, u.Ref.Capability, u.Value.Data))
		case u.Ref != nil && u.Ref.Target.Aggregate() == id:
			got = append(got, fmt.Sprintf("%s %s none", u.Kind, u.Ref.Capability))
		case u.Kind == AvailabilityChanged && u.Target == TargetAggregate(id):
			got = append(got, "availability "+string(u.Availability))
		}
	}
	return got
}

func occupied(h *Home, address string, occupied bool) {
	port(h).Report(address, []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: occupied}}, time.Now())
}

func TestAggregateAvailability(t *testing.T) {
	h, updates, _, _ := aggregates(t)
	id, _ := h.CreateAggregate("Veranda", members(t, h, "0xm1", "0xm2"), "", "")
	if av := snapshot(h).Availability[TargetAggregate(id)]; av != Unknown {
		t.Fatalf("availability without any member's = %q, want unknown", av)
	}
	port(h).SetAvailability("0xm1", Offline) // offline: no member is online
	port(h).SetAvailability("0xm2", Offline)
	port(h).SetAvailability("0xm2", Online) // online: one member is
	port(h).SetAvailability("0xm1", Online)
	port(h).SetOnline(false) // unknown: every member is
	port(h).SetOnline(true)
	port(h).SetAvailability("0xm3", Offline) // not a member
	want := []string{"availability offline", "availability online", "availability unknown", "availability online"}
	if got := about(drain(updates), id); !slices.Equal(got, want) {
		t.Fatalf("updates = %v, want %v", got, want)
	}
	if av := snapshot(h).Availability[TargetAggregate(id)]; av != Online {
		t.Fatalf("snapshot availability = %q, want online", av)
	}
}

func TestDetachedMemberIsNotCountedUntilItsHardwareReturns(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	ms := members(t, h, "0xm1", "0xm2")
	pair, _ := h.CreateAggregate("Veranda", ms, Any, "")
	alone, _ := h.CreateAggregate("Seul", ms[:1], Any, "")
	port(h).SetAvailability("0xm1", Online)
	port(h).SetAvailability("0xm2", Offline)
	occupied(h, "0xm1", true)
	occupied(h, "0xm2", false)
	savedBefore := *saved
	drain(updates)

	check := func(step string, wantPair, wantAlone []string) {
		t.Helper()
		us := drain(updates)
		if got := about(us, pair); !slices.Equal(got, wantPair) {
			t.Errorf("%s: Veranda updates = %v, want %v", step, got, wantPair)
		}
		if got := about(us, alone); !slices.Equal(got, wantAlone) {
			t.Errorf("%s: Seul updates = %v, want %v", step, got, wantAlone)
		}
		if as := snapshot(h).Aggregates; len(as) != 2 || !slices.Equal(as[0].Members, ms[:1]) || !slices.Equal(as[1].Members, ms) || !reflect.DeepEqual(*saved, savedBefore) {
			t.Errorf("%s: memberships changed: %+v", step, as)
		}
	}

	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm2"), motion("0xm3")}) // 0xm1 leaves the network
	check("detached",
		[]string{"value occupancy=false", "availability offline"},
		[]string{"value occupancy none", "availability unknown"})

	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2"), motion("0xm3")}) // and rejoins
	check("reattached",
		[]string{"value occupancy=true", "availability online"},
		[]string{"value occupancy=true", "availability online"})

	// 0xm1 dies for good; 0xm4 takes its place.
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm2"), motion("0xm3"), motion("0xm4")})
	port(h).SetAvailability("0xm4", Online)
	occupied(h, "0xm4", true)
	drain(updates)
	if err := h.Replace(ms[0].Device(), idOf(t, h, "0xm4")); err != nil {
		t.Fatal(err)
	}
	check("replaced",
		[]string{"value occupancy=true", "availability online"},
		[]string{"value occupancy=true", "availability online"})
}

func TestMissingMemberFunctionIsIgnoredButKept(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	ms := members(t, h, "0xm1", "0xm2")
	id, _ := h.CreateAggregate("Veranda", ms, Any, "")
	port(h).SetAvailability("0xm1", Online)
	port(h).SetAvailability("0xm2", Offline)
	occupied(h, "0xm1", true)
	occupied(h, "0xm2", false)
	savedBefore := *saved
	drain(updates)

	// A re-interview finds 0xm1 no longer provides occupancy.
	port(h).SyncDevices([]bridge.Device{bulb, thermometer("0xm1"), motion("0xm2"), motion("0xm3")})
	want := []string{"value occupancy=false", "availability offline"}
	if got := about(drain(updates), id); !slices.Equal(got, want) {
		t.Errorf("updates = %v, want %v", got, want)
	}
	if a := snapshot(h).Aggregates[0]; !slices.Equal(a.Members, ms) || a.Kind != "occupancy" || !reflect.DeepEqual(*saved, savedBefore) {
		t.Fatalf("the missing member left the definition: %+v", a)
	}

	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2"), motion("0xm3")})
	occupied(h, "0xm1", true)
	want = []string{"availability online", "value occupancy=true"}
	if got := about(drain(updates), id); !slices.Equal(got, want) {
		t.Errorf("after the Function returns: updates = %v, want %v", got, want)
	}
}

func TestDeletingDeviceRemovesItsMemberships(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	ms := members(t, h, "0xm1", "0xm2")
	pair, _ := h.CreateAggregate("Veranda", ms, Any, "")
	alone, _ := h.CreateAggregate("Seul", ms[:1], Any, "")
	other, _ := h.CreateAggregate("Living room", ms[1:], Any, "")
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm2"), motion("0xm3")})
	drain(updates)

	if err := h.Delete(ms[0].Device()); err != nil {
		t.Fatal(err)
	}
	want := map[AggregateID][]Target{pair: ms[1:], alone: {}, other: ms[1:]}
	check := func(what string, as []Aggregate) {
		t.Helper()
		if len(as) != len(want) {
			t.Fatalf("%s: %+v", what, as)
		}
		for _, a := range as {
			if !slices.Equal(a.Members, want[a.ID]) {
				t.Errorf("%s: %s members = %+v, want %+v", what, a.Name, a.Members, want[a.ID])
			}
		}
	}
	check("saved", *saved)
	var announced []Aggregate
	for _, u := range drain(updates) {
		if u.Kind == AggregatesChanged {
			announced = u.Aggregates
		}
	}
	check("announced", announced)
}

func TestNestedAggregateRefusals(t *testing.T) {
	h, updates, _, saved := aggregates(t)
	m1, m2 := members(t, h, "0xm1")[0], members(t, h, "0xm2")[0]
	light := TargetDevice(idOf(t, h, "0xbulb"), "light")
	office, _ := h.CreateAggregate("Office", []Target{m1}, "", "")
	living, _ := h.CreateAggregate("Living room", []Target{TargetAggregate(office), m2}, "", "")
	general, _ := h.CreateAggregate("General", []Target{TargetAggregate(living)}, "", "")
	before, savedBefore := snapshot(h).Aggregates, *saved
	drain(updates)

	if _, err := h.CreateAggregate("Kitchen", []Target{TargetAggregate("nope")}, "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown nested aggregate: err = %v, want ErrNotFound", err)
	}
	if _, err := h.CreateAggregate("Kitchen", []Target{TargetAggregate(general), light}, "", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("mixed kinds through nesting: err = %v, want ErrInvalid", err)
	}
	for name, ms := range map[string][]Target{
		"contains itself":         {TargetAggregate(office)},
		"contains a parent":       {m1, TargetAggregate(general)},
		"makes its parents mixed": {light},
	} {
		if err := h.EditAggregate(office, "Office", ms, "", ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if len(drain(updates)) != 0 || !reflect.DeepEqual(*saved, savedBefore) || !reflect.DeepEqual(snapshot(h).Aggregates, before) {
		t.Fatal("a refused definition changed, saved or announced the Aggregates")
	}
}

func TestNestedAggregateFollowsTheFunctionsItResolvesTo(t *testing.T) {
	h, updates, devices, saved := aggregates(t)
	port(h).SyncDevices([]bridge.Device{thermometer("0xt1"), thermometer("0xt2"), thermometer("0xt3")})
	var t1, t2, t3 Target
	for address, m := range map[string]*Target{"0xt1": &t1, "0xt2": &t2, "0xt3": &t3} {
		*m = TargetDevice(idOf(t, h, address), "temperature")
		port(h).Report(address, []bridge.Reading{{Function: "temperature", Capability: "temperature", Data: map[string]float64{"0xt1": 20, "0xt2": 30, "0xt3": 40}[address]}}, time.Now())
	}
	office, _ := h.CreateAggregate("Office", []Target{t1}, "", "")
	living, err := h.CreateAggregate("Living room", []Target{TargetAggregate(office), t1, t2}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	house, _ := h.CreateAggregate("House", []Target{TargetAggregate(living)}, "", "")
	s := snapshot(h)
	if !slices.ContainsFunc(s.Values, func(rv RefValue) bool {
		return rv.Ref == TargetAggregate(living).Ref("temperature") && rv.Value.Data == 25.0
	}) {
		t.Fatalf("values %+v, want Living room at 25: 0xt1 reachable twice weighs once", s.Values)
	}
	drain(updates)

	if err := h.EditAggregate(office, "Office", []Target{t3}, "", ""); err != nil {
		t.Fatal(err)
	}
	port(h).SetAvailability("0xt3", Online)
	if got, want := about(drain(updates), living), []string{"value temperature=30", "availability online"}; !slices.Equal(got, want) {
		t.Errorf("after editing Office: Living room updates = %v, want %v", got, want)
	}

	restarted := newHome(&fakeBridge{}, *devices, *saved, nil, nil, nil, nil, nil)
	for _, a := range snapshot(restarted).Aggregates {
		if a.ID == house && (a.Kind != "temperature" || len(a.Capabilities) == 0) {
			t.Errorf("restored House %+v, want derived through Living room", a)
		}
	}

	if err := h.DeleteAggregate(office); err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if got, want := about(us, living), []string{"value temperature=25", "availability unknown"}; !slices.Equal(got, want) {
		t.Errorf("after deleting Office: Living room updates = %v, want %v", got, want)
	}
	announced := us[slices.IndexFunc(us, func(u Update) bool { return u.Kind == AggregatesChanged })].Aggregates
	for what, as := range map[string][]Aggregate{"saved": *saved, "announced": announced} {
		i := slices.IndexFunc(as, func(a Aggregate) bool { return a.ID == living })
		if len(as) != 2 || i < 0 || !slices.Equal(as[i].Members, []Target{t1, t2}) {
			t.Errorf("%s: %+v, want Office gone from Living room", what, as)
		}
	}
}
