package home

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

var camera = bridge.Device{NativeAddress: "cam1", Name: "Garden", Functions: []bridge.Function{{Key: "camera", Kind: Camera}}}

func TestACameraWithoutCapabilitiesIsADeviceLikeAnyOther(t *testing.T) {
	h, disk := registry(t, nil)
	port(h).SyncDevices([]bridge.Device{camera})
	id := idOf(t, h, "cam1")
	area, err := h.CreateArea("Garden")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SetArea(TargetDevice(id, ""), area); err != nil {
		t.Fatal(err)
	}

	b, _ := json.Marshal(deviceByID(t, h, id))
	if !strings.Contains(string(b), `"functions":[{"key":"camera","kind":"camera","capabilities":[]}]`) {
		t.Errorf("as the web client reads it: %s", b)
	}
	snap, _, cancel := h.Subscribe()
	cancel()
	for _, a := range snap.Aggregates {
		if a.Area == area {
			t.Errorf("an Area Aggregate for a camera: %+v", a)
		}
	}

	restarted, _ := registry(t, *disk)
	port(restarted).SyncDevices(nil) // the camera is gone from its Bridge
	if _, err := restarted.Picture(context.Background(), TargetDevice(id, "camera")); !errors.Is(err, ErrNotFound) {
		t.Errorf("a Detached camera's Picture: %v, want ErrNotFound", err)
	}
	if err := restarted.Delete(id); err != nil {
		t.Errorf("deleting the Detached camera: %v", err)
	}
}
