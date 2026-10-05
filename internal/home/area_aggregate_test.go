package home

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

// derived is the Area Aggregate of area and kind in h's snapshot, if any.
func derived(h *Home, area AreaID, kind string) (Aggregate, bool) {
	for _, a := range snapshot(h).Aggregates {
		if a.ID == areaAggregateID(area, kind) {
			return a, true
		}
	}
	return Aggregate{}, false
}

func TestAreaAggregatesFollowTheAreas(t *testing.T) {
	h, disk := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2")})
	bulbID, m1, m2 := idOf(t, h, "0xbulb"), idOf(t, h, "0xm1"), idOf(t, h, "0xm2")
	living, _ := h.CreateArea("Living room")
	office, _ := h.CreateArea("Office")
	if _, ok := derived(h, living, "light"); ok {
		t.Fatal("an Area with no light has no light Aggregate")
	}
	for _, id := range []DeviceID{bulbID, m1, m2} {
		if err := h.SetArea(TargetDevice(id, ""), living); err != nil {
			t.Fatal(err)
		}
	}

	lights, ok := derived(h, living, "light")
	if !ok || !lights.Derived || lights.Area != living || lights.Name != "Living room lights" || lights.Kind != "light" ||
		!slices.Equal(lights.Members, []Target{TargetDevice(bulbID, "light")}) {
		t.Fatalf("light Aggregate: %+v", lights)
	}
	presence, ok := derived(h, living, "occupancy")
	if !ok || presence.Binary != Any || len(presence.Members) != 2 {
		t.Fatalf("occupancy Aggregate: %+v", presence)
	}
	if len(disk.aggregates) != 0 {
		t.Fatalf("an Area Aggregate is never saved: %+v", disk.aggregates)
	}

	port(h).Report("0xm2", []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: true}}, time.Now())
	if v := snapshotValue(h, TargetAggregate(presence.ID).Ref("occupancy")); v != true {
		t.Fatalf("occupancy: %v", v)
	}
	if _, err := h.Command(TargetAggregate(lights.ID), Request{Values: map[string]any{"state": true}}); err != nil {
		t.Fatalf("command: %v", err)
	}

	if err := h.RenameArea(living, "Lounge"); err != nil {
		t.Fatal(err)
	}
	if a, _ := derived(h, living, "light"); a.Name != "Lounge lights" {
		t.Fatalf("after rename: %q", a.Name)
	}

	// The bulb's light moves to the Office: the Lounge loses its light Aggregate.
	if err := h.SetArea(TargetDevice(bulbID, "light"), office); err != nil {
		t.Fatal(err)
	}
	if _, ok := derived(h, living, "light"); ok {
		t.Fatal("an Area Aggregate with no member left is gone")
	}
	if a, ok := derived(h, office, "light"); !ok || len(a.Members) != 1 {
		t.Fatalf("Office lights: %+v", a)
	}
	if err := h.SetArea(TargetDevice(bulbID, "light"), ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := derived(h, living, "light"); !ok {
		t.Fatal("back in the Lounge, under the same identity")
	}

	restarted, _ := areas(t, disk)
	port(restarted).SyncDevices([]bridge.Device{bulb, motion("0xm1"), motion("0xm2")})
	if _, ok := derived(restarted, living, "occupancy"); !ok {
		t.Fatal("after restart")
	}
}

// climate is a room sensor with one Function per quantity, as Netatmo's are.
func climate(address string) bridge.Device {
	fn := func(kind string) bridge.Function {
		return bridge.Function{Key: kind, Kind: kind, Capabilities: []Capability{{Key: kind, Type: Numeric, Category: Primary, Access: Access{Observable: true}}}}
	}
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{fn("temperature"), fn("humidity"), fn("co2")}}
}

var door = bridge.Device{NativeAddress: "0xdoor", Name: "0xdoor", Functions: []bridge.Function{{Key: "contact", Kind: "contact",
	Capabilities: []Capability{{Key: "contact", Type: Binary, Category: Primary, Access: Access{Observable: true}}}}}}

func TestAreasAggregateDoorsAndClimate(t *testing.T) {
	h, _ := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{climate("0xc1"), climate("0xc2"), door})
	living, _ := h.CreateArea("Living room")
	for _, a := range []string{"0xc1", "0xc2", "0xdoor"} {
		if err := h.SetArea(TargetDevice(idOf(t, h, a), ""), living); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for a, v := range map[string]float64{"0xc1": 20, "0xc2": 24} {
		port(h).Report(a, []bridge.Reading{
			{Function: "temperature", Capability: "temperature", Data: v},
			{Function: "humidity", Capability: "humidity", Data: v * 2},
			{Function: "co2", Capability: "co2", Data: v * 50},
		}, now)
	}
	port(h).Report("0xdoor", []bridge.Reading{{Function: "contact", Capability: "contact", Data: true}}, now)

	for kind, want := range map[string]any{
		"temperature": 22.0, // the mean of the room
		"humidity":    44.0,
		"co2":         1200.0, // its worst corner: airing is for the highest
		"contact":     true,   // a door open
	} {
		a, ok := derived(h, living, kind)
		if !ok {
			t.Fatalf("no %s Aggregate", kind)
		}
		if v := snapshotValue(h, TargetAggregate(a.ID).Ref(kind)); v != want {
			t.Errorf("%s: %v, want %v", kind, v, want)
		}
	}
}
func snapshotValue(h *Home, r Ref) any {
	for _, rv := range snapshot(h).Values {
		if rv.Ref == r {
			return rv.Value.Data
		}
	}
	return nil
}

func TestAnAreaAggregateIsNotEditedByHand(t *testing.T) {
	h, _ := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{motion("0xm1")})
	living, _ := h.CreateArea("Living room")
	if err := h.SetArea(TargetDevice(idOf(t, h, "0xm1"), ""), living); err != nil {
		t.Fatal(err)
	}
	id := areaAggregateID(living, "occupancy")
	for name, err := range map[string]error{
		"edit":   h.EditAggregate(id, "Mine", members(t, h, "0xm1"), Any, ""),
		"delete": h.DeleteAggregate(id),
		"icon":   h.SetIcon(TargetAggregate(id), "lamp"),
		"area":   h.SetArea(TargetAggregate(id), ""),
	} {
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
}

func TestDeletingAnAreaDropsItsAggregatesFromOthers(t *testing.T) {
	h, disk := areas(t, nil)
	port(h).SyncDevices([]bridge.Device{motion("0xm1"), motion("0xm2")})
	living, _ := h.CreateArea("Living room")
	if err := h.SetArea(TargetDevice(idOf(t, h, "0xm1"), ""), living); err != nil {
		t.Fatal(err)
	}
	house, err := h.CreateAggregate("House", append(members(t, h, "0xm2"), TargetAggregate(areaAggregateID(living, "occupancy"))), Any, "")
	if err != nil {
		t.Fatalf("an Area Aggregate nests in another: %v", err)
	}
	if got := h.Members(TargetAggregate(house)); len(got) != 2 {
		t.Fatalf("members through the Area Aggregate: %v", got)
	}
	if err := h.DeleteArea(living); err != nil {
		t.Fatal(err)
	}
	if _, ok := derived(h, living, "occupancy"); ok {
		t.Fatal("the Area's Aggregates go with it")
	}
	if got := disk.aggregates[0].Members; !slices.Equal(got, members(t, h, "0xm2")) {
		t.Fatalf("House still holds the deleted Area's Aggregate: %v", got)
	}
}
