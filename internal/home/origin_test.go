package home

import (
	"encoding/json"
	"slices"
	"strings"
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

		api, err := h.Command(TargetDevice(idOf(t, h, "0xl1"), "light"), Request{Values: map[string]any{"state": true}, Transition: time.Second})
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
			case c.ID == api:
				return "api"
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
		want := []string{"api pending", "aggregate pending", "member pending", "api superseded", "member pending",
			"member confirmed", "member confirmed", "aggregate confirmed"}
		if !slices.Equal(got, want) {
			t.Errorf("commands = %q, want %q", got, want)
		}
		if a := announced[0]; a.Origin != (Origin{}) {
			t.Errorf("api command origin = %+v", a.Origin)
		}
		if a := announced[1]; a.Origin != run || a.AggregateCommand != "" {
			t.Errorf("aggregate command = %+v", a)
		}
		if r := kept[0]; !r.Time.Equal(time.Now()) || r.Transition != 1 || r.Values["state"] != true {
			t.Errorf("api record = %+v", r)
		}
		if r := kept[1]; r.Values["state"] != true { // the toggle, resolved
			t.Errorf("aggregate record = %+v", r)
		}

		data, _ := json.Marshal(announced[0])
		if !strings.Contains(string(data), `"origin":"api"`) {
			t.Errorf("api command JSON = %s", data)
		}
		data, _ = json.Marshal(announced[1])
		var back CommandState
		if err := json.Unmarshal(data, &back); err != nil || back != announced[1] {
			t.Errorf("round trip of %s = %+v, %v", data, back, err)
		}
	})
}
