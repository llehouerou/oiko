package home

import (
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/llehouerou/oiko/bridge"
)

func TestTargetKeyRoundTrips(t *testing.T) {
	for key, want := range map[string]Target{
		"device:d1":           TargetDevice("d1", ""),
		"device:d1/light":     TargetDevice("d1", "light"),
		"device:d1/switch/l2": TargetDevice("d1", "switch/l2"),
		"aggregate:a1":        TargetAggregate("a1"),
		"flag:f1":             TargetFlag("f1"),
	} {
		got, err := ParseTarget(key)
		if err != nil || got != want || got.Key() != key {
			t.Errorf("%q: parsed %v, %v; want %v", key, got, err, want)
		}
	}
	for _, key := range []string{"", "d1", "device:", "device:d1/", "room:r1", "flag:f1/on", "aggregate:"} {
		if _, err := ParseTarget(key); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err = %v, want invalid", key, err)
		}
	}

	// In JSON a Target is its Key, also as a map key; "" is none.
	in := struct {
		Ref   Ref                     `json:"ref"`
		None  Target                  `json:"none"`
		ByKey map[Target]Availability `json:"byKey"`
	}{TargetDevice("d1", "light").Ref("state"), Target{}, map[Target]Availability{TargetFlag("f1"): Online}}
	data, err := json.Marshal(in)
	if want := `{"ref":{"target":"device:d1/light","capability":"state"},"none":"","byKey":{"flag:f1":"online"}}`; err != nil || string(data) != want {
		t.Fatalf("marshalled %s, %v; want %s", data, err, want)
	}
	out := in
	out.Ref, out.ByKey = Ref{}, nil
	if err := json.Unmarshal(data, &out); err != nil || out.Ref != in.Ref || !out.None.IsZero() || out.ByKey[TargetFlag("f1")] != Online {
		t.Errorf("unmarshalled %+v, %v", out, err)
	}
	if err := json.Unmarshal([]byte(`{"none":"nope"}`), &out); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad key: err = %v, want invalid", err)
	}
}

// plug has a Function and a settable device-level Capability, so that the
// Device itself can be commanded.
var plug = bridge.Device{
	NativeAddress: "0xplug",
	Name:          "Plug",
	Functions: []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: []Capability{
		{Key: "state", Type: Binary, Access: settable},
	}}},
	Capabilities: []Capability{
		{Key: "power_on_behavior", Type: Enum, Options: []string{"off", "on", "previous"}, Category: Config, Access: settable},
	},
}

// Every kind of Target is commanded through the same contract: unknown ones
// and unknown Capabilities are not found, bad values are invalid, only those
// needing a Bridge are refused while it is offline, and an accepted Command is
// announced pending on its Target.
func TestEveryTargetKindHonoursTheCommandContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      func(id DeviceID, agg AggregateID, flag FlagID) Target
		unknown     Target
		good, bad   map[string]any
		needsBridge bool
	}{
		{"function", func(id DeviceID, _ AggregateID, _ FlagID) Target { return TargetDevice(id, "switch") },
			TargetDevice("nope", "switch"),
			map[string]any{"state": true}, map[string]any{"state": "yes"}, true},
		{"device", func(id DeviceID, _ AggregateID, _ FlagID) Target { return TargetDevice(id, "") },
			TargetDevice("nope", ""),
			map[string]any{"power_on_behavior": "on"}, map[string]any{"power_on_behavior": "later"}, true},
		{"aggregate", func(_ DeviceID, agg AggregateID, _ FlagID) Target { return TargetAggregate(agg) },
			TargetAggregate("nope"),
			map[string]any{"state": true}, map[string]any{"state": 1.0}, true},
		{"flag", func(_ DeviceID, _ AggregateID, flag FlagID) Target { return TargetFlag(flag) },
			TargetFlag("nope"),
			map[string]any{"on": true}, map[string]any{"on": "yes"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHome(&fakeBridge{}, nil, nil, nil, nil, nil, nil, nil)
				port(h).SetOnline(true)
				port(h).SyncDevices([]bridge.Device{plug})
				id := idOf(t, h, "0xplug")
				agg, err := h.CreateAggregate("Plugs", []Target{TargetDevice(id, "switch")}, "", "")
				if err != nil {
					t.Fatal(err)
				}
				flag, err := h.CreateFlag("Away")
				if err != nil {
					t.Fatal(err)
				}
				target := tc.target(id, agg, flag)

				if _, err := h.Command(tc.unknown, Request{Values: tc.good}); !errors.Is(err, ErrNotFound) {
					t.Errorf("unknown Target: %v, want not found", err)
				}
				if _, err := h.Command(target, Request{Values: map[string]any{"nope": true}}); !errors.Is(err, ErrNotFound) {
					t.Errorf("unknown Capability: %v, want not found", err)
				}
				if _, err := h.Command(target, Request{Values: tc.bad}); !errors.Is(err, ErrInvalid) {
					t.Errorf("bad value: %v, want invalid", err)
				}
				for key := range tc.good {
					if c, err := h.Capability(target.Ref(key)); err != nil || c.Key != key {
						t.Errorf("Capability %s: %+v, %v", key, c, err)
					}
				}
				if _, err := h.Capability(target.Ref("")); err != nil {
					t.Errorf("Availability: %v", err)
				}
				for _, ref := range []Ref{target.Ref("nope"), tc.unknown.Ref("")} {
					if _, err := h.Capability(ref); !errors.Is(err, ErrNotFound) {
						t.Errorf("Capability of %v: %v, want not found", ref, err)
					}
				}

				port(h).SetOnline(false)
				_, err = h.Command(target, Request{Values: tc.good})
				port(h).SetOnline(true)
				if refused := errors.Is(err, ErrBridgeOffline); refused != tc.needsBridge || !refused && err != nil {
					t.Errorf("Bridge offline: %v, needs the Bridge %v", err, tc.needsBridge)
				}

				_, updates, cancel := h.Subscribe()
				defer cancel()
				cid, err := h.Command(target, Request{Values: tc.good})
				if err != nil {
					t.Fatal(err)
				}
				for _, u := range drain(updates) {
					if u.Kind == CommandChanged && u.Command.ID == cid {
						if u.Command.Status != Pending || u.Command.Target != target {
							t.Errorf("first announced %+v, want pending on %+v", *u.Command, target)
						}
						return
					}
				}
				t.Errorf("Command %s never announced", cid)
			})
		})
	}
}
