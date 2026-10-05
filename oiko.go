// Package oiko runs the Oiko home automation server. A build of Oiko is a
// main package calling Main; it imports, for their side effect, the packages
// of the types of Bridge it adds to the built-in ones (see package bridge and
// ADR 0017).
package oiko

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/api"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
	"github.com/llehouerou/oiko/internal/release"
	"github.com/llehouerou/oiko/internal/telegram"
	"github.com/llehouerou/oiko/web"

	// The built-in types of Bridge.
	_ "github.com/llehouerou/oiko/internal/homekit"
	_ "github.com/llehouerou/oiko/internal/zigbee2mqtt"
)

// config is Oiko's hand-written configuration.
type config struct {
	Location *automation.Place          `json:"location"` // for the sun triggers
	Telegram *telegram.Config           `json:"telegram"` // no Notifications without it
	Bridges  map[string]json.RawMessage `json:"bridges"`  // each one's section, by name
}

// configured is a Bridge of the configuration, of a type compiled in.
type configured struct {
	env    bridge.Env
	module bridge.Module
}

// Main runs Oiko, or with arguments upgrades it or runs a command of one of
// its Bridges: oiko [flags] [upgrade [-y] | <bridge> <command> [args]].
func Main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dataDir := flag.String("data", "data", "directory holding Oiko's own configuration")
	configPath := flag.String("config", "", "hand-written configuration (default <data>/config.json)")
	showVersion := flag.Bool("version", false, "print what this Oiko is built from and exit")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: oiko [flags] [upgrade [-y] | <bridge> <command> [args]]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		printBuild(os.Stdout, build.Current())
		return
	}
	inst, err := install(os.Getenv("OIKO_INSTALL"))
	if err != nil {
		log.Fatal(err)
	}
	if flag.Arg(0) == "upgrade" {
		if err := upgrade(inst, flag.Args()[1:], os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("data directory: %v", err)
	}
	configFile := *configPath
	if configFile == "" {
		configFile = filepath.Join(*dataDir, "config.json") // written by hand
	}
	var c config
	if err := loadConfig(configFile, &c); err != nil {
		log.Fatalf("loading %s: %v", configFile, err)
	}
	if l := c.Location; l != nil && (math.Abs(l.Latitude) > 90 || math.Abs(l.Longitude) > 180) {
		log.Fatalf("%s: location out of range: %+v", configFile, *l)
	}
	bridges, err := configure(c.Bridges, *dataDir)
	if err != nil {
		log.Fatalf("%s: %v", configFile, err)
	}
	if flag.NArg() > 0 {
		if err := command(bridges, flag.Args()); err != nil {
			log.Fatal(err)
		}
		return
	}
	serve(*listen, *dataDir, configFile, inst, c, bridges)
}

// install is how this Oiko runs on its host, its Install (see CONTEXT.md), as
// OIKO_INSTALL tells: nixos, docker, or a plain binary when unset.
func install(env string) (string, error) {
	switch env {
	case "":
		return "binary", nil
	case "nixos", "docker":
		return env, nil
	}
	return "", fmt.Errorf("OIKO_INSTALL=%q: want nixos, docker, or unset for a plain binary", env)
}

// loadConfig decodes the configuration at path into c, refusing what it does
// not know: a section left over from an earlier layout. A missing file is an
// empty configuration.
func loadConfig(path string, c *config) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	return d.Decode(c)
}

// configure resolves each Bridge of the configuration, by name, to its type,
// "type" in its section or else its name, and gives it a data directory of
// its own, <data>/<name>.
func configure(sections map[string]json.RawMessage, dataDir string) ([]configured, error) {
	var bridges []configured
	for _, name := range slices.Sorted(maps.Keys(sections)) {
		if name == "" || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return nil, fmt.Errorf("bridge %q: a name is made of lowercase letters, digits and dashes", name)
		}
		if name == "upgrade" {
			return nil, errors.New("bridge upgrade: the name is oiko upgrade's")
		}
		var section map[string]json.RawMessage
		var t string
		if err := json.Unmarshal(sections[name], &section); err != nil {
			return nil, fmt.Errorf("bridge %s: %w", name, err)
		}
		if raw, ok := section["type"]; ok {
			if err := json.Unmarshal(raw, &t); err != nil {
				return nil, fmt.Errorf("bridge %s: type: %w", name, err)
			}
			delete(section, "type") // Oiko's key, not the type's
		}
		t = cmp.Or(t, name)
		m, ok := bridge.Lookup(t)
		if !ok {
			return nil, fmt.Errorf("bridge %s: no type %q in this build of Oiko", name, t)
		}
		config, err := json.Marshal(section)
		if err != nil {
			return nil, fmt.Errorf("bridge %s: %w", name, err)
		}
		env := bridge.Env{Name: name, Config: config, DataDir: filepath.Join(dataDir, name), Log: slog.With("bridge", name)}
		if err := os.MkdirAll(env.DataDir, 0o700); err != nil { // tokens and keys
			return nil, err
		}
		bridges = append(bridges, configured{env, m})
	}
	return bridges, nil
}

// printBuild writes b: Oiko's version, then one line per type of Bridge with
// its package, module and version.
func printBuild(w io.Writer, b build.Build) {
	unknown := func(s string) string { return cmp.Or(s, "unknown") }
	fmt.Fprintln(w, "oiko", unknown(b.Version))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, t := range b.Types {
		origin := map[bool]string{true: "built-in", false: "added"}[t.BuiltIn]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.Type, origin, t.Package, unknown(t.Module), unknown(t.Version))
	}
	tw.Flush()
}

