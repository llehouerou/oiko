package home

import (
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

type sent struct {
	address, function string
	values            map[string]any
	transition        time.Duration
}

type fakeBridge struct {
	mu    sync.Mutex
	sends []sent
	err   error
}

func (b *fakeBridge) Send(address, function string, values map[string]any, transition time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sends = append(b.sends, sent{address, function, values, transition})
	return b.err
}

func (b *fakeBridge) all() []sent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]sent(nil), b.sends...)
}

func ptr(f float64) *float64 { return &f }

// zb is the name of the one Bridge of most tests.
const zb = "zigbee2mqtt"

// newHome is a Home with b attached as zb.
func newHome(b Bridge, saved []Device, savedAggregates []Aggregate, savedFlags []SavedFlag,
	save func([]Device) error, saveAggregates func([]Aggregate) error, saveFlags func([]SavedFlag) error,
	history func(CommandRecord),
) *Home {
	h := New(saved, savedAggregates, savedFlags, nil, save, saveAggregates, saveFlags, nil, history)
	h.Attach(zb, b)
	return h
}

// port is how zb feeds h.
func port(h *Home) *Port { return &Port{h, zb} }

var bulb = bridge.Device{
	NativeAddress: "0xbulb",
	Name:          "Kitchen bulb",
	Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []Capability{
		{Key: "state", Type: Binary, Access: Access{Observable: true, Settable: true}},
		{Key: "brightness", Type: Numeric, Min: ptr(0), Max: ptr(254), Access: Access{Observable: true, Settable: true}},
	}}},
	Capabilities: []Capability{{Key: "linkquality", Type: Numeric, Category: Diagnostic, Access: Access{Observable: true}}},
}

var remote = bridge.Device{
	NativeAddress: "0xremote",
	Name:          "Remote",
	Functions: []bridge.Function{{Key: "button", Kind: "button", Capabilities: []Capability{
		{Key: "action", Type: Enum, Options: []string{"single", "double"}, Stateless: true, Access: Access{Observable: true}},
	}}},
}

func setup(t *testing.T) (*Home, *fakeBridge, <-chan Update, DeviceID) {
	t.Helper()
	b := &fakeBridge{}
	h := newHome(b, nil, nil, nil, nil, nil, nil, nil)
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{bulb, remote})
	snap, updates, cancel := h.Subscribe()
	t.Cleanup(cancel)
	for _, d := range snap.Devices {
		if d.NativeAddress == "0xbulb" {
			return h, b, updates, d.ID
		}
	}
	t.Fatal("bulb not registered")
	return nil, nil, nil, ""
}

// drain returns the Updates emitted so far.
func drain(ch <-chan Update) []Update {
	var got []Update
	for {
		select {
		case u := <-ch:
			got = append(got, u)
		default:
			return got
		}
	}
}

func kinds(us []Update) []UpdateKind {
	var ks []UpdateKind
	for _, u := range us {
		ks = append(ks, u.Kind)
	}
	return ks
}

func commandStatuses(us []Update) []CommandStatus {
	var ss []CommandStatus
	for _, u := range us {
		if u.Kind == CommandChanged {
			ss = append(ss, u.Command.Status)
		}
	}
	return ss
}

func TestReportDistinguishesChangeRefreshAndEvent(t *testing.T) {
	h, _, updates, _ := setup(t)
	at := time.Now()
	port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, at)
	port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, at.Add(time.Second))
	port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "unknown", Data: 1.0}}, at)
	port(h).Report("0xremote", []bridge.Reading{{Function: "button", Capability: "action", Data: "single"}}, at)

	got := drain(updates)
	want := []UpdateKind{ValueChanged, ValueRefreshed, EventOccurred}
	if ks := kinds(got); !slices.Equal(ks, want) {
		t.Fatalf("kinds = %v, want %v", ks, want)
	}
	if !got[1].Value.At.Equal(at.Add(time.Second)) {
		t.Errorf("refresh must reset the Value's age")
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Errorf("Seq not consecutive: %d then %d", got[i-1].Seq, got[i].Seq)
		}
	}
	snap, _, cancel := h.Subscribe()
	defer cancel()
	if len(snap.Values) != 1 {
		t.Errorf("snapshot holds %d Values, want 1 (Events have no Value)", len(snap.Values))
	}
	if len(snap.Events) != 1 || snap.Events[0].Value.Data != "single" {
		t.Errorf("snapshot events = %+v, want the last button press", snap.Events)
	}
}

func TestCommandIsConfirmedByMatchingReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, id := setup(t)
		if _, err := h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": true, "brightness": 128.0}}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if s := b.all(); len(s) != 1 || s[0].address != "0xbulb" || s[0].function != "light" {
			t.Fatalf("sends = %+v", s)
		}
		// A stale report does not confirm; the matching one does.
		port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: true}, {Function: "light", Capability: "brightness", Data: 10.0}}, time.Now())
		port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: true}, {Function: "light", Capability: "brightness", Data: 128.0}}, time.Now())
		if got := commandStatuses(drain(updates)); len(got) != 2 || got[0] != Pending || got[1] != Confirmed {
			t.Fatalf("statuses = %v", got)
		}
	})
}

// As a Hue light does: a blink leaves the ongoing effect reported, and the
// effect speed comes back at the device's resolution.
func TestCommandIsConfirmedDespiteTriggersAndRounding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &fakeBridge{}
		h := newHome(b, nil, nil, nil, nil, nil, nil, nil)
		port(h).SetOnline(true)
		rw := Access{Observable: true, Settable: true}
		port(h).SyncDevices([]bridge.Device{{NativeAddress: "0xstrip", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []Capability{
			{Key: "effect", Type: Enum, Options: []string{"blink", "colorloop"}, Triggers: []string{"blink"}, Access: rw},
			{Key: "effect_speed", Type: Numeric, Min: ptr(0), Max: ptr(1), Step: ptr(0.01), Access: rw},
		}}}}})
		snap, updates, cancel := h.Subscribe()
		defer cancel()
		id := snap.Devices[0].ID
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"effect": "blink", "effect_speed": 0.5}})
		synctest.Wait()
		port(h).Report("0xstrip", []bridge.Reading{{Function: "light", Capability: "effect", Data: "colorloop"}, {Function: "light", Capability: "effect_speed", Data: 0.4980392156862745}}, time.Now())
		if got := commandStatuses(drain(updates)); !slices.Equal(got, []CommandStatus{Pending, Confirmed}) {
			t.Fatalf("statuses = %v, want pending then confirmed", got)
		}
	})
}

func TestBurstOfCommandsIsThrottledLatestWins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, id := setup(t)
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"brightness": 10.0}, Transition: 300 * time.Millisecond})
		synctest.Wait() // first one goes out immediately
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"brightness": 20.0}, Transition: 300 * time.Millisecond})
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": false}})
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"brightness": 30.0}, Transition: 300 * time.Millisecond})
		synctest.Wait()
		if n := len(b.all()); n != 1 {
			t.Fatalf("sent %d times within the interval, want 1", n)
		}
		time.Sleep(sendInterval)
		synctest.Wait()
		s := b.all()
		if len(s) != 2 {
			t.Fatalf("sends = %+v", s)
		}
		if v := s[1].values; len(v) != 2 || v["brightness"] != 30.0 || v["state"] != false || s[1].transition != 300*time.Millisecond {
			t.Errorf("second send = %v over %v, want latest merged values and transition", v, s[1].transition)
		}
		want := []CommandStatus{Pending, Pending, Superseded, Pending, Superseded, Pending, Superseded} // the newer first
		if got := commandStatuses(drain(updates)); !slices.Equal(got, want) {
			t.Errorf("statuses = %v, want %v", got, want)
		}
	})
}

func TestCommandTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := setup(t)
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": true}})
		time.Sleep(commandTimeout)
		synctest.Wait()
		if got := commandStatuses(drain(updates)); len(got) != 2 || got[1] != TimedOut {
			t.Fatalf("statuses = %v", got)
		}
	})
}

func TestCommandFailsWhenBridgeCannotSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, id := setup(t)
		b.err = errors.New("broker down")
		h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": true}})
		synctest.Wait()
		if got := commandStatuses(drain(updates)); len(got) != 2 || got[1] != Failed {
			t.Fatalf("statuses = %v", got)
		}
	})
}

