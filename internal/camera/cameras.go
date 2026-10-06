package camera

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
)

// ErrNoAnswer is a camera, or its Bridge, that failed. What it said is
// logged, never told: it may hold a URL giving its media to whoever reads it.
var ErrNoAnswer = errors.New("the camera or its Bridge gave no answer; Oiko's log tells why")

const (
	// maxRecordings bounds a listing of Recordings (ADR 0034): the newest.
	maxRecordings = 1000
	// recordingWithin is how far from its Event's time a Recording's start may
	// be (ADR 0039).
	recordingWithin = 5 * time.Second
)

// Variables so tests can shorten them.
var (
	// liveViewFor bounds a Live view of a camera on battery (ADR 0036).
	liveViewFor = 5 * time.Minute
	// pictureFor is how long a camera's Picture is kept, so that many screens
	// reach its Bridge once a minute at most (ADR 0036).
	pictureFor = time.Minute
)

// Cameras are the home's cameras as Oiko serves them, whoever asks.
type Cameras struct {
	h      *home.Home
	relay  *relay
	record func(history.LiveView)

	// ponytail: screens missing a Picture at the same moment each reach the
	// Bridge; one fetch per camera in flight if many Kiosks ever refresh in
	// step.
	mu   sync.Mutex
	kept map[home.Target]keptPicture // only Pictures a Bridge gave: as many as the cameras
}

type keptPicture struct {
	bridge.Picture
	until time.Time
}

// New serves the cameras of h, handing each Live view to record once it
// ended; record must return at once.
func New(h *home.Home, record func(history.LiveView)) *Cameras {
	return &Cameras{
		h: h,
		relay: newRelay(func(ctx context.Context, t home.Target) (string, error) {
			cameras, address, err := home.CameraBridge[bridge.Cameras](h, t)
			if err != nil {
				return "", err
			}
			return cameras.Stream(ctx, address, t.Function())
		}),
		record: record,
		kept:   map[home.Target]keptPicture{},
	}
}

// Picture is the Picture of camera Function t, as its Bridge gave it within
// pictureFor, or else asks it now. It is refused with home.ErrNotFound when
// t is no camera, home.ErrBridgeOffline, or ErrNoAnswer.
func (c *Cameras) Picture(ctx context.Context, t home.Target) (bridge.Picture, error) {
	now := time.Now()
	c.mu.Lock()
	maps.DeleteFunc(c.kept, func(_ home.Target, k keptPicture) bool { return !now.Before(k.until) })
	k, ok := c.kept[t]
	c.mu.Unlock()
	if ok {
		return k.Picture, nil
	}
	cameras, address, err := home.CameraBridge[bridge.Cameras](c.h, t)
	if err != nil {
		return bridge.Picture{}, err
	}
	pic, err := cameras.Picture(ctx, address, t.Function())
	if err != nil {
		return bridge.Picture{}, failed(ctx, "reading a Picture", t, err)
	}
	c.mu.Lock()
	c.kept[t] = keptPicture{pic, time.Now().Add(pictureFor)}
	c.mu.Unlock()
	return pic, nil
}

// LiveView is a Live view of a camera (ADR 0037): its fragmented MP4, from a
// key frame on.
type LiveView struct {
	ContentType string    // its media's, with their codecs, for a MediaSource
	Until       time.Time // when it ends, for a camera on battery; zero for no limit
	v           *viewer
	close       func()
}

// WriteTo writes the Live view to w until it ends: closed, past Until, its
// context done, its camera gone, or a write to w failed.
func (l *LiveView) WriteTo(w io.Writer) (int64, error) { return l.v.WriteTo(w) }

// Close ends the Live view and records it, with who watched, in its camera's
// History; once only.
func (l *LiveView) Close() { l.close() }

