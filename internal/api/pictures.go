package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// pictureFor is how long Oiko keeps a camera's Picture, so that many screens
// reach its Bridge once a minute at most (ADR 0036); a variable so tests can
// shorten it.
var pictureFor = time.Minute

// pictures keeps each camera's Picture in memory for pictureFor. Only a
// Picture its Bridge gave is kept, so they are as many as the cameras.
//
// ponytail: screens missing at the same moment each reach the Bridge; one
// fetch per camera in flight if many Kiosks ever refresh in step.
type pictures struct {
	h    *home.Home
	mu   sync.Mutex
	kept map[home.Target]keptPicture
}

type keptPicture struct {
	bridge.Picture
	until time.Time
}

func (p *pictures) get(ctx context.Context, t home.Target) (bridge.Picture, error) {
	now := time.Now()
	p.mu.Lock()
	maps.DeleteFunc(p.kept, func(_ home.Target, k keptPicture) bool { return !now.Before(k.until) })
	k, ok := p.kept[t]
	p.mu.Unlock()
	if ok {
		return k.Picture, nil
	}
	pic, err := p.h.Picture(ctx, t)
	if err == nil {
		p.mu.Lock()
		p.kept[t] = keptPicture{pic, time.Now().Add(pictureFor)}
		p.mu.Unlock()
	}
	return pic, err
}

// picture serves a camera's Picture, ?target=<key>, with when it was taken
// as its Last-Modified; a HEAD tells only that. What a Bridge's error says
// is logged, never answered: it may hold the presigned URL of a picture.
func picture(pics *pictures) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := home.ParseTarget(r.URL.Query().Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		pic, err := pics.get(r.Context(), t)
		switch {
		case errors.Is(err, home.ErrNotFound), errors.Is(err, home.ErrBridgeOffline):
			reply(w, err)
			return
		case err != nil:
			slog.Error("api: reading a Picture", "target", t, "err", err)
			http.Error(w, "the camera's Bridge gave no Picture", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", pic.ContentType)
		http.ServeContent(w, r, "", pic.Taken, bytes.NewReader(pic.Data))
	}
}
