package home

import (
	"errors"
	"slices"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

// areaDisk is what a Home saved, as it would be on disk.
type areaDisk struct {
	devices    []Device
	aggregates []Aggregate
	flags      []SavedFlag
	areas      []Area
}

// areas is a Home with no Device yet, started from saved, whose saves land
// in the areaDisk it returns.
func areas(t *testing.T, saved *areaDisk) (*Home, *areaDisk) {
	t.Helper()
	disk := &areaDisk{}
	if saved == nil {
		saved = &areaDisk{}
	}
	h := New(saved.devices, saved.aggregates, saved.flags, saved.areas,
		func(d []Device) error { disk.devices = d; return nil },
		func(a []Aggregate) error { disk.aggregates = a; return nil },
		func(f []SavedFlag) error { disk.flags = f; return nil },
		func(a []Area) error { disk.areas = a; return nil },
		nil)
	h.Attach(zb, &fakeBridge{})
	port(h).SetOnline(true)
	return h, disk
}

func areaIDs(as []Area) []AreaID {
	var ids []AreaID
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestAreasKeepTheirOrderAndSurviveARestart(t *testing.T) {
	h, disk := areas(t, nil)
	if snapshot(h).Areas == nil {
		t.Fatal("no Area is [] in JSON, not null")
	}
	living, _ := h.CreateArea("Living room")
	office, _ := h.CreateArea("Office")
	if err := h.RenameArea(office, "Dave's office"); err != nil {
		t.Fatal(err)
	}
	if err := h.OrderAreas([]AreaID{office, living}); err != nil {
		t.Fatal(err)
	}
	restarted, _ := areas(t, disk)
	got := snapshot(restarted).Areas
	if !slices.Equal(areaIDs(got), []AreaID{office, living}) || got[0].Name != "Dave's office" {
		t.Fatalf("after restart: %+v", got)
	}
	for _, order := range [][]AreaID{{office}, {office, living, living}, {office, "nope"}} {
		if err := h.OrderAreas(order); !errors.Is(err, ErrInvalid) {
			t.Errorf("OrderAreas(%v) = %v, want ErrInvalid", order, err)
		}
	}
	if _, err := h.CreateArea(" "); !errors.Is(err, ErrInvalid) {
		t.Errorf("an empty name: %v", err)
	}
}

func TestAreaAssignmentSurvivesSyncRestartAndReplace(t *testing.T) {
	h, disk := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room")
	office, _ := h.CreateArea("Office")
	if err := h.SetArea(TargetDevice(id, ""), living); err != nil {
		t.Fatal(err)
	}
	if err := h.SetArea(TargetDevice(id, "light"), office); err != nil {
		t.Fatal(err)
	}
	check := func(when string, h *Home) {
		t.Helper()
		d := deviceByID(t, h, id)
		if d.Area != living || d.Functions[0].Area != office {
			t.Fatalf("%s: device in %q, light in %q", when, d.Area, d.Functions[0].Area)
		}
	}
	check("assigned", h)

	restarted, _ := areas(t, disk)
	port(restarted).SyncDevices([]bridge.Device{bulb}) // described again, with no Area
	check("after restart", restarted)

	newBulb := bulb
	newBulb.NativeAddress = "0xnew"
	port(restarted).SyncDevices([]bridge.Device{newBulb})
	if err := restarted.Replace(id, idOf(t, restarted, "0xnew")); err != nil {
		t.Fatal(err)
	}
	check("after replace", restarted)

	if err := restarted.SetArea(TargetDevice(id, "light"), ""); err != nil {
		t.Fatal(err)
	}
	if got := deviceByID(t, restarted, id).Functions[0].Area; got != "" {
		t.Fatalf("back to its Device's: %q", got)
	}
}

func TestFlagsAndAggregatesHaveAnArea(t *testing.T) {
	h, disk := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{motion("0xm1")})
	living, _ := h.CreateArea("Living room")
	flag, _ := h.CreateFlag("Guests")
	agg, _ := h.CreateAggregate("Motion", members(t, h, "0xm1"), Any, "")
	for _, target := range []Target{TargetFlag(flag), TargetAggregate(agg)} {
		if err := h.SetArea(target, living); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.EditAggregate(agg, "Motion", members(t, h, "0xm1"), All, ""); err != nil {
		t.Fatal(err)
	}
	restarted, _ := areas(t, disk)
	s := snapshot(restarted)
	if s.Flags[0].Area != living || s.Aggregates[0].Area != living {
		t.Fatalf("after edit and restart: flag in %q, aggregate in %q", s.Flags[0].Area, s.Aggregates[0].Area)
	}
}

func TestDeletingAnAreaLeavesWhatItHeldWithoutOne(t *testing.T) {
	h, disk := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1")})
	bulbID := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room")
	office, _ := h.CreateArea("Office")
	flag, _ := h.CreateFlag("Guests")
	agg, _ := h.CreateAggregate("Motion", members(t, h, "0xm1"), Any, "")
	for _, target := range []Target{TargetDevice(bulbID, ""), TargetDevice(bulbID, "light"), TargetFlag(flag), TargetAggregate(agg)} {
		if err := h.SetArea(target, living); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.SetArea(TargetDevice(idOf(t, h, "0xm1"), ""), office); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteArea(living); err != nil {
		t.Fatal(err)
	}
	s := snapshot(h)
	d := deviceByID(t, h, bulbID)
	i := slices.IndexFunc(s.Aggregates, func(a Aggregate) bool { return a.ID == agg })
	if d.Area != "" || d.Functions[0].Area != "" || s.Flags[0].Area != "" || s.Aggregates[i].Area != "" {
		t.Fatalf("still in the deleted Area: %+v / %+v / %+v", d, s.Flags[0], s.Aggregates[i])
	}
	if got := deviceByID(t, h, idOf(t, h, "0xm1")).Area; got != office {
		t.Fatalf("another Area's Device moved: %q", got)
	}
	if !slices.Equal(areaIDs(disk.areas), []AreaID{office}) {
		t.Fatalf("saved Areas: %+v", disk.areas)
	}
	for _, d := range disk.devices {
		if d.ID == bulbID && (d.Area != "" || d.Functions[0].Area != "") {
			t.Fatalf("saved bulb still in the deleted Area: %+v", d)
		}
	}
}

func TestAreaDisplaySurvivesARestart(t *testing.T) {
	h, disk := areas(t, nil)
	living, _ := h.CreateArea("Living room")
	hidden := []Target{TargetDevice("d1", ""), TargetFlag("f1")}
	if err := h.SetAreaDisplay(living, hidden, []string{"occupancy"}); err != nil {
		t.Fatal(err)
	}
	if err := h.RenameArea(living, "Lounge"); err != nil {
		t.Fatal(err)
	}
	restarted, _ := areas(t, disk)
	a := snapshot(restarted).Areas[0]
	if !slices.Equal(a.Hidden, hidden) || !slices.Equal(a.HiddenAggregates, []string{"occupancy"}) {
		t.Fatalf("after rename and restart: %+v", a)
	}
	if err := h.SetAreaDisplay(living, nil, []string{"pressure"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a kind no Area aggregates: %v", err)
	}
	if err := h.SetAreaDisplay("nope", nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown Area: %v", err)
	}
}

func TestAreaLayoutSurvivesARestart(t *testing.T) {
	h, disk := areas(t, nil)
	living, _ := h.CreateArea("Living room")
	lamp, flag := TargetDevice("d1", ""), TargetFlag("f1")
	layout := []Placement{{Tile: lamp, Col: 1, Row: 0, Width: 2, Height: 3}, {Tile: flag, Col: 0, Row: 2, Width: 1}}
	if err := h.SetAreaLayout(living, 3, layout); err != nil {
		t.Fatal(err)
	}
	restarted, _ := areas(t, disk)
	a := snapshot(restarted).Areas[0]
	if a.Columns != 3 || !slices.Equal(a.Layout, layout) {
		t.Fatalf("after restart: %+v", a)
	}
	for name, c := range map[string]struct {
		columns int
		layout  []Placement
	}{
		"no column":         {0, nil},
		"too many":          {maxColumns + 1, nil},
		"past the edge":     {3, []Placement{{Tile: lamp, Col: 2, Width: 2}}},
		"no width":          {3, []Placement{{Tile: lamp}}},
		"negative row":      {3, []Placement{{Tile: lamp, Row: -1, Width: 1}}},
		"placed twice":      {3, []Placement{{Tile: lamp, Width: 1}, {Tile: lamp, Row: 1, Width: 1}}},
		"overlapping":       {3, []Placement{{Tile: lamp, Width: 2}, {Tile: flag, Col: 1, Width: 1}}},
		"overlapping below": {3, []Placement{{Tile: lamp, Width: 1, Height: 2}, {Tile: flag, Row: 1, Width: 1}}},
		"negative height":   {3, []Placement{{Tile: lamp, Width: 1, Height: -1}}},
	} {
		if err := h.SetAreaLayout(living, c.columns, c.layout); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if err := h.SetAreaLayout("nope", 3, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown Area: %v", err)
	}
}

func TestSetAreaRefusals(t *testing.T) {
	h, _ := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room")
	for _, c := range []struct {
		target Target
		area   AreaID
		want   error
	}{
		{TargetDevice(id, ""), "nope", ErrNotFound},
		{TargetDevice(id, "nope"), living, ErrNotFound},
		{TargetDevice("nope", ""), living, ErrNotFound},
		{TargetFlag("nope"), living, ErrNotFound},
		{TargetAggregate("nope"), living, ErrNotFound},
		{Target{}, living, ErrInvalid},
	} {
		if err := h.SetArea(c.target, c.area); !errors.Is(err, c.want) {
			t.Errorf("SetArea(%s, %q) = %v, want %v", c.target, c.area, err, c.want)
		}
	}
	for _, err := range []error{h.RenameArea("nope", "x"), h.DeleteArea("nope")} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown Area: %v", err)
		}
	}
}
