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

// Recordings asks the Bridge of camera Function t for its Recordings that
// started within [from, to] (ADR 0038): ErrNotFound when t is no camera
// Function or its Bridge has no Recordings, ErrBridgeOffline while that
// Bridge is offline.
func (h *Home) Recordings(ctx context.Context, t Target, from, to time.Time) ([]bridge.Recording, error) {
	recordings, address, err := CameraBridge[bridge.Recordings](h, t)
	if err != nil {
		return nil, err
	}
	return recordings.Recordings(ctx, address, t.Function(), from, to)
}

// RecordingMedia asks the Bridge of camera Function t for part of its
// Recording id, refused as Recordings is.
func (h *Home) RecordingMedia(ctx context.Context, t Target, id string, part bridge.RecordingPart, header http.Header) (*http.Response, error) {
	recordings, address, err := CameraBridge[bridge.Recordings](h, t)
	if err != nil {
		return nil, err
	}
	return recordings.RecordingMedia(ctx, address, t.Function(), id, part, header)
}

// CameraBridge is the Bridge of camera Function t, as I, an optional
// interface of the contract (bridge.Cameras, bridge.Recordings), and its
// Device's Native Address: ErrNotFound when t is no camera Function of an
// attached Device or its Bridge does not implement I, ErrBridgeOffline while
// that Bridge is offline.
func CameraBridge[I any](h *Home, t Target) (I, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
