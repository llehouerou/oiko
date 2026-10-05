// Package bridgetest tests a type of Bridge against Oiko's own home: what the
// Bridge hands its Port is applied by the same rules as in Oiko, and Commands
// reach it as Oiko issues them.
//
//	h := bridgetest.New(b)
//	go b.Run(ctx, h.Port()) // or hand h.Port() to the part under test
//	…
//	if v, _ := h.Value("0xbulb", "light", "state"); v != true { … }
//	if err := h.Command("0xbulb", "light", map[string]any{"state": false}); err != nil { … }
//
// Reads give the home as it is at the call: a test following a Bridge that
// runs on its own waits for it, with testing/synctest for instance.
package bridgetest

import (
	"fmt"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// name is the Bridge's name in the home.
const name = "test"

// Home is a home with one Bridge attached, offline until it says otherwise.
type Home struct {
	h    *home.Home
	port *home.Port
}

// New attaches b, which receives the Commands; nil if the test issues none.
func New(b bridge.Bridge) *Home {
	h := home.New(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return &Home{h: h, port: h.Attach(name, b)}
}

// Port is what the Bridge feeds the home through.
func (h *Home) Port() bridge.Port { return h.port }

// Online is whether the Bridge last said it is online.
func (h *Home) Online() bool { return h.snapshot().Bridges[name] }

// Replayed is whether the Bridge has handed its Replay.
func (h *Home) Replayed() bool { return len(h.h.Waiting()) == 0 }

// Devices are those the Bridge last listed, as the home holds them.
func (h *Home) Devices() []bridge.Device {
	var devices []bridge.Device
	for _, d := range h.snapshot().Devices {
		if d.Detached {
			continue
		}
		b := bridge.Device{NativeAddress: d.NativeAddress, Name: d.Name, Model: d.Model, Vendor: d.Vendor, Capabilities: d.Capabilities}
		for _, f := range d.Functions {
			b.Functions = append(b.Functions, bridge.Function{Key: f.Key, Kind: f.Kind, Capabilities: f.Capabilities})
		}
		devices = append(devices, b)
	}
	return devices
}

// Availability is that of the Device at address as Oiko shows it: unknown
// while the Bridge is offline, or for a Device it does not list.
func (h *Home) Availability(address string) bridge.Availability {
	s := h.snapshot()
	if id, ok := device(s, address); ok {
		if a, ok := s.Availability[home.TargetDevice(id, "")]; ok {
			return a
		}
	}
	return bridge.Unknown
}

// Value is the current Value of Capability capability of Function function
// ("" for the Device itself) of the Device at address; false if unknown.
func (h *Home) Value(address, function, capability string) (any, bool) {
	s := h.snapshot()
	return find(s, s.Values, address, function, capability)
}

// Event is the last occurrence of Stateless Capability capability, as Value.
func (h *Home) Event(address, function, capability string) (any, bool) {
	s := h.snapshot()
	return find(s, s.Events, address, function, capability)
}

// Command issues values to Function function ("" for the Device itself) of
// the Device at address, as Oiko does: refused when the Bridge is offline or
// the Capabilities do not accept them, otherwise sent through the Bridge's
// Send. It returns once the Command is over: nil when a Report confirms it.
func (h *Home) Command(address, function string, values map[string]any) error {
	snap, updates, cancel := h.h.Subscribe()
	defer cancel()
	id, ok := device(snap, address)
	if !ok {
		return fmt.Errorf("no Device at %s", address)
	}
	cmd, err := h.h.Command(home.TargetDevice(id, function), home.Request{Values: values})
	if err != nil {
		return err
	}
	for u := range updates {
		if c := u.Command; c != nil && c.ID == cmd && c.Status != home.Pending {
			if c.Status != home.Confirmed {
				return fmt.Errorf("command %s", c.Status)
			}
			return nil
		}
	}
	return fmt.Errorf("command %s: outcome lost", cmd) // its updates fell too far behind
}

func (h *Home) snapshot() home.Snapshot {
	s, _, cancel := h.h.Subscribe()
	cancel()
	return s
}

// device finds the Device the Bridge lists at address.
func device(s home.Snapshot, address string) (home.DeviceID, bool) {
	for _, d := range s.Devices {
		if d.NativeAddress == address && !d.Detached {
			return d.ID, true
		}
	}
	return "", false
}

func find(s home.Snapshot, values []home.RefValue, address, function, capability string) (any, bool) {
	id, ok := device(s, address)
	if !ok {
		return nil, false
	}
	ref := home.TargetDevice(id, function).Ref(capability)
	for _, v := range values {
		if v.Ref == ref {
			return v.Value.Data, true
		}
	}
	return nil, false
}
