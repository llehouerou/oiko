// Package homekit makes Oiko a HomeKit controller (ADR 0008): it keeps an
// encrypted session with each paired accessory and relays what it reports.
package homekit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/store"
)

func init() {
	bridge.Register(bridge.Module{Type: "homekit", New: open,
		Commands: map[string]func(bridge.Env, []string) error{"pair": pair}})
}

// Pairings is Oiko's identity as a HomeKit controller and the accessories
// paired with it. It holds a private key: only pairing writes it.
type Pairings struct {
	Controller  Controller `json:"controller"`
	Accessories []Paired   `json:"accessories"`
}

type Controller struct {
	ID  string `json:"id"`
	Key string `json:"key"` // ed25519 private key, hex
}

type Paired struct {
	ID     string `json:"id"`     // the accessory's device ID: its Native Address
	Public string `json:"public"` // its ed25519 public key, hex
}

const (
	requestTimeout = 10 * time.Second
	keepAlive      = time.Minute // a request this often notices a dead session
	maxBackoff     = time.Minute
)

// Bridge is Oiko's HomeKit controller.
type Bridge struct {
	pairings Pairings
	save     func(map[string]json.RawMessage) error // the last /accessories of each accessory

	mu        sync.Mutex
	port      bridge.Port
	raw       map[string]json.RawMessage // by accessory ID
	described map[string]described
}

// The formats of pairings.json and accessories.json (ADR 0019).
var pairingsFormat, accessoriesFormat store.Format

// open follows the accessories paired in the data directory: pairings.json,
// which only pairing writes, and accessories.json, their last descriptions.
func open(env bridge.Env) (bridge.Bridge, error) {
	pairings, accessories := filepath.Join(env.DataDir, "pairings.json"), filepath.Join(env.DataDir, "accessories.json")
	var p Pairings
	var saved map[string]json.RawMessage
	for _, s := range []struct {
		file string
		f    store.Format
		v    any
	}{{pairings, pairingsFormat, &p}, {accessories, accessoriesFormat, &saved}} {
		if err := store.Load(s.file, s.f, s.v); err != nil {
			return nil, fmt.Errorf("homekit: %w", err)
		}
	}
	return newBridge(p, saved, func(a map[string]json.RawMessage) error { return store.Save(accessories, accessoriesFormat, a) }), nil
}

// newBridge starts from the /accessories documents last saved, so that
// accessories out of reach at startup keep their Device.
func newBridge(p Pairings, saved map[string]json.RawMessage, save func(map[string]json.RawMessage) error) *Bridge {
	b := &Bridge{pairings: p, save: save, raw: map[string]json.RawMessage{}, described: map[string]described{}}
	for _, a := range p.Accessories {
		if raw, ok := saved[a.ID]; ok {
			if d, _, err := describe(a.ID, raw); err == nil {
				b.raw[a.ID], b.described[a.ID] = raw, d
			}
		}
	}
	return b
}

// Run keeps a session with each paired accessory and feeds Oiko through p
// until ctx is cancelled. The Bridge has replayed once each accessory has
// had its first session: it reported its Values, or it is out of reach.
func (b *Bridge) Run(ctx context.Context, p bridge.Port) {
	b.mu.Lock()
	b.port = p
	b.syncDevices()
	b.mu.Unlock()
	p.SetOnline(true)
	var first sync.WaitGroup
	first.Add(len(b.pairings.Accessories))
	go func() {
		first.Wait()
		p.Replayed()
	}()
	var wg sync.WaitGroup
	for _, a := range b.pairings.Accessories {
		tried := sync.OnceFunc(first.Done)
		wg.Go(func() { b.keep(ctx, a, tried) })
	}
	wg.Wait()
}

// Send refuses: no Capability of a HomeKit Device is settable yet.
func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error {
	return errors.New("homekit: commands are not supported")
}

// syncDevices hands Oiko every accessory described so far. Callers hold b.mu.
func (b *Bridge) syncDevices() {
	devices := make([]bridge.Device, 0, len(b.described))
	for _, d := range b.described {
		devices = append(devices, d.device)
	}
	b.port.SyncDevices(devices)
}

