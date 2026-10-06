package home

import (
	"context"
	"fmt"
	"net/http"
	"time"

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

// Recordings asks the Bridge of camera Function t for its Recordings that
// started within [from, to] (ADR 0038): ErrNotFound when t is no camera
// Function or its Bridge has no Recordings, ErrBridgeOffline while that
// Bridge is offline.
func (h *Home) Recordings(ctx context.Context, t Target, from, to time.Time) ([]bridge.Recording, error) {
	h.mu.Lock()
	recordings, address, err := ofCamera[bridge.Recordings](h, t)
	h.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return recordings.Recordings(ctx, address, t.Function(), from, to)
}

// RecordingMedia asks the Bridge of camera Function t for part of its
// Recording id, refused as Recordings is.
func (h *Home) RecordingMedia(ctx context.Context, t Target, id string, part bridge.RecordingPart, header http.Header) (*http.Response, error) {
	h.mu.Lock()
	recordings, address, err := ofCamera[bridge.Recordings](h, t)
	h.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return recordings.RecordingMedia(ctx, address, t.Function(), id, part, header)
}

// camera is the Bridge of camera Function t and its Device's Native Address.
// Callers hold h.mu.
func (h *Home) camera(t Target) (bridge.Cameras, string, error) {
	return ofCamera[bridge.Cameras](h, t)
}

// ofCamera is the Bridge of camera Function t, as the optional interface I
// of the contract, and its Device's Native Address. Callers hold h.mu.
func ofCamera[I any](h *Home, t Target) (I, string, error) {
	var none I
	d := h.devices[t.Device()]
	if d == nil || d.Detached || t.Function() == "" {
		return none, "", ErrNotFound
	}
	if f := d.function(t.Function()); f == nil || f.Kind != Camera {
		return none, "", fmt.Errorf("%w: %s is no camera", ErrNotFound, t)
	}
	l := h.links[d.Bridge] // nil once the configuration no longer has it
	var b I
	ok := false
	if l != nil {
		b, ok = l.bridge.(I)
	}
	switch {
	case !ok:
		return none, "", fmt.Errorf("%w: Bridge %s offers no %T", ErrNotFound, d.Bridge, &none)
	case !l.online:
		return none, "", fmt.Errorf("%w: %s", ErrBridgeOffline, d.Bridge)
	}
	return b, d.NativeAddress, nil
}