func TestCommandValidation(t *testing.T) {
	h, _, _, id := setup(t)
	for name, tc := range map[string]struct {
		function   string
		values     map[string]any
		transition time.Duration
		want       error
	}{
		"unknown function": {"cover", map[string]any{"state": true}, 0, ErrNotFound},
		"unknown key":      {"light", map[string]any{"color": true}, 0, ErrNotFound},
		"wrong type":       {"light", map[string]any{"state": "ON"}, 0, ErrInvalid},
		"out of range":     {"light", map[string]any{"brightness": 300.0}, 0, ErrInvalid},
		"not settable":     {"", map[string]any{"linkquality": 1.0}, 0, ErrInvalid},
		"empty":            {"light", map[string]any{}, 0, ErrInvalid},
		"negative fade":    {"light", map[string]any{"state": true}, -time.Second, ErrInvalid},
		"endless fade":     {"light", map[string]any{"state": true}, 6553600 * time.Millisecond, ErrInvalid},
		"toggle numeric":   {"light", map[string]any{"brightness": "toggle"}, 0, ErrInvalid},
		"two toggles":      {"light", map[string]any{"state": "toggle", "brightness": "toggle"}, 0, ErrInvalid},
	} {
		if _, err := h.Command(TargetDevice(id, tc.function), Request{Values: tc.values, Transition: tc.transition}); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if _, err := h.Command(Target{}, Request{Values: map[string]any{"state": true}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no target: err = %v, want %v", err, ErrInvalid)
	}
	for _, fade := range []time.Duration{900 * time.Second, maxTransition} {
		if _, err := h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": true}, Transition: fade}); err != nil {
			t.Errorf("%v fade: err = %v", fade, err)
		}
	}
	for _, c := range []Capability{
		{Key: "update", Type: Binary, Access: Access{Observable: true}},
		{Key: "effect", Type: Enum, Options: []string{"toggle"}, Access: settable},
		{Key: "label", Type: Text, Access: settable},
	} {
		if err := check(&c, toggle); !errors.Is(err, ErrInvalid) {
			t.Errorf("toggle %s: err = %v, want %v", c.Key, err, ErrInvalid)
		}
	}
	port(h).SetOnline(false)
	if _, err := h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": true}}); !errors.Is(err, ErrBridgeOffline) {
		t.Errorf("bridge offline: err = %v", err)
	}
}

func TestToggle(t *testing.T) {
	for name, tc := range map[string]struct {
		state   []any // reported before the toggles; none: unknown
		toggles int
		want    map[string]any // last transmitted
	}{
		"off turns on with the values":   {[]any{false}, 1, map[string]any{"state": true, "brightness": 128.0}},
		"on turns off alone":             {[]any{true}, 1, map[string]any{"state": false}},
		"unknown turns on":               {nil, 1, map[string]any{"state": true, "brightness": 128.0}},
		"second toggle undoes a pending": {[]any{false}, 2, map[string]any{"state": false}},
	} {
		synctest.Test(t, func(t *testing.T) {
			h, b, _, id := setup(t)
			for _, s := range tc.state {
				port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: s}}, time.Now())
			}
			for range tc.toggles {
				if _, err := h.Command(TargetDevice(id, "light"), Request{Values: map[string]any{"state": "toggle", "brightness": 128.0}}); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
			}
			time.Sleep(sendInterval)
			synctest.Wait()
			s := b.all()
			if got := s[len(s)-1].values; !maps.Equal(got, tc.want) {
				t.Errorf("%s: sent %v, want %v", name, got, tc.want)
			}
		})
	}
}

func TestDeviceIdentitySurvivesDetachAndRepairing(t *testing.T) {
	h, _, updates, id := setup(t)
	port(h).SetAvailability("0xbulb", Online)
	port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, time.Now())

	port(h).SyncDevices([]bridge.Device{remote}) // bulb left the network
	snap, _, cancel := h.Subscribe()
	cancel()
	var d Device
	for _, x := range snap.Devices {
		if x.ID == id {
			d = x
		}
	}
	if !d.Detached || snap.Availability[TargetDevice(id, "")] != Unknown || len(snap.Values) != 1 {
		t.Fatalf("detached device = %+v, availability %v, %d values", d, snap.Availability[TargetDevice(id, "")], len(snap.Values))
	}

	renamed := bulb
	renamed.Name = "0xbulb" // the Bridge's default name after re-pairing
	port(h).SyncDevices([]bridge.Device{renamed, remote})
	snap, _, cancel = h.Subscribe()
	cancel()
	for _, x := range snap.Devices {
		if x.NativeAddress == "0xbulb" && (x.ID != id || x.Detached || x.Name != "Kitchen bulb") {
			t.Fatalf("re-paired device = %+v, want same ID and Name, attached", x)
		}
	}
	if a := snap.Availability[TargetDevice(id, "")]; a != Online {
		t.Errorf("availability after reattach = %v", a)
	}
	drain(updates)
}

func TestBridgeOfflineMakesAvailabilityUnknown(t *testing.T) {
	h, _, updates, id := setup(t)
	port(h).SetAvailability("0xbulb", Online)
	port(h).SetOnline(false)
	var last Availability
	for _, u := range drain(updates) {
		if u.Kind == AvailabilityChanged && u.Target == TargetDevice(id, "") {
			last = u.Availability
		}
	}
	if last != Unknown {
		t.Errorf("availability = %v, want unknown", last)
	}
}
