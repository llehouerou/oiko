package camera

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/camera/cameratest"
	"github.com/llehouerou/oiko/internal/home"
)

// sink is what a viewer reads, telling once it holds a media fragment.
type sink struct {
	mu       sync.Mutex
	b        bytes.Buffer
	fragment chan struct{}
	once     sync.Once
}

func newSink() *sink { return &sink{fragment: make(chan struct{})} }

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Write(p)
	if bytes.Contains(s.b.Bytes(), []byte("moof")) {
		s.once.Do(func() { close(s.fragment) })
	}
	return len(p), nil
}

// watching reads v into a sink until it ends, which ended tells.
func watching(t *testing.T, v *Viewer) (s *sink, ended <-chan struct{}) {
	t.Helper()
	s = newSink()
	done := make(chan struct{})
	go func() { v.WriteTo(s); close(done) }()
	select {
	case <-s.fragment:
	case <-time.After(5 * time.Second):
		t.Fatal("no media fragment")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !bytes.HasPrefix(s.b.Bytes()[4:], []byte("ftyp")) {
		t.Errorf("the Live view starts with %q, want its initialization segment", s.b.Bytes()[:8])
	}
	return s, done
}

func relayOf(t *testing.T, cam *cameratest.Camera, asked *atomic.Int32) *Relay {
	t.Helper()
	return New(func(context.Context, home.Target) (string, error) {
		asked.Add(1)
		return cam.URL(), nil
	})
}

var garden = home.TargetDevice("garden", "camera")

func TestViewersShareOneConnectionKeptOpenForAGrace(t *testing.T) {
	grace = 300 * time.Millisecond
	t.Cleanup(func() { grace = 10 * time.Second })
	cam := cameratest.New(t)
	var asked atomic.Int32
	r := relayOf(t, cam, &asked)

	a, err := r.Watch(context.Background(), garden)
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentType != `video/mp4; codecs="avc1.42001F"` {
		t.Errorf("content type %q", a.ContentType)
	}
	watching(t, a)
	b, err := r.Watch(context.Background(), garden)
	if err != nil {
		t.Fatal(err)
	}
	watching(t, b)
	a.Close()
	b.Close()

	// Back within the grace: the same connection.
	c, err := r.Watch(context.Background(), garden)
	if err != nil {
		t.Fatal(err)
	}
	watching(t, c)
	if n, m := asked.Load(), cam.Sessions(); n != 1 || m != 1 {
		t.Errorf("for three viewers, the Bridge was asked %d times and the camera opened %d sessions, want 1 and 1", n, m)
	}
	c.Close()

	// Past it, closed: the next viewer opens a new one.
	time.Sleep(3 * grace)
	r.mu.Lock()
	open := len(r.feeds)
	r.mu.Unlock()
	if open != 0 {
		t.Errorf("%d connections open with nobody watching", open)
	}
	d, err := r.Watch(context.Background(), garden)
	if err != nil {
		t.Fatal(err)
	}
	watching(t, d)
	d.Close()
	if n := asked.Load(); n != 2 {
		t.Errorf("the Bridge was asked %d times, want twice", n)
	}
}

func TestViewersAreBounded(t *testing.T) {
	cam := cameratest.New(t)
	var asked atomic.Int32
	r := relayOf(t, cam, &asked)
	var viewers []*Viewer
	t.Cleanup(func() {
		for _, v := range viewers {
			v.Close()
		}
	})
	watch := func(t home.Target) error {
		v, err := r.Watch(context.Background(), t)
		if err == nil {
			viewers = append(viewers, v)
		}
		return err
	}
	for range PerCamera {
		if err := watch(garden); err != nil {
			t.Fatal(err)
		}
	}
	if err := watch(garden); !errors.Is(err, ErrBusy) {
		t.Errorf("one more viewer of a camera: %v, want ErrBusy", err)
	}
	for i := 0; len(viewers) < InAll; i++ {
		if err := watch(home.TargetDevice(home.DeviceID("cam"+string(rune('a'+i/PerCamera))), "camera")); err != nil {
			t.Fatal(err)
		}
	}
	if err := watch(home.TargetDevice("other", "camera")); !errors.Is(err, ErrBusy) {
		t.Errorf("one more viewer in all: %v, want ErrBusy", err)
	}
	viewers[0].Close()
	viewers = viewers[1:]
	if err := watch(garden); err != nil {
		t.Errorf("once one left: %v", err)
	}
}

func TestACameraGoingAwayEndsItsViewers(t *testing.T) {
	cam := cameratest.New(t)
	var asked atomic.Int32
	r := relayOf(t, cam, &asked)
	v, err := r.Watch(context.Background(), garden)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	_, ended := watching(t, v)
	cam.Drop()
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the Live view outlived its camera")
	}
}

func TestACameraThatCannotBeReachedIsTriedAgain(t *testing.T) {
	var asked atomic.Int32
	r := New(func(context.Context, home.Target) (string, error) {
		asked.Add(1)
		return "", errors.New("the camera is asleep")
	})
	for range 2 {
		if _, err := r.Watch(context.Background(), garden); err == nil {
			t.Fatal("watched a camera that cannot be reached")
		}
	}
	if n := asked.Load(); n != 2 {
		t.Errorf("asked %d times, want each time", n)
	}
}
