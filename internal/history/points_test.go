package history

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// points lists the points of a series, oldest first.
func points(t *testing.T, s *Store, target home.Target, capability string) []stored {
	t.Helper()
	rows, err := s.db.Query(`SELECT p.ts, p.value FROM points p JOIN series s ON s.id = p.series
		WHERE s.target = ? AND s.capability = ? ORDER BY p.ts`, target.Key(), capability)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ps []stored
	for rows.Next() {
		var p stored
		if err := rows.Scan(&p.ts, &p.value); err != nil {
			t.Fatal(err)
		}
		ps = append(ps, p)
	}
	return ps
}

var lamp = home.TargetDevice("lamp", "light")

func value(kind home.UpdateKind, ref home.Ref, data any, at time.Time) home.Update {
	return home.Update{Kind: kind, Ref: &ref, Value: &home.Value{Data: data, At: at}}
}

func TestEachTypeOfValueIsRecordedAndDecodesBack(t *testing.T) {
	s := open(t)
	at := time.Now()
	cases := []struct {
		capability string
		typ        home.ValueType
		data       any
	}{
		{"brightness", home.Numeric, 42.5},
		{"state", home.Binary, true},
		{"effect", home.Enum, "colorloop"},
		{"name", home.Text, "hello"},
		{"color", home.Composite, map[string]any{"x": 0.3, "y": 0.4}},
		{"scenes", home.List, []any{"a", map[string]any{"id": 1.0}}},
	}
	for _, c := range cases {
		s.Follow(value(home.ValueChanged, lamp.Ref(c.capability), c.data, at))
	}
	written(s)
	for _, c := range cases {
		ps := points(t, s, lamp, c.capability)
		if len(ps) != 1 || ps[0].ts != at.UnixNano() {
			t.Errorf("%s: points = %+v, want one at %d", c.capability, ps, at.UnixNano())
			continue
		}
		if got, err := decode(c.typ, ps[0].value); err != nil || !home.Equal(got, c.data) {
			t.Errorf("%s: decoded %#v, %v; want %#v", c.capability, got, err, c.data)
		}
	}
}

func TestOnlyChangesAndEveryEventAreRecorded(t *testing.T) {
	s := open(t)
	at := time.Now()
	ref, action := lamp.Ref("brightness"), home.TargetDevice("remote", "button").Ref("action")
	s.Follow(value(home.ValueChanged, ref, 10.0, at))
	s.Follow(value(home.ValueRefreshed, ref, 10.0, at.Add(time.Second)))
	s.Follow(value(home.ValueChanged, ref, 10.0, at.Add(2*time.Second))) // Home's flag is not trusted
	s.Follow(value(home.ValueChanged, ref, 20.0, at.Add(3*time.Second)))
	for i := range 3 {
		s.Follow(value(home.EventOccurred, action, "single", at.Add(time.Duration(i)*time.Second)))
	}
	s.Follow(home.Update{Kind: home.ValueChanged, Ref: &ref}) // an Aggregate's Value lost: nothing is synthesized
	written(s)

	if ps := points(t, s, lamp, "brightness"); len(ps) != 2 || ps[0].value != 10.0 || ps[1].value != 20.0 || ps[1].ts != at.Add(3*time.Second).UnixNano() {
		t.Errorf("values = %+v, want 10 then 20", ps)
	}
	if ps := points(t, s, action.Target, "action"); len(ps) != 3 {
		t.Errorf("events = %+v, want all 3", ps)
	}
}

func TestAStampAtOrBeforeTheLastPointIsClamped(t *testing.T) {
	s := open(t)
	at := time.Now()
	ref := lamp.Ref("brightness")
	s.Follow(value(home.ValueChanged, ref, 1.0, at))
	s.Follow(value(home.ValueChanged, ref, 2.0, at))
	s.Follow(value(home.ValueChanged, ref, 3.0, at.Add(-time.Hour)))
	written(s)
	want := []int64{at.UnixNano(), at.UnixNano() + 1, at.UnixNano() + 2}
	ps := points(t, s, lamp, "brightness")
	if len(ps) != 3 || ps[0].ts != want[0] || ps[1].ts != want[1] || ps[2].ts != want[2] {
		t.Errorf("points = %+v, want at %v", ps, want)
	}
}

