package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/camera/cameratest"
	"github.com/llehouerou/oiko/internal/home"
)

func TestRecordingsAreListedWithinTheRange(t *testing.T) {
	h, do := server(t)
	at := func(m int) time.Time { return cameratest.Taken.Add(time.Duration(m) * time.Minute) }
	cam, _ := cameratest.Attach(t, h, "arlo", cameratest.NewRecorder(t, []bridge.Recording{
		{ID: "a", Start: at(0), Duration: 12 * time.Second, Trigger: "motion"},
		{ID: "b", Start: at(10), Duration: time.Minute},
		{ID: "late", Start: at(120)},
	}))

	resp := do("GET", fmt.Sprintf("/api/recordings?target=%s&from=%d&to=%d", cam.Key(), at(0).UnixMilli(), at(60).UnixMilli()), "")
	var got []map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if resp.StatusCode != http.StatusOK || len(got) != 2 || got[0]["id"] != "b" || got[1]["id"] != "a" {
		t.Fatalf("%d %v, want b, a", resp.StatusCode, got)
	}
	if got[1]["duration"] != 12000.0 || got[1]["trigger"] != "motion" || got[0]["trigger"] != nil {
		t.Errorf("a and b: %v, %v", got[1], got[0])
	}
	if start, _ := time.Parse(time.RFC3339, got[1]["start"].(string)); !start.Equal(at(0)) {
		t.Errorf("a starts %v, want %v", got[1]["start"], at(0))
	}
}

func TestARecordingsVideoIsRelayedRangesIncluded(t *testing.T) {
	h, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, _ := signedIn(t, acc)
	cam, _ := cameratest.Attach(t, h, "arlo", cameratest.NewRecorder(t, nil))
	get := func(path, rng string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.AddCookie(cookie)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}

	video := cameratest.Video
	resp, body := get("/api/recordings/video?id=r1&target="+cam.Key(), "")
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, video) || resp.Header.Get("Content-Type") != "video/mp4" ||
		resp.Header.Get("Accept-Ranges") != "bytes" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("whole: %d, %d bytes, %v", resp.StatusCode, len(body), resp.Header)
	}
	resp, body = get("/api/recordings/video?id=r1&target="+cam.Key(), "bytes=100-199")
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(body, video[100:200]) ||
		resp.Header.Get("Content-Range") != fmt.Sprintf("bytes 100-199/%d", len(video)) || resp.Header.Get("Content-Length") != "100" {
		t.Errorf("range: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	resp, body = get("/api/recordings/thumbnail?id=r1&target="+cam.Key(), "")
	if resp.StatusCode != http.StatusOK || string(body) != "jpeg of /r1/thumbnail" || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("thumbnail: %d %q %v", resp.StatusCode, body, resp.Header)
	}
}

func TestARecordingIsRefusedWhenThereIsNone(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", cameratest.NewRecorder(t, nil))
	plain, _ := cameratest.Attach(t, h, "cams", &cameratest.Bridge{})
	failing := cameratest.NewRecorder(t, nil)
	failing.Err = errors.New(`Get "https://media.example/secret": timeout`)
	broken, _ := cameratest.Attach(t, h, "cloud", failing)
	off, port := cameratest.Attach(t, h, "away", cameratest.NewRecorder(t, nil))
	port.SetOnline(false)

	for path, want := range map[string]int{
		"/api/recordings/video?id=gone&target=" + cam.Key():                                        http.StatusNotFound,
		"/api/recordings/video?id=expired&target=" + cam.Key():                                     http.StatusBadGateway,
		"/api/recordings/video?id=r1&target=" + plain.Key():                                        http.StatusNotFound,
		"/api/recordings/thumbnail?id=r1&target=" + broken.Key():                                   http.StatusBadGateway,
		"/api/recordings/video?id=r1&target=" + off.Key():                                          http.StatusServiceUnavailable,
		"/api/recordings/video?id=r1&target=" + home.TargetDevice(cam.Device(), "occupancy").Key(): http.StatusNotFound,
		"/api/recordings?from=0&to=1&target=" + broken.Key():                                       http.StatusBadGateway,
		"/api/recordings?from=0&to=1&target=" + plain.Key():                                        http.StatusNotFound,
		"/api/recordings?from=2&to=1&target=" + cam.Key():                                          http.StatusBadRequest,
		"/api/recordings?to=1&target=" + cam.Key():                                                 http.StatusBadRequest,
		"/api/recordings?from=0&to=1&target=nonsense":                                              http.StatusBadRequest,
	} {
		resp := do("GET", path, "")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Errorf("%s: %d %s, want %d", path, resp.StatusCode, body, want)
		}
		if strings.Contains(string(body), "secret") {
			t.Errorf("%s: answered what the Bridge's error says: %q", path, body)
		}
	}
}
