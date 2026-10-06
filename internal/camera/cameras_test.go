package camera

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/camera/cameratest"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
)

// plain is a Bridge without cameras.
type plain struct{}

func (plain) Send(context.Context, string, string, map[string]any, time.Duration) error { return nil }

// cameras are the cameras of a Home without Devices, and the Live views they
// recorded.
func cameras(t *testing.T) (*home.Home, *Cameras, <-chan history.LiveView) {
	t.Helper()
	h := home.New(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	recorded := make(chan history.LiveView, 10)
	return h, New(h, func(v history.LiveView) { recorded <- v }), recorded
}

func TestALiveViewIsRecordedWithWhoWatchedOnceItEnds(t *testing.T) {
	h, c, recorded := cameras(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: cameratest.New(t).URL()})
	alice := home.Origin{Person: "alice"}
	before := time.Now()

	l, err := c.Watch(context.Background(), cam, alice)
	if err != nil {
		t.Fatal(err)
	}
	if l.ContentType != `video/mp4; codecs="avc1.42001F"` || !l.Until.IsZero() {
		t.Errorf("content type %q, until %v; want H.264 without a time limit", l.ContentType, l.Until)
	}
	watching(t, l.v)
	select {
	case v := <-recorded:
		t.Fatalf("recorded while watched: %+v", v)
	default:
	}
	l.Close()
	l.Close()

	v := <-recorded
	if v.Target != cam || v.Origin != alice || v.Start.Before(before) || v.End.Before(v.Start) {
		t.Errorf("recorded %+v, want the camera's, by Alice, after %v", v, before)
	}
	if len(recorded) != 0 {
		t.Error("recorded more than once")
	}
}

func TestALiveViewOfACameraOnBatteryEndsInTime(t *testing.T) {
	liveViewFor = 300 * time.Millisecond
	t.Cleanup(func() { liveViewFor = 5 * time.Minute })
	h, c, _ := cameras(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: cameratest.New(t).URL()}, cameratest.Battery)

	l, err := c.Watch(context.Background(), cam, home.Origin{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if time.Until(l.Until) > liveViewFor {
		t.Errorf("until %v, want within %v", l.Until, liveViewFor)
	}
	done := make(chan struct{})
	go func() { l.WriteTo(newSink()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the Live view went on past its time")
	}
}

func TestALiveViewIsRefusedWhenThereIsNone(t *testing.T) {
	h, c, _ := cameras(t)
	cam, _ := cameratest.Attach(t, h, "arlo", &cameratest.Bridge{URL: cameratest.New(t).URL()})
	other, _ := cameratest.Attach(t, h, "z2m", plain{})
	asleep, _ := cameratest.Attach(t, h, "cloud", &cameratest.Bridge{Err: errors.New(`Get "https://media.example/secret": timeout`)})
	away, port := cameratest.Attach(t, h, "away", &cameratest.Bridge{URL: cameratest.New(t).URL()})
	port.SetOnline(false)

	for target, want := range map[home.Target]error{
		home.TargetDevice(cam.Device(), "occupancy"): home.ErrNotFound,
		home.TargetDevice("unknown", "camera"):       home.ErrNotFound,
		other:                                        home.ErrNotFound,
		away:                                         home.ErrBridgeOffline,
		asleep:                                       ErrNoAnswer,
	} {
		_, err := c.Watch(context.Background(), target, home.Origin{})
		if !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", target, err, want)
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: tells what the Bridge said: %v", target, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Watch(ctx, cam, home.Origin{}); !errors.Is(err, context.Canceled) {
		t.Errorf("for whoever left: %v, want context.Canceled", err)
	}
}
