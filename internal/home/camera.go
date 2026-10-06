package home

import (
	"context"
	"fmt"

	"github.com/llehouerou/oiko/bridge"
)

// Camera is the kind of a camera Function (ADR 0036).
const Camera = "camera"

// Picture asks the Bridge of camera Function t for its Picture: ErrNotFound
// when t is no camera Function, or its Bridge has no cameras, and
// ErrBridgeOffline while that Bridge is offline.
func (h *Home) Picture(ctx context.Context, t Target) (bridge.Picture, error) {
	h.mu.Lock()
	cameras, address, err := h.camera(t)
	h.mu.Unlock()
	if err != nil {
		return bridge.Picture{}, err
	}
	return cameras.Picture(ctx, address, t.Function())
}

// Stream asks the Bridge of camera Function t for the URL of its live video,
// refused as Picture is.
func (h *Home) Stream(ctx context.Context, t Target) (string, error) {
	h.mu.Lock()
	cameras, address, err := h.camera(t)
	h.mu.Unlock()
	if err != nil {
		return "", err
	}
	return cameras.Stream(ctx, address, t.Function())
}

// OnBattery reports whether the Device of Target t reports a battery.
func (h *Home) OnBattery(t Target) bool {
	_, err := h.Capability(TargetDevice(t.Device(), "").Ref("battery"))
	return err == nil && t.Device() != ""
}

// camera is the Bridge of camera Function t and its Device's Native Address.
// Callers hold h.mu.
func (h *Home) camera(t Target) (bridge.Cameras, string, error) {
	d := h.devices[t.Device()]
	if d == nil || d.Detached || t.Function() == "" {
		return nil, "", ErrNotFound
	}
	if f := d.function(t.Function()); f == nil || f.Kind != Camera {
		return nil, "", fmt.Errorf("%w: %s is no camera", ErrNotFound, t)
	}
	l := h.links[d.Bridge] // nil once the configuration no longer has it
	var cameras bridge.Cameras
	ok := false
	if l != nil {
		cameras, ok = l.bridge.(bridge.Cameras)
	}
	switch {
	case !ok:
		return nil, "", fmt.Errorf("%w: Bridge %s has no cameras", ErrNotFound, d.Bridge)
	case !l.online:
		return nil, "", fmt.Errorf("%w: %s", ErrBridgeOffline, d.Bridge)
	}
	return cameras, d.NativeAddress, nil
}
