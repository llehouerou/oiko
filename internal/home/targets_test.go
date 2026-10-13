package home

import "testing"

func TestTargetsFollowTheHome(t *testing.T) {
	lamp := Device{ID: "lamp", Name: "Desk lamp", Functions: []Function{{Key: "light", Kind: "light"}}}
	cam := Device{ID: "cam", Name: "Porch camera", Functions: []Function{{Key: "camera", Kind: Camera}}}
	ts := TargetsIn(Snapshot{
		Devices:    []Device{lamp, cam},
		Aggregates: []Aggregate{{ID: "all", Name: "All lights"}},
		Flags:      []Flag{{ID: "guests", Name: "Guests"}},
		Presences:  []Presence{{ID: HomePresence, Name: "Home"}, {ID: "alice", Name: "Alice"}},
	})
	for target, name := range map[Target]string{
		TargetDevice("lamp", ""):      "Desk lamp",
		TargetDevice("lamp", "light"): "Desk lamp",
		TargetAggregate("all"):        "All lights",
		TargetFlag("guests"):          "Guests",
		TargetPresence(HomePresence):  "Home",
		TargetPresence("alice"):       "Alice",
	} {
		if !ts.Has(target) || ts.Name(target) != name {
			t.Errorf("%s: has %v, name %q; want %q", target, ts.Has(target), ts.Name(target), name)
		}
	}
	for _, gone := range []Target{{}, TargetDevice("lamp", "switch"), TargetDevice("nope", ""), TargetFlag("nope"), TargetPresence("bob")} {
		if ts.Has(gone) || ts.Name(gone) != "" {
			t.Errorf("%s: known", gone)
		}
	}
	if f, ok := ts.Function(TargetDevice("cam", "camera")); !ok || f.Kind != Camera {
		t.Errorf("camera Function = %+v, %v", f, ok)
	}
	if _, ok := ts.Function(TargetDevice("cam", "")); ok {
		t.Error("a Device itself is a Function")
	}

	before := ts
	if ts.Apply(Update{Kind: ValueChanged}) {
		t.Error("a Value changed the Targets")
	}
	if !ts.Apply(Update{Kind: DevicesChanged, Devices: []Device{lamp}}) || ts.Has(TargetDevice("cam", "camera")) || !ts.Has(TargetFlag("guests")) {
		t.Error("the camera's deletion not followed alone")
	}
	if !before.Has(TargetDevice("cam", "camera")) {
		t.Error("a copy changed with the original")
	}
}
