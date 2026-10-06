package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// cameras is a Bridge with cameras, counting the Pictures asked of it. Its
// live video is at url.
type cameras struct {
	nopBridge
	asked atomic.Int32
	err   error
	url   string
}

func (c *cameras) Stream(context.Context, string, string) (string, error) { return c.url, c.err }

var taken = time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)

func (c *cameras) Picture(_ context.Context, address, function string) (bridge.Picture, error) {
	c.asked.Add(1)
	if c.err != nil {
		return bridge.Picture{}, c.err
	}
	return bridge.Picture{Data: []byte("jpeg of " + address + "/" + function), ContentType: "image/jpeg", Taken: taken}, nil
}

// withCamera attaches b as Bridge name, online, with a Device whose Functions
// are a camera, without Capabilities, and its motion: the camera's Target.
func withCamera(t *testing.T, h *home.Home, name string, b home.Bridge) (home.Target, *home.Port) {
	t.Helper()
	port := h.Attach(name, b)
	port.SetOnline(true)
	port.SyncDevices([]bridge.Device{{NativeAddress: "cam1", Name: "Garden", Functions: []bridge.Function{
		{Key: "camera", Kind: "camera"},
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

func TestAPictureIsServedWithWhenItWasTakenAndKeptAMinute(t *testing.T) {
	h, do := server(t)
	b := &cameras{}
	cam, _ := withCamera(t, h, "arlo", b)

	for range 2 {
		resp := do("GET", "/api/picture?target="+cam.Key(), "")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "jpeg of cam1/camera" ||
			resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Last-Modified") != taken.Format(http.TimeFormat) {
			t.Fatalf("GET: %d %q %v", resp.StatusCode, body, resp.Header)
		}
	}
	if resp := do("HEAD", "/api/picture?target="+cam.Key(), ""); resp.StatusCode != http.StatusOK || resp.Header.Get("Last-Modified") == "" {
		t.Errorf("HEAD: %d %v", resp.StatusCode, resp.Header)
	}
	if n := b.asked.Load(); n != 1 {
		t.Errorf("the Bridge was asked %d times, want once", n)
	}

	// Kept for no time, a Picture is asked of the Bridge each time.
	pictureFor = 0
	t.Cleanup(func() { pictureFor = time.Minute })
	other := &cameras{}
	cam, _ = withCamera(t, h, "other", other)
	do("GET", "/api/picture?target="+cam.Key(), "")
	do("GET", "/api/picture?target="+cam.Key(), "")
	if n := other.asked.Load(); n != 2 {
		t.Errorf("kept for no time, the Bridge was asked %d times, want twice", n)
	}
}

func TestAPictureIsRefusedWhenThereIsNone(t *testing.T) {
	h, do := server(t)
	cam, _ := withCamera(t, h, "arlo", &cameras{})
	plain, _ := withCamera(t, h, "z2m", nopBridge{})
	failing, _ := withCamera(t, h, "cloud", &cameras{err: errors.New(`Get "https://media.example/secret": timeout`)})
	off, port := withCamera(t, h, "away", &cameras{})
	port.SetOnline(false)

	for path, want := range map[string]int{
		"?target=" + home.TargetDevice(cam.Device(), "occupancy").Key(): http.StatusNotFound,
		"?target=" + home.TargetDevice(cam.Device(), "").Key():          http.StatusNotFound,
		"?target=device:unknown/camera":                                 http.StatusNotFound,
		"?target=" + plain.Key():                                        http.StatusNotFound,
		"?target=" + off.Key():                                          http.StatusServiceUnavailable,
		"?target=" + failing.Key():                                      http.StatusBadGateway,
		"?target=nonsense":                                              http.StatusBadRequest,
		"":                                                              http.StatusBadRequest,
	} {
		resp := do("GET", "/api/picture"+path, "")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Errorf("%s: %d %s, want %d", path, resp.StatusCode, body, want)
		}
		if strings.Contains(string(body), "secret") {
			t.Errorf("%s: answered what the Bridge's error says: %q", path, body)
		}
	}
}
