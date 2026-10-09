# Write a type of Bridge

This guide takes you from an empty directory to a type of Bridge listed in the
[catalogue][catalogue]. Read it once from the top: the first four sections are a walk through a
first type, end to end; the ones after it are what you come back to. It says why and when; the
contracts themselves are in the godoc of [`bridge`][bridge], [`bridge/bridgetest`][bridgetest]
and [`bridge/store`][store], which each symbol below links.

## What a type of Bridge is

A type of Bridge (a plugin, in other projects' words) connects Oiko to one kind of external
system it doesn't know: a hub, a cloud API, devices on the local network. Two types are built in,
zigbee2mqtt and homekit ([ADR 0021](adr/0021-built-in-the-local-standard-protocols.md)); the
others live in their authors' repositories and the catalogue lists them. Once Oiko is built with
yours, a Bridge of that type is [configured](configure.md#bridges) like any other.

A type of Bridge is a Go module compiled into Oiko with `oiko-build`
([ADR 0017](adr/0017-bridge-types-compiled-in-through-a-public-contract.md)): no runtime, no
separate process, no RPC. It runs in Oiko's process with Oiko's rights, so whoever adds your type
trusts it as they trust Oiko, and a panic in it takes Oiko down.

You need Go 1.27 and the API or protocol of your system. You write no web code: the web client
shapes each Device's Tile from what your type says the Device can do
([Describing Devices](#describing-devices-so-oiko-shows-them-well)), and your Devices are on
Dashboards, in Automations and in the History like any other.

## The model in five minutes

```
     its section of "bridges" ──► Module.New(env) ──► Bridge
                                                        │ Run(ctx, port), once
 external system ◄── Send(ctx, address, function, values) ◄── Oiko, per Command
 external system ──► port: SyncDevices, SetOnline, SetAvailability, Report, Replayed ──► Oiko
```

Each linked term is [GLOSSARY.md](../GLOSSARY.md)'s, which says more of it.

[**Bridge**](../GLOSSARY.md) and **Port**. Oiko creates each Bridge with your Module's `New`,
from its section of the configuration, calls its `Run` once and its `Send` for each Command
([`Bridge`][Bridge]). `Run` follows the external system until its context ends: it never returns
before, and reconnects by itself, since Oiko never restarts it. What it learns it hands Oiko
through the [`Port`][Port].

[**Device**](../GLOSSARY.md), [**Function**](../GLOSSARY.md) and
[**Capability**](../GLOSSARY.md). A Device is the hardware as your Bridge describes it, found by
its Native Address: unique within the Bridge, and stable. Oiko gives it its identity and keeps
its Name, Icon and Areas; the Name you give only names a new Device. A Function is what the Device
does for the household (`switch`, `light`, `temperature`): a sensor measuring several quantities
has one Function each. A Capability is a typed property of a Function (`state`, `brightness`), or
of the Device itself for its settings and diagnostics.

**Reading**. A Capability's data in a Report, as of when the device sent it: a Value, or an Event
for a Stateless Capability, such as a button press.

[**Command**](../GLOSSARY.md). Oiko checks the values against the Capabilities, calls `Send`, and
waits for a Report that confirms them. An error from `Send` fails the Command; a `Send` that
returns nil without a Report following times out.

**online** and [**Availability**](../GLOSSARY.md). Whether the external system is reachable, for
the Bridge as a whole, and whether each Device is. A Bridge starts offline, and Oiko refuses
Commands to it until it says otherwise; while it is offline, its Devices' Availability is unknown.

[**Replay**](../GLOSSARY.md). What your Bridge hands Oiko on connecting so that its Devices' state
is known. Once the state is as known as it gets, your Bridge says so; the automation engine waits
for every Bridge's, about 10 s at most. A replayed Value is state, not a change: it fires no Value
trigger.

## Your first type: `plug`

The plug is made up: a smart plug on the local network that answers two HTTP routes, so the walk
needs no hardware.

```
GET  /status           → {"on": true, "power": 12.5}
POST /status {"on": …} → the new status
```

Its code is in this repository, [`write-a-bridge/plug`](write-a-bridge/plug), laid out as your own
repository will be and tested with every change to Oiko: each Go block below is quoted from it.

### Start a module

```sh
mkdir oiko-plug && cd oiko-plug
go mod init example.com/oiko-plug   # where you will publish it: github.com/you/oiko-plug
```

The type is the module's root package: `oiko-build` and the catalogue import that one. Once
`plug.go` is written, `go mod tidy` requires the latest Oiko; that version is the oldest Oiko your
type builds with ([Keep up with Oiko](#keep-up-with-oiko)).

### Register the type, read its configuration

```go
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
```

`init` registers the type under the name configurations give it, `plug` ([`Register`][Register]).
Oiko calls `New` once per Bridge of the type, with an [`Env`][Env]: the Bridge's section of the
configuration, a data directory and a logger. [`Env.Decode`][Env.Decode] refuses a key `config`
has no field for, so a misspelt key stops Oiko from starting instead of being ignored. Check the
rest yourself: an error from `New` stops Oiko too, and its message is all the user gets, so name
the key or the plug at fault.

A secret goes in a file. Take a key ending in `File`, as oiko-netatmo's `clientSecretFile`, and read
the file in `New`: the configuration, which Nix may write to its world-readable store, never holds
the secret itself.

### Describe the Devices

```go
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
```

Each plug is a [`Device`][Device] with one [`Function`][Function], of kind `switch`, and two
[`Capability`][Capability]s. The kind and the Capabilities' keys shape the Tile: `state`, Binary,
Primary and Settable, is its main control, an on/off bar; `power`, which can only be observed, a
reading. [Describing Devices](#describing-devices-so-oiko-shows-them-well) lists the keys Oiko
knows. [`Access`][Access] matters beyond the Tile: a Command waits for a Report only on what is
Observable.

A plug's NativeAddress is its name here because the plug has nothing better. Use your system's own
identifier when it has one (a serial number, an id from its API): a Device keeps its identity, its
History and its place on the Dashboards only as long as its NativeAddress stays the same.

### Run: list, go online, replay, poll

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
```

`Run` hands Oiko the Devices ([`Port.SyncDevices`][Port]: one it no longer lists
becomes Detached), then goes online ([`Port.SetOnline`][Port]). Oiko sends no Command to
an offline Bridge, so `Send` comes only after that, once `b.port` is set. The plug has no hub or
cloud to connect to, so it is online at once and each plug's Availability tells whether it
answers; a type with a hub goes online once connected, and offline while the hub is gone.

The first poll is the Replay: [`Port.Replayed`][Port] follows it. Automations wait for
it, so it must not wait on a device that doesn't answer:

```go
// poll reads every plug at once: one that doesn't answer delays neither the
// others nor the Replay by more than the client's timeout.
func (b *Bridge) poll(ctx context.Context) {
	var wg sync.WaitGroup
	for name, addr := range b.plugs {
		wg.Go(func() {
			s, err := b.call(ctx, http.MethodGet, addr, nil)
			switch {
			case ctx.Err() != nil: // Oiko is stopping: the plug is no less reachable
			case err != nil:
				b.log.Debug("poll", "plug", name, "err", err) // every 10 s: Debug, not Warn
				b.port.SetAvailability(name, bridge.Offline)
			default:
				b.port.SetAvailability(name, bridge.Online)
				b.report(name, s)
			}
		})
	}
	wg.Wait()
}

func (b *Bridge) report(name string, s status) {
	b.port.Report(name, []bridge.Reading{
		{Function: "switch", Capability: "state", Data: s.On},
		{Function: "switch", Capability: "power", Data: s.Power},
	}, time.Now()) // the plug gives no time of its own
}
```

[`Port.SetAvailability`][Port] records whether each plug answers.
[`Port.Report`][Port] records its Readings, each `Data` of the Go type its Capability's
[`ValueType`][ValueType] says, as of `at`: when the device sent them. Report everything you read,
every time: a Value that hasn't changed is only a refresh, and the History records changes.

### Send, confirmed by a Report

```go
func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, _ time.Duration) error {
	on, ok := values["state"].(bool) // Oiko sends only what the Capabilities accept
	if !ok {
		return errors.New("no state")
	}
	s, err := b.call(ctx, http.MethodPost, b.plugs[address], map[string]bool{"on": on})
	if err != nil {
		return err // fails the Command
	}
	b.report(address, s) // confirms it
	return nil
}
```

The values are keyed by Capability key and already checked: `state` is a bool, a toggle already
resolved ([`Bridge.Send`][Bridge] says what else Oiko guarantees, such as one `Send` at a time
per Function or Device). The plug answers with its new status, so `Send` reports it at once. A
system that tells later, through its event stream or the next poll, returns nil and lets that
Report confirm the Command. `call`, which reads or sets a status with `net/http`, is in
[`plug.go`](write-a-bridge/plug/plug.go).

## Test it, then run it

### Test it with `bridgetest`

[`bridgetest`][bridgetest] attaches your Bridge to a home that applies what it hands its Port by
Oiko's own rules, and issues Commands as Oiko does ([`Home.Command`][Home.Command]). Run the test
inside [`synctest.Test`][synctest]:

```go
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
```

Inside the bubble the clock is fake, and `synctest.Wait` returns once every goroutine in it is
blocked: here, once `Run` waits for its ticker, after the Replay. A socket doesn't count as
blocked, so the fake plug is an `http.Handler` reached through an in-memory `http.RoundTripper`,
the pattern for any type that speaks HTTP:

```go
// network is an in-memory LAN, an http.RoundTripper: each host's handler
// answers its requests, with no socket, so synctest.Wait sees the Bridge idle
// and the poll's ticker fast-forwards. Any other host is unreachable.
type network map[string]http.Handler
```

Ten seconds of the poll then pass at once:

```go
		// Someone switches the plug on by hand: the next poll, 10 s on, reports it.
		desk.set(status{On: true, Power: 40})
		time.Sleep(10 * time.Second)
		synctest.Wait()
```

The same file has `TestOpen`, the configurations `open` accepts and refuses, through
[`bridgetest.Env`][bridgetest.Env], and `TestManifest`, which checks the
[Manifest](#publish) against the types registered.

### Build an Oiko with it

```sh
go test ./...
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -with example.com/oiko-plug=. -o oiko
./oiko -version   # plug  added  example.com/oiko-plug  example.com/oiko-plug  unknown
```

`-with package=directory` builds your type from its directory; `-version` lists it as added, its
version unknown since it comes from a directory. Give it a Bridge in `data/config.json`:

```json
{"bridges": {"plug": {"plugs": {"Desk lamp": "http://192.0.2.10"}}}}
```

A Bridge's name, `plug`, is its type unless its section says `"type"`
([Bridges](configure.md#bridges)). Run `./oiko`, sign in as the [Install](install.md) page says,
and the Desk lamp is on the built-in Dashboard.

To work on Oiko and your type at once, build from a checkout of Oiko:
`-oiko ../oiko -with example.com/oiko-plug=.`. A checkout's web client is there once `make build`
has run in it.

## Describing Devices so Oiko shows them well

The web client shapes a Function's Tile from its kind and its Capabilities' keys, types, access and
category, never from the hardware model
([ADR 0014](adr/0014-tiles-shaped-by-capability-keys.md)). Each Capability has a Role on its
Function, told by its key:

| Role | Keys | When |
|---|---|---|
| Main control | `state`, `on`, `alarm`, `mode` | Primary, Settable, not Stateless: the first of these keys, a light's `state` only. An on/off bar when Binary. At most one per Function |
| Adjustment | `brightness`, `color_temp`, `color_hs` | a `light`'s, Settable: they refine its state, beside it |
| State | `occupancy`, `presence`, `contact`, `water_leak`, `smoke` | Binary, not Settable; on while active (occupied, open, leaking), so a contact reports open as on |
| Health | `battery`, `battpercentage`, `battery_low`, `tamper` | any: shown only when something is wrong |
| Diagnostic | `current`, `voltage` | and any Capability of Category Diagnostic: shown apart (a plug's current and voltage, beside its power) |
| Reading | `temperature`, `humidity`, `co2`, `noise`, `pressure`, `illuminance` | the rest that can only be observed, these first, in this order |

The other Roles go by type, not key. A Stateless Capability's are Events (under `action`, a
button's presses), and every other Settable Capability, with any of Category Config, is a setting
behind the Tile's ⋯, but for a light's composites other than its colour, which have none.

- A main control under a key the table doesn't list shows no control: the Tile shows it as a
  setting. Automations see every Capability, whatever the Tile shows.
- When in doubt, take zigbee2mqtt's names: the keys above come from its exposes.
- Some kinds are treated apart, a `light`'s adjustments, a `camera`, the kinds each Area
  aggregates: [`Function`][Function] lists them. Give a sensor one Function per quantity, each of
  its kind (`temperature`, `humidity`), so each is placed and aggregated on its own.
- The [`Category`][Category]: Primary for what the Function is for, Config for a setting that
  changes how the Device behaves (only an Admin sets it), Diagnostic for how the Device itself is
  doing. A Device's own Capabilities are its Config and Diagnostic ones.
- A [`Capability`][Capability] may be Stateless (Events: a button, a doorbell), a Counter (a
  running total, such as energy in kWh, so Oiko tells what was used over a period), an Enum with
  Options a Command may set and Triggers that start a one-off action, or a Composite of Fields.
- Report the device's own time as `at` when it gives one: the History then tells when a reading
  was taken, not when you polled it.

## Keep data

[`Env.DataDir`][Env] is a directory of the Bridge's own, `data/<its name>/`, for what it keeps:
tokens, sessions, pairings. Keep each document there with [`store.Load`][store.Load] and
[`store.Save`][store.Save], in a [`store.Format`][store.Format].

When the document's shape must change, add a migration to its Format instead of breaking it:
`Load` migrates an older document forward, after keeping a copy, so a user upgrading your type
never loses a pairing or has to sign in again. Oiko treats its own data this way and recommends
it to every type ([ADR 0019](adr/0019-release-and-compatibility-policy.md)).

The reference is [oiko-netatmo][netatmo], a cloud API polled, which keeps its OAuth token with
`store`.

## Cameras and Recordings

A camera is a Function of kind `camera`, which may have no Capability. Its Bridge implements
[`bridge.Cameras`][Cameras], which Oiko finds by a type assertion: a Picture, read every few minutes,
so it must never wake the camera, and the URL of a Live view, RTSP, which Oiko plays as H.264 and
AAC without transcoding ([ADR 0036](adr/0036-cameras-a-picture-and-a-live-view-relayed-by-oiko.md)).

When the camera's system keeps Recordings of its own, implement
[`bridge.Recordings`][Recordings] too: Oiko lists them and relays their video, keeping none
([ADR 0038](adr/0038-recordings-stay-with-the-cameras-system-oiko-lists-and-relays-them.md)).
Announce each new Recording with an Event of the camera's [`RecordingEvent`][RecordingEvent]
Capability, as of its Start
([ADR 0039](adr/0039-a-new-recording-is-an-event-and-a-notification-may-carry-it.md)): it can
start an Automation, and a Notification can carry its video.

Oiko logs the errors of these methods: never put a URL that grants the media in one. Test them
with [`Home.Picture`][Home.Picture], [`Home.Recordings`][Home.Recordings] and
[`Home.RecordingMedia`][Home.RecordingMedia].

The reference is [oiko-arlo][arlo], cameras with Recordings in Arlo's cloud.

## More you may need

- **Command-line commands.** A pairing or a sign-in the user runs once is a
  [`Module.Commands`][Module] entry, run as `oiko <bridge> <command> [args]` while Oiko is not
  serving, such as homekit's [`pair`](configure.md#homekit).
- **Reconnecting.** When the system goes away, call `SetOnline(false)`, retry with a growing delay,
  and `SetOnline(true)` once back. `Run` never returns before its context ends.
- **Never panic.** In `Run` or `Send`, a panic takes all of Oiko down: return an error, or log one.
- **Logging.** Log through `env.Log`, whose records carry the Bridge's name. Debug for what
  happens every poll, Warn for what a human should look at, and never a secret or a URL that grants
  access.
- **Several types in one module.** The root package registers each, and the Manifest lists each.

## Publish and keep up

### Publish

Your repository holds the module at its root:

```
go.mod             module github.com/you/oiko-plug, requiring the oldest Oiko it builds with
plug.go            the root package, registering every type of the module
plug_test.go
oiko-bridge.json   the Manifest
README.md
LICENSE
flake.nix          if you use Nix
```

The catalogue lists a type only when GitHub detects an OSI-approved license on its repository, and
shows that license: keep the license's text whole in `LICENSE`. Apache-2.0, Oiko's
([ADR 0047](adr/0047-oiko-and-its-types-of-bridge-take-apache-2-0.md)), is recommended.

The Manifest, `oiko-bridge.json`, tells the catalogue each type the root package registers, with a
description and an example of its section of the configuration
([ADR 0020](adr/0020-a-catalogue-of-types-of-bridge-indexed-from-a-manifest.md)). The plug's:

```json
{
  "types": {
    "plug": {
      "description": "Smart plugs on the local network through their HTTP API, polled every 10 seconds: whether each is on, its power draw, and whether it answers. Each can be switched on and off.",
      "config": {
        "plugs": {"Desk lamp": "http://192.0.2.10"}
      }
    }
  }
}
```

Its spec is in [the catalogue's README][manifest], next to the code that validates it.

The plug's [README](write-a-bridge/plug/README.md) is the shape a type's README takes: what it
connects to and follows; the vendor's setup, when there is one; Configure, with an example, every
key and "any other key stops Oiko from starting"; Add it to Oiko; Versions; Develop; License.

Then give the repository the GitHub topic `oiko-bridge` and tag a release, `v0.1.0` (pre-releases
are left out). The catalogue indexes every day: it reads the Manifest of your latest release,
builds that release with the latest Oiko, runs its `-version`, and shows whether it builds, with
the end of the build's output when it doesn't.

### How users add it

`oiko-build -with github.com/you/oiko-plug@v0.1.0`, or on NixOS an override of Oiko's package with
your module's version: [Add a type of Bridge](configure.md#add-a-type-of-bridge) holds both, and
your catalogue entry gives them with your latest version.

### Keep up with Oiko

Oiko's versions follow [its compatibility rules](upgrade.md#releases)
([ADR 0019](adr/0019-release-and-compatibility-policy.md)), and the Bridge contract is part of what
they protect:

- The Oiko your `go.mod` requires is the oldest your type builds with: during v0, your type builds
  with the releases of that minor from it on.
- During v0, a minor release of Oiko may break the contract, and its notes list each change; a
  patch neither breaks nor adds anything. Until you move to a new minor, the catalogue shows your
  type failing to build with it.
- Your versions are read the same way: during v0, a minor release of yours may break your
  configuration, a patch never.
- Your users' data is yours to migrate ([Keep data](#keep-data)). Your configuration refuses
  unknown keys, so a key you remove stops Oiko loudly rather than being ignored.

## What pkg.go.dev holds instead

The guide gives the why and the when; the godoc gives the contract, and stays the reference.

| Topic | Here | In the godoc |
|---|---|---|
| `Run`, `Send` and the `Port`: order, concurrency, what Oiko guarantees | one sentence each | [`Bridge`][Bridge], [`Port`][Port] |
| Capability fields, value types and their Go types | how to choose | [`Capability`][Capability], [`ValueType`][ValueType] |
| Cameras and Recordings | when to implement them, pitfalls | [`Cameras`][Cameras], [`Recordings`][Recordings] |
| Keeping data | why and when | [`bridge/store`][store] |
| Testing | the walk-through test | [`bridge/bridgetest`][bridgetest] |
| The keys that shape Tiles | [the Roles table](#describing-devices-so-oiko-shows-them-well) | none: the web client's |
| `oiko-build`'s flags | the commands you run | [`cmd/oiko-build`][oiko-build] |

[catalogue]: https://llehouerou.github.io/oiko-catalogue/
[manifest]: https://github.com/llehouerou/oiko-catalogue#oiko-bridgejson
[netatmo]: https://github.com/llehouerou/oiko-netatmo
[arlo]: https://github.com/llehouerou/oiko-arlo
[synctest]: https://pkg.go.dev/testing/synctest
[oiko-build]: https://pkg.go.dev/github.com/llehouerou/oiko/cmd/oiko-build
[bridge]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge
[Access]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Access
[Bridge]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Bridge
[Cameras]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Cameras
[Capability]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Capability
[Category]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Category
[Device]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Device
[Env]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Env
[Env.Decode]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Env.Decode
[Function]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Function
[Module]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Module
[Port]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Port
[RecordingEvent]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#RecordingEvent
[Recordings]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Recordings
[Register]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#Register
[ValueType]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge#ValueType
[bridgetest]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest
[bridgetest.Env]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest#Env
[Home.Command]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest#Home.Command
[Home.Picture]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest#Home.Picture
[Home.RecordingMedia]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest#Home.RecordingMedia
[Home.Recordings]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/bridgetest#Home.Recordings
[store]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/store
[store.Format]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/store#Format
[store.Load]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/store#Load
[store.Save]: https://pkg.go.dev/github.com/llehouerou/oiko/bridge/store#Save
