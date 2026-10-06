package zigbee2mqtt

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

// newTestBridge feeds the fixture to a Bridge wired to a Home, without MQTT.
func newTestBridge(t testing.TB) (*Bridge, *home.Home) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/devices.json")
	if err != nil {
		t.Fatal(err)
	}
	b := newBridge("zigbee2mqtt", nil, slog.Default())
	h := home.New(nil)
	b.home = h.Attach("zigbee2mqtt", b)
	b.handle("zigbee2mqtt/bridge/state", []byte(`{"state":"online"}`), true)
	b.handle("zigbee2mqtt/bridge/devices", fixture, true)
	return b, h
}

func snapshot(h *home.Home) home.Snapshot {
	s, _, cancel := h.Subscribe()
	cancel()
	return s
}

func byName(s home.Snapshot, name string) home.Device {
	for _, d := range s.Devices {
		if d.Name == name {
			return d
		}
	}
	return home.Device{}
}

// layout renders a Device as "fn[caps] fn[caps] | device caps".
func layout(d home.Device) string {
	keys := func(cs []home.Capability) string {
		var ks []string
		for _, c := range cs {
			ks = append(ks, c.Key)
		}
		return strings.Join(ks, ",")
	}
	var parts []string
	for _, f := range d.Functions {
		parts = append(parts, f.Key+"["+keys(f.Capabilities)+"]")
	}
	return strings.Join(parts, " ") + " | " + keys(d.Capabilities)
}

func TestDeriveFunctions(t *testing.T) {
	_, h := newTestBridge(t)
	s := snapshot(h)
	for name, want := range map[string]string{
		"kitchen_bulb":        "light[state,brightness,color_temp,color_temp_startup,color_xy,color_hs,color_mode,effect] | power_on_behavior,linkquality",
		"desk_plug":           "switch[state,power,energy,produced_energy,child_lock] | linkquality",
		"living_room_climate": "temperature[temperature] humidity[humidity] pressure[pressure] | battery,voltage,linkquality",
		"hallway_switch":      "switch/left[state,operation_mode] switch/right[state] button[action] | device_temperature,linkquality",
		"bedroom_remote":      "button[action] | battery,linkquality",
		"veranda_motion":      "occupancy[occupancy] illuminance[illuminance] temperature[temperature] | motion_sensitivity,battery,linkquality",
		"front_door":          "contact[contact] | battery,linkquality",
		"dual_probe":          "temperature/l1[temperature] temperature/l2[temperature] | linkquality",
		"mystery":             " | ",
	} {
		if got := layout(byName(s, name)); got != want {
			t.Errorf("%s:\n got  %s\n want %s", name, got, want)
		}
	}
	if len(s.Devices) != 9 {
		t.Errorf("%d devices, want 9 (coordinator excluded)", len(s.Devices))
	}
	if f := byName(s, "veranda_motion").Functions[0]; f.Kind != "occupancy" {
		t.Errorf("occupancy Function kind = %q, want occupancy", f.Kind)
	}
	if f := byName(s, "dual_probe").Functions[1]; f.Kind != "temperature" {
		t.Errorf("temperature/l2 Function kind = %q, want temperature", f.Kind)
	}
	if c := byName(s, "bedroom_remote").Functions[0].Capabilities[0]; c.Key != "action" || c.Category != home.Primary {
		t.Errorf("action category = %q, want primary despite zigbee2mqtt's diagnostic flag", c.Category)
	}
	var counters []string
	for _, d := range s.Devices {
		for _, f := range d.Functions {
			for _, c := range f.Capabilities {
				if c.Counter {
					counters = append(counters, d.Name+" "+f.Key+"."+c.Key)
				}
			}
		}
		for _, c := range d.Capabilities {
			if c.Counter {
				counters = append(counters, d.Name+" ."+c.Key)
			}
		}
	}
	if want := []string{"desk_plug switch.energy", "desk_plug switch.produced_energy"}; !reflect.DeepEqual(counters, want) {
		t.Errorf("counters = %v, want %v", counters, want)
	}
}

