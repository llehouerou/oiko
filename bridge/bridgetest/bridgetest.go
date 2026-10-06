// Package bridgetest tests a type of Bridge against Oiko's own home: what the
// Bridge hands its Port is applied by the same rules as in Oiko, and Commands
// reach it as Oiko issues them.
//
//	h := bridgetest.New(b)
//	go b.Run(ctx, h.Port()) // or hand h.Port() to the part under test
//	…
//	if v, _ := h.Value("0xbulb", "light", "state"); v.Data != true { … }
//	if err := h.Command("0xbulb", "light", map[string]any{"state": false}); err != nil { … }
//	if pic, err := h.Picture("cam1", "camera"); err != nil { … } // a Bridge with bridge.Cameras
//	if rs, err := h.Recordings("cam1", "camera", from, to); err != nil { … } // with bridge.Recordings
//
// Env builds what Oiko hands a type's Module.New, to test its creation.
//
// Reads give the home as it is at the call: a test following a Bridge that
// runs on its own waits for it, with testing/synctest for instance.
package bridgetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// name is the Bridge's name in the home.
const name = "test"

// What became of a Command, for errors.Is.
var (
	ErrRefused  = errors.New("refused") // offline, unknown Function or Capability, or a value it does not accept
	ErrFailed   = errors.New("failed")  // the Bridge's Send returned an error
	ErrTimedOut = errors.New("timed out")
)

// Env is what Oiko hands a Bridge named name, or one of its Commands: its
// section of the configuration, config, as Oiko hands it (without "type"),
// a data directory of its own, removed after the test, and a logger writing
// into the test's output.
func Env(t testing.TB, name, config string) bridge.Env {
	return bridge.Env{Name: name, Config: json.RawMessage(config), DataDir: t.TempDir(),
		Log: slog.New(slog.NewTextHandler(t.Output(), nil)).With("bridge", name)}
}

// Home is a home with one Bridge attached, offline until it says otherwise.
type Home struct {
	h    *home.Home
	port port
}

// port is the home's, remembering the order the Bridge last listed its
// Devices in.
type port struct {
	*home.Port
	mu     *sync.Mutex
	listed *[]string // Native Addresses
}

func (p port) SyncDevices(devices []bridge.Device) {
	p.mu.Lock()
	*p.listed = nil
	for _, d := range devices {
		*p.listed = append(*p.listed, d.NativeAddress)
	}
	p.mu.Unlock()
	p.Port.SyncDevices(devices)
}

// New attaches b, which receives the Commands; nil if the test issues none.
func New(b bridge.Bridge) *Home {
	h := home.New(nil)
	return &Home{h: h, port: port{Port: h.Attach(name, b), mu: &sync.Mutex{}, listed: &[]string{}}}
}

// Port is what the Bridge feeds the home through.
func (h *Home) Port() bridge.Port { return h.port }

// Online is whether the Bridge last said it is online.
func (h *Home) Online() bool { return h.snapshot().Bridges[name] }

// Replayed is whether the Bridge has handed its Replay.
func (h *Home) Replayed() bool { return len(h.h.Waiting()) == 0 }

