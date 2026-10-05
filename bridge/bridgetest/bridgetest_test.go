package bridgetest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/bridgetest"
)

// lamp is a Bridge whose one lamp confirms what it is sent, unless broken.
type lamp struct {
	port   bridge.Port
	broken bool
}

func (l *lamp) Run(context.Context, bridge.Port) {}

func (l *lamp) Send(ctx context.Context, address, function string, values map[string]any, _ time.Duration) error {
	if l.broken {
		return errors.New("broken")
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("no deadline")
	}
	var rs []bridge.Reading
	for k, v := range values {
		rs = append(rs, bridge.Reading{Function: function, Capability: k, Data: v})
	}
	go l.port.Report(address, rs, time.Now()) // as a device would, later
	return nil
}

var device = bridge.Device{NativeAddress: "0xlamp", Name: "Lamp", Functions: []bridge.Function{{
	Key: "light", Kind: "light", Capabilities: []bridge.Capability{
		{Key: "state", Type: bridge.Binary, Access: bridge.Access{Observable: true, Settable: true}},
		{Key: "effect", Type: bridge.Enum, Options: []string{"blink"}, Access: bridge.Access{Observable: true, Settable: true}},
		{Key: "pressed", Type: bridge.Text, Stateless: true, Access: bridge.Access{Observable: true}},
	},
}}}

func TestHomeAppliesOikosRules(t *testing.T) {
	l := &lamp{}
	h := bridgetest.New(l)
	l.port = h.Port()
	p := h.Port()

	p.Report("0xlamp", []bridge.Reading{{Function: "light", Capability: "state", Data: true}}, time.Now())
	p.SyncDevices([]bridge.Device{device})
	if _, ok := h.Value("0xlamp", "light", "state"); ok {
		t.Error("a report before the Device was listed was kept")
	}
	p.SetAvailability("0xlamp", bridge.Online)
	if h.Online() || h.Availability("0xlamp") != bridge.Unknown {
		t.Errorf("offline Bridge: online %v, availability %s, want false, unknown", h.Online(), h.Availability("0xlamp"))
	}
	if err := h.Command("0xlamp", "light", map[string]any{"state": true}); err == nil {
		t.Error("Command accepted while offline")
	}

	p.SetOnline(true)
	p.Report("0xlamp", []bridge.Reading{{Function: "light", Capability: "state", Data: false}, {Function: "light", Capability: "pressed", Data: "single"}}, time.Now())
	if v, _ := h.Value("0xlamp", "light", "state"); v != false || h.Availability("0xlamp") != bridge.Online {
		t.Errorf("state %v, availability %s", v, h.Availability("0xlamp"))
	}
	if e, _ := h.Event("0xlamp", "light", "pressed"); e != "single" {
		t.Errorf("event %v", e)
	}
	if d := h.Devices(); len(d) != 1 || d[0].Name != "Lamp" || len(d[0].Functions[0].Capabilities) != 3 {
		t.Errorf("devices %+v", d)
	}
	if h.Replayed() {
		t.Error("replayed before Replayed")
	}
	p.Replayed()
	if !h.Replayed() {
		t.Error("not replayed")
	}

	if err := h.Command("0xlamp", "light", map[string]any{"effect": "custom"}); err == nil {
		t.Error("Command outside the Options accepted")
	}
	if err := h.Command("0xlamp", "light", map[string]any{"state": true}); err != nil {
		t.Errorf("Command: %v", err)
	}
	if v, _ := h.Value("0xlamp", "light", "state"); v != true {
		t.Errorf("state %v after the Command", v)
	}
	l.broken = true
	if err := h.Command("0xlamp", "light", map[string]any{"state": false}); err == nil {
		t.Error("Command confirmed though Send failed")
	}

	p.SyncDevices(nil)
	if len(h.Devices()) != 0 || h.Availability("0xlamp") != bridge.Unknown {
		t.Errorf("unlisted Device still there: %+v", h.Devices())
	}
}
