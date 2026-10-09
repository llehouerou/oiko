// Package plug is the type of Bridge "plug", for smart plugs on the local
// network, each answering a two-route HTTP API:
//
//	GET  /status           → {"on": true, "power": 12.5}
//	POST /status {"on": …} → the new status
//
// The plug is made up: it is the first type of Oiko's guide, "Write a type of
// Bridge".
package plug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

func init() { bridge.Register(bridge.Module{Type: "plug", New: open}) }

// config is the type's section of "bridges": each plug's name and address.
// A secret, such as a password, would be a …File key naming the file that
// holds it, read in open: never the secret itself.
type config struct {
	Plugs map[string]string `json:"plugs"` // name → base URL, "http://192.0.2.10"
}

func open(env bridge.Env) (bridge.Bridge, error) {
	var c config
	if err := env.Decode(&c); err != nil { // refuses unknown keys: Oiko won't start
		return nil, err
	}
	if len(c.Plugs) == 0 {
		return nil, errors.New("no plugs")
	}
	for name, addr := range c.Plugs {
		if u, err := url.Parse(addr); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("plug %s: %q is no http:// URL", name, addr)
		}
	}
	return &Bridge{plugs: c.Plugs, client: &http.Client{Timeout: 5 * time.Second}, log: env.Log}, nil
}

// Bridge follows the plugs, polling each every 10 s.
type Bridge struct {
	plugs  map[string]string // name → base URL
	client *http.Client
	log    *slog.Logger
	port   bridge.Port // Run's: Send never comes before Run has gone online
}

// switchCapabilities make a Tile with an on/off bar and a power reading.
var switchCapabilities = []bridge.Capability{
	{Key: "state", Label: "State", Type: bridge.Binary, Category: bridge.Primary,
		Access: bridge.Access{Observable: true, Settable: true}},
	{Key: "power", Label: "Power", Type: bridge.Numeric, Unit: "W", Category: bridge.Primary,
		Access: bridge.Access{Observable: true}},
}

// devices are the plugs, each found by its configured name: renaming a plug
// makes it a new Device. A real system's own identifier is a better address.
func (b *Bridge) devices() []bridge.Device {
	var ds []bridge.Device
	for _, name := range slices.Sorted(maps.Keys(b.plugs)) {
		ds = append(ds, bridge.Device{NativeAddress: name, Name: name, Model: "Plug",
			Functions: []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: switchCapabilities}}})
	}
	return ds
}

func (b *Bridge) Run(ctx context.Context, port bridge.Port) {
	b.port = port // before SetOnline: Oiko sends nothing to an offline Bridge
	port.SyncDevices(b.devices())
	port.SetOnline(true) // nothing to reach but the plugs: their Availability says which answer
	b.poll(ctx)
	port.Replayed()
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			b.poll(ctx)
		}
	}
}

func (b *Bridge) poll(ctx context.Context) {
	for name, addr := range b.plugs {
		s, err := b.status(ctx, http.MethodGet, addr, nil)
		if err != nil {
			b.log.Debug("poll", "plug", name, "err", err) // every 10 s: Debug, not Warn
			b.port.SetAvailability(name, bridge.Offline)
			continue
		}
		b.port.SetAvailability(name, bridge.Online)
		b.report(name, s)
	}
}

func (b *Bridge) report(name string, s status) {
	b.port.Report(name, []bridge.Reading{
		{Function: "switch", Capability: "state", Data: s.On},
		{Function: "switch", Capability: "power", Data: s.Power},
	}, time.Now()) // the plug gives no time of its own
}

func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, _ time.Duration) error {
	on, ok := values["state"].(bool) // Oiko sends only what the Capabilities accept
	if !ok {
		return errors.New("no state")
	}
	s, err := b.status(ctx, http.MethodPost, b.plugs[address], map[string]bool{"on": on})
	if err != nil {
		return err // fails the Command
	}
	b.report(address, s) // confirms it
	return nil
}

// status is a plug's, as its API gives it.
type status struct {
	On    bool    `json:"on"`
	Power float64 `json:"power"` // W
}

// status sends the plug at addr a request for its status: GET reads it, POST
// sets body and returns the new one.
func (b *Bridge) status(ctx context.Context, method, addr string, body any) (status, error) {
	var data []byte
	if body != nil {
		var err error
		if data, err = json.Marshal(body); err != nil {
			return status{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, addr+"/status", bytes.NewReader(data))
	if err != nil {
		return status{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return status{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return status{}, fmt.Errorf("%s %s/status: %s", method, addr, resp.Status)
	}
	var s status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return status{}, fmt.Errorf("%s %s/status: %w", method, addr, err)
	}
	return s, nil
}
