// Package camera is what Oiko does with its cameras (ADR 0036-0039): their
// Live views, relayed through one connection to each camera's RTSP URL, read
// by go2rtc and shared by everyone watching it, its media muxed into
// fragmented MP4 for each viewer, without transcoding.
package camera

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/pion/rtp"

	"github.com/llehouerou/oiko/internal/home"
)

// ErrBusy refuses a viewer past PerCamera on its camera or InAll in all.
var ErrBusy = errors.New("too many Live views")

// ErrEnded ends the viewers of a camera whose connection ended.
var ErrEnded = errors.New("the camera's connection ended")

const (
	PerCamera = 4
	InAll     = 16
	// queued bounds the fragments waiting for a viewer, a few seconds of
	// video: one that falls further behind is ended.
	queued = 128
)

// Variables so tests can shorten them.
var (
	// grace keeps a camera's connection open after its last viewer leaves:
	// one coming back meanwhile does not wake it again.
	grace = 10 * time.Second
	// openWithin bounds asking the Bridge for a URL and the RTSP handshake.
	openWithin = 30 * time.Second
)

// relay holds the connection to each camera someone watches. One mutex
// guards every feed and viewer: a few viewers, a few hundred packets a
// second.
type relay struct {
	open    func(context.Context, home.Target) (string, error)
	mu      sync.Mutex
	feeds   map[home.Target]*feed
	viewers int // in all
}

// feed is one camera's connection and who watches it.
type feed struct {
	t       home.Target
	ready   chan struct{} // closed once conn is set, or the feed ended
	conn    io.Closer     // its network connection: closing it ends connect's
	codecs  []*core.Codec // of its tracks: H.264 video, AAC audio
	viewers map[*viewer]bool
	idle    *time.Timer // closes it, once nobody watches
	ended   bool
}

// newRelay is a relay asking open for a camera's URL: the Bridge's Stream.
func newRelay(open func(context.Context, home.Target) (string, error)) *relay {
	return &relay{open: open, feeds: map[home.Target]*feed{}}
}

// viewer is one Live view of a camera, from a key frame on.
type viewer struct {
	r       *relay
	f       *feed
	mux     *mp4.Muxer    // from its first key frame
	out     chan []byte   // its initialization segment, then its fragments
	started chan struct{} // closed once ContentType is set
	gone    chan struct{} // closed once it ended
	err     error         // why it ended, if not by Close
	// ContentType is its media's, with their codecs, for a MediaSource.
	ContentType string
}

// Watch opens a Live view of camera t, connecting to it unless someone
// already watches it or did within grace, and returns once it starts with a
// key frame. ctx bounds that wait only.
func (r *relay) Watch(ctx context.Context, t home.Target) (*viewer, error) {
	r.mu.Lock()
	f := r.feeds[t]
	if r.viewers >= InAll || f != nil && len(f.viewers) >= PerCamera {
		r.mu.Unlock()
		return nil, ErrBusy
	}
	if f == nil {
		f = &feed{t: t, ready: make(chan struct{}), viewers: map[*viewer]bool{}}
		r.feeds[t] = f
		go r.connect(f)
	}
	if f.idle != nil {
		f.idle.Stop()
		f.idle = nil
	}
	v := &viewer{r: r, f: f, out: make(chan []byte, queued), started: make(chan struct{}), gone: make(chan struct{})}
	f.viewers[v] = true
	r.viewers++
	r.mu.Unlock()

	select {
	case <-v.started:
		return v, nil
	case <-v.gone:
		return nil, v.err
	case <-ctx.Done():
		v.Close()
		return nil, ctx.Err()
	}
}

// WriteTo writes the Live view to w until it is closed, falls behind, or
// its camera's connection ends: the initialization segment, then fragments.
func (v *viewer) WriteTo(w io.Writer) (int64, error) {
	var n int64
	for b := range v.out {
		m, err := w.Write(b)
		n += int64(m)
		if err != nil {
			v.Close()
			return n, err
		}
	}
	return n, v.err
}

// Close ends the Live view; WriteTo returns.
func (v *viewer) Close() { v.r.leave(v) }

// connect asks the Bridge for f's URL, opens it and plays it. A camera that
// cannot be reached leaves no feed behind.
func (r *relay) connect(f *feed) {
	ctx, cancel := context.WithTimeout(context.Background(), openWithin)
	defer cancel()
	u, err := r.open(ctx, f.t)
	var conn *rtsp.Conn
	if err == nil {
		conn, err = dial(u)
	}
	if err == nil {
		if err = r.tracks(f, conn); err != nil {
			conn.Stop()
		}
	}
	if err != nil {
		r.end(f, fmt.Errorf("%s: %w", f.t, err))
		return
	}
	r.mu.Lock()
	f.conn, _ = conn.Connection.Transport.(io.Closer)
	r.mu.Unlock()
	close(f.ready)
	conn.Start() // until the camera, or closeIdle, ends it
	r.end(f, ErrEnded)
	conn.Stop()
}

