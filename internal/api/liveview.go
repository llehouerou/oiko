package api

import (
	"net/http"
	"time"

	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/home"
)

// liveView answers a POST of {"target"} with the camera's Live view (ADR
// 0037), for as long as the client reads it and the camera sends it. A
// camera on battery stops at the time its Live-View-Until header tells, after
// which the client may ask again.
func liveView(cams *camera.Cameras) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target home.Target `json:"target"`
		}
		if !decode(w, r, &req) {
			return
		}
		v, err := cams.Watch(r.Context(), req.Target, origin(r))
		if err != nil {
			reply(w, err)
			return
		}
		defer v.Close()
		if !v.Until.IsZero() {
			w.Header().Set("Live-View-Until", v.Until.UTC().Format(time.RFC3339))
		}
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
