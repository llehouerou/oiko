package automation

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

func TestAPresenceSetByHandFiresItsValueTriggerUntilItsPersonGoes(t *testing.T) {
	c, lamp, _ := realHome(t)
	const alice = home.PersonID("0190a1ce-0000-7000-8000-000000000001")
	c.SetPersons([]home.Person{{ID: alice, Name: "Alice"}})
	e := New(c, []Document{doc("alice arrives",
		[]string{
			fmt.Sprintf(`on valueTrigger {"target": "presence:%s", "capability": "home", "op": "eq", "value": true}`, alice),
			lampOn(lamp),
		}, "on.out act.in")}, nil, nil)
	set := func(v bool) {
		c.Home.Command(home.TargetPresence(alice), home.Request{Values: map[string]any{"home": v}})
		settle(e)
	}

	set(false) // the first Value: unknown before
	set(true)
	if n := c.n.Load(); n != 1 {
		t.Errorf("the arrival fired %d Commands, want 1", n)
	}
	if err := c.SetPersons(nil); err != nil { // Alice is removed
		t.Fatal(err)
	}
	settle(e)
	if s := statusIn(c.Home, e.Documents()[0].ID); s.Status != home.AutomationBroken {
		t.Errorf("once Alice is removed: %+v, want broken", s)
	}
}

func TestAPresenceFiresItsValueTriggerOnItsRuleNotOnReplayedValues(t *testing.T) {
	// A Bridge may replay retained state before it says it is online, or after.
	for _, onlineFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("online first %v", onlineFirst), func(t *testing.T) { replayedPresence(t, onlineFirst) })
	}
}

func replayedPresence(t *testing.T, onlineFirst bool) {
	synctest.Test(t, func(t *testing.T) {
		const alice = home.PersonID("0190a1ce-0000-7000-8000-000000000001")
		phone := bridge.Device{NativeAddress: "0xphone", Functions: []bridge.Function{{Key: "network", Kind: "network", Capabilities: []home.Capability{
			{Key: "connected", Type: home.Binary, Category: home.Primary, Access: home.Access{Observable: true}},
		}}}}
		connected := func(z *home.Port, on bool) {
			z.Report("0xphone", []bridge.Reading{{Function: "network", Capability: "connected", Data: on}}, time.Now())
		}
		// Before a restart: Alice home, by her phone.
		dir := t.TempDir()
		before, err := home.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		z := before.Attach("zigbee2mqtt", nopBridge{})
		z.SetOnline(true)
		z.SyncDevices(append(slices.Clone(described), phone))
		before.SetPersons([]home.Person{{ID: alice, Name: "Alice"}})
		var lamp, mobile home.DeviceID
		snap, _, cancel := before.Subscribe()
		cancel()
		for _, d := range snap.Devices {
			switch d.NativeAddress {
			case "0xlamp":
				lamp = d.ID
			case "0xphone":
				mobile = d.ID
			}
		}
		if err := before.SetPresenceSources(alice, []home.PresenceSource{{Target: home.TargetDevice(mobile, "network"), DepartureDelay: 900}}); err != nil {
			t.Fatal(err)
		}
		connected(z, true)

		// She left while Oiko was down: her phone's first Value says so an hour ago.
		h, err := home.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		h.Recall(func(home.Ref, any) (time.Time, bool) { return time.Now().Add(-time.Hour), true })
		c := &counted{Home: h, z: h.Attach("zigbee2mqtt", nopBridge{})}
		if onlineFirst {
			c.z.SetOnline(true)
		}
		h.SetPersons([]home.Person{{ID: alice, Name: "Alice"}})
		e := New(c, []Document{doc("alice leaves",
			[]string{
				fmt.Sprintf(`on valueTrigger {"target": "presence:%s", "capability": "home", "op": "eq", "value": false}`, alice),
				lampOn(lamp),
			}, "on.out act.in")}, nil, nil)
		c.z.SyncDevices(append(slices.Clone(described), phone))
		if v := presenceHome(h, alice); v != true {
			t.Fatalf("home = %v as Oiko starts, want home", v)
		}
		connected(c.z, false)
		c.z.SetOnline(true)
		c.z.Replayed()
		settle(e)
		if v := presenceHome(h, alice); v != false {
			t.Fatalf("home = %v after the replay, want away", v)
		}
		if n := c.n.Load(); n != 0 {
			t.Fatalf("the replayed departure fired %d Commands", n)
		}

		flag, _ := h.CreateFlag("Alice home")
		if err := h.SetPresenceSources(alice, []home.PresenceSource{{Target: home.TargetDevice(mobile, "network"), DepartureDelay: 900}, {Target: home.TargetFlag(flag)}}); err != nil {
			t.Fatal(err)
		}
		setFlag := func(on bool) {
			h.Command(home.TargetFlag(flag), home.Request{Values: map[string]any{"on": on}})
			settle(e)
		}
		setFlag(true)
		setFlag(false) // the rule's change, at once
		if n := c.n.Load(); n != 1 {
			t.Errorf("leaving by the Flag fired %d Commands, want 1", n)
		}
		connected(c.z, true)
		connected(c.z, false)
		settle(e)
		time.Sleep(15 * time.Minute) // the delay runs out
		synctest.Wait()
		settle(e)
		if n := c.n.Load(); n != 2 {
			t.Errorf("the phone's delay running out fired %d Commands in all, want 2", n)
		}
	})
}

