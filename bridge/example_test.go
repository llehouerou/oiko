package bridge_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/bridgetest"
)

// A type of Bridge for one simulated lamp, which obeys at once. A real one
// follows its external system in Run, and transmits to it in Send.
func init() {
	bridge.Register(bridge.Module{Type: "lamp", New: newLamp})
}

type lamp struct {
	name string
	port bridge.Port // Run's: Send never comes before Run has gone online
}

func newLamp(env bridge.Env) (bridge.Bridge, error) {
	var c struct {
		Name string `json:"name"`
	}
	if err := env.Decode(&c); err != nil {
		return nil, err
	}
	if c.Name == "" {
		return nil, errors.New(`no "name"`)
	}
	return &lamp{name: c.Name}, nil
}

func (l *lamp) Run(ctx context.Context, port bridge.Port) {
	l.port = port
	port.SyncDevices([]bridge.Device{{NativeAddress: "lamp1", Name: l.name, Functions: []bridge.Function{
		{Key: "light", Kind: "light", Capabilities: []bridge.Capability{
			{Key: "state", Label: "State", Type: bridge.Binary, Category: bridge.Primary, Access: bridge.Access{Observable: true, Settable: true}},
		}},
	}}})
	port.SetOnline(true)
	port.Report("lamp1", []bridge.Reading{{Function: "light", Capability: "state", Data: false}}, time.Now())
	port.Replayed()
	<-ctx.Done()
}

func (l *lamp) Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error {
	// The lamp reports its new state, which confirms the Command.
	l.port.Report(address, []bridge.Reading{{Function: function, Capability: "state", Data: values["state"]}}, time.Now())
	return nil
}

func Example() {
	m, _ := bridge.Lookup("lamp")
	b, err := m.New(bridge.Env{Name: "desk", Config: json.RawMessage(`{"name": "Desk lamp"}`)})
	if err != nil {
		panic(err)
	}
	h := bridgetest.New(b)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go b.Run(ctx, h.Port())
	for !h.Replayed() { // a test waits with testing/synctest
		time.Sleep(time.Millisecond)
	}

	err = h.Command("lamp1", "light", map[string]any{"state": true})
	v, _ := h.Value("lamp1", "light", "state")
	fmt.Println(h.Devices()[0].Name, err, v.Data)
	// Output: Desk lamp <nil> true
}
