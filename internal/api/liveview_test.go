package api

import (
	"bytes"
	"encoding/json"
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

func TestALiveViewIsFragmentedMP4RecordedOnceItEnds(t *testing.T) {
	h, do := server(t)
	cam, _ := withCamera(t, h, "arlo", &cameras{url: cameratest.New(t).URL()})
	before := time.Now()

	resp := do("POST", "/api/live-view", fmt.Sprintf(`{"target": %q}`, cam))
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
	resp.Body.Close()

	// Recorded once it ended, in the History of its Device's Functions.
	motion := home.TargetDevice(cam.Device(), "occupancy").Ref("occupancy")
	query := fmt.Sprintf(`{"from": %d, "to": %d, "points": 10, "markers": true, "refs": [%s]}`,
		before.Add(-time.Minute).UnixMilli(), time.Now().Add(time.Minute).UnixMilli(), mustJSON(motion))
	deadline := time.Now().Add(5 * time.Second)
	for {
		var a struct {
			LiveViews []struct {
				Target home.Target
				Origin map[string]any
			}
		}
		json.NewDecoder(do("POST", "/api/history", query).Body).Decode(&a)
		if len(a.LiveViews) == 1 {
			if a.LiveViews[0].Target != cam || a.LiveViews[0].Origin["person"] == nil {
				t.Errorf("live view %+v, want the camera's, by Alice", a.LiveViews[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live views %+v, want the one just watched", a.LiveViews)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestALiveViewOfACameraOnBatteryEndsInTime(t *testing.T) {
	liveViewFor = 300 * time.Millisecond
	t.Cleanup(func() { liveViewFor = 5 * time.Minute })
	h, do := server(t)
	port := h.Attach("arlo", &cameras{url: cameratest.New(t).URL()})
	port.SetOnline(true)
	port.SyncDevices([]bridge.Device{{NativeAddress: "cam1", Name: "Garden", Functions: []bridge.Function{{Key: "camera", Kind: "camera"}},
		Capabilities: []home.Capability{{Key: "battery", Type: home.Numeric, Unit: "%", Category: home.Diagnostic, Access: home.Access{Observable: true}}}}})
	snap, _, cancel := h.Subscribe()
	cancel()
	cam := home.TargetDevice(snap.Devices[0].ID, "camera")

	resp := do("POST", "/api/live-view", fmt.Sprintf(`{"target": %q}`, cam))
	until, err := time.Parse(time.RFC3339, resp.Header.Get("Live-View-Until"))
	if resp.StatusCode != http.StatusOK || err != nil || time.Until(until) > time.Second {
		t.Fatalf("POST: %d, until %q", resp.StatusCode, resp.Header.Get("Live-View-Until"))
	}
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, resp.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the Live view went on past its time")
	}
}

func TestALiveViewIsRefusedWhenThereIsNone(t *testing.T) {
	h, do := server(t)
	cam, _ := withCamera(t, h, "arlo", &cameras{url: "rtsp://127.0.0.1:1/nothing"})
	plain, _ := withCamera(t, h, "z2m", nopBridge{})
	for body, want := range map[string]int{
		fmt.Sprintf(`{"target": %q}`, home.TargetDevice(cam.Device(), "occupancy")): http.StatusNotFound,
		fmt.Sprintf(`{"target": %q}`, plain):                                        http.StatusNotFound,
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
	cam, _ := withCamera(t, h, "arlo", &cameras{url: camera.URL()})
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
