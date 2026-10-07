package home

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

func areaIDs(as []Area) []AreaID {
	var ids []AreaID
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestAreasKeepTheirOrderAndSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	h := opened(t, dir)
	if snapshot(h).Areas == nil {
		t.Fatal("no Area is [] in JSON, not null")
	}
	living, _ := h.CreateArea("Living room", "sofa")
	office, _ := h.CreateArea("Office", "")
	if err := h.EditArea(office, "Dave's office", "desk"); err != nil {
		t.Fatal(err)
	}
	if err := h.OrderAreas([]AreaID{office, living}); err != nil {
		t.Fatal(err)
	}
	restarted := opened(t, dir)
	got := snapshot(restarted).Areas
	if !slices.Equal(areaIDs(got), []AreaID{office, living}) || got[0].Name != "Dave's office" || got[0].Icon != "desk" || got[1].Icon != "sofa" {
		t.Fatalf("after restart: %+v", got)
	}
	for _, order := range [][]AreaID{{office}, {office, living, living}, {office, "nope"}} {
		if err := h.OrderAreas(order); !errors.Is(err, ErrInvalid) {
			t.Errorf("OrderAreas(%v) = %v, want ErrInvalid", order, err)
		}
	}
	if _, err := h.CreateArea(" ", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("an empty name: %v", err)
	}
	for _, icon := range []string{"Sofa", "sofa bed", "../sofa"} {
		if _, err := h.CreateArea("Den", icon); !errors.Is(err, ErrInvalid) {
			t.Errorf("CreateArea with icon %q: %v, want ErrInvalid", icon, err)
		}
		if err := h.EditArea(living, "Living room", icon); !errors.Is(err, ErrInvalid) {
			t.Errorf("EditArea with icon %q: %v, want ErrInvalid", icon, err)
		}
	}
	if err := h.EditArea(living, "Living room", ""); err != nil || snapshot(h).Areas[1].Icon != "" {
		t.Errorf("clearing its icon: %v, %+v", err, snapshot(h).Areas[1])
	}
}

func TestAnAreasFileWrittenBeforeIconsLoadsUnchanged(t *testing.T) {
	dir := t.TempDir()
	before := `{"format":1,"data":[{"id":"a1","name":"Office","hiddenAggregates":["co2"],"columns":2,"layout":[{"tile":"flag:f1","col":1,"row":0,"width":1}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "areas.json"), []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	want := []Area{{ID: "a1", Name: "Office", HiddenAggregates: []string{"co2"}, Columns: 2, Layout: []Placement{{TargetFlag("f1"), Place{Col: 1, Width: 1}}}}}
	if got := snapshot(opened(t, dir)).Areas; !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %+v, want %+v", got, want)
	}
}

func TestAreaAssignmentSurvivesSyncRestartAndReplace(t *testing.T) {
	dir := t.TempDir()
	h := opened(t, dir)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room", "")
	office, _ := h.CreateArea("Office", "")
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

	restarted := opened(t, dir)
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
	dir := t.TempDir()
	h := opened(t, dir)
	port(h).SyncDevices([]bridge.Device{motion("0xm1")})
	living, _ := h.CreateArea("Living room", "")
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
	restarted := opened(t, dir)
	s := snapshot(restarted)
	if s.Flags[0].Area != living || s.Aggregates[0].Area != living {
		t.Fatalf("after edit and restart: flag in %q, aggregate in %q", s.Flags[0].Area, s.Aggregates[0].Area)
	}
}

func TestDeletingAnAreaLeavesWhatItHeldWithoutOne(t *testing.T) {
	dir := t.TempDir()
	h := opened(t, dir)
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1")})
	bulbID := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room", "")
	office, _ := h.CreateArea("Office", "")
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
	disk := onDisk(t, dir)
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
	dir := t.TempDir()
	h := opened(t, dir)
	living, _ := h.CreateArea("Living room", "")
	hidden := []Target{TargetDevice("d1", ""), TargetFlag("f1")}
	if err := h.SetAreaDisplay(living, hidden, []string{"occupancy"}); err != nil {
		t.Fatal(err)
	}
	if err := h.EditArea(living, "Lounge", ""); err != nil {
		t.Fatal(err)
	}
	restarted := opened(t, dir)
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
	dir := t.TempDir()
	h := opened(t, dir)
	living, _ := h.CreateArea("Living room", "")
	lamp, flag := TargetDevice("d1", ""), TargetFlag("f1")
	layout := []Placement{{lamp, Place{Col: 1, Row: 0, Width: 2, Height: 3}}, {flag, Place{Col: 0, Row: 2, Width: 1}}}
	if err := h.SetAreaLayout(living, 3, layout); err != nil {
		t.Fatal(err)
	}
	restarted := opened(t, dir)
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
		"past the edge":     {3, []Placement{{lamp, Place{Col: 2, Width: 2}}}},
		"no width":          {3, []Placement{{Tile: lamp}}},
		"negative row":      {3, []Placement{{lamp, Place{Row: -1, Width: 1}}}},
		"placed twice":      {3, []Placement{{lamp, Place{Width: 1}}, {lamp, Place{Row: 1, Width: 1}}}},
		"overlapping":       {3, []Placement{{lamp, Place{Width: 2}}, {flag, Place{Col: 1, Width: 1}}}},
		"overlapping below": {3, []Placement{{lamp, Place{Width: 1, Height: 2}}, {flag, Place{Row: 1, Width: 1}}}},
		"negative height":   {3, []Placement{{lamp, Place{Width: 1, Height: -1}}}},
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
	h := opened(t, t.TempDir())
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	living, _ := h.CreateArea("Living room", "")
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
	for _, err := range []error{h.EditArea("nope", "x", ""), h.DeleteArea("nope")} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown Area: %v", err)
		}
	}
}
