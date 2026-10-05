package home

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

var settable = Access{Observable: true, Settable: true}

func lamp(address string, minBrightness, maxBrightness float64, caps ...Capability) bridge.Device {
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: append([]Capability{
		{Key: "state", Type: Binary, Category: Primary, Access: settable},
		{Key: "brightness", Type: Numeric, Min: ptr(minBrightness), Max: ptr(maxBrightness), Category: Primary, Access: settable},
	}, caps...)}}}
}

// lights is a Home with lamps 0xl1 (brightness 1–254) and 0xl2 (0–100), and
// the Aggregate "Parents' bedroom" of both.
func lights(t *testing.T) (*Home, *fakeBridge, <-chan Update, AggregateID) {
	t.Helper()
	b := &fakeBridge{}
	h := newHome(b, nil, nil, nil, nil, nil, nil, nil)
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{lamp("0xl1", 1, 254), lamp("0xl2", 0, 100)})
	id, err := h.CreateAggregate("Parents' bedroom", lightMembers(t, h, "0xl1", "0xl2"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, updates, cancel := h.Subscribe()
	t.Cleanup(cancel)
	return h, b, updates, id
}

func lightMembers(t *testing.T, h *Home, addresses ...string) []Target {
	t.Helper()
	var ms []Target
	for _, a := range addresses {
		ms = append(ms, TargetDevice(idOf(t, h, a), "light"))
	}
	return ms
}

// aggregateCommands returns the statuses of the Commands on Aggregate id.
func aggregateCommands(us []Update, id AggregateID) []CommandStatus {
	var ss []CommandStatus
	for _, u := range us {
		if u.Kind == CommandChanged && u.Command.Target == TargetAggregate(id) {
			ss = append(ss, u.Command.Status)
		}
	}
	return ss
}

func lit(h *Home, address string, brightness float64) {
	port(h).Report(address, []bridge.Reading{{Function: "light", Capability: "state", Data: true}, {Function: "light", Capability: "brightness", Data: brightness}}, time.Now())
}

func TestAggregateCapabilitiesIntersectSettableAndBounds(t *testing.T) {
	effect := func(options ...string) Capability {
		return Capability{Key: "effect", Type: Enum, Options: options, Category: Primary, Access: settable}
	}
	colorTemp := func(access Access) Capability {
		return Capability{Key: "color_temp", Type: Numeric, Min: ptr(150), Max: ptr(500), Category: Primary, Access: access}
	}
	h := newHome(&fakeBridge{}, nil, nil, nil, nil, nil, nil, nil)
	port(h).SyncDevices([]bridge.Device{
		lamp("0xl1", 1, 254, effect("blink", "breathe"), colorTemp(settable)),
		lamp("0xl2", 0, 100, effect("blink", "okay"), colorTemp(Access{Observable: true})),
	})
	if _, err := h.CreateAggregate("Parents' bedroom", lightMembers(t, h, "0xl1", "0xl2"), "", ""); err != nil {
		t.Fatal(err)
	}
	caps := map[string]Capability{}
	for _, c := range snapshot(h).Aggregates[0].Capabilities {
		caps[c.Key] = c
	}
	if c := caps["state"]; !c.Access.Settable {
		t.Errorf("state = %+v, want settable: every member's is", c)
	}
	if c := caps["brightness"]; !c.Access.Settable || *c.Min != 1 || *c.Max != 100 {
		t.Errorf("brightness = %+v, want settable within 1–100", c)
	}
	if c := caps["color_temp"]; c.Access.Settable || !c.Access.Observable {
		t.Errorf("color_temp = %+v, want observable only: one member's is not settable", c)
	}
	if c := caps["effect"]; !slices.Equal(c.Options, []string{"blink"}) {
		t.Errorf("effect options = %v, want those of every member", c.Options)
	}
}

func TestAggregateCommandIsRefusedAsAWhole(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, id := lights(t)
		port(h).SyncDevices([]bridge.Device{
			lamp("0xl1", 1, 254, Capability{Key: "color_temp", Type: Numeric, Category: Primary, Access: settable}),
			lamp("0xl2", 0, 100, Capability{Key: "color_temp", Type: Numeric, Category: Primary, Access: Access{Observable: true}}),
		})
		drain(updates)
		for name, tc := range map[string]struct {
			id         AggregateID
			values     map[string]any
			transition time.Duration
			want       error
		}{
			"unknown aggregate":      {"nope", map[string]any{"state": true}, 0, ErrNotFound},
			"unknown key":            {id, map[string]any{"state": true, "color": 1.0}, 0, ErrNotFound},
			"rejected by one member": {id, map[string]any{"state": true, "brightness": 200.0}, 0, ErrInvalid},
			"settable on one member": {id, map[string]any{"color_temp": 300.0}, 0, ErrInvalid},
			"wrong type":             {id, map[string]any{"state": "ON"}, 0, ErrInvalid},
			"empty":                  {id, map[string]any{}, 0, ErrInvalid},
			"endless fade":           {id, map[string]any{"state": true}, 6553600 * time.Millisecond, ErrInvalid},
			"toggle numeric":         {id, map[string]any{"brightness": "toggle"}, 0, ErrInvalid},
		} {
			if _, err := h.Command(TargetAggregate(tc.id), Request{Values: tc.values, Transition: tc.transition}); !errors.Is(err, tc.want) {
				t.Errorf("%s: err = %v, want %v", name, err, tc.want)
			}
		}
		port(h).SetOnline(false)
		if _, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true}}); !errors.Is(err, ErrBridgeOffline) {
			t.Errorf("bridge offline: err = %v", err)
		}
		synctest.Wait()
		if s := b.all(); len(s) != 0 {
			t.Errorf("sent %+v", s)
		}
		if ss := commandStatuses(drain(updates)); len(ss) != 0 {
			t.Errorf("command updates %v for refused Commands", ss)
		}
	})
}

