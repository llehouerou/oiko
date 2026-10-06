// Package cameratest is a made-up camera serving RTSP, for tests of what
// reads it.
package cameratest

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/pion/rtp"
)

// avc is its video: parameter sets in the SDP, then frames that are each a
// key frame, enough for an MP4 muxer.
var avc = &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96,
	FmtpLine: "packetization-mode=1;profile-level-id=42001f;sprop-parameter-sets=Z0IAH5WoFAFuQA==,aM48gA=="}

// Camera serves one H.264 video over RTSP to each client, 50 frames a
// second, until the test ends or Drop cuts its clients off.
type Camera struct {
	ln       net.Listener
	sessions atomic.Int32
	mu       sync.Mutex
	conns    []net.Conn
}

// New starts a Camera, stopped when t ends.
func New(t testing.TB) *Camera {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &Camera{ln: ln}
	t.Cleanup(func() { ln.Close(); c.Drop() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			c.mu.Lock()
			c.conns = append(c.conns, nc)
			c.mu.Unlock()
			go c.serve(nc)
		}
	}()
	return c
}

// URL is where it serves its video.
func (c *Camera) URL() string { return "rtsp://" + c.ln.Addr().String() + "/live" }

// Sessions is how many RTSP sessions it described.
func (c *Camera) Sessions() int { return int(c.sessions.Load()) }

// Drop cuts every client off, as a camera going away.
func (c *Camera) Drop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, nc := range c.conns {
		nc.Close()
	}
	c.conns = nil
}

func (c *Camera) serve(nc net.Conn) {
	conn := rtsp.NewServer(nc)
	video := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{avc}}
	track := core.NewReceiver(video, avc)
	conn.Listen(func(msg any) {
		if msg == rtsp.MethodDescribe {
			c.sessions.Add(1)
			out := &core.Media{Kind: core.KindVideo, Direction: core.DirectionSendonly, Codecs: []*core.Codec{avc}}
			conn.Medias = []*core.Media{out}
			conn.AddTrack(out, avc, track)
		}
	})
	if conn.Accept() != nil {
		nc.Close()
		return
	}
	done := make(chan struct{})
	go func() { conn.Handle(); close(done) }()
	nal := [][]byte{{0x67, 0x42, 0x00, 0x1f, 0x95, 0xa8, 0x14, 0x01, 0x6e, 0x40}, {0x68, 0xce, 0x3c, 0x80}, {0x65, 0x88, 0x84, 0x00, 0x33, 0xff}}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for seq, ts := uint16(0), uint32(0); ; ts += 1800 {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		for i, p := range nal {
			seq++
			track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: seq, Timestamp: ts, Marker: i == len(nal)-1}, Payload: p})
		}
	}
}
