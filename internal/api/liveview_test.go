package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/camera/cameratest"
	"github.com/llehouerou/oiko/internal/home"
)

// fragment reads resp's body until it holds a media fragment, and answers
// what it read.
func fragment(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	var got []byte
	buf := make([]byte, 64<<10)
	for !bytes.Contains(got, []byte("moof")) {
		n, err := resp.Body.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			t.Fatalf("the Live view ended after %d bytes: %v", len(got), err)
		}
	}
	return got
}

func TestALiveViewIsFragmentedMP4(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: cameratest.New(t).URL()})

	resp := do("POST", "/api/live-view", fmt.Sprintf(`{"target": %q}`, cam))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != `video/mp4; codecs="avc1.42001F"` {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST: %d %v %s", resp.StatusCode, resp.Header, b)
	}
	if resp.Header.Get("Live-View-Until") != "" {
		t.Error("a camera without battery has a time limit")
	}
	if got := fragment(t, resp); !bytes.Equal(got[4:8], []byte("ftyp")) {
		t.Errorf("it starts with %q, want its initialization segment", got[:8])
	}
}

func TestALiveViewOfACameraOnBatteryTellsUntilWhen(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: cameratest.New(t).URL()}, cameratest.Battery)

	resp := do("POST", "/api/live-view", fmt.Sprintf(`{"target": %q}`, cam))
	resp.Body.Close()
	until, err := time.Parse(time.RFC3339, resp.Header.Get("Live-View-Until"))
	if resp.StatusCode != http.StatusOK || err != nil || time.Until(until) > 5*time.Minute {
		t.Errorf("POST: %d, until %q", resp.StatusCode, resp.Header.Get("Live-View-Until"))
	}
}

func TestALiveViewIsRefusedWhenThereIsNone(t *testing.T) {
	h, do := server(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: "rtsp://127.0.0.1:1/nothing"})
	plain, _ := cameratest.Attach(t, h, "z2m", nopBridge{})
	off, port := cameratest.Attach(t, h, "away", &cameratest.Bridge{})
	port.SetOnline(false)
	for body, want := range map[string]int{
		fmt.Sprintf(`{"target": %q}`, home.TargetDevice(cam.Device(), "occupancy")): http.StatusNotFound,
		fmt.Sprintf(`{"target": %q}`, plain):                                        http.StatusNotFound,
		fmt.Sprintf(`{"target": %q}`, off):                                          http.StatusServiceUnavailable,
		fmt.Sprintf(`{"target": %q}`, cam):                                          http.StatusBadGateway, // nothing answers at its URL
		`{"target": "nonsense"}`:                                                    http.StatusBadRequest,
	} {
		if resp := do("POST", "/api/live-view", body); resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", body, resp.StatusCode, want)
		}
	}
}

func TestALiveViewIsNeverOpenedFromAnotherSite(t *testing.T) {
	h, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, _ := signedIn(t, acc)
	camera := cameratest.New(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: camera.URL()})
	req, _ := http.NewRequest("POST", srv.URL+"/api/live-view", strings.NewReader(fmt.Sprintf(`{"target": %q}`, cam)))
	req.AddCookie(cookie)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || camera.Sessions() != 0 {
		t.Errorf("from another site: %d, camera sessions %d; want refused before reaching it", resp.StatusCode, camera.Sessions())
	}
}
