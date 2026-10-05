// Package netatmo follows Netatmo weather stations through Netatmo's cloud API
// (ADR 0012): each module is a Device, each of its measures a Function.
package netatmo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/store"
)

func init() { bridge.Register(bridge.Module{Type: "netatmo", New: open}) }

// Config is a netatmo Bridge's section of Oiko's configuration: the app
// created on dev.netatmo.com. Its secret is read from a file, so that it
// stays out of it.
type Config struct {
	ClientID         string `json:"clientId"`
	ClientSecretFile string `json:"clientSecretFile"`
}

// pollEvery is half the stations' upload interval to Netatmo, 10 minutes:
// 12 requests an hour, far under the API's limit.
const pollEvery = 5 * time.Minute

// backoff is the wait before trying a failed poll again, after last (0 after a
// success): 15 s, then twice as long each time, up to pollEvery. Netatmo often
// answers 503 for a moment, and a restart should not leave the stations
// unknown for a whole poll.
func backoff(last time.Duration) time.Duration {
	if last == 0 {
		return 15 * time.Second
	}
	return min(2*last, pollEvery)
}

const api = "https://api.netatmo.com"

// Bridge is Oiko's Netatmo account.
type Bridge struct {
	api    string // overridden by tests
	client *http.Client
	log    *slog.Logger
	// Only Run's goroutine touches these.
	port      bridge.Port
	described []bridge.Device
	measured  map[string]int64 // time_utc last reported, by module
}

// tokenFormat is the format of token.json (ADR 0019).
var tokenFormat store.Format

// open reads the secret and the token kept in the data directory, which Oiko
// rewrites on every refresh: Netatmo invalidates the previous refresh token.
func open(env bridge.Env) (bridge.Bridge, error) {
	var c Config
	if err := env.Decode(&c); err != nil {
		return nil, fmt.Errorf("netatmo: %w", err)
	}
	if c.ClientID == "" {
		return nil, errors.New("netatmo: clientId is required")
	}
	secret, err := os.ReadFile(c.ClientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("netatmo: %w", err)
	}
	tokenPath := filepath.Join(env.DataDir, "token.json")
	var tok oauth2.Token
	if err := store.Load(tokenPath, tokenFormat, &tok); err != nil {
		return nil, fmt.Errorf("netatmo: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf(`netatmo: no token: write {"refresh_token": "…"} from the app's token generator (scope read_station) to %s`, tokenPath)
	}
	return newBridge(api, c.ClientID, strings.TrimSpace(string(secret)), &tok, tokenPath, env.Log), nil
}

func newBridge(base, id, secret string, tok *oauth2.Token, tokenPath string, log *slog.Logger) *Bridge {
	conf := &oauth2.Config{ClientID: id, ClientSecret: secret,
		Endpoint: oauth2.Endpoint{TokenURL: base + "/oauth2/token", AuthStyle: oauth2.AuthStyleInParams}}
	src := &saver{conf.TokenSource(context.Background(), tok), tokenPath, tok.RefreshToken}
	client := oauth2.NewClient(context.Background(), src)
	client.Timeout = 30 * time.Second
	return &Bridge{api: base, client: client, log: log, measured: map[string]int64{}}
}

// saver writes each new token to path, before it is used.
type saver struct {
	src     oauth2.TokenSource
	path    string
	refresh string // the refresh token on disk
}

func (s *saver) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil || t.RefreshToken == s.refresh {
		return t, err
	}
	if err := store.Save(s.path, tokenFormat, t); err != nil {
		return nil, fmt.Errorf("saving token: %w", err)
	}
	s.refresh = t.RefreshToken
	return t, nil
}