// dial opens an RTSP session on u and reads what it offers. go2rtc bounds
// each step, and ends the session once the camera sends nothing for 5 s.
func dial(u string) (*rtsp.Conn, error) {
	conn := rtsp.NewClient(u)
	if err := conn.Dial(); err != nil {
		return nil, err
	}
	if err := conn.Describe(); err != nil {
		conn.Stop()
		return nil, err
	}
	return conn, nil
}

// tracks sets up what a browser plays of what conn offers, H.264 video and
// AAC audio, each read once for every viewer of f.
func (r *relay) tracks(f *feed, conn *rtsp.Conn) error {
	for _, m := range conn.GetMedias() {
		if m.Direction != core.DirectionRecvonly {
			continue
		}
		for _, c := range m.Codecs {
			if c.Name != core.CodecH264 && c.Name != core.CodecAAC || hasKind(f.codecs, m.Kind) {
				continue
			}
			track, err := conn.GetTrack(m, c)
			if err != nil {
				return err
			}
			i := len(f.codecs)
			f.codecs = append(f.codecs, c.Clone())
			deliver := func(p *rtp.Packet) { r.deliver(f, i, p) }
			s := core.NewSender(m, c)
			switch {
			case c.Name == core.CodecAAC && c.IsRTP():
				s.Handler = aac.RTPDepay(deliver)
			case c.Name == core.CodecAAC:
				s.Handler = deliver
			case c.IsRTP():
				s.Handler = h264.RTPDepay(c, deliver)
			default:
				s.Handler = h264.RepairAVCC(c, deliver)
			}
			s.HandleRTP(track)
			break
		}
	}
	if len(f.codecs) == 0 {
		return errors.New("no media a browser plays")
	}
	return nil
}

func hasKind(codecs []*core.Codec, kind string) bool {
	for _, c := range codecs {
		if core.GetKind(c.Name) == kind {
			return true
		}
	}
	return false
}

// deliver hands packet p of track i to every viewer of f. A viewer starts
// with a key frame, with an initialization segment for the codecs as known
// then; one with no room left is ended.
func (r *relay) deliver(f *feed, i int, p *rtp.Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	video := f.codecs[i].Name == core.CodecH264
	key := video && h264.IsKeyframe(p.Payload)
	if key && !strings.Contains(f.codecs[i].FmtpLine, "sprop-parameter-sets") {
		// The camera tells its parameter sets in band only: from its key frame.
		if c := h264.AVCCToCodec(p.Payload); c != nil {
			f.codecs[i] = c
		}
	}
	for v := range f.viewers {
		if v.mux == nil {
			if !key && (video || hasKind(f.codecs, core.KindVideo)) {
				continue
			}
			v.mux = &mp4.Muxer{}
			for _, c := range f.codecs {
				v.mux.AddTrack(c)
			}
			init, err := v.mux.GetInit()
			if err != nil {
				r.endViewer(v, err)
				continue
			}
			v.ContentType = mp4.ContentType(f.codecs)
			v.out <- init
			close(v.started)
		}
		select {
		case v.out <- v.mux.GetPayload(byte(i), p):
		default:
			r.endViewer(v, errors.New("the viewer fell behind"))
		}
	}
}

// endViewer ends v with err. Callers hold r.mu.
func (r *relay) endViewer(v *viewer, err error) {
	if !v.f.viewers[v] {
		return
	}
	delete(v.f.viewers, v)
	r.viewers--
	v.err = err
	close(v.out)
	close(v.gone)
}

// end ends f and each of its viewers with err. Its connection, if open,
// stops: connect stops it.
func (r *relay) end(f *feed, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.ended {
		return
	}
	f.ended = true
	if r.feeds[f.t] == f {
		delete(r.feeds, f.t)
	}
	if f.idle != nil {
		f.idle.Stop()
	}
	for v := range f.viewers {
		r.endViewer(v, err)
	}
	if f.conn == nil {
		close(f.ready)
	}
}

// leave takes v off its feed. The last one leaves the connection open for
// grace.
func (r *relay) leave(v *viewer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := v.f
	r.endViewer(v, nil)
	if f.ended || len(f.viewers) > 0 || f.idle != nil {
		return
	}
	f.idle = time.AfterFunc(grace, func() { r.closeIdle(f) })
}

// closeIdle closes f if nobody came back within grace.
func (r *relay) closeIdle(f *feed) {
	<-f.ready
	r.mu.Lock()
	idle := !f.ended && f.idle != nil && len(f.viewers) == 0
	conn := f.conn
	r.mu.Unlock()
	if idle {
		r.end(f, nil)
		conn.Close() // connect returns, and stops the session
	}
}
