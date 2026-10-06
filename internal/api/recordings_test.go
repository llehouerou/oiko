package api

import (
	"bytes"
	"context"
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

// recorder is a Bridge with cameras whose system keeps Recordings, served by
// media as a vendor's storage would: at a path per id and part, ranges
// included.
type recorder struct {
	cameratest.Bridge
	list  []bridge.Recording
	media *httptest.Server
	err   error
}

func (r *recorder) Recordings(_ context.Context, address, function string, from, to time.Time) ([]bridge.Recording, error) {
	var in []bridge.Recording
	for _, x := range r.list {
		if !x.Start.Before(from) && !x.Start.After(to) {
			in = append(in, x)
		}
	}
	return in, r.err
}

func (r *recorder) RecordingMedia(ctx context.Context, address, function, id string, part bridge.RecordingPart, header http.Header) (*http.Response, error) {
	if r.err != nil {
		return nil, r.err
	}
	if id == "gone" {
		return nil, fmt.Errorf("%w: https://media.example/secret", bridge.ErrNotFound)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.media.URL+"/"+id+"/"+string(part)+"?sig=secret", nil)
	req.Header = header
	return http.DefaultClient.Do(req)
}

// video is the content of every Recording's video, made up.
var video = bytes.Repeat([]byte("0123456789"), 1000)

func newRecorder(t *testing.T, list []bridge.Recording) *recorder {
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/expired/"):
			http.Error(w, "expired", http.StatusForbidden)
		case strings.HasSuffix(r.URL.Path, "/video"):
			w.Header().Set("Content-Type", "video/mp4")
			http.ServeContent(w, r, "", cameratest.Taken, bytes.NewReader(video))
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("jpeg of " + r.URL.Path))
		}
	}))
	t.Cleanup(media.Close)
	return &recorder{list: list, media: media}
}

func TestRecordingsAreListedNewestFirstWithinTheRange(t *testing.T) {
	h, do := server(t)
	at := func(m int) time.Time { return cameratest.Taken.Add(time.Duration(m) * time.Minute) }
	cam, _ := cameratest.Attach(t, h, "arlo", newRecorder(t, []bridge.Recording{
		{ID: "a", Start: at(0), Duration: 12 * time.Second, Trigger: "motion"},
		{ID: "c", Start: at(20), Duration: 30 * time.Second},
		{ID: "b", Start: at(10), Duration: time.Minute, Trigger: "person"},
		{ID: "late", Start: at(120)},
	}))

	resp := do("GET", fmt.Sprintf("/api/recordings?target=%s&from=%d&to=%d", cam.Key(), at(0).UnixMilli(), at(60).UnixMilli()), "")
	var got []map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if resp.StatusCode != http.StatusOK || len(got) != 3 || got[0]["id"] != "c" || got[1]["id"] != "b" || got[2]["id"] != "a" {
		t.Fatalf("%d %v, want c, b, a", resp.StatusCode, got)
	}
	if got[1]["duration"] != 60000.0 || got[1]["trigger"] != "person" || got[0]["trigger"] != nil {
		t.Errorf("b and c: %v, %v", got[1], got[0])
	}
	if start, _ := time.Parse(time.RFC3339, got[2]["start"].(string)); !start.Equal(at(0)) {
		t.Errorf("a starts %v, want %v", got[2]["start"], at(0))
	}
}

func TestAListingKeepsTheNewestRecordings(t *testing.T) {
	h, do := server(t)
	var list []bridge.Recording
	for i := range maxRecordings + 5 {
		list = append(list, bridge.Recording{ID: fmt.Sprint(i), Start: cameratest.Taken.Add(time.Duration(i) * time.Second)})
	}
	cam, _ := cameratest.Attach(t, h, "arlo", newRecorder(t, list))
	var got []struct{ ID string }
	json.NewDecoder(do("GET", fmt.Sprintf("/api/recordings?target=%s&from=0&to=%d", cam.Key(), cameratest.Taken.Add(time.Hour).UnixMilli()), "").Body).Decode(&got)
	if len(got) != maxRecordings || got[0].ID != fmt.Sprint(maxRecordings+4) {
		t.Errorf("%d Recordings from %+v, want the %d newest", len(got), got[0], maxRecordings)
	}
}

func TestARecordingsVideoIsRelayedRangesIncluded(t *testing.T) {
	h, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	cookie, _ := signedIn(t, acc)
	cam, _ := cameratest.Attach(t, h, "arlo", newRecorder(t, nil))
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
	cam, _ := cameratest.Attach(t, h, "arlo", newRecorder(t, nil))
	plain, _ := cameratest.Attach(t, h, "cams", &cameratest.Bridge{})
	failing := newRecorder(t, nil)
	failing.err = errors.New(`Get "https://media.example/secret": timeout`)
	broken, _ := cameratest.Attach(t, h, "cloud", failing)
	off, port := cameratest.Attach(t, h, "away", newRecorder(t, nil))
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
