package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// maxRecordings bounds what a listing answers (ADR 0034): the newest.
const maxRecordings = 1000

// recording is a bridge.Recording as the API tells it.
type recording struct {
	ID       string    `json:"id"`
	Start    time.Time `json:"start"`
	Duration int64     `json:"duration"` // ms
	Trigger  string    `json:"trigger,omitempty"`
}

// recordings lists a camera's Recordings, ?target=<key>&from=&to= in Unix
// ms, the newest first (ADR 0038). What a Bridge's error says is logged,
// never answered.
func recordings(h *home.Home) http.HandlerFunc {
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
		rs, err := h.Recordings(r.Context(), t, time.UnixMilli(from), time.UnixMilli(to))
		switch {
		case errors.Is(err, home.ErrNotFound), errors.Is(err, home.ErrBridgeOffline):
			reply(w, err)
			return
		case r.Context().Err() != nil: // the client left
			return
		case err != nil:
			slog.Error("api: listing Recordings", "target", t, "err", err)
			http.Error(w, "the camera's Bridge listed no Recordings", http.StatusBadGateway)
			return
		}
		slices.SortStableFunc(rs, func(a, b bridge.Recording) int { return b.Start.Compare(a.Start) })
		answer := make([]recording, 0, min(len(rs), maxRecordings))
		for _, x := range rs[:min(len(rs), maxRecordings)] {
			answer = append(answer, recording{x.ID, x.Start, x.Duration.Milliseconds(), x.Trigger})
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
func recordingMedia(h *home.Home, part bridge.RecordingPart) http.HandlerFunc {
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
		resp, err := h.RecordingMedia(r.Context(), t, q.Get("id"), part, header)
		switch {
		case errors.Is(err, home.ErrNotFound), errors.Is(err, home.ErrBridgeOffline):
			reply(w, err)
			return
		case errors.Is(err, bridge.ErrNotFound):
			http.Error(w, "no such Recording", http.StatusNotFound)
			return
		case r.Context().Err() != nil: // the client left
			return
		case err != nil:
			slog.Error("api: reading a Recording", "target", t, "part", part, "err", err)
			http.Error(w, "the camera's Bridge gave no Recording", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		case http.StatusNotFound, http.StatusGone:
			http.Error(w, "no such Recording", http.StatusNotFound)
			return
		default:
			slog.Error("api: reading a Recording", "target", t, "part", part, "status", resp.StatusCode)
			http.Error(w, "the camera's Bridge gave no Recording", http.StatusBadGateway)
			return
		}
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
