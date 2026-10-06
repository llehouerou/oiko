package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/camera/cameratest"
	"github.com/llehouerou/oiko/internal/home"
)

func TestAPictureIsServedWithWhenItWasTaken(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{})

	resp := do("GET", "/api/picture?target="+cam.Key(), "")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "jpeg of cam1/camera" ||
		resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Last-Modified") != cameratest.Taken.Format(http.TimeFormat) {
		t.Fatalf("GET: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	if resp := do("HEAD", "/api/picture?target="+cam.Key(), ""); resp.StatusCode != http.StatusOK || resp.Header.Get("Last-Modified") == "" {
		t.Errorf("HEAD: %d %v", resp.StatusCode, resp.Header)
	}
}

func TestAPictureIsRefusedWhenThereIsNone(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{})
	failing, _ := cameratest.Attach(t, h, "cloud", &cameratest.Bridge{Err: errors.New(`Get "https://media.example/secret": timeout`)})
	off, port := cameratest.Attach(t, h, "away", &cameratest.Bridge{})
	port.SetOnline(false)

	for path, want := range map[string]int{
		"?target=" + home.TargetDevice(cam.Device(), "occupancy").Key(): http.StatusNotFound,
		"?target=" + off.Key():     http.StatusServiceUnavailable,
		"?target=" + failing.Key(): http.StatusBadGateway,
		"?target=nonsense":         http.StatusBadRequest,
		"":                         http.StatusBadRequest,
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