// Devices are those the Bridge last listed, in its order, as the home holds
// them.
func (h *Home) Devices() []bridge.Device {
	h.port.mu.Lock()
	listed := slices.Clone(*h.port.listed)
	h.port.mu.Unlock()
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
	slices.SortFunc(devices, func(a, b bridge.Device) int {
		return slices.Index(listed, a.NativeAddress) - slices.Index(listed, b.NativeAddress)
	})
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

// Value is a Capability's data as Oiko holds it, and when the device sent
// it: the at of the Report that brought it.
type Value struct {
	Data any
	At   time.Time
}

// Value is the current Value of Capability capability of Function function
// ("" for the Device itself) of the Device at address; false if unknown.
func (h *Home) Value(address, function, capability string) (Value, bool) {
	s := h.snapshot()
	return find(s, s.Values, address, function, capability)
}

// Event is the last occurrence of Stateless Capability capability, as Value.
func (h *Home) Event(address, function, capability string) (Value, bool) {
	s := h.snapshot()
	return find(s, s.Events, address, function, capability)
}

// Command issues values to Function function ("" for the Device itself) of
// the Device at address, as Oiko does: refused (ErrRefused) when the Device
// is not listed, the Bridge is offline or the Capabilities do not accept
// them, otherwise sent through the Bridge's Send. It returns once the Command
// is over: nil when a Report confirms it, else ErrFailed or ErrTimedOut.
func (h *Home) Command(address, function string, values map[string]any) error {
	snap, updates, cancel := h.h.Subscribe()
	defer cancel()
	id, ok := device(snap, address)
	if !ok {
		return fmt.Errorf("%w: no Device at %s", ErrRefused, address)
	}
	cmd, err := h.h.Command(home.TargetDevice(id, function), home.Request{Values: values})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRefused, err)
	}
	for u := range updates {
		if c := u.Command; c != nil && c.ID == cmd && c.Status != home.Pending {
			switch c.Status {
			case home.Confirmed:
				return nil
			case home.Failed:
				return ErrFailed
			case home.TimedOut:
				return ErrTimedOut
			}
			return fmt.Errorf("command %s", c.Status) // superseded by a concurrent one
		}
	}
	return fmt.Errorf("command %s: outcome lost", cmd) // its updates fell too far behind
}

// Picture reads the Picture of camera Function function of the Device at
// address as Oiko does, through the Bridge's bridge.Cameras: refused
// (ErrRefused) when the Device is not listed, the Function is no camera, the
// Bridge has no cameras or is offline; else what Picture returned.
func (h *Home) Picture(address, function string) (bridge.Picture, error) {
	id, ok := device(h.snapshot(), address)
	if !ok {
		return bridge.Picture{}, fmt.Errorf("%w: no Device at %s", ErrRefused, address)
	}
	cameras, native, err := home.CameraBridge[bridge.Cameras](h.h, home.TargetDevice(id, function))
	if err != nil {
		return bridge.Picture{}, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	return cameras.Picture(context.Background(), native, function)
}

// Recordings lists the Recordings of camera Function function of the Device
// at address within [from, to] as Oiko does, through the Bridge's
// bridge.Recordings: refused (ErrRefused) as Picture is, or when the Bridge
// has no Recordings; else what Recordings returned.
func (h *Home) Recordings(address, function string, from, to time.Time) ([]bridge.Recording, error) {
	id, ok := device(h.snapshot(), address)
	if !ok {
		return nil, fmt.Errorf("%w: no Device at %s", ErrRefused, address)
	}
	recordings, native, err := home.CameraBridge[bridge.Recordings](h.h, home.TargetDevice(id, function))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	return recordings.Recordings(context.Background(), native, function, from, to)
}

// RecordingMedia fetches part of Recording id as Oiko does, passing header
// on, refused as Recordings is; else what RecordingMedia returned, whose body
// the caller closes.
func (h *Home) RecordingMedia(address, function, id string, part bridge.RecordingPart, header http.Header) (*http.Response, error) {
	dev, ok := device(h.snapshot(), address)
	if !ok {
		return nil, fmt.Errorf("%w: no Device at %s", ErrRefused, address)
	}
	recordings, native, err := home.CameraBridge[bridge.Recordings](h.h, home.TargetDevice(dev, function))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	return recordings.RecordingMedia(context.Background(), native, function, id, part, header)
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

func find(s home.Snapshot, values []home.RefValue, address, function, capability string) (Value, bool) {
	id, ok := device(s, address)
	if !ok {
		return Value{}, false
	}
	ref := home.TargetDevice(id, function).Ref(capability)
	for _, v := range values {
		if v.Ref == ref {
			return Value{v.Value.Data, v.Value.At}, true
		}
	}
	return Value{}, false
}
