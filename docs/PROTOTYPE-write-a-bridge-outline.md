# PROTOTYPE — outline of "Write a type of Bridge" (throwaway, wayfinder ticket #88)

Rough outline to react to, not final text. Inputs: no starter, the guide walks
from `go mod init` and quotes a first type compiled and tested in the oiko repo
(#87); the guide is `docs/write-a-bridge.md` (#86); Caddy is the model: concept
first, a one-liner build, local-path iteration (#81); Apache-2.0 recommended,
an OSI license required by the catalogue (#85).

Open points are marked **→ DECIDE**.

## 0. Shape

- One page, `docs/write-a-bridge.md`, read top to bottom once, then skimmed.
  About 400 lines: sections 1–4 are the walk, 5–9 are look-ups.
- It explains and walks; it never restates a method's contract. Each symbol it
  names links its godoc on pkg.go.dev, which stays the reference (table in
  section 10).
- Its first type is a real package in the oiko repo, next to the guide:
  `docs/write-a-bridge/plug/` (**→ DECIDE**: there, or a godoc `Example`).

## 1. What a type of Bridge is (½ screen)

- A type of Bridge (a plugin, in other projects' words) connects Oiko to one
  kind of external system: a hub, a cloud API, devices on the LAN. Built in:
  zigbee2mqtt, homekit. Others: the catalogue.
- It is a Go module, compiled into Oiko with `oiko-build`: no runtime, no
  process boundary, no RPC. Consequence stated plainly: it runs with Oiko's
  rights, and users trust it as they trust Oiko (SECURITY.md's trust model).
- What you need: Go 1.27, an API or protocol for your system, an hour.
- What you get: Devices on Dashboards, in Automations, in History, with no web
  code: the web client shapes each Tile from your Capabilities (section 5).

## 2. The model in five minutes

One diagram, then one paragraph per term, each linking `GLOSSARY.md`:

```
            config section ──► Module.New(env) ──► Bridge
                                                     │ Run(ctx, port)
 external system ◄──── Send(address, function, values) ◄── Oiko (a Command)
 external system ────► port.SyncDevices / Report / SetAvailability ──► Oiko
```

- **Bridge**, **Port**: Oiko calls `Run` once and `Send` per Command; the
  Bridge calls the Port. Run never returns before ctx ends; it reconnects
  itself.
- **Device**, **Function**, **Capability**: a Device is found by its
  NativeAddress; Oiko keeps its Name, Icon and Areas. A Function is what it
  does for the household (`switch`, `light`, `temperature`).
- **Reading**: a Value, or an Event for a Stateless Capability.
- **Command**: Oiko validates the values, calls `Send`, and waits for a Report
  that confirms it. A Send that returns nil without a Report times out.
- **online / Availability**: the system reachable or not; each Device
  reachable or not.
- **Replay**: call `Replayed` once the state is as known as it gets;
  Automations wait for it (10 s at most).

## 3. Your first type, end to end: `plug`

A made-up smart plug on the LAN with a two-route HTTP API, so the guide needs
no hardware and its test needs no network beyond `httptest`:

```
GET  /status          → {"on": true, "power": 12.5}
POST /status {"on":…} → the new status
```

**→ DECIDE**: this made-up plug, or something with no network at all (an
in-memory virtual switch), shorter but further from what authors write.

Steps, each one a code block quoted from `docs/write-a-bridge/plug/`:

3.1 `go mod init example.com/oiko-plug`, `go get github.com/llehouerou/oiko@v0.N.0`.
    Why the root package: `oiko-build` imports it.

3.2 Register and read the configuration:

```go
package plug

func init() { bridge.Register(bridge.Module{Type: "plug", New: open}) }

// config is the type's section of "bridges": each plug's name and address.
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
	return &Bridge{plugs: c.Plugs, client: &http.Client{Timeout: 5 * time.Second}, log: env.Log}, nil
}
```

    Callouts: `env.Decode`; secrets as `…File` keys read at `New`
    (oiko-netatmo's `clientSecretFile`), never inline.

3.3 Describe the Devices:

```go
// switchCapabilities make a Tile with an on/off bar and a power reading.
var switchCapabilities = []bridge.Capability{
	{Key: "state", Label: "State", Type: bridge.Binary, Category: bridge.Primary,
		Access: bridge.Access{Observable: true, Settable: true}},
	{Key: "power", Label: "Power", Type: bridge.Numeric, Unit: "W", Category: bridge.Primary,
		Access: bridge.Access{Observable: true}},
}

func (b *Bridge) devices() []bridge.Device {
	var ds []bridge.Device
	for _, name := range slices.Sorted(maps.Keys(b.plugs)) {
		ds = append(ds, bridge.Device{NativeAddress: name, Name: name, Model: "Plug",
			Functions: []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: switchCapabilities}}})
	}
	return ds
}
```

    Callout: NativeAddress must be stable; here it is the configured name, so
    renaming a plug makes a new Device. A real system's own id is better.

3.4 Run: list, report, replay, poll:

```go
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
	for name, url := range b.plugs {
		s, err := b.status(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.log.Debug("poll", "plug", name, "err", err)
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
	}, time.Now())
}
```

3.5 Send, and confirm with a Report:

```go
func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, _ time.Duration) error {
	on := values["state"].(bool) // Oiko sends only what the Capabilities accept
	s, err := b.status(ctx, http.MethodPost, b.plugs[address], map[string]bool{"on": on})
	if err != nil {
		return err // fails the Command
	}
	b.report(address, s) // confirms it
	return nil
}
```

    (`status`, the HTTP helper, is in the package, not quoted: 20 lines of
    net/http.)

## 4. Test it, then run it

4.1 Test with `bridgetest`: a fake plug on `httptest`, Oiko's own rules.

```go
func TestPlug(t *testing.T) {
	fake := newFakePlug(t, status{On: true, Power: 12.5}) // httptest server
	b, err := open(bridgetest.Env(t, "plugs", `{"plugs": {"Desk lamp": "`+fake.URL+`"}}`))
	if err != nil {
		t.Fatal(err)
	}
	h := bridgetest.New(b)
	go b.Run(t.Context(), h.Port())
	waitFor(t, h.Replayed)

	if v, _ := h.Value("Desk lamp", "switch", "power"); v.Data != 12.5 {
		t.Errorf("power = %v, want 12.5", v.Data)
	}
	if err := h.Command("Desk lamp", "switch", map[string]any{"state": false}); err != nil {
		t.Fatalf("command: %v", err) // ErrRefused, ErrFailed or ErrTimedOut
	}
	if fake.on() {
		t.Error("plug still on")
	}
}

func TestUnknownKey(t *testing.T) { /* open refuses {"plug": …} */ }
```

    **→ DECIDE** (small): `waitFor` polls `h.Replayed` with a deadline. The
    bridgetest doc suggests `testing/synctest`, which does not wait on real
    sockets; a fake `http.RoundTripper` would allow it at the cost of noise.

4.2 Build an Oiko with it, from directories, and run it:

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -with example.com/oiko-plug=. -o oiko
./oiko -version                 # lists "plug  example.com/oiko-plug (devel)"
```

    `config.json`: `{"bridges": {"plugs": {"type": "plug", "plugs": {…}}}}`;
    a Bridge's name vs its type (link docs/configure.md). Then the plug on a
    Dashboard: one screenshot (fog: Visuals).
    Against an oiko checkout: `-oiko ../oiko` (needs `make build` there once).

## 5. Describing Devices so Oiko shows them well

The section authors will come back to. Today this knowledge sits only in
`web/src/roles.ts` and ADR 0014.

- Kind and Capability keys decide the Tile, never the model: one table of
  the keys Oiko recognises, per Role (main control `state`/`on`/`alarm`/`mode`;
  light adjustments; states `occupancy`, `contact`…; health `battery`…;
  reading order `temperature`, `humidity`…).
  **→ DECIDE**: how this table stays true: hand-kept with a test in `web/`
  comparing it to `roles.ts`, or only a link to `roles.ts`.
- Use zigbee2mqtt's names when in doubt: the keys come from its exposes.
- Category: `primary`, `config` (a setting behind ⋯), `diagnostic`.
- Stateless (Events: a button press, a doorbell), Counter (energy totals),
  Enum with Options and Triggers, Composite with Fields.
- Report the device's own time as `at` when it gives one.

## 6. Keep data: `bridge/store`

- `env.DataDir` is the type's own; `store.Load`/`store.Save` with a `Format`.
- Change the format later by adding a migration, never by breaking it: users
  upgrade your type without losing a pairing (ADR 0019 applied to you).
- Reference: oiko-netatmo, its OAuth token (link at its latest tag).

## 7. Cameras and Recordings

- A Function of kind `camera`; `bridge.Cameras`: Picture never wakes the
  camera, Stream gives an RTSP URL, H.264/AAC (ADR 0036).
- `bridge.Recordings` when the system keeps clips; announce each new one with
  an Event of `bridge.RecordingEvent` as of its Start (ADR 0038, 0039).
- Never put a URL that grants the media in an error.
- Test with `h.Picture`, `h.Recordings`, `h.RecordingMedia`.
- Reference: oiko-arlo.

## 8. More you may need (short items)

- Command-line commands (`Module.Commands`): pairing, a sign-in; run as
  `oiko <bridge> <command>` while Oiko is not serving.
- Reconnect with backoff; `SetOnline(false)` while the system is gone; never
  panic in `Run` (it takes Oiko down).
- Logging through `env.Log`: Debug for per-poll noise, no secrets.
- Several types in one module: all registered from the root package.

## 9. Publish and keep up

9.1 Publish:
    - Repository layout: `go.mod`, the package, its test, `oiko-bridge.json`,
      `README.md`, `LICENSE` (Apache-2.0 recommended; the catalogue lists
      OSI-approved licenses only), a flake if you use Nix.
    - The manifest: a one-block example for `plug`, then a link to its spec.
      **→ DECIDE** (fog "Ecosystem alignment"): the manifest's spec stays in
      the catalogue's README, or moves here.
    - GitHub topic `oiko-bridge`, tag `v0.1.0`; the catalogue picks it up the
      next day, builds it against the latest Oiko, and shows the build error
      if it fails.
9.2 Users install it: `oiko-build -with example.com/oiko-plug@v0.1.0`, or the
    flake override (`bridges."example.com/oiko-plug" = "v0.1.0"`,
    `vendorHash`) — link docs/configure.md, which holds both.
9.3 Keep up with Oiko (ADR 0019):
    - Your `go.mod` requirement on oiko is your minimum Oiko.
    - During v0, a minor release may break the contract; a patch never does.
      Each minor's release notes list contract changes; the catalogue marks
      your type incompatible until you move.
    - Your own users' data: migrate it (section 6); your own config: refuse
      unknown keys, so a removed key stops Oiko loudly.

## 10. What pkg.go.dev holds instead

| Topic | Guide | godoc (`bridge`, `bridge/store`, `bridge/bridgetest`) |
|---|---|---|
| Method contracts (Run, Send concurrency, Port semantics) | one sentence each, link | full text, already there |
| Capability fields, ValueType ↔ Go types | how to choose | each field and type, already there |
| Cameras / Recordings contracts | when to implement, pitfalls | full text, already there |
| `store.Format`, migrations, Check | why and when | API |
| `bridgetest` API | the walk-through test | each method |
| Tile-shaping keys (Roles) | **the table** (section 5) | nothing: not part of the contract |
| oiko-build flags | the two commands authors run | `cmd/oiko-build` doc |

Gaps the walk exposed (fog: API reference gaps, each its own issue later):

- `bridge` has no `Example` at all; the package doc's snippet is three lines.
- `Category` and `Access` carry no doc comment.
- `Function.Kind`'s known values are an ellipsis ("light", "occupancy",
  "temperature"…).
- Whether `Send` may run before `Run` has stored the Port is unsaid (the
  sketch relies on "nothing is sent while offline").

## 11. The in-repo code

`docs/write-a-bridge/plug/`: `plug.go`, `plug_test.go`, `oiko-bridge.json`.
Built and tested by `make test` with every change; never imported by Oiko, so
it adds nothing to the binary.

How the Markdown stays true to it (**→ DECIDE**):

- A test in `plug_test.go` reads `docs/write-a-bridge.md` and fails when a Go
  block of the guide is not found verbatim in the package's files.
- Or the guide links the files, quoting nothing (drift-proof, worse to read).
