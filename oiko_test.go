package oiko

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

// fake is a type of Bridge doing nothing, with a ping command.
type fake struct{}

func (fake) Run(context.Context, bridge.Port)                                          {}
func (fake) Send(context.Context, string, string, map[string]any, time.Duration) error { return nil }

var pinged []string // by ping: the Bridge's name and its argument

func init() {
	bridge.Register(bridge.Module{Type: "fake",
		New: func(bridge.Env) (bridge.Bridge, error) { return fake{}, nil },
		Commands: map[string]func(bridge.Env, []string) error{"ping": func(env bridge.Env, args []string) error {
			pinged = append(pinged, env.Name+" "+args[0])
			return nil
		}}})
}

func TestConfigureResolvesTypesAndRunsCommands(t *testing.T) {
	pinged = nil
	var logged strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	dir := t.TempDir()
	bridges, err := configure(map[string]json.RawMessage{
		"fake":   json.RawMessage(`{}`),                          // its type is its name
		"garage": json.RawMessage(`{"type": "fake", "door": 2}`), // a second one
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range bridges {
		names = append(names, b.env.Name)
		if b.module.Type != "fake" {
			t.Errorf("%s: type %q", b.env.Name, b.module.Type)
		}
		if _, err := os.Stat(filepath.Join(dir, b.env.Name)); err != nil {
			t.Errorf("%s: no data directory: %v", b.env.Name, err)
		}
	}
	if !slices.Equal(names, []string{"fake", "garage"}) {
		t.Errorf("bridges %v", names)
	}
	if got := string(bridges[1].env.Config); got != `{"door":2}` {
		t.Errorf("garage's section %s, want it without its type", got)
	}
	bridges[1].env.Log.Info("hello")
	if !strings.Contains(logged.String(), "msg=hello bridge=garage") {
		t.Errorf("logged %q, want it scoped to garage", logged.String())
	}

	if err := command(bridges, []string{"garage", "ping", "x"}); err != nil || !slices.Equal(pinged, []string{"garage x"}) {
		t.Errorf("command: %v, pinged %v", err, pinged)
	}
	for _, args := range [][]string{{"nobody", "ping"}, {"fake"}, {"fake", "pong"}} {
		if err := command(bridges, args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}

	for name, section := range map[string]string{"hue": `{}`, "x": `{"type": "hue"}`, "Fake": `{"type": "fake"}`, "../up": `{"type": "fake"}`, "upgrade": `{"type": "fake"}`} {
		if _, err := configure(map[string]json.RawMessage{name: json.RawMessage(section)}, dir); err == nil {
			t.Errorf("%s %s: configured", name, section)
		}
	}
}

// Register records the package registering a type: this one for fake.
func TestTypesRecordTheirPackage(t *testing.T) {
	i := slices.IndexFunc(bridge.Types(), func(r bridge.Registered) bool { return r.Type == "fake" })
	if i < 0 || bridge.Types()[i].Package != "github.com/llehouerou/oiko" {
		t.Errorf("types %+v", bridge.Types())
	}
}

func TestLoadConfigRefusesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	for doc, ok := range map[string]bool{
		`{"location": {"latitude": 1, "longitude": 2}}`: true,
		`{"locaton": {}}`:               false,
		`{"Location": {"latitude": 1}}`: false, // another case is another key
		`{"location": {"Latitude": 1}}`: false,
	} {
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		var c config
		if err := loadConfig(path, &c); (err == nil) != ok {
			t.Errorf("%s: %v", doc, err)
		}
	}
}