func TestAReplayedValueIsRecordedOnlyIfItDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	state, brightness := lamp.Ref("state"), lamp.Ref("brightness")
	s.Follow(value(home.ValueChanged, state, false, at))
	s.Follow(value(home.ValueChanged, brightness, 10.0, at))
	written(s)
	s.Close()

	s, err = Open(path) // Oiko restarts, its Bridge replays
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Home recalls when a first Value took its Data, if the History's last point holds it.
	if since, ok := s.Recall(state, false); !ok || !since.Equal(time.Unix(0, at.UnixNano())) {
		t.Errorf("recalled state = %v %v, want since %v", since, ok, at)
	}
	if _, ok := s.Recall(brightness, 30.0); ok {
		t.Error("recalled a brightness the History never held last")
	}
	if _, ok := s.Recall(lamp.Ref("color"), "red"); ok {
		t.Error("recalled a series never recorded")
	}
	replayed := at.Add(time.Minute)
	s.Follow(home.Update{Kind: home.ValueChanged, Ref: &state, Value: &home.Value{Data: false, At: replayed}, Initial: true})
	s.Follow(home.Update{Kind: home.ValueChanged, Ref: &brightness, Value: &home.Value{Data: 30.0, At: replayed}, Initial: true})
	written(s)

	if ps := points(t, s, lamp, "state"); len(ps) != 1 {
		t.Errorf("state = %+v, want the replayed equal Value not recorded", ps)
	}
	if ps := points(t, s, lamp, "brightness"); len(ps) != 2 || ps[1].ts != replayed.UnixNano() || ps[1].value != 30.0 {
		t.Errorf("brightness = %+v, want the replayed change at its time", ps)
	}
}

func TestADroppedPointIsRecordedAtTheNextRefresh(t *testing.T) {
	s := open(t)
	at := time.Now()
	ref := lamp.Ref("brightness")
	s.Follow(value(home.ValueChanged, ref, 10.0, at))
	written(s)
	for range buffered {
		s.Command(command("c", home.Confirmed, at, home.Origin{}))
	}
	s.Follow(value(home.ValueChanged, ref, 20.0, at.Add(time.Second))) // dropped
	if s.Lost() != 1 {
		t.Fatalf("lost = %d, want the change dropped", s.Lost())
	}
	written(s)
	s.Follow(value(home.ValueRefreshed, ref, 20.0, at.Add(2*time.Second)))
	written(s)
	if ps := points(t, s, lamp, "brightness"); len(ps) != 2 || ps[1].value != 20.0 || ps[1].ts != at.Add(2*time.Second).UnixNano() {
		t.Errorf("points = %+v, want 20 at the refresh", ps)
	}
}

// following feeds a new Home to s and runs s's writer until stop is called.
func following(t *testing.T, s *Store) (*home.Home, func()) {
	t.Helper()
	h := home.New(s.Command)
	h.Follow(s.Follow)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	return h, func() {
		cancel()
		<-done
	}
}

type nopBridge struct{}

func (nopBridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

func plug(address string) bridge.Device {
	caps := []home.Capability{{Key: "state", Type: home.Binary, Access: home.Access{Observable: true, Settable: true}}}
	for _, k := range []string{"power", "current", "voltage", "energy"} {
		caps = append(caps, home.Capability{Key: k, Type: home.Numeric, Access: home.Access{Observable: true}})
	}
	return bridge.Device{NativeAddress: address,
		Functions:    []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: caps}},
		Capabilities: []home.Capability{{Key: "linkquality", Type: home.Numeric, Access: home.Access{Observable: true}}}}
}

