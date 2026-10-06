package api

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	_, hdl := handler(t)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; " +
			"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Strict-Transport-Security":    "max-age=31536000",
	}
	for _, c := range []struct{ path, cache string }{
		{"/", "no-cache"},
		{"/favicon.svg", "no-cache"},
		{"/assets/index-abc.js", "public, max-age=31536000, immutable"},
		{"/api/build", "no-store"},
		{"/api/updates", "no-store"},
	} {
		resp, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", c.path, resp.StatusCode)
		}
		for k, v := range want {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", c.path, k, got, v)
			}
		}
		p := resp.Header.Get("Permissions-Policy")
		for _, feature := range []string{"camera=()", "geolocation=()", "microphone=()", "web-share=(self)", "clipboard-write=(self)"} {
			if !strings.Contains(p, feature) {
				t.Errorf("%s: Permissions-Policy = %q, want %s", c.path, p, feature)
			}
		}
		if strings.Contains(p, "publickey-credentials") {
			t.Errorf("%s: Permissions-Policy = %q, want passkeys left to Oiko", c.path, p)
		}
		if got := resp.Header.Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control = %q, want %q", c.path, got, c.cache)
		}
	}
}

func TestIndexIsRevalidatedAgainstAnETag(t *testing.T) {
	_, hdl := handler(t)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	get := func(etag string) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	etag := get("").Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on index.html")
	}
	if resp := get(etag); resp.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match %s: %d, want 304", etag, resp.StatusCode)
	}
	if resp := get(`"stale"`); resp.StatusCode != http.StatusOK {
		t.Errorf("a stale ETag: %d, want 200", resp.StatusCode)
	}
}

func TestTheEventStreamOutlivesTheReadTimeoutAndEndsOnAStalledClient(t *testing.T) {
	saved := []time.Duration{readTimeout, streamWriteLimit, keepaliveEvery}
	readTimeout, streamWriteLimit, keepaliveEvery = 100*time.Millisecond, 100*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { readTimeout, streamWriteLimit, keepaliveEvery = saved[0], saved[1], saved[2] })

	_, hdl := handler(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan *stallingConn, 1)
	srv := httptest.NewUnstartedServer(nil)
	srv.Listener = stallingListener{ln, conns}
	srv.Config = Server("", hdl)
	srv.Start()
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/updates")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	conn := <-conns

	r := bufio.NewReader(resp.Body)
	for start := time.Now(); time.Since(start) < 10*readTimeout; {
		if _, err := r.ReadString('\n'); err != nil {
			t.Fatalf("the stream ended after %v: %v", time.Since(start), err)
		}
	}
	conn.stalled.Store(true)
	select {
	case <-conn.closed:
	case <-time.After(5 * time.Second):
		conn.Close() // lets the server stop
		t.Fatal("the stream goes on writing to a stalled client")
	}
}

// stallingListener hands each connection it accepts to conns, if it has room.
type stallingListener struct {
	net.Listener
	conns chan *stallingConn
}

func (l stallingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	s := &stallingConn{Conn: c, closed: make(chan struct{})}
	select {
	case l.conns <- s:
	default:
	}
	return s, nil
}

// stallingConn is a connection whose client, once stalled, reads nothing: a
// write blocks until its deadline, or forever without one.
type stallingConn struct {
	net.Conn
	stalled  atomic.Bool
	mu       sync.Mutex
	deadline time.Time
	once     sync.Once
	closed   chan struct{}
}

func (c *stallingConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}

func (c *stallingConn) Write(p []byte) (int, error) {
	if !c.stalled.Load() {
		return c.Conn.Write(p)
	}
	c.mu.Lock()
	d := c.deadline
	c.mu.Unlock()
	var expired <-chan time.Time // nil: never
	if !d.IsZero() {
		expired = time.After(time.Until(d))
	}
	select {
	case <-expired:
		return 0, os.ErrDeadlineExceeded
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *stallingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}