func TestAggregateCommandIsRelayedToCountedMembers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &fakeBridge{}
		h := newHome(b, nil, nil, nil, nil, nil, nil, nil)
		port(h).SetOnline(true)
		port(h).SyncDevices([]bridge.Device{lamp("0xl1", 1, 254), lamp("0xl2", 0, 100), lamp("0xl3", 0, 254), lamp("0xl4", 0, 254)})
		id, err := h.CreateAggregate("Parents' bedroom", lightMembers(t, h, "0xl1", "0xl2", "0xl3", "0xl4"), "", "")
		if err != nil {
			t.Fatal(err)
		}
		// 0xl3 leaves the network; 0xl4 no longer provides a light. A value
		// they would reject does not matter.
		port(h).SyncDevices([]bridge.Device{lamp("0xl1", 1, 254), lamp("0xl2", 0, 100), thermometer("0xl4")})
		_, updates, cancel := h.Subscribe()
		defer cancel()

		cmd, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true, "brightness": 50.0}, Transition: 2 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		s := b.all()
		slices.SortFunc(s, func(a, b sent) int { return strings.Compare(a.address, b.address) })
		if len(s) != 2 || s[0].address != "0xl1" || s[1].address != "0xl2" {
			t.Fatalf("sends = %+v, want 0xl1 and 0xl2", s)
		}
		for _, x := range s {
			if x.function != "light" || len(x.values) != 2 || x.values["state"] != true || x.values["brightness"] != 50.0 || x.transition != 2*time.Second {
				t.Errorf("send = %+v, want the values with the transition", x)
			}
		}

		us := drain(updates)
		var members []string
		for _, u := range us {
			switch {
			case u.Kind != CommandChanged:
			case u.Command.Target.Aggregate() != "":
				if u.Command.ID != cmd || u.Command.Target != TargetAggregate(id) || u.Command.Status != Pending {
					t.Errorf("aggregate command update = %+v, want %s pending", u.Command, cmd)
				}
			default:
				members = append(members, u.Command.ID)
				if u.Command.ID == cmd || u.Command.Status != Pending {
					t.Errorf("member command update = %+v, want its own pending Command", u.Command)
				}
			}
		}
		if len(members) != 2 || len(aggregateCommands(us, id)) != 1 {
			t.Fatalf("updates = %+v, want the Aggregate Command and two member Commands pending", us)
		}
	})
}

func TestAggregateToggleFollowsTheAggregateValue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, _, id := lights(t)
		off := func(address string) {
			port(h).Report(address, []bridge.Reading{{Function: "light", Capability: "state", Data: false}}, time.Now())
		}
		check := func(want map[string]any) {
			t.Helper()
			if _, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": "toggle", "brightness": 50.0}}); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			s := b.all()
			for _, x := range s[len(s)-2:] {
				if !maps.Equal(x.values, want) {
					t.Errorf("%s sent %v, want %v", x.address, x.values, want)
				}
			}
			time.Sleep(sendInterval)
		}
		lit(h, "0xl1", 100)
		off("0xl2")
		check(map[string]any{"state": false}) // any: one on is on
		off("0xl1")
		check(map[string]any{"state": true, "brightness": 50.0})
	})
}

func TestAggregateAdjustmentTurnsNoMemberOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, _, id := lights(t)
		adjust := func(want ...string) {
			t.Helper()
			before := len(b.all())
			if _, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"brightness": 50.0}}); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			var got []string
			for _, x := range b.all()[before:] {
				got = append(got, x.address)
			}
			if slices.Sort(got); !slices.Equal(got, want) {
				t.Errorf("sent to %v, want %v", got, want)
			}
			time.Sleep(sendInterval)
		}
		lit(h, "0xl1", 100)
		port(h).Report("0xl2", []bridge.Reading{{Function: "light", Capability: "state", Data: false}}, time.Now())
		adjust("0xl1")
		port(h).Report("0xl1", []bridge.Reading{{Function: "light", Capability: "state", Data: false}}, time.Now())
		adjust("0xl1", "0xl2") // none on: as a light's, it turns them on
	})
}

