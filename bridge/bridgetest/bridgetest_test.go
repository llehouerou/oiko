package bridgetest_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/bridgetest"
)

// lamp is a Bridge whose one lamp confirms what it is sent, unless broken
// (Send fails) or silent (nothing ever confirms).
type lamp struct {
	port           bridge.Port
	broken, silent bool
}

func (l *lamp) Run(context.Context, bridge.Port) {}

func (l *lamp) Send(ctx context.Context, address, function string, values map[string]any, _ time.Duration) error {
	if l.broken {
		return errors.New("broken")
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("no deadline")
	}
	if l.silent {
		return nil
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
	sent := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) // when the device sent it, not now
	p.Report("0xlamp", []bridge.Reading{{Function: "light", Capability: "state", Data: false}, {Function: "light", Capability: "pressed", Data: "single"}}, sent)
	if v, _ := h.Value("0xlamp", "light", "state"); v.Data != false || !v.At.Equal(sent) || h.Availability("0xlamp") != bridge.Online {
		t.Errorf("state %+v, availability %s", v, h.Availability("0xlamp"))
	}
	if e, _ := h.Event("0xlamp", "light", "pressed"); e.Data != "single" || !e.At.Equal(sent) {
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
	if v, _ := h.Value("0xlamp", "light", "state"); v.Data != true {
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

func TestDevicesInTheBridgesOrder(t *testing.T) {
	h := bridgetest.New(nil)
	h.Port().SyncDevices([]bridge.Device{{NativeAddress: "0xa"}})
	h.Port().SyncDevices([]bridge.Device{{NativeAddress: "0xb"}, {NativeAddress: "0xa"}}) // 0xa is older in the home
	if d := h.Devices(); len(d) != 2 || d[0].NativeAddress != "0xb" || d[1].NativeAddress != "0xa" {
		t.Errorf("devices %+v, want 0xb then 0xa", d)
	}
}

func TestCommandErrorsTellWhatHappened(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &lamp{}
		h := bridgetest.New(l)
		l.port = h.Port()
		h.Port().SyncDevices([]bridge.Device{device})
		if err := h.Command("0xlamp", "light", map[string]any{"state": true}); !errors.Is(err, bridgetest.ErrRefused) {
			t.Errorf("offline: %v, want ErrRefused", err)
		}
		h.Port().SetOnline(true)
		if err := h.Command("0xlamp", "light", map[string]any{"effect": "custom"}); !errors.Is(err, bridgetest.ErrRefused) {
			t.Errorf("outside the Options: %v, want ErrRefused", err)
		}
		l.broken = true
		if err := h.Command("0xlamp", "light", map[string]any{"state": true}); !errors.Is(err, bridgetest.ErrFailed) {
			t.Errorf("Send failing: %v, want ErrFailed", err)
		}
		l.broken, l.silent = false, true
		if err := h.Command("0xlamp", "light", map[string]any{"state": true}); !errors.Is(err, bridgetest.ErrTimedOut) {
			t.Errorf("never confirmed: %v, want ErrTimedOut", err)
		}
	})
}

func TestEnvAsOikoHandsIt(t *testing.T) {
	env := bridgetest.Env(t, "garage", `{"door": 2}`)
	var c struct {
		Door int `json:"door"`
	}
	if err := env.Decode(&c); err != nil || c.Door != 2 || env.Name != "garage" {
		t.Errorf("decode: %v, %+v, name %q", err, c, env.Name)
	}
	if fi, err := os.Stat(env.DataDir); err != nil || !fi.IsDir() {
		t.Errorf("data directory: %v", err)
	}
	env.Log.Info("hello") // into the test's output
}

// camera is a Bridge with one camera, which has a Picture.
type camera struct{ lamp }

func (camera) Picture(_ context.Context, address, function string) (bridge.Picture, error) {
	return bridge.Picture{Data: []byte(address + "/" + function), ContentType: "image/jpeg"}, nil
}

func (camera) Stream(_ context.Context, address, function string) (string, error) {
	return "rtsp://camera.example/" + address + "/" + function, nil
}

func TestAPictureIsReadThroughTheBridgesCameras(t *testing.T) {
	cam := bridge.Device{NativeAddress: "cam1", Name: "Garden", Functions: []bridge.Function{{Key: "camera", Kind: "camera"}}}
	h := bridgetest.New(&camera{})
	h.Port().SyncDevices([]bridge.Device{cam})
	if _, err := h.Picture("cam1", "camera"); !errors.Is(err, bridgetest.ErrRefused) {
		t.Errorf("offline: %v, want refused", err)
	}
	h.Port().SetOnline(true)
	if pic, err := h.Picture("cam1", "camera"); err != nil || string(pic.Data) != "cam1/camera" {
		t.Errorf("Picture: %q, %v", pic.Data, err)
	}
	if _, err := h.Picture("cam2", "camera"); !errors.Is(err, bridgetest.ErrRefused) {
		t.Errorf("an unlisted Device: %v, want refused", err)
	}

	plain := bridgetest.New(&lamp{})
	plain.Port().SetOnline(true)
	plain.Port().SyncDevices([]bridge.Device{cam})
	if _, err := plain.Picture("cam1", "camera"); !errors.Is(err, bridgetest.ErrRefused) {
		t.Errorf("a Bridge without cameras: %v, want refused", err)
	}
}
