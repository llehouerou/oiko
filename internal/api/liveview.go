package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
)

// liveViewFor bounds a Live view of a camera on battery (ADR 0036); a
// variable so tests can shorten it.
var liveViewFor = 5 * time.Minute

// liveView answers a POST of {"target"} with the camera's Live view (ADR
// 0037): its fragmented MP4, from a key frame on, for as long as the client
// reads it and the camera sends it. A camera on battery stops at the time
// its Live-View-Until header tells, after which the client may ask again.
// Each Live view is recorded once it ended, with who watched. What a
// Bridge's error says is logged, never answered.
func liveView(h *home.Home, relay *camera.Relay, hist *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target home.Target `json:"target"`
		}
		if !decode(w, r, &req) {
			return
		}
		v, err := relay.Watch(r.Context(), req.Target)
		switch {
		case err == nil:
		case errors.Is(err, camera.ErrBusy):
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		case errors.Is(err, home.ErrNotFound), errors.Is(err, home.ErrBridgeOffline), errors.Is(err, home.ErrInvalid):
			reply(w, err)
			return
		case r.Context().Err() != nil: // the client left
			return
		default:
			slog.Error("api: opening a Live view", "target", req.Target, "err", err)
			http.Error(w, "the camera sent no Live view", http.StatusBadGateway)
			return
		}
		start := time.Now()
		defer func() {
			v.Close()
			hist.LiveView(history.LiveView{Target: req.Target, Origin: origin(r), Start: start, End: time.Now()})
		}()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		if h.OnBattery(req.Target) {
			until := start.Add(liveViewFor)
			w.Header().Set("Live-View-Until", until.UTC().Format(time.RFC3339))
			ctx, cancel = context.WithDeadline(ctx, until)
			defer cancel()
		}
		go func() {
			<-ctx.Done()
			v.Close() // WriteTo returns
		}()
		w.Header().Set("Content-Type", v.ContentType)
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		defer rc.SetWriteDeadline(time.Time{}) // the connection may serve other requests
		v.WriteTo(streamWriter{w, rc})
	}
}

// streamWriter flushes each write through at once, giving up on one that
// does not go through within streamWriteLimit, as the event stream does.
type streamWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (s streamWriter) Write(p []byte) (int, error) {
	if err := s.rc.SetWriteDeadline(time.Now().Add(streamWriteLimit)); err != nil {
		return 0, err
	}
	n, err := s.w.Write(p)
	if err == nil {
		err = s.rc.Flush()
	}
	return n, err
}
