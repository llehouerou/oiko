package camera

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
)

// ErrNoAnswer is a camera, or its Bridge, that failed. What it said is
// logged, never told: it may hold a URL giving its media to whoever reads it.
var ErrNoAnswer = errors.New("the camera or its Bridge gave no answer; Oiko's log tells why")

// liveViewFor bounds a Live view of a camera on battery (ADR 0036); a
// variable so tests can shorten it.
var liveViewFor = 5 * time.Minute

// Cameras are the home's cameras as Oiko serves them, whoever asks.
type Cameras struct {
	h      *home.Home
	relay  *relay
	record func(history.LiveView)
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
	}
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

// onBattery reports whether the Device of camera t reports a battery.
func (c *Cameras) onBattery(t home.Target) bool {
	_, err := c.h.Capability(home.TargetDevice(t.Device(), "").Ref("battery"))
	return err == nil
}

// failed is err, from doing what for camera t, as Watch tells it: Oiko's own
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
