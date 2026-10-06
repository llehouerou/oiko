package homekit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// accessoryFake answers like an FP2 over an already verified session: its
// description, a subscription, then one event; later requests get 204.
func accessoryFake(t *testing.T, conn net.Conn, subscribed chan<- []byte) {
	fixture, err := os.ReadFile("testdata/fp2.json")
	if err != nil {
		t.Error(err)
		return
	}
	r := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(r)
		if err != nil {
			return
		}
		body, _ := io.ReadAll(req.Body)
		switch {
		case req.Method == "GET" && req.URL.Path == "/accessories":
			fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/hap+json\r\nContent-Length: %d\r\n\r\n%s", len(fixture), fixture)
		case req.Method == "PUT" && req.URL.Path == "/characteristics":
			subscribed <- body
			fmt.Fprint(conn, "HTTP/1.1 204 No Content\r\n\r\n")
			event := `{"characteristics":[{"aid":1,"iid":2694,"value":1},{"aid":1,"iid":2674,"value":30}]}`
			fmt.Fprintf(conn, "EVENT/1.0 200 OK\r\nContent-Type: application/hap+json\r\nContent-Length: %d\r\n\r\n%s", len(event), event)
		default:
			fmt.Fprint(conn, "HTTP/1.1 204 No Content\r\n\r\n")
		}
	}
}

func TestFollowDescribesReportsAndRelaysEvents(t *testing.T) {
	var saved map[string]json.RawMessage
	b := newBridge(Pairings{Accessories: []Paired{{ID: "AA:BB"}}}, nil, func(a map[string]json.RawMessage) error { saved = a; return nil }, slog.Default())
	h := home.New(nil)
	b.port = h.Attach("homekit", b)
	b.port.SetOnline(true)

	ours, theirs := net.Pipe()
	defer ours.Close()
	subscribed := make(chan []byte, 1)
	go accessoryFake(t, theirs, subscribed)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	followed := make(chan error, 1)
	reported := make(chan struct{})
	go func() { followed <- b.follow(ctx, "AA:BB", ours, func() { close(reported) }) }()

	var sub struct {
		Characteristics []struct {
			IID uint64 `json:"iid"`
			EV  bool   `json:"ev"`
		} `json:"characteristics"`
	}
	json.Unmarshal(<-subscribed, &sub)
	select {
	case <-reported: // before subscribing: its Values are known
	default:
		t.Error("not reported once described")
	}
	var iids []uint64
	for _, c := range sub.Characteristics {
		if c.EV {
			iids = append(iids, c.IID)
		}
	}
	slices.Sort(iids)
	if !slices.Equal(iids, []uint64{2674, 2690, 2694}) {
		t.Errorf("subscribed to %v, want 2674, 2690 and 2694", iids)
	}

	value := func(s home.Snapshot, fn, capability string) any {
		for _, rv := range s.Values {
			if rv.Ref.Target.Function() == fn && rv.Ref.Capability == capability {
				return rv.Value.Data
			}
		}
		return nil
	}
	var s home.Snapshot
	for deadline := time.Now().Add(time.Second); ; time.Sleep(time.Millisecond) {
		var cancel func()
		s, _, cancel = h.Subscribe()
		cancel()
		if value(s, "occupancy/2692", "occupancy") == true || time.Now().After(deadline) {
			break
		}
	}
	if len(s.Devices) != 1 {
		t.Fatalf("devices = %+v, want the FP2", s.Devices)
	}
	d := s.Devices[0]
	var keys []string
	for _, f := range d.Functions {
		keys = append(keys, f.Key)
	}
	if d.Name != "Presence-Sensor-FP2-1A2B" || d.Model != "PS-S02D" || d.Vendor != "Aqara" || d.Bridge != "homekit" ||
		!slices.Equal(keys, []string{"illuminance", "occupancy", "occupancy/2692"}) {
		t.Errorf("device = %+v, functions %v", d, keys)
	}
	for _, want := range []struct {
		fn, capability string
		data           any
	}{
		{"occupancy", "occupancy", true},      // as described
		{"occupancy/2692", "occupancy", true}, // from the event
		{"illuminance", "illuminance", float64(30)},
	} {
		if got := value(s, want.fn, want.capability); got != want.data {
			t.Errorf("%s/%s = %v, want %v", want.fn, want.capability, got, want.data)
		}
	}
	if a := s.Availability[home.TargetDevice(d.ID, "")]; a != home.Online {
		t.Errorf("availability = %s, want online", a)
	}
	if saved["AA:BB"] == nil {
		t.Error("description not saved")
	}

	theirs.Close() // the accessory goes away
	if err := <-followed; err == nil {
		t.Error("follow returned no error once the session broke")
	}
}

// An accessory that cannot be reached has nothing to replay: it does not hold
// the home back.
func TestUnreachableAccessoryDoesNotHoldTheReplay(t *testing.T) {
	b := newBridge(Pairings{Accessories: []Paired{{ID: "AA:BB", Public: "not hex"}}}, nil, nil, slog.Default())
	h := home.New(nil)
	b.Run(context.Background(), h.Attach("homekit", b)) // returns: its pairing is unusable
	select {
	case <-h.Known():
	case <-time.After(time.Second):
		t.Errorf("home not known; waiting for %v", h.Waiting())
	}
}