// Run polls the stations and feeds Oiko through p until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context, p bridge.Port) {
	b.port = p
	var retry time.Duration // the last wait after a failure; 0 after a success
	for {
		stations, err := b.fetch(ctx)
		wait := pollEvery
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			retry = backoff(retry)
			wait = retry
			b.log.Warn("poll failed", "err", err, "retry", wait)
			p.SetOnline(false)
		default:
			retry = 0
			b.update(stations)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Send refuses: no Netatmo Capability is settable, so Oiko lets none through.
func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error {
	return errors.New("netatmo: read-only")
}

// module is a station's main module or one of its modules, as getstationsdata
// describes it. dashboard_data is missing while it is unreachable.
type module struct {
	ID          string         `json:"_id"`
	Type        string         `json:"type"`
	Name        string         `json:"module_name"`
	StationName string         `json:"station_name"`
	DataType    []string       `json:"data_type"`
	Reachable   bool           `json:"reachable"`
	Battery     *float64       `json:"battery_percent"`
	Dashboard   map[string]any `json:"dashboard_data"`
	Modules     []module       `json:"modules"`
}

func (b *Bridge) fetch(ctx context.Context) ([]module, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.api+"/api/getstationsdata", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("getstationsdata: %s: %s", resp.Status, e.Error.Message)
	}
	var r struct {
		Body struct {
			Devices []module `json:"devices"`
		} `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("getstationsdata: %w", err)
	}
	return r.Body.Devices, nil
}

// update reports what one getstationsdata response says; the first one is
// the Replay.
func (b *Bridge) update(stations []module) {
	var modules []module
	for _, s := range stations {
		modules = append(modules, s)
		modules = append(modules, s.Modules...)
	}
	described := make([]bridge.Device, len(modules))
	for i, m := range modules {
		described[i] = describe(m)
	}
	if !reflect.DeepEqual(described, b.described) { // SyncDevices rewrites devices.json
		b.port.SyncDevices(described)
		b.described = described
	}
	b.port.SetOnline(true)
	for _, m := range modules {
		b.port.SetAvailability(m.ID, map[bool]bridge.Availability{true: bridge.Online, false: bridge.Offline}[m.Reachable])
		at, _ := m.Dashboard["time_utc"].(float64)
		if int64(at) <= b.measured[m.ID] {
			continue // not measured since the last poll
		}
		b.measured[m.ID] = int64(at)
		var readings []bridge.Reading
		for _, t := range m.DataType {
			for _, ms := range measures[t] {
				if v, ok := m.Dashboard[ms.field].(float64); ok {
					readings = append(readings, bridge.Reading{Function: strings.ToLower(t), Capability: ms.cap.Key, Data: v})
				}
			}
		}
		if m.Battery != nil {
			readings = append(readings, bridge.Reading{Capability: "battery", Data: *m.Battery})
		}
		b.port.Report(m.ID, readings, time.Unix(int64(at), 0))
	}
	b.port.Replayed()
}

type measure struct {
	field string // in dashboard_data
	cap   bridge.Capability
}

func numeric(key, label, unit string) bridge.Capability {
	return bridge.Capability{Key: key, Label: label, Type: bridge.Numeric, Unit: unit, Access: observable, Category: bridge.Primary}
}

var (
	observable = bridge.Access{Observable: true}
	percent    = [2]float64{0, 100}
)

// measures maps a module's data_type to the Capabilities of its Function,
// whose key is the data_type in lower case.
var measures = map[string][]measure{
	"Temperature": {{"Temperature", numeric("temperature", "Temperature", "°C")}},
	"Humidity":    {{"Humidity", numeric("humidity", "Humidity", "%")}},
	"CO2":         {{"CO2", numeric("co2", "CO₂", "ppm")}},
	"Noise":       {{"Noise", numeric("noise", "Noise", "dB")}},
	"Pressure":    {{"Pressure", numeric("pressure", "Pressure", "hPa")}},
	"Rain": {
		{"Rain", numeric("rain", "Rain", "mm")},
		{"sum_rain_1", numeric("rain_1h", "Rain (1 h)", "mm")},
		{"sum_rain_24", numeric("rain_24h", "Rain (today)", "mm")},
	},
	// ponytail: no Wind: no wind gauge in this home; add its four measures with one.
}

// describe is the Oiko view of a module. Only the main module has no battery.
func describe(m module) bridge.Device {
	name := m.Name
	if name == "" {
		name = m.StationName
	}
	d := bridge.Device{NativeAddress: m.ID, Name: name, Model: m.Type, Vendor: "Netatmo", Capabilities: []bridge.Capability{}}
	for _, t := range m.DataType {
		ms, ok := measures[t]
		if !ok {
			continue
		}
		f := bridge.Function{Key: strings.ToLower(t), Kind: strings.ToLower(t)}
		for _, m := range ms {
			f.Capabilities = append(f.Capabilities, m.cap)
		}
		d.Functions = append(d.Functions, f)
	}
	if m.Type != "NAMain" {
		d.Capabilities = append(d.Capabilities, bridge.Capability{
			Key: "battery", Label: "Battery", Type: bridge.Numeric, Unit: "%",
			Min: &percent[0], Max: &percent[1], Access: observable, Category: bridge.Diagnostic,
		})
	}
	return d
}