func TestStateAvailabilityAndEvents(t *testing.T) {
	b, h := newTestBridge(t)
	_, updates, cancel := h.Subscribe()
	defer cancel()
	b.handle("zigbee2mqtt/kitchen_bulb/availability", []byte(`{"state":"online"}`), true)
	b.handle("zigbee2mqtt/kitchen_bulb", []byte(`{"state":"ON","brightness":128,"color":{"x":0.3,"y":0.4},"color_mode":"xy","linkquality":100,"update":{"state":"idle"}}`), false)
	b.handle("zigbee2mqtt/hallway_switch", []byte(`{"state_left":"OFF","action":"single_left"}`), false)
	b.handle("zigbee2mqtt/hallway_switch", []byte(`{"action":"single_right"}`), true) // replayed by the broker: not a new press
	b.handle("zigbee2mqtt/kitchen_bulb/set", []byte(`{"state":"OFF"}`), false)        // another client's command: ignored

	s := snapshot(h)
	bulb, sw := byName(s, "kitchen_bulb"), byName(s, "hallway_switch")
	values := map[string]any{}
	for _, rv := range s.Values {
		values[rv.Ref.Target.Function()+"."+rv.Ref.Capability] = rv.Value.Data
	}
	want := map[string]any{
		"light.state":       true,
		"light.brightness":  128.0,
		"light.color_xy":    map[string]any{"x": 0.3, "y": 0.4},
		"light.color_mode":  "xy",
		".linkquality":      100.0,
		"switch/left.state": false,
	}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values = %v\nwant %v", values, want)
	}
	if s.Availability[home.TargetDevice(bulb.ID, "")] != home.Online {
		t.Errorf("bulb availability = %v", s.Availability[home.TargetDevice(bulb.ID, "")])
	}
	var events []string
	for {
		select {
		case u := <-updates:
			if u.Kind == home.EventOccurred && u.Ref.Target.Device() == sw.ID {
				events = append(events, u.Ref.Target.Function()+"."+u.Value.Data.(string))
			}
			continue
		default:
		}
		break
	}
	if !slices.Equal(events, []string{"button.single_left"}) {
		t.Errorf("events = %v", events)
	}
}

func TestValueAgeComesFromLastSeen(t *testing.T) {
	b, h := newTestBridge(t)
	b.handle("zigbee2mqtt/living_room_climate", []byte(`{"temperature":21.4,"last_seen":"2026-09-25T12:44:02.365Z"}`), true)
	s := snapshot(h)
	if len(s.Values) != 1 || !s.Values[0].Value.At.Equal(time.Date(2026, 9, 25, 12, 44, 2, 365e6, time.UTC)) {
		t.Fatalf("values = %+v, want temperature reported at last_seen", s.Values)
	}
}

func TestSetPayload(t *testing.T) {
	b, _ := newTestBridge(t)
	for _, tc := range []struct {
		address, function string
		values            map[string]any
		want              string
	}{
		{"0x00158d0000000004", "switch/right", map[string]any{"state": true}, `{"state_right":"ON"}`},
		{"0x0017880100000001", "light", map[string]any{"state": false, "color_xy": map[string]any{"x": 0.1, "y": 0.2}}, `{"color":{"x":0.1,"y":0.2},"state":"OFF"}`},
		{"0xa4c1380000000002", "switch", map[string]any{"child_lock": true}, `{"child_lock":"LOCK"}`},
	} {
		payload, err := b.byAddress[tc.address].setPayload(tc.function, tc.values)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := json.Marshal(payload); string(got) != tc.want {
			t.Errorf("%s %v: got %s, want %s", tc.function, tc.values, got, tc.want)
		}
	}
}