// keep reconnects to accessory a whenever its session ends, backing off
// while it stays out of reach. tried is called once its first session has
// reported, or failed.
func (b *Bridge) keep(ctx context.Context, a Paired, tried func()) {
	defer tried() // never connected: nothing to replay
	c := &hap.Client{DeviceID: a.ID, ClientID: b.pairings.Controller.ID}
	var err error
	if c.DevicePublic, err = hex.DecodeString(a.Public); err == nil {
		c.ClientPrivate, err = hex.DecodeString(b.pairings.Controller.Key)
	}
	if err != nil {
		log.Printf("homekit: %s: bad pairing: %v", a.ID, err)
		return
	}
	backoff := time.Second
	for {
		started := time.Now()
		err := b.connect(ctx, c, tried)
		tried() // out of reach: nothing to replay until it comes back
		b.port.SetAvailability(a.ID, bridge.Offline)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > maxBackoff {
			backoff = time.Second
		}
		log.Printf("homekit: %s: %v; retrying in %v", a.ID, err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = min(2*backoff, maxBackoff)
	}
}

// connect opens a session with c's accessory, found by mDNS, and follows it
// until it ends.
func (b *Bridge) connect(ctx context.Context, c *hap.Client, reported func()) error {
	if err := c.Dial(); err != nil { // pair-verify: the session is encrypted from here
		c.Close() // Dial leaves its connection open when verifying fails
		return err
	}
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	defer c.Close()
	return b.follow(ctx, c.DeviceID, c.Conn, reported)
}

// follow describes the accessory, reports its Values, then calls reported,
// subscribes to their changes and relays them until the session fails. conn
// is the session.
func (b *Bridge) follow(ctx context.Context, id string, conn io.ReadWriter, reported func()) error {
	s := &session{conn: conn, messages: make(chan message), failed: make(chan struct{}), done: make(chan struct{})}
	defer close(s.done)
	go s.read()

	raw, err := s.do("GET", "/accessories", nil)
	if err != nil {
		return err
	}
	d, values, err := describe(id, raw)
	if err != nil {
		return err
	}
	b.describe(id, raw, d)
	b.port.Report(id, values, time.Now())
	reported()
	s.onEvent = func(body []byte) {
		var v struct {
			Characteristics []characteristic `json:"characteristics"`
		}
		if err := json.Unmarshal(body, &v); err != nil {
			log.Printf("homekit: %s: event: %v", id, err)
			return
		}
		b.port.Report(id, d.values(v.Characteristics), time.Now())
	}

	type ev struct {
		AID uint64 `json:"aid"`
		IID uint64 `json:"iid"`
		EV  bool   `json:"ev"`
	}
	var sub struct {
		Characteristics []ev `json:"characteristics"`
	}
	ping := "/accessories"
	for _, iid := range d.events {
		sub.Characteristics = append(sub.Characteristics, ev{1, iid, true})
		ping = fmt.Sprintf("/characteristics?id=1.%d", iid)
	}
	body, _ := json.Marshal(sub)
	if _, err := s.do("PUT", "/characteristics", body); err != nil {
		return err
	}
	b.port.SetAvailability(id, bridge.Online)

	tick := time.NewTicker(keepAlive)
	defer tick.Stop()
	for {
		select {
		case m := <-s.messages:
			if m.event {
				s.onEvent(m.body)
			}
		case <-s.failed:
			return s.failure
		case <-tick.C:
			if _, err := s.do("GET", ping, nil); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// describe records accessory id's description and, if its Device changed,
// saves it and hands Oiko the Devices.
func (b *Bridge) describe(id string, raw []byte, d described) {
	b.mu.Lock()
	defer b.mu.Unlock()
	old, known := b.described[id]
	b.raw[id], b.described[id] = raw, d
	if known && reflect.DeepEqual(old.device, d.device) {
		return
	}
	if err := b.save(b.raw); err != nil {
		log.Printf("homekit: saving accessories: %v", err)
	}
	b.syncDevices()
}

// session is HTTP over a HAP session: one request at a time, its response
// interleaved with events. Only the goroutine following it reads messages.
type session struct {
	conn     io.ReadWriter
	messages chan message
	failed   chan struct{} // closed once reading failed, with failure
	failure  error
	done     chan struct{} // closed once the session is left
	onEvent  func(body []byte)
}

type message struct {
	event bool
	body  []byte
}

// read hands messages over until the session fails or is left.
func (s *session) read() {
	r := bufio.NewReaderSize(s.conn, 64<<10) // a HAP frame is read whole: 1 KiB at least
	for {
		res, err := hap.ReadResponse(r, nil)
		var body []byte
		if err == nil {
			body, err = io.ReadAll(res.Body)
		}
		event := err == nil && res.Proto == hap.ProtoEvent
		if err == nil && !event && res.StatusCode >= 400 {
			err = fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(body)))
		}
		if err != nil {
			s.failure = err
			close(s.failed)
			return
		}
		select {
		case s.messages <- message{event, body}:
		case <-s.done:
			return
		}
	}
}

// do sends a request and waits for its response body, handing events that
// arrive meanwhile to onEvent.
func (s *session) do(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, "http://accessory"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", hap.MimeJSON)
	}
	var buf bytes.Buffer
	req.Write(&buf)
	if _, err := s.conn.Write(buf.Bytes()); err != nil { // one write: one frame for a small request
		return nil, err
	}
	timeout := time.NewTimer(requestTimeout)
	defer timeout.Stop()
	for {
		select {
		case m := <-s.messages:
			if !m.event {
				return m.body, nil
			}
			if s.onEvent != nil {
				s.onEvent(m.body)
			}
		case <-s.failed:
			return nil, s.failure
		case <-timeout.C:
			return nil, fmt.Errorf("%s %s: no response after %v", method, path, requestTimeout)
		}
	}
}