func plugReport(i int) []bridge.Reading {
	return []bridge.Reading{
		{Function: "switch", Capability: "state", Data: i%2 == 0},
		{Function: "switch", Capability: "power", Data: float64(i)},
		{Function: "switch", Capability: "current", Data: float64(i) / 230},
		{Function: "switch", Capability: "voltage", Data: 230.0},
		{Function: "switch", Capability: "energy", Data: float64(i) / 1000},
		{Capability: "linkquality", Data: float64(i % 7)},
	}
}

// firstDevice is the ID of h's first Device.
func firstDevice(h *home.Home) home.DeviceID {
	snap, cancel := h.Follow(func(home.Update) {})
	cancel()
	return snap.Devices[0].ID
}

func TestAvailabilityIsRecordedForDevicesAggregatesAndFlags(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.SyncDevices([]bridge.Device{plug("0xplug")})
	z.SetAvailability("0xplug", home.Online)
	device := firstDevice(h)
	aggregate, err := h.CreateAggregate("plugs", []home.Target{home.TargetDevice(device, "switch")}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	flag, err := h.CreateFlag("away")
	if err != nil {
		t.Fatal(err)
	}
	z.Report("0xplug", plugReport(0), time.Now())
	z.SetAvailability("0xplug", home.Offline)
	stop()

	for target, want := range map[home.Target][]any{
		home.TargetDevice(device, ""):   {"online", "offline"},
		home.TargetAggregate(aggregate): {"online", "offline"},
		home.TargetFlag(flag):           {"online"},
	} {
		ps := points(t, s, target, "")
		got := make([]any, len(ps))
		for i, p := range ps {
			got[i] = p.value
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: availability = %v, want %v", target, got, want)
		}
	}
	for _, ref := range []home.Ref{home.TargetDevice(device, "switch").Ref("power"), home.TargetDevice(device, "").Ref("linkquality"),
		home.TargetAggregate(aggregate).Ref("power"), home.TargetFlag(flag).Ref("on")} {
		if ps := points(t, s, ref.Target, ref.Capability); len(ps) != 1 {
			t.Errorf("%s %s: points = %+v, want one", ref.Target, ref.Capability, ps)
		}
	}
}

func TestABurstOf1000ReportsDropsNothing(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.SyncDevices([]bridge.Device{plug("0xplug")})
	for i := range 1000 {
		z.Report("0xplug", plugReport(i), time.Now())
	}
	stop()
	if n := s.Lost(); n != 0 {
		t.Errorf("%d points lost", n)
	}
	if ps := points(t, s, home.TargetDevice(firstDevice(h), "switch"), "power"); len(ps) != 1000 {
		t.Errorf("%d power points, want 1000", len(ps))
	}
}

func TestAStartupReplayOfEveryDeviceDropsNothing(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	var devices []bridge.Device
	for i := range 200 {
		devices = append(devices, plug(string(rune('a'+i%26))+string(rune('a'+i/26))))
	}
	// As zigbee2mqtt's retained messages arrive on connection: the
	// description, then each Device's Availability and state at once.
	z.SyncDevices(devices)
	z.SetOnline(true)
	for i, d := range devices {
		z.SetAvailability(d.NativeAddress, home.Online)
		z.Report(d.NativeAddress, plugReport(i), time.Now())
	}
	z.Replayed()
	stop()
	if n := s.Lost(); n != 0 {
		t.Errorf("%d points lost", n)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM points`).Scan(&n); err != nil || n != 200*7 {
		t.Errorf("%d points, %v; want 7 per Device", n, err)
	}
}

func TestTheWriterSustains100ChangesPerSecond(t *testing.T) {
	s := open(t)
	start, at := time.Now(), time.Now()
	for i := range 100 { // at 100/s, each change comes alone: one commit each
		s.Follow(value(home.ValueChanged, lamp.Ref("brightness"), float64(i), at))
		written(s)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("100 changes written in %v, want under 1 s", d)
	}
}