// BenchmarkStateMessage measures Oiko's share of the hot path: a device
// state message from MQTT until its Updates are handed to an observer.
func BenchmarkStateMessage(bm *testing.B) {
	b, h := newTestBridge(bm)
	_, updates, cancel := h.Subscribe()
	defer cancel()
	go func() {
		for range updates {
		}
	}()
	payloads := make([][]byte, 256)
	for i := range payloads {
		payloads[i] = []byte(`{"state":"ON","brightness":` + strconv.Itoa(i) + `,"color_temp":300,"color":{"x":0.3,"y":0.4},"linkquality":100}`)
	}
	i := 0
	for bm.Loop() {
		b.handle("zigbee2mqtt/kitchen_bulb", payloads[i%len(payloads)], false)
		i++
	}
}

// issuing is a Home that signals each Command the engine issues.
type issuing struct {
	*home.Home
	issued chan struct{}
}

func (h issuing) Command(t home.Target, req home.Request) (string, error) {
	id, err := h.Home.Command(t, req)
	h.issued <- struct{}{}
	return id, err
}

func TestReplayedOnceOnlineAndItsRetainedStateReplayed(t *testing.T) {
	for _, online := range []bool{true, false} {
		b := newBridge("zigbee2mqtt", nil, slog.Default())
		h := home.New(nil)
		b.home = h.Attach("zigbee2mqtt", b)
		replayed := func(*Bridge) bool {
			select {
			case <-h.Known():
				return true
			default:
				return false
			}
		}
		b.handle("zigbee2mqtt/bridge/state", []byte(`{"state":"offline"}`), true)
		if online {
			b.handle("zigbee2mqtt/bridge/state", []byte(`{"state":"online"}`), true)
		}
		if replayed(b) {
			t.Fatalf("online %v: replayed before the marker", online)
		}
		b.handle(b.marker, []byte("replayed"), false) // after every retained message
		if replayed(b) != online {
			t.Errorf("online %v: replayed %v after the marker", online, !online)
		}
		b.handle("zigbee2mqtt/bridge/state", []byte(`{"state":"online"}`), false)
		b.handle(b.marker, []byte("replayed"), false) // on a reconnection
		if !replayed(b) {
			t.Errorf("online %v: not replayed once online", online)
		}
	}
}

// BenchmarkReportToCommand measures a button press from MQTT until the
// Command of the one Automation it triggers is issued, the engine's
// goroutine hop included; the Bridge's send throttle is not Oiko's cost.
// Compare with BenchmarkStateMessage: the budget is 2×.
func BenchmarkReportToCommand(bm *testing.B) {
	b, h := newTestBridge(bm)
	_, updates, cancel := h.Subscribe()
	defer cancel()
	go func() {
		for range updates {
		}
	}()
	s := snapshot(h)
	doc := automation.Document{Name: "press", Enabled: true,
		Steps: []automation.Step{
			{ID: "press", Kind: "eventTrigger", Params: json.RawMessage(`{"target": "device:` + string(byName(s, "bedroom_remote").ID) + `/button", "capability": "action", "events": ["single"]}`)},
			{ID: "on", Kind: "command", Params: json.RawMessage(`{"targets": ["device:` + string(byName(s, "kitchen_bulb").ID) + `/light"], "values": {"state": true}}`)},
		},
		Edges: []automation.Edge{{From: automation.Port{Step: "press", Handle: "out"}, To: automation.Port{Step: "on", Handle: "in"}}},
	}
	issued := make(chan struct{})
	e := automation.New(issuing{h, issued}, nil, nil, nil)
	id, err := e.Create(doc)
	if err != nil {
		bm.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go e.Run(ctx)
	payload := []byte(`{"action":"single","battery":100,"linkquality":100}`)
	i := 0
	for bm.Loop() {
		if i++; i%20 == 0 { // re-enabled before the runaway guard trips
			bm.StopTimer()
			e.Replace(id, doc)
			bm.StartTimer()
		}
		b.handle("zigbee2mqtt/bedroom_remote", payload, false)
		<-issued
	}
}