func TestTheHomePresenceFiresOnItsExitGraceNotOnReplayedValues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const alice = home.PersonID("0190a1ce-0000-7000-8000-000000000001")
		binary := func(key string) []bridge.Function {
			return []bridge.Function{{Key: key, Kind: key, Capabilities: []home.Capability{
				{Key: key, Type: home.Binary, Category: home.Primary, Access: home.Access{Observable: true}},
			}}}
		}
		devices := append(slices.Clone(described),
			bridge.Device{NativeAddress: "0xphone", Functions: binary("connected")},
			bridge.Device{NativeAddress: "0xdoor", Functions: binary("contact")},
			bridge.Device{NativeAddress: "0xhall", Functions: binary("occupancy")})
		report := func(z *home.Port, address, key string, on bool) {
			z.Report(address, []bridge.Reading{{Function: key, Capability: key, Data: on}}, time.Now())
		}
		// Before a restart: Alice home by her phone, so someone home.
		dir := t.TempDir()
		before, err := home.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		z := before.Attach("zigbee2mqtt", nopBridge{})
		z.SetOnline(true)
		z.SyncDevices(devices)
		before.SetPersons([]home.Person{{ID: alice, Name: "Alice"}})
		ids := map[string]home.DeviceID{}
		snap, _, cancel := before.Subscribe()
		cancel()
		for _, d := range snap.Devices {
			ids[d.NativeAddress] = d.ID
		}
		if err := before.SetPresenceSources(alice, []home.PresenceSource{{Target: home.TargetDevice(ids["0xphone"], "connected")}}); err != nil {
			t.Fatal(err)
		}
		report(z, "0xphone", "connected", true)

		// She left while Oiko was down.
		h, err := home.Open(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		c := &counted{Home: h, z: h.Attach("zigbee2mqtt", nopBridge{})}
		h.SetPersons([]home.Person{{ID: alice, Name: "Alice"}})
		e := New(c, []Document{doc("home empty",
			[]string{
				`on valueTrigger {"target": "presence:home", "capability": "home", "op": "eq", "value": false}`,
				lampOn(ids["0xlamp"]),
			}, "on.out act.in")}, nil, nil)
		c.z.SetOnline(true)
		c.z.SyncDevices(devices)
		report(c.z, "0xphone", "connected", false)
		report(c.z, "0xdoor", "contact", false)
		c.z.Replayed()
		settle(e)
		if v := presenceHome(h, home.HomePresence); v != false {
			t.Fatalf("the Home presence = %v after the replay, want away", v)
		}
		if n := c.n.Load(); n != 0 {
			t.Fatalf("the replayed departure fired %d Commands", n)
		}

		// A guest stays, then leaves: the exit grace runs out after the start.
		if err := h.SetHomeBindings(home.HomeBindings{
			EntranceDoors: []home.Target{home.TargetDevice(ids["0xdoor"], "contact")},
			SignsOfLife:   []home.Target{home.TargetDevice(ids["0xhall"], "occupancy")},
			ExitGrace:     home.DefaultExitGrace,
		}); err != nil {
			t.Fatal(err)
		}
		report(c.z, "0xhall", "occupancy", true)
		report(c.z, "0xdoor", "contact", true)
		report(c.z, "0xdoor", "contact", false)
		report(c.z, "0xhall", "occupancy", false)
		settle(e)
		time.Sleep(home.DefaultExitGrace * time.Second)
		synctest.Wait()
		settle(e)
		if n := c.n.Load(); n != 1 {
			t.Errorf("the exit grace running out fired %d Commands, want 1", n)
		}
	})
}

func presenceHome(h *home.Home, id home.PersonID) any {
	snap, _, cancel := h.Subscribe()
	cancel()
	for _, rv := range snap.Values {
		if rv.Ref == home.TargetPresence(id).Ref("home") {
			return rv.Value.Data
		}
	}
	return nil
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
