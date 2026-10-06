package api

import (
	"bytes"
	"net/http"

	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/home"
)

// picture serves a camera's Picture, ?target=<key>, with when it was taken
// as its Last-Modified; a HEAD tells only that.
func picture(cams *camera.Cameras) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := home.ParseTarget(r.URL.Query().Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		pic, err := cams.Picture(r.Context(), t)
		if err != nil {
			reply(w, err)
			return
		}
		w.Header().Set("Content-Type", pic.ContentType)
		http.ServeContent(w, r, "", pic.Taken, bytes.NewReader(pic.Data))
	}
}
