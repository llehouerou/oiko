// Package oiko runs the Oiko home automation server. A build of Oiko is a
// main package calling Main; it imports, for their side effect, the packages
// of the types of Bridge it adds to the built-in ones (see package bridge and
// ADR 0017).
package oiko

import (
	"cmp"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/api"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/dashboard"
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
	PublicURL *string                    `json:"publicUrl"` // where users reach Oiko; sign-in needs it
	Location  *automation.Place          `json:"location"`  // for the sun triggers
	Telegram  *telegram.Config           `json:"telegram"`  // no Notifications without it
	Bridges   map[string]json.RawMessage `json:"bridges"`   // each one's section, by name
}

// configured is a Bridge of the configuration, of a type compiled in.
type configured struct {
	env    bridge.Env
	module bridge.Module
}

// Main runs Oiko, or with arguments upgrades it, signs a Person in from its
// host, or runs a command of one of its Bridges:
// oiko [flags] [upgrade [-y] | sign-in-link [<id>] | <bridge> <command> [args]].
func Main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dataDir := flag.String("data", "data", "directory holding Oiko's own configuration")
	configPath := flag.String("config", "", "hand-written configuration (default <data>/config.json)")
	showVersion := flag.Bool("version", false, "print what this Oiko is built from and exit")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: oiko [flags] [upgrade [-y] | sign-in-link [<id>] | <bridge> <command> [args]]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		printBuild(os.Stdout, build.Current())
		return
	}
	// The last way back in, from the host: it needs no configuration (ADR 0035).
	if flag.Arg(0) == "sign-in-link" {
		if err := signInLink(*dataDir, flag.Args()[1:], os.Stdout); err != nil {
			log.Fatal(err)
		}
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

	configFile := *configPath
	if configFile == "" {
		configFile = filepath.Join(*dataDir, "config.json") // written by hand
	}
	c, public, bridges, err := load(*dataDir, configFile)
	if err != nil {
		log.Fatal(err)
	}
	if flag.NArg() > 0 {
		if err := command(bridges, flag.Args()); err != nil {
			log.Fatal(err)
		}
		return
	}
	if public == nil {
		log.Printf("oiko: no publicUrl in %s: only http://localhost offers sign-in; anywhere else, only a Program's Token is served", configFile)
	}
	serve(*listen, *dataDir, configFile, inst, c, public, bridges)
}

// load readies Oiko to run from dataDir: it creates the data directory if it
// is missing, private since it holds secrets (an existing one may be
// shared, and is left as it is), then reads and checks the configuration at
// configFile, and resolves its Bridges. The Public URL is nil when the
// configuration has none.
func load(dataDir, configFile string) (config, *url.URL, []configured, error) {
	var c config
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return c, nil, nil, fmt.Errorf("data directory: %w", err)
	}
	if err := loadConfig(configFile, &c); err != nil {
		return c, nil, nil, fmt.Errorf("loading %s: %w", configFile, err)
	}
	var public *url.URL
	if c.PublicURL != nil {
		var err error
		if public, err = parsePublicURL(*c.PublicURL); err != nil {
			return c, nil, nil, fmt.Errorf("%s: publicUrl %q: %w", configFile, *c.PublicURL, err)
		}
	}
	if l := c.Location; l != nil && (math.Abs(l.Latitude) > 90 || math.Abs(l.Longitude) > 180) {
		return c, nil, nil, fmt.Errorf("%s: location out of range: %+v", configFile, *l)
	}
	bridges, err := configure(c.Bridges, dataDir)
	if err != nil {
		return c, nil, nil, fmt.Errorf("%s: %w", configFile, err)
	}
	return c, public, bridges, nil
}

// parsePublicURL reads s as the Public URL, an HTTPS origin with nothing after
// it but a "/" (ADR 0035): a __Host- cookie needs Path=/, the RP ID is the
// host, and the web client is served from /. The origin is returned as a
// browser sends it: its host in lowercase, without the default port.
func parsePublicURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	switch {
	case err != nil:
		return nil, err
	case u.Scheme != "https":
		return nil, errors.New("want https://host[:port]")
	case u.User != nil:
		return nil, errors.New("no userinfo allowed")
	case u.Hostname() == "":
		return nil, errors.New("no host")
	case u.Path != "" && u.Path != "/", u.Opaque != "":
		return nil, errors.New("no path allowed: Oiko is served from /")
	case u.RawQuery != "" || u.ForceQuery:
		return nil, errors.New("no query allowed")
	case strings.Contains(s, "#"):
		return nil, errors.New("no fragment allowed")
	}
	if p := u.Port(); p != "" || strings.HasSuffix(u.Host, ":") {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return nil, errors.New("want a port from 1 to 65535")
		}
	}
	return &url.URL{Scheme: "https", Host: strings.TrimSuffix(strings.ToLower(u.Host), ":443")}, nil
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
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return jsonv2.Unmarshal(data, c, jsonv2.RejectUnknownMembers(true)) // names match in their case only
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
		if name == "upgrade" || name == "sign-in-link" {
			return nil, fmt.Errorf("bridge %s: the name is oiko %[1]s's", name)
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
		if err := os.MkdirAll(env.DataDir, 0o700); err != nil { // its secrets
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

// serve runs Oiko until it receives SIGINT or SIGTERM. public is the Public
// URL, nil when the configuration has none.
func serve(listen, dataDir, configFile, install string, c config, public *url.URL, bridges []configured) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// ponytail: each store migrates or refuses as it is opened, so a refusal
	// can follow another store's migration; a pre-pass over every store,
	// Bridges' included, if stores ever disagree on an update or rollback.
	h, err := home.Open(dataDir, hist.Command)
	if err != nil {
		log.Fatal(err)
	}
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
	engine, err := automation.Open(dataDir, h, c.Location, hist.Record)
	if err != nil {
		log.Fatal(err)
	}
	h.Follow(hist.Follow)
	cams := camera.New(h, hist.LiveView)
	if c.Telegram != nil {
		bot, err := telegram.New(*c.Telegram, cams)
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
	acc, err := access.Open(dataDir, time.Now, hist.Audit)
	if err != nil {
		log.Fatalf("access: %v", err)
	}
	if secret, ok := acc.Setup(); ok {
		log.Printf("oiko: no Admin yet: open %s/setup#%s to claim this Oiko", setupOrigin(listen, public), secret)
	}
	dash, err := dashboard.Open(dataDir, h)
	if err != nil {
		log.Fatalf("dashboards: %v", err)
	}
	hostLinks, err := listenHost(dataDir)
	if err != nil {
		log.Fatalf("oiko sign-in-link: %v", err)
	}
	go serveHost(ctx, hostLinks, acc, setupOrigin(listen, public))
	built := build.Current()
	releases := release.New(built)
	go releases.Run(ctx)
	srv := api.Server(listen, api.Handler(h, engine, hist, cams, acc, dash, built, install, releases, types, public, web.Dist()))
	srv.BaseContext = func(net.Listener) context.Context { return ctx } // ends SSE streams on shutdown
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
	if err := acc.Flush(); err != nil {
		log.Printf("access: %v", err)
	}
}

// setupOrigin is where the Setup link and the host's Sign-in links open: the
// Public URL, or without one http://localhost on the port Oiko listens on,
// the only place sign-in works then (ADR 0027).
func setupOrigin(listen string, public *url.URL) string {
	if public != nil {
		return public.String()
	}
	_, port, _ := net.SplitHostPort(listen)
	return "http://localhost:" + port
}