// Watch opens a Live view of camera Function t for by, which lasts until ctx
// is done or, for a camera on battery, Until. It is refused with
// home.ErrNotFound when t is no camera, home.ErrBridgeOffline, ErrBusy, or
// ErrNoAnswer. The caller closes it.
func (c *Cameras) Watch(ctx context.Context, t home.Target, by home.Origin) (*LiveView, error) {
	v, err := c.relay.Watch(ctx, t)
	if err != nil {
		return nil, failed(ctx, "opening a Live view", t, err)
	}
	start := time.Now()
	l := &LiveView{ContentType: v.ContentType, v: v}
	var cancel context.CancelFunc
	if c.onBattery(t) {
		l.Until = start.Add(liveViewFor)
		ctx, cancel = context.WithDeadline(ctx, l.Until)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	go func() {
		<-ctx.Done()
		v.Close() // WriteTo returns
	}()
	l.close = sync.OnceFunc(func() {
		cancel()
		c.record(history.LiveView{Target: t, Origin: by, Start: start, End: time.Now()})
	})
	return l, nil
}

// Recordings lists the Recordings of camera Function t that started within
// [from, to], the newest first, maxRecordings at most (ADR 0034, 0038). It
// is refused with home.ErrNotFound when t is no camera or its Bridge keeps
// no Recordings, home.ErrBridgeOffline, or ErrNoAnswer.
func (c *Cameras) Recordings(ctx context.Context, t home.Target, from, to time.Time) ([]bridge.Recording, error) {
	recordings, address, err := home.CameraBridge[bridge.Recordings](c.h, t)
	if err != nil {
		return nil, err
	}
	rs, err := recordings.Recordings(ctx, address, t.Function(), from, to)
	if err != nil {
		return nil, failed(ctx, "listing Recordings", t, err)
	}
	slices.SortStableFunc(rs, func(a, b bridge.Recording) int { return b.Start.Compare(a.Start) })
	return rs[:min(len(rs), maxRecordings)], nil
}

// RecordingMedia fetches part of Recording id of camera Function t, passing
// header on, as its Bridge answers it, with its status, 200, 206 or 416, its
// headers and its body, which the caller closes. It is refused as
// Recordings is, and with home.ErrNotFound when its system no longer has id.
func (c *Cameras) RecordingMedia(ctx context.Context, t home.Target, id string, part bridge.RecordingPart, header http.Header) (*http.Response, error) {
	recordings, address, err := home.CameraBridge[bridge.Recordings](c.h, t)
	if err != nil {
		return nil, err
	}
	resp, err := recordings.RecordingMedia(ctx, address, t.Function(), id, part, header)
	switch {
	case errors.Is(err, bridge.ErrNotFound):
		return nil, errNoRecording
	case err != nil:
		return nil, failed(ctx, "reading a Recording", t, err)
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		return resp, nil
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, errNoRecording
	}
	resp.Body.Close()
	return nil, failed(ctx, "reading a Recording", t, fmt.Errorf("%s: %s", part, resp.Status))
}

var errNoRecording = fmt.Errorf("%w: no such Recording", home.ErrNotFound)

// Video fetches the video of the Recording of camera Function t that started
// at, within a few seconds (ADR 0039), as RecordingMedia answers it. It is
// refused as RecordingMedia is, and with home.ErrNotFound when no Recording
// started then.
func (c *Cameras) Video(ctx context.Context, t home.Target, at time.Time) (*http.Response, error) {
	rs, err := c.Recordings(ctx, t, at.Add(-recordingWithin), at.Add(recordingWithin))
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return nil, fmt.Errorf("%w: no Recording of %s started at %s", home.ErrNotFound, t, at.Format(time.RFC3339))
	}
	nearest := slices.MinFunc(rs, func(a, b bridge.Recording) int {
		return cmp.Compare(a.Start.Sub(at).Abs(), b.Start.Sub(at).Abs())
	})
	return c.RecordingMedia(ctx, t, nearest.ID, bridge.Video, http.Header{})
}

// onBattery reports whether the Device of camera t reports a battery.
func (c *Cameras) onBattery(t home.Target) bool {
	_, err := c.h.Capability(home.TargetDevice(t.Device(), "").Ref("battery"))
	return err == nil
}

// failed is err, from doing what for camera t, as Cameras tell it: Oiko's own
// refusals as they are, the end of ctx as it is, anything else logged and
// told as ErrNoAnswer.
func failed(ctx context.Context, what string, t home.Target, err error) error {
	switch {
	case errors.Is(err, home.ErrNotFound), errors.Is(err, home.ErrBridgeOffline), errors.Is(err, ErrBusy):
		return err
	case ctx.Err() != nil: // whoever asked left
		return ctx.Err()
	}
	slog.Error("camera: "+what, "target", t, "err", err)
	return ErrNoAnswer
}
