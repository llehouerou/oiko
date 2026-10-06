package cameratest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// Taken is when every Picture of a Bridge was taken.
var Taken = time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)

// Bridge is a made-up Bridge with cameras: their live video at URL, their
// Picture "jpeg of <address>/<function>", unless Err, which it fails both
// with. It counts what it is asked.
type Bridge struct {
	URL      string
	Err      error
	Streams  atomic.Int32
	Pictures atomic.Int32
}

func (b *Bridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

func (b *Bridge) Stream(context.Context, string, string) (string, error) {
	b.Streams.Add(1)
	return b.URL, b.Err
}

func (b *Bridge) Picture(_ context.Context, address, function string) (bridge.Picture, error) {
	b.Pictures.Add(1)
	if b.Err != nil {
		return bridge.Picture{}, b.Err
	}
	return bridge.Picture{Data: []byte("jpeg of " + address + "/" + function), ContentType: "image/jpeg", Taken: Taken}, nil
}

// Attach attaches b to h as Bridge name, online, with one Device, Garden, of
// Device Capabilities caps, whose Functions are a camera, without
// Capabilities, and its motion: the camera's Target.
func Attach(t testing.TB, h *home.Home, name string, b home.Bridge, caps ...home.Capability) (home.Target, *home.Port) {
	t.Helper()
	port := h.Attach(name, b)
	port.SetOnline(true)
	port.SyncDevices([]bridge.Device{{NativeAddress: "cam1", Name: "Garden", Capabilities: caps, Functions: []bridge.Function{
		{Key: "camera", Kind: home.Camera},
		{Key: "occupancy", Kind: "occupancy", Capabilities: []home.Capability{
			{Key: "occupancy", Type: home.Binary, Category: home.Primary, Access: home.Access{Observable: true}}}},
	}}})
	snap, _, cancel := h.Subscribe()
	cancel()
	for _, d := range snap.Devices {
		if d.Bridge == name {
			return home.TargetDevice(d.ID, "camera"), port
		}
	}
	t.Fatal("no camera")
	return home.Target{}, nil
}

// Battery is a Device Capability telling a camera runs on battery.
var Battery = home.Capability{Key: "battery", Type: home.Numeric, Unit: "%", Category: home.Diagnostic, Access: home.Access{Observable: true}}
