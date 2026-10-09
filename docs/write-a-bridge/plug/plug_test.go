package plug

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/bridgetest"
)

func TestPlug(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		desk := &fakePlug{status: status{On: true, Power: 12.5}}
		b, err := open(bridgetest.Env(t, "plug",
			`{"plugs": {"Desk lamp": "http://desk.test", "Kettle": "http://kettle.test"}}`))
		if err != nil {
			t.Fatal(err)
		}
		b.(*Bridge).client.Transport = network{"desk.test": desk} // the kettle is unplugged
		h := bridgetest.New(b)
		go b.Run(t.Context(), h.Port())
		synctest.Wait()

		if !h.Online() || !h.Replayed() {
			t.Fatalf("online %v, replayed %v: want both", h.Online(), h.Replayed())
		}
		if v, _ := h.Value("Desk lamp", "switch", "power"); v.Data != 12.5 {
			t.Errorf("power = %v, want 12.5", v.Data)
		}
		if a := h.Availability("Kettle"); a != bridge.Offline {
			t.Errorf("unplugged kettle: %s, want offline", a)
		}

		if err := h.Command("Desk lamp", "switch", map[string]any{"state": false}); err != nil {
			t.Fatalf("command: %v", err) // ErrRefused, ErrFailed or ErrTimedOut
		}
		if desk.get().On {
			t.Error("plug still on")
		}
		if err := h.Command("Kettle", "switch", map[string]any{"state": true}); !errors.Is(err, bridgetest.ErrFailed) {
			t.Errorf("command to the unplugged kettle: %v, want ErrFailed", err)
		}

		// Someone switches the plug on by hand: the next poll, 10 s on, reports it.
		desk.set(status{On: true, Power: 40})
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if v, _ := h.Value("Desk lamp", "switch", "state"); v.Data != true {
			t.Errorf("state after a poll = %v, want true", v.Data)
		}
	})
}

// TestOpen creates the Bridge as Oiko does, from its section of the
// configuration, which it refuses unless it can work with it.
func TestOpen(t *testing.T) {
	for _, c := range []struct {
		name, config, err string // err: in the error; "" for none
	}{
		{"one plug", `{"plugs": {"Desk lamp": "http://192.0.2.10"}}`, ""},
		{"no plugs", `{}`, "no plugs"},
		{"no scheme", `{"plugs": {"Desk lamp": "192.0.2.10"}}`, "Desk lamp"},
		{"unknown key", `{"plug": {"Desk lamp": "http://192.0.2.10"}}`, `"plug"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := open(bridgetest.Env(t, "plug", c.config))
			if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
				t.Errorf("err = %v, want one about %q", err, c.err)
			}
		})
	}
}

// TestManifest checks the catalogue's manifest: the types registered, each
// with an example section the type accepts.
func TestManifest(t *testing.T) {
	data, err := os.ReadFile("oiko-bridge.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Types map[string]struct {
			Description string
			Config      json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	types := bridge.Types()
	if len(m.Types) != len(types) {
		t.Errorf("manifest types: %d, registered: %v", len(m.Types), types)
	}
	for _, r := range types {
		typ, ok := m.Types[r.Type]
		if !ok {
			t.Errorf("type %s missing from the manifest", r.Type)
			continue
		}
		module, _ := bridge.Lookup(r.Type)
		if _, err := module.New(bridgetest.Env(t, r.Type, string(typ.Config))); err != nil || typ.Description == "" {
			t.Errorf("type %s: example section: %v; description %q", r.Type, err, typ.Description)
		}
	}
}

// fakePlug is a plug's HTTP API, whose status a test changes as a hand on
// the plug would.
type fakePlug struct {
	mu     sync.Mutex
	status status
}

func (f *fakePlug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path != "/status":
		http.NotFound(w, r)
		return
	case r.Method == http.MethodPost:
		var req struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.status.On = req.On
		if !req.On {
			f.status.Power = 0
		}
	}
	json.NewEncoder(w).Encode(f.status)
}

func (f *fakePlug) get() status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakePlug) set(s status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = s
}

// network is an in-memory LAN, an http.RoundTripper: each host's handler
// answers its requests, with no socket, so synctest.Wait sees the Bridge idle
// and the poll's ticker fast-forwards. Any other host is unreachable.
type network map[string]http.Handler

func (n network) RoundTrip(r *http.Request) (*http.Response, error) {
	h, ok := n[r.URL.Host]
	if !ok {
		return nil, errors.New("connection refused")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Result(), nil
}
