package home

import (
	"encoding/json"
	"slices"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge"
)

func TestCommandsCarryTheirOriginAndAreKept(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var kept []CommandRecord
		h := newHome(&fakeBridge{}, nil, nil, nil, nil, nil, nil, func(c CommandRecord) { kept = append(kept, c) })
		port(h).SetOnline(true)
		port(h).SyncDevices([]bridge.Device{lamp("0xl1", 1, 254), lamp("0xl2", 0, 100)})
		both, err := h.CreateAggregate("Both", lightMembers(t, h, "0xl1", "0xl2"), "", "")
		if err != nil {
			t.Fatal(err)
		}
		_, updates, cancel := h.Subscribe()
		defer cancel()
		run := Origin{Automation: "auto", Step: "step", Run: uuid.NewV7()}
		alice := Origin{Person: "alice"}

		byHand, err := h.Command(TargetDevice(idOf(t, h, "0xl1"), "light"), Request{Values: map[string]any{"state": true}, Transition: time.Second, Origin: alice})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Command(TargetDevice(idOf(t, h, "0xl1"), "light"), Request{Values: map[string]any{"state": "ON"}, Origin: run}); err == nil {
			t.Fatal("a wrong value was accepted")
		}
		relay, err := h.Command(TargetAggregate(both), Request{Values: map[string]any{"state": "toggle"}, Origin: run})
		if err != nil {
			t.Fatal(err)
		}
		port(h).Report("0xl1", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, time.Now())
		port(h).Report("0xl2", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, time.Now())

		// Updates and records tell the same story, refusals left out.
		var announced []CommandState
		for _, u := range drain(updates) {
			if u.Kind == CommandChanged {
				announced = append(announced, *u.Command)
			}
		}
		if len(kept) != len(announced) {
			t.Fatalf("%d records for %d command Updates", len(kept), len(announced))
		}
		name := func(c CommandState) string {
			switch {
			case c.ID == byHand:
				return "alice"
			case c.ID == relay:
				return "aggregate"
			case c.AggregateCommand == relay && c.Origin == run:
				return "member"
			}
			return "?"
		}
		var got []string
		for i, c := range announced {
			if kept[i].CommandState != c {
				t.Errorf("record %d = %+v, Update %+v", i, kept[i].CommandState, c)
			}
			got = append(got, name(c)+" "+string(c.Status))
		}
		want := []string{"alice pending", "aggregate pending", "member pending", "alice superseded", "member pending",
			"member confirmed", "member confirmed", "aggregate confirmed"}
		if !slices.Equal(got, want) {
			t.Errorf("commands = %q, want %q", got, want)
		}
		if a := announced[0]; a.Origin != alice {
			t.Errorf("Alice's command origin = %+v", a.Origin)
		}
		if a := announced[1]; a.Origin != run || a.AggregateCommand != "" {
			t.Errorf("aggregate command = %+v", a)
		}
		if r := kept[0]; !r.Time.Equal(time.Now()) || r.Transition != 1 || r.Values["state"] != true {
			t.Errorf("Alice's record = %+v", r)
		}
		if r := kept[1]; r.Values["state"] != true { // the toggle, resolved
			t.Errorf("aggregate record = %+v", r)
		}
	})
}

func TestOriginJSON(t *testing.T) {
	run := uuid.NewV7()
	for _, c := range []struct {
		o    Origin
		json string
	}{
		{Origin{}, `"unknown"`},
		{Origin{Person: "alice"}, `{"person":"alice"}`},
		{Origin{Kiosk: "hall"}, `{"kiosk":"hall"}`},
		{Origin{Program: "nodered"}, `{"program":"nodered"}`},
		{Origin{Automation: "auto", Step: "step", Run: run}, `{"automation":"auto","step":"step","run":"` + run.String() + `"}`},
	} {
		data, err := json.Marshal(c.o)
		if err != nil || string(data) != c.json {
			t.Errorf("%+v = %s, %v; want %s", c.o, data, err, c.json)
		}
		back := Origin{Program: "left over"}
		if err := json.Unmarshal([]byte(c.json), &back); err != nil || back != c.o {
			t.Errorf("%s read back as %+v, %v", c.json, back, err)
		}
	}
}
