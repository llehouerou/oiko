package api

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/home"
)

// recording is a bridge.Recording as the API tells it.
type recording struct {
	ID       string    `json:"id"`
	Start    time.Time `json:"start"`
	Duration int64     `json:"duration"` // ms
	Trigger  string    `json:"trigger,omitempty"`
}

// recordings lists a camera's Recordings, ?target=<key>&from=&to= in Unix
// ms, the newest first (ADR 0038).
func recordings(cams *camera.Cameras) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		t, err := home.ParseTarget(q.Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		from, errFrom := strconv.ParseInt(q.Get("from"), 10, 64)
		to, errTo := strconv.ParseInt(q.Get("to"), 10, 64)
		if errFrom != nil || errTo != nil || to < from {
			http.Error(w, "from and to are Unix ms, from first", http.StatusBadRequest)
			return
		}
		rs, err := cams.Recordings(r.Context(), t, time.UnixMilli(from), time.UnixMilli(to))
		if err != nil {
			reply(w, err)
			return
		}
		answer := make([]recording, len(rs))
		for i, x := range rs {
			answer[i] = recording{x.ID, x.Start, x.Duration.Milliseconds(), x.Trigger}
		}
		writeJSON(w, http.StatusOK, answer)
	}
}

// relayed are the headers of a Bridge's answer passed on to the browser, and
// passedOn those of the browser's request passed on to the Bridge.
var (
	relayed  = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"}
	passedOn = []string{"Range", "If-Range"}
)

// recordingMedia relays part of a Recording, ?target=<key>&id=, as its
// Bridge answers it, ranges included, so that a browser seeks in a video.
func recordingMedia(cams *camera.Cameras, part bridge.RecordingPart) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		t, err := home.ParseTarget(q.Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		header := http.Header{}
		for _, k := range passedOn {
			if v := r.Header.Get(k); v != "" {
				header.Set(k, v)
			}
		}
		resp, err := cams.RecordingMedia(r.Context(), t, q.Get("id"), part, header)
		if err != nil {
			reply(w, err)
			return
		}
		defer resp.Body.Close()
		for _, k := range relayed {
			if v := resp.Header.Get(k); v != "" {
				w.Header().Set(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		rc := http.NewResponseController(w)
		defer rc.SetWriteDeadline(time.Time{}) // the connection may serve other requests
		io.Copy(streamWriter{w, rc}, resp.Body)
	}
}