// command runs args, <bridge> <command> [args], on its Bridge.
func command(bridges []configured, args []string) error {
	i := slices.IndexFunc(bridges, func(b configured) bool { return b.env.Name == args[0] })
	if i < 0 {
		return fmt.Errorf("no bridge %q in the configuration", args[0])
	}
	b := bridges[i]
	if len(args) < 2 || b.module.Commands[args[1]] == nil {
		return fmt.Errorf("usage: oiko %s <command> [args]; commands: %s", b.env.Name,
			strings.Join(slices.Sorted(maps.Keys(b.module.Commands)), ", "))
	}
	return b.module.Commands[args[1]](b.env, args[2:])
}

// serve runs Oiko until it receives SIGINT or SIGTERM.
func serve(listen, dataDir, configFile, install string, c config, bridges []configured) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	devicesFile := filepath.Join(dataDir, "devices.json")
	aggregatesFile := filepath.Join(dataDir, "aggregates.json")
	flagsFile := filepath.Join(dataDir, "flags.json")
	areasFile := filepath.Join(dataDir, "areas.json")
	automationsFile := filepath.Join(dataDir, "automations.json")
	automationStateFile := filepath.Join(dataDir, "automation-state.json")
	var saved []home.Device
	var savedAggregates []home.Aggregate
	var savedFlags []home.SavedFlag
	var savedAreas []home.Area
	var savedAutomations []automation.Document
	var automationState map[string]automation.State
	// ponytail: each store migrates or refuses as it is loaded, so a refusal
	// can follow another store's migration; a pre-pass over every store,
	// Bridges' included, if stores ever disagree on an update or rollback.
	for _, s := range []struct {
		file string
		f    store.Format
		v    any
	}{
		{devicesFile, home.DevicesFormat, &saved}, {aggregatesFile, home.AggregatesFormat, &savedAggregates},
		{flagsFile, home.FlagsFormat, &savedFlags}, {areasFile, home.AreasFormat, &savedAreas},
		{automationsFile, automation.DocumentsFormat, &savedAutomations}, {automationStateFile, automation.StateFormat, &automationState},
	} {
		if err := store.Load(s.file, s.f, s.v); err != nil {
			log.Fatalf("loading %s: %v", s.file, err)
		}
	}

	hist, err := history.Open(filepath.Join(dataDir, "history.db"))
	if err != nil {
		log.Fatalf("history: %v", err)
	}
	defer hist.Close()
	// The writer outlives ctx: it stops once the engine has, writing what is
	// still buffered.
	// ponytail: Command outcomes and History points Home announces after that
	// (a timeout firing, a late report during shutdown) are not written; stop
	// Home's timers and Bridges first if that matters.
	writerCtx, stopWriter := context.WithCancel(context.Background())
	written := make(chan struct{})
	go func() {
		hist.Run(writerCtx)
		close(written)
	}()

	h := home.New(saved, savedAggregates, savedFlags, savedAreas,
		func(devices []home.Device) error { return store.Save(devicesFile, home.DevicesFormat, devices) },
		func(aggregates []home.Aggregate) error {
			return store.Save(aggregatesFile, home.AggregatesFormat, aggregates)
		},
		func(flags []home.SavedFlag) error { return store.Save(flagsFile, home.FlagsFormat, flags) },
		func(areas []home.Area) error { return store.Save(areasFile, home.AreasFormat, areas) },
		hist.Command,
	)
	h.Recall(hist.Recall)
	running := make([]func(), 0, len(bridges))
	for _, b := range bridges {
		br, err := b.module.New(b.env)
		if err != nil {
			log.Fatalf("%s: bridge %s: %v", configFile, b.env.Name, err)
		}
		port := h.Attach(b.env.Name, br)
		running = append(running, func() { br.Run(ctx, port) })
	}
	// Created before the Bridges run, so the engine and the History see every
	// Update.
	engine := automation.New(h, savedAutomations, automationState, c.Location,
		func(docs []automation.Document) error {
			return store.Save(automationsFile, automation.DocumentsFormat, docs)
		},
		func(state map[string]automation.State) error {
			return store.Save(automationStateFile, automation.StateFormat, state)
		},
		hist.Record)
	h.Follow(hist.Follow)
	if c.Telegram != nil {
		bot, err := telegram.New(*c.Telegram)
		if err != nil {
			log.Fatalf("%s: %v", configFile, err)
		}
		engine.NotifyThrough(bot.Notify)
		go bot.Run(ctx)
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		engine.Run(ctx) // its clock starts once the home is known
	}()
	defer func() { // once the HTTP server is done: nothing changes the Step state any more
		<-stopped
		engine.Flush()
		stopWriter()
		<-written
	}()
	for _, run := range running {
		go run()
	}

	types := map[string]string{} // each Bridge's type, by name
	for _, b := range bridges {
		types[b.env.Name] = b.module.Type
	}
	built := build.Current()
	releases := release.New(built)
	go releases.Run(ctx)
	srv := &http.Server{
		Addr:        listen,
		Handler:     api.Handler(h, engine, hist, built, install, releases, types, web.Dist()),
		BaseContext: func(net.Listener) context.Context { return ctx }, // ends SSE streams on shutdown
	}
	served := make(chan struct{})
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
		close(served)
	}()
	log.Printf("oiko: listening on %s", listen)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-served // requests under way are done
}