// A double press before any report: the second toggle sees the first,
// pending, as a Function's does.
func TestAggregateToggleSeesItsPendingCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, _, id := lights(t)
		for _, want := range []bool{true, false} {
			if _, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": "toggle"}}); err != nil {
				t.Fatal(err)
			}
			time.Sleep(sendInterval)
			synctest.Wait()
			for _, x := range b.all()[len(b.all())-2:] {
				if x.values["state"] != want {
					t.Errorf("%s sent %v, want state %v", x.address, x.values, want)
				}
			}
		}
	})
}
func TestAggregateCommandIsConfirmedOnceEveryMemberIs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := lights(t)
		h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true, "brightness": 50.0}})
		synctest.Wait()
		lit(h, "0xl1", 50)
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Pending}) {
			t.Fatalf("after one member confirmed: %v, want still pending", got)
		}
		lit(h, "0xl2", 50)
		us := drain(updates)
		if got := aggregateCommands(us, id); !slices.Equal(got, []CommandStatus{Confirmed}) {
			t.Fatalf("after both: %v, want confirmed", got)
		}
		if got := commandStatuses(us); !slices.Equal(got, []CommandStatus{Confirmed, Confirmed}) {
			t.Errorf("statuses = %v, want the member's then the Aggregate's confirmation", got)
		}
	})
}

func TestNestedAggregateCommandReachesEachFunctionOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, bedroom := lights(t)
		living, err := h.CreateAggregate("Living room", append([]Target{TargetAggregate(bedroom)}, lightMembers(t, h, "0xl1")...), "", "")
		if err != nil {
			t.Fatal(err)
		}
		h.Command(TargetAggregate(living), Request{Values: map[string]any{"state": true, "brightness": 50.0}})
		synctest.Wait()
		s := b.all()
		slices.SortFunc(s, func(a, b sent) int { return strings.Compare(a.address, b.address) })
		if len(s) != 2 || s[0].address != "0xl1" || s[1].address != "0xl2" {
			t.Fatalf("sends = %+v, want 0xl1 and 0xl2 once each", s)
		}
		lit(h, "0xl1", 50)
		lit(h, "0xl2", 50)
		if got := aggregateCommands(drain(updates), living); !slices.Equal(got, []CommandStatus{Pending, Confirmed}) {
			t.Fatalf("statuses = %v, want pending then confirmed", got)
		}
	})
}

func TestAggregateCommandFailsWithOneMember(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, b, updates, id := lights(t)
		b.err = errors.New("broker down")
		h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true}})
		synctest.Wait()
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Pending, Failed}) {
			t.Fatalf("statuses = %v, want pending then failed once", got)
		}
	})
}

func TestAggregateCommandTimesOutWithOneMember(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := lights(t)
		h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true, "brightness": 50.0}})
		synctest.Wait()
		lit(h, "0xl1", 50)
		time.Sleep(commandTimeout)
		synctest.Wait()
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Pending, TimedOut}) {
			t.Fatalf("statuses = %v, want pending then timed out", got)
		}
	})
}

func TestAggregateCommandIsSuperseded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := lights(t)
		first, _ := h.Command(TargetAggregate(id), Request{Values: map[string]any{"brightness": 10.0}})
		second, _ := h.Command(TargetAggregate(id), Request{Values: map[string]any{"brightness": 20.0}})
		var got []string
		for _, u := range drain(updates) {
			if u.Kind == CommandChanged && u.Command.Target == TargetAggregate(id) {
				got = append(got, map[string]string{first: "first", second: "second"}[u.Command.ID]+" "+string(u.Command.Status))
			}
		}
		want := []string{"first pending", "second pending", "first superseded"}
		if !slices.Equal(got, want) {
			t.Fatalf("updates = %v, want %v", got, want)
		}

		// A Command on one member supersedes the Aggregate's.
		if _, err := h.Command(TargetDevice(idOf(t, h, "0xl1"), "light"), Request{Values: map[string]any{"state": false}}); err != nil {
			t.Fatal(err)
		}
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Superseded}) {
			t.Fatalf("after a member Command: %v, want superseded", got)
		}
		// The remaining member confirming no longer affects it.
		lit(h, "0xl2", 20)
		if got := aggregateCommands(drain(updates), id); len(got) != 0 {
			t.Fatalf("after superseding: %v", got)
		}
	})
}

func TestAggregateCommandFailsWhenAMemberDeviceIsForgotten(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := lights(t)
		l1 := idOf(t, h, "0xl1")
		h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true}})
		port(h).SyncDevices([]bridge.Device{lamp("0xl2", 0, 100)}) // 0xl1 leaves before its Command is sent
		if err := h.Delete(l1); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // its pending transmission is dropped
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Pending, Failed}) {
			t.Fatalf("statuses = %v, want pending then failed", got)
		}
	})
}

func TestDeletedAggregateAnnouncesNoCommandOutcome(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, updates, id := lights(t)
		h.Command(TargetAggregate(id), Request{Values: map[string]any{"state": true}})
		synctest.Wait()
		h.DeleteAggregate(id)
		time.Sleep(commandTimeout)
		synctest.Wait()
		if got := aggregateCommands(drain(updates), id); !slices.Equal(got, []CommandStatus{Pending}) {
			t.Fatalf("statuses = %v, want nothing after deletion", got)
		}
	})
}
