package oiko

import (
	"context"
	"encoding/json"
	"fmt"
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

	for name, section := range map[string]string{"hue": `{}`, "x": `{"type": "hue"}`, "Fake": `{"type": "fake"}`, "../up": `{"type": "fake"}`, "upgrade": `{"type": "fake"}`, "sign-in-link": `{"type": "fake"}`} {
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

func TestPublicURLIsAnHTTPSOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.org":          "https://example.org",
		"https://example.org/":         "https://example.org",
		"https://example.org:8443":     "https://example.org:8443",
		"https://oiko.example.ts.net/": "https://oiko.example.ts.net",
		"https://Oiko.Example.org":     "https://oiko.example.org", // as a browser sends it
		"https://example.org:443/":     "https://example.org",
	} {
		u, err := parsePublicURL(in)
		if err != nil || u.String() != want {
			t.Errorf("%s: %v, %v, want %s", in, u, err, want)
		}
	}
	for in, reason := range map[string]string{
		"http://example.org":        "https",
		"example.org":               "https",
		"https://":                  "host",
		"https://:8443":             "host",
		"https://example.org:port":  "port",
		"https://example.org/oiko":  "path",
		"https://example.org/oiko/": "path",
		"https://example.org/?a=1":  "query",
		"https://example.org?":      "query",
		"https://example.org/#x":    "fragment",
		"https://example.org#":      "fragment",
		"https://alice@example.org": "userinfo",
		"https://a:b@example.org/":  "userinfo",
		"https://example.org:":      "port",
		"https://example.org:0":     "port",
		"https://example.org:65536": "port",
	} {
		if u, err := parsePublicURL(in); err == nil || !strings.Contains(err.Error(), reason) {
			t.Errorf("%s: %v, %v, want an error about its %s", in, u, err, reason)
		}
	}
}

// load checks publicUrl with the rest of the configuration, and starts
// without it.
func TestLoadChecksThePublicURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	write := func(doc string) {
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for doc, want := range map[string]string{
		`{}`:                                    "<nil>",
		`{"publicUrl": "https://example.org/"}`: "https://example.org",
	} {
		write(doc)
		if _, u, _, err := load(dir, path); err != nil || fmt.Sprint(u) != want {
			t.Errorf("%s: %v, %v, want %s", doc, u, err, want)
		}
	}
	for _, doc := range []string{`{"publicUrl": "http://example.org"}`, `{"publicUrl": ""}`} {
		write(doc)
		if _, _, _, err := load(dir, path); err == nil || !strings.Contains(err.Error(), "publicUrl") {
			t.Errorf("%s: %v, want an error naming publicUrl", doc, err)
		}
	}
}

// The data directory holds secrets: a missing one is created 0700,
// an existing one left as it is (it may be shared).
func TestLoadCreatesAPrivateDataDirectory(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), "data")
	existing := t.TempDir()
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]os.FileMode{fresh: 0o700, existing: 0o755} {
		if _, _, _, err := load(dir, filepath.Join(dir, "config.json")); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, %v, want %v", dir, fi.Mode().Perm(), err, want)
		}
	}
}
