package home

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

// registry is a Home whose saves are captured, as they would be on disk.
func registry(t *testing.T, saved []Device) (*Home, *[]Device) {
	t.Helper()
	disk := &[]Device{}
	h := newHome(&fakeBridge{}, saved, nil, nil, func(d []Device) error { *disk = d; return nil }, nil, nil, nil)
	port(h).SetOnline(true)
	return h, disk
}

func idOf(t *testing.T, h *Home, address string) DeviceID {
	t.Helper()
	snap, _, cancel := h.Subscribe()
	defer cancel()
	for _, d := range snap.Devices {
		if d.NativeAddress == address {
			return d.ID
		}
	}
	t.Fatalf("no device at %s", address)
	return ""
}

func deviceByID(t *testing.T, h *Home, id DeviceID) Device {
	t.Helper()
	snap, _, cancel := h.Subscribe()
	defer cancel()
	for _, d := range snap.Devices {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no device %s", id)
	return Device{}
}

func TestIdentityAndNameSurviveRestart(t *testing.T) {
	h, disk := registry(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	if err := h.Rename(id, "  Ceiling  "); err != nil {
		t.Fatal(err)
	}

	restarted, _ := registry(t, *disk)
	port(restarted).SyncDevices([]bridge.Device{bulb}) // z2m describes it again, with its own name
	if got := deviceByID(t, restarted, id); got.Name != "Ceiling" {
		t.Fatalf("after restart: name %q, want %q", got.Name, "Ceiling")
	}
}

func TestRenameRejectsBlankNames(t *testing.T) {
	h, _ := registry(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	if err := h.Rename(idOf(t, h, "0xbulb"), "   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
	if err := h.Rename("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestDeleteOnlyDetached(t *testing.T) {
	h, disk := registry(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	if err := h.Delete(id); !errors.Is(err, ErrInvalid) {
		t.Fatalf("deleting an attached device: got %v, want ErrInvalid", err)
	}
	port(h).SyncDevices(nil) // the bulb leaves the network
	if err := h.Delete(id); err != nil {
		t.Fatal(err)
	}
	if len(*disk) != 0 {
		t.Fatalf("saved %d devices, want 0", len(*disk))
	}
}

func TestReplaceKeepsIdentityAndTakesNewHardware(t *testing.T) {
	h, disk := registry(t, nil)
	port(h).SyncDevices([]bridge.Device{bulb})
	oldID := idOf(t, h, "0xbulb")
	if err := h.Rename(oldID, "Ceiling"); err != nil {
		t.Fatal(err)
	}

	// The bulb dies; a new one joins.
	newBulb := bulb
	newBulb.NativeAddress = "0xnew"
	port(h).SyncDevices([]bridge.Device{newBulb})
	newID := idOf(t, h, "0xnew")
	port(h).Report("0xnew", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, time.Now())

	if err := h.Replace(newID, oldID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replace in the wrong direction: got %v, want ErrInvalid", err)
	}
	if err := h.Replace(oldID, newID); err != nil {
		t.Fatal(err)
	}

	got := deviceByID(t, h, oldID)
	if got.Name != "Ceiling" || got.NativeAddress != "0xnew" || got.Detached {
		t.Fatalf("replaced device = %+v", got)
	}
	if len(*disk) != 1 {
		t.Fatalf("saved %d devices, want 1", len(*disk))
	}
	snap, _, cancel := h.Subscribe()
	defer cancel()
	want := TargetDevice(oldID, "light").Ref("state")
	if !slices.ContainsFunc(snap.Values, func(rv RefValue) bool { return rv.Ref == want }) {
		t.Fatal("the new hardware's Value was lost")
	}
	if idOf(t, h, "0xnew") != oldID {
		t.Fatal("the new Native Address does not resolve to the kept identity")
	}
}
