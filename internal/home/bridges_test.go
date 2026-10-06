package home

import (
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

func TestHomeIsKnownOnceEveryBridgeHasReplayed(t *testing.T) {
	known := func(h *Home) bool {
		select {
		case <-h.Known():
			return true
		default:
			return false
		}
	}
	h := New(nil)
	if !known(h) {
		t.Error("with no Bridge, the home is known at once")
	}

	h = New(nil)
	zb, hk := h.Attach("zigbee2mqtt", &fakeBridge{}), h.Attach("homekit", &fakeBridge{})
	if known(h) || !slices.Equal(h.Waiting(), []string{"homekit", "zigbee2mqtt"}) {
		t.Fatalf("known before any Replay; waiting for %v", h.Waiting())
	}
	zb.Replayed()
	if known(h) || !slices.Equal(h.Waiting(), []string{"homekit"}) {
		t.Fatalf("known with homekit yet to replay; waiting for %v", h.Waiting())
	}
	hk.Replayed()
	zb.Replayed() // on a reconnection
	if !known(h) || len(h.Waiting()) != 0 {
		t.Errorf("not known once both replayed; waiting for %v", h.Waiting())
	}
}

// Each Bridge owns its Devices: the same Native Address on two Bridges is two
// Devices, a Bridge's list detaches only its own, and a Bridge offline blanks
// and refuses only its own.
func TestBridgesKeepToTheirOwnDevices(t *testing.T) {
	h := New(nil)
	zb, hk := &fakeBridge{}, &fakeBridge{}
	z, k := h.Attach("zigbee2mqtt", zb), h.Attach("homekit", hk)
	z.SetOnline(true)
	k.SetOnline(true)
	z.SyncDevices([]bridge.Device{bulb})
	k.SyncDevices([]bridge.Device{bulb})

	snap := snapshot(h)
	if len(snap.Devices) != 2 || snap.Devices[0].ID == snap.Devices[1].ID {
		t.Fatalf("devices = %+v, want one per Bridge", snap.Devices)
	}
	byBridge := map[string]DeviceID{}
	for _, d := range snap.Devices {
		byBridge[d.Bridge] = d.ID
	}
	z.SetAvailability("0xbulb", Online)
	k.SetAvailability("0xbulb", Online)

	z.SyncDevices(nil)
	for _, d := range snapshot(h).Devices {
		if d.Detached != (d.Bridge == "zigbee2mqtt") {
			t.Fatalf("after zigbee2mqtt lists nothing: %s detached = %v", d.Bridge, d.Detached)
		}
	}
	z.SyncDevices([]bridge.Device{bulb})

	z.SetOnline(false)
	snap = snapshot(h)
	if a := snap.Availability[TargetDevice(byBridge["zigbee2mqtt"], "")]; a != Unknown {
		t.Errorf("zigbee2mqtt offline: its bulb is %s, want unknown", a)
	}
	if a := snap.Availability[TargetDevice(byBridge["homekit"], "")]; a != Online {
		t.Errorf("zigbee2mqtt offline: the homekit bulb is %s, want online", a)
	}
	if !snap.Bridges["homekit"] || snap.Bridges["zigbee2mqtt"] {
		t.Errorf("bridges = %v", snap.Bridges)
	}
	on := Request{Values: map[string]any{"state": true}}
	if _, err := h.Command(TargetDevice(byBridge["zigbee2mqtt"], "light"), on); !errors.Is(err, ErrBridgeOffline) {
		t.Errorf("command to the zigbee2mqtt bulb: %v, want ErrBridgeOffline", err)
	}
	synctest.Test(t, func(t *testing.T) {
		if _, err := h.Command(TargetDevice(byBridge["homekit"], "light"), on); err != nil {
			t.Fatal(err)
		}
		time.Sleep(sendInterval)
		if got := hk.all(); len(got) != 1 || got[0].address != "0xbulb" || len(zb.all()) != 0 {
			t.Errorf("sent through homekit %v, zigbee2mqtt %v; want one through homekit", got, zb.all())
		}
	})
}
