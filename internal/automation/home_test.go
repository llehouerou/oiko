package automation

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

type nopBridge struct{}

func (nopBridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

// counted is a real Home that counts the Commands the engine issues, fed
// through the Port of its one Bridge.
type counted struct {
	*home.Home
	z *home.Port
	n atomic.Int64
}

func (c *counted) Command(t home.Target, req home.Request) (string, error) {
	c.n.Add(1)
	return c.Home.Command(t, req)
}

// described is what the Bridge of realHome describes: a lamp and a remote.
var described = []bridge.Device{
	{NativeAddress: "0xlamp", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []home.Capability{
		{Key: "state", Type: home.Binary, Access: home.Access{Observable: true, Settable: true}},
	}}}},
	{NativeAddress: "0xremote", Functions: []bridge.Function{{Key: "button", Kind: "button", Capabilities: []home.Capability{
		{Key: "action", Type: home.Enum, Stateless: true, Access: home.Access{Observable: true}},
	}}}},
}

// realHome is a Home with its Bridge online and replayed, a lamp and a remote.
func realHome(t *testing.T) (c *counted, lamp, remote home.DeviceID) {
	t.Helper()
	h := home.New(nil)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.Replayed()
	z.SyncDevices(described)
	snap, _, cancel := h.Subscribe()
	cancel()
	for _, d := range snap.Devices {
		if d.NativeAddress == "0xlamp" {
			lamp = d.ID
		} else {
			remote = d.ID
		}
	}
	return &counted{Home: h, z: z}, lamp, remote
}

func lampOn(lamp home.DeviceID) string {
	return fmt.Sprintf(`act command {"targets": ["device:%s/light"], "values": {"state": true}}`, lamp)
}

func statusIn(h *home.Home, id string) home.AutomationStatus {
	snap, _, cancel := h.Subscribe()
	cancel()
	i := slices.IndexFunc(snap.Automations, func(s home.AutomationStatus) bool { return s.ID == id })
	if i < 0 {
		return home.AutomationStatus{}
	}
	return snap.Automations[i]
}

func TestRetainedStateReplayedOnConnectFiresNoValueTrigger(t *testing.T) {
	c, lamp, _ := realHome(t)
	e := New(c, []Document{doc("lamp on",
		[]string{
			fmt.Sprintf(`on valueTrigger {"target": "device:%s/light", "capability": "state", "op": "eq", "value": true}`, lamp),
			lampOn(lamp),
		}, "on.out act.in")}, nil, nil)

	state := func(on bool) {
		c.z.Report("0xlamp", []bridge.Reading{{Function: "light", Capability: "state", Data: on}}, time.Now())
		settle(e)
	}

	state(true) // replayed at startup: a first Value
	c.z.SetOnline(false)
	c.z.SetOnline(true)
	state(true) // replayed on reconnect: a refresh
	if n := c.n.Load(); n != 0 {
		t.Fatalf("replayed state fired %d Commands", n)
	}
	state(false)
	state(true)
	if n := c.n.Load(); n != 1 {
		t.Errorf("a real change fired %d Commands, want 1", n)
	}
}

func TestRetainedStateReplayedIntoAnAggregateFiresNoValueTrigger(t *testing.T) {
	c, lamp, _ := realHome(t)
	c.z.SyncDevices(append(slices.Clone(described), bridge.Device{NativeAddress: "0xlamp2", Functions: described[0].Functions}))
	snap, _, cancel := c.Subscribe()
	cancel()
	var members []home.Target
	for _, d := range snap.Devices {
		if d.NativeAddress != "0xremote" {
			members = append(members, home.TargetDevice(d.ID, "light"))
		}
	}
	lights, err := c.CreateAggregate("Lights", members, home.Any, "")
	if err != nil {
		t.Fatal(err)
	}
	e := New(c, []Document{doc("lights on",
		[]string{
			fmt.Sprintf(`on valueTrigger {"target": "aggregate:%s", "capability": "state", "op": "eq", "value": true}`, lights),
			lampOn(lamp),
		}, "on.out act.in")}, nil, nil)

	state := func(address string, on bool) {
		c.z.Report(address, []bridge.Reading{{Function: "light", Capability: "state", Data: on}}, time.Now())
		settle(e)
	}

	state("0xlamp", false) // replayed at startup: the Aggregate's first Value
	state("0xlamp2", true) // replayed: the Aggregate turns on, from a first Value
	if n := c.n.Load(); n != 0 {
		t.Fatalf("replayed state fired %d Commands", n)
	}
	state("0xlamp2", false)
	state("0xlamp", true)
	if n := c.n.Load(); n != 1 {
		t.Errorf("a real change fired %d Commands, want 1", n)
	}
}

func TestAutomationsTriggeringEachOtherBecomeRunaway(t *testing.T) {
	c, _, _ := realHome(t)
	x, _ := c.CreateFlag("X")
	y, _ := c.CreateFlag("Y")
	// Each turns its own Flag off and the other one on.
	chase := func(name string, own, other home.FlagID) Document {
		return doc(name, []string{
			fmt.Sprintf(`on valueTrigger {"target": "flag:%s", "capability": "on", "op": "eq", "value": true}`, own),
			fmt.Sprintf(`off command {"targets": ["flag:%s"], "values": {"on": false}}`, own),
			fmt.Sprintf(`next command {"targets": ["flag:%s"], "values": {"on": true}}`, other),
		}, "on.out off.in", "off.then next.in")
	}
	e := New(c, nil, nil, nil)
	a := create(t, e, chase("a", x, y))
	b := create(t, e, chase("b", y, x))

	c.Home.Command(home.TargetFlag(x), home.Request{Values: map[string]any{"on": true}})
	settle(e) // returns only once the loop is broken
	if s := statusIn(c.Home, a); s.Status != home.AutomationRunaway {
		t.Errorf("a: %+v", s)
	}
	if n := c.n.Load(); n != 2*2*runawayRuns {
		t.Errorf("%d Commands, want %d", n, 2*2*runawayRuns)
	}

	if err := e.Replace(a, chase("a", x, y)); err != nil { // re-enabled
		t.Fatal(err)
	}
	if s := statusIn(c.Home, a); s.Status != home.AutomationEnabled {
		t.Errorf("a after re-enabling: %+v", s)
	}
	if s := statusIn(c.Home, b); s.Status != home.AutomationEnabled {
		t.Errorf("b: %+v", s)
	}
}

func TestBurstOf1000ReportsLosesNoTrigger(t *testing.T) {
	c, lamp, remote := realHome(t)
	e := New(c, []Document{doc("press",
		[]string{
			fmt.Sprintf(`single eventTrigger {"target": "device:%s/button", "capability": "action", "events": ["single"]}`, remote),
			lampOn(lamp),
		}, "single.out act.in")}, nil, nil)

	e.now = spaced()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	for range 1000 {
		c.z.Report("0xremote", []bridge.Reading{{Function: "button", Capability: "action", Data: "single"}}, time.Now())
	}
	deadline := time.Now().Add(10 * time.Second)
	for c.n.Load() < 1000 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := c.n.Load(); n != 1000 {
		t.Errorf("%d Commands for 1000 presses", n)
	}
}
