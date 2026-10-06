package home

import (
	"errors"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

func TestDeviceIconSurvivesRestartAndReplace(t *testing.T) {
	dir := t.TempDir()
	h := opened(t, dir)
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	if err := h.SetIcon(TargetDevice(id, ""), "floor-lamp"); err != nil {
		t.Fatal(err)
	}

	restarted := opened(t, dir)
	port(restarted).SyncDevices([]bridge.Device{bulb}) // described again, with no icon
	if got := deviceByID(t, restarted, id).Icon; got != "floor-lamp" {
		t.Fatalf("after restart: icon %q", got)
	}

	newBulb := bulb
	newBulb.NativeAddress = "0xnew"
	port(restarted).SyncDevices([]bridge.Device{newBulb})
	if err := restarted.Replace(id, idOf(t, restarted, "0xnew")); err != nil {
		t.Fatal(err)
	}
	if got := deviceByID(t, restarted, id).Icon; got != "floor-lamp" {
		t.Fatalf("after replace: icon %q", got)
	}
}

func TestAggregateIconSurvivesEdit(t *testing.T) {
	h, _, dir := aggregates(t)
	id, _ := h.CreateAggregate("Veranda", members(t, h, "0xm1", "0xm2"), Any, "")
	if err := h.SetIcon(TargetAggregate(id), "ceiling-light-multiple"); err != nil {
		t.Fatal(err)
	}
	if err := h.EditAggregate(id, "Motion", members(t, h, "0xm1"), All, Max); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(h).Aggregates[0].Icon; got != "ceiling-light-multiple" {
		t.Fatalf("after edit: icon %q", got)
	}
	if got := onDisk(t, dir).aggregates[0].Icon; got != "ceiling-light-multiple" {
		t.Fatalf("saved icon %q", got)
	}
	if err := h.SetIcon(TargetAggregate(id), ""); err != nil || snapshot(h).Aggregates[0].Icon != "" {
		t.Fatalf("back to the default: %v", err)
	}
}

func TestSetIconRefusals(t *testing.T) {
	h := opened(t, t.TempDir())
	port(h).SyncDevices([]bridge.Device{bulb})
	id := idOf(t, h, "0xbulb")
	for _, c := range []struct {
		target Target
		icon   string
		want   error
	}{
		{TargetDevice(id, ""), "Floor Lamp", ErrInvalid},
		{TargetDevice(id, "light"), "lamp", ErrInvalid}, // a Function's icon is its Device's
		{TargetFlag("f"), "lamp", ErrInvalid},
		{TargetDevice("nope", ""), "lamp", ErrNotFound},
		{TargetAggregate("nope"), "lamp", ErrNotFound},
	} {
		if err := h.SetIcon(c.target, c.icon); !errors.Is(err, c.want) {
			t.Errorf("SetIcon(%s, %q) = %v, want %v", c.target, c.icon, err, c.want)
		}
	}
}
