# Oiko

Local home automation platform: observe and control the devices of a home in real time.
Vocabulary: [CONTEXT.md](CONTEXT.md). Decisions: [docs/adr](docs/adr).

Oiko learns about devices through Bridges, each configured by name under `bridges` in
`data/config.json`, and keeping its state in `data/<its name>/`. A key a Bridge does not
know stops Oiko from starting. Two types are built in (ADR 0021).

**zigbee2mqtt**: one [zigbee2mqtt](https://www.zigbee2mqtt.io) instance, through an
existing MQTT broker. Enable zigbee2mqtt's `availability` option, or every Device's
Availability stays unknown.

```json
{"bridges": {"zigbee2mqtt": {"broker": "mqtt://user:password@broker:1883"}}}
```

`"baseTopic"` defaults to `zigbee2mqtt`.

**homekit**: Oiko as a HomeKit controller for Wi-Fi accessories that speak HomeKit on the
local network (read-only for now: occupancy and light sensors, such as the Aqara FP2).
Configure it as `"homekit": {}`, pair an accessory while it waits to be paired (not paired
with Apple Home), then restart Oiko:

```sh
./oiko homekit pair 123-45-678     # its setup code; -id <device id> if several wait
```

The pairing, with Oiko's private key, is kept in `data/homekit/pairings.json`.

Other systems are followed by types of Bridge added to Oiko like any other (see below), such
as `github.com/llehouerou/oiko-arlo` for Arlo cameras (ADR 0010) and
`github.com/llehouerou/oiko-netatmo` for Netatmo weather stations.

A Bridge's type is its name, unless its section says `"type"`: two zigbee2mqtt instances
are `"zigbee2mqtt": {…}` and `"garage": {"type": "zigbee2mqtt", …}`. A Bridge's name is
recorded on its Devices: renaming it makes them new Devices.

Automations send Notifications to one Telegram chat through a bot:
`{"telegram": {"tokenFile": "…", "chatIdFile": "…"}}` in `data/config.json`.

## Run

```sh
direnv allow                       # or `nix develop`: Go 1.27, Node 24, Mosquitto, make
make build
./oiko
```

Then open http://localhost:8080. Flags: `-listen`, `-data` (directory holding Oiko's own configuration, `./data` by default, created `0700` if missing; back it up), `-config` (`<data>/config.json` by default), `-version` (what this Oiko is built from: its version and each type of Bridge, with its module and version; the dashboard shows it too, under Oiko's logo).
`./oiko [flags] <bridge> <command> [args]` runs a command of a Bridge instead, such as `homekit pair`;
`upgrade` (below) and `sign-in-link` are reserved for Oiko's own commands, never Bridge names.

Anywhere but on this machine, sign-in needs a Public URL: see [Expose](#expose).

Sun triggers need the home's location, written by hand in `data/config.json`:
`{"location": {"latitude": 48.86, "longitude": 2.35}}`. Times of day follow the host timezone.

Oiko tells on the dashboard when newer releases of itself or of an added type of Bridge exist,
and never applies them. It asks the Go module proxy a few times a day, following Go's
environment: the first proxy of `GOPROXY` (`proxy.golang.org` by default), none with
`GOPROXY=off`; modules matching `GONOPROXY`, by default `GOPRIVATE`, are never asked about.
It also tells how to apply them for its install, which `OIKO_INSTALL` names: `nixos` (set by
the NixOS module), `docker` (set by the image's Dockerfile), or unset for a plain binary;
any other value stops Oiko at start.

A plain binary upgrades itself: `./oiko upgrade` lists the newer releases of Oiko and of each
added type of Bridge that cannot break it, asks for confirmation (`-y` skips it), rebuilds
Oiko with the same types at those versions (Go needed), and replaces its executable, keeping the
previous one as `oiko.old`. Restart Oiko afterwards; to roll back, `mv oiko.old oiko`. A release
that may break it is only named: upgrade to it by hand, following its release notes. It refuses
with NixOS and Docker, where the dashboard tells what to change instead, and for a development
build.

Releases follow semantic versioning as Go reads it (ADR 0019). Oiko's own data is migrated on
start by every release, and never broken. The `bridge` contract, the HTTP and WebSocket API,
what Code Steps see and the configuration may break in a minor release while Oiko is in v0, and
only in a major one from v1.0.0; a v0 patch neither breaks nor adds anything. A type of Bridge's
versions are read the same way.

## Deploy

The flake exports the package and a NixOS module, `nixosModules.default`
(`services.oiko`: `listen`, `publicUrl`, `mqtt` for the zigbee2mqtt Bridge, `settings` for config.json, and
`credentials`, files holding secrets that the service reads as
`/run/credentials/oiko.service/<name>`, where `settings` point to them; a homekit Bridge
is always configured). State lives in `/var/lib/oiko`, mode `0700`; pair an accessory with
`sudo -u oiko oiko -data /var/lib/oiko -config /etc/oiko/config.json homekit pair <code>`,
then restart `oiko`.
After changing `web/package-lock.json` or `go.sum`, update `npmDepsHash` or
`vendorHash` in `nix/package.nix` (`nix build` prints the new one).

**Backup.** Oiko produces no backup: the host owns its schedule, destination and retention.
1. Copy `history.db` with `sqlite3 /var/lib/oiko/history.db "VACUUM INTO '/backup/history.db'"`
   (not `.backup`), and leave the live `history.db*` files (db, `-wal`, `-shm`) out.
2. Copy the JSON files of `/var/lib/oiko` and its subdirectories as they are, among them the
   Persons, Kiosks, Programs and Sessions: `persons.json`, `kiosks.json`, `programs.json` and
   `sessions.json`.
3. To restore: stop `oiko`, put the copy back as `history.db` with no `-wal`/`-shm`, then start it.
   The restored `alive` mark makes Oiko record a Gap from the backup time (up to a minute early) to the restart.
   Older JSON files bring back the Sessions and Tokens they hold, and the Persons, Kiosks and
   Programs: sign out, remove or revoke again what should stay gone.
4. A type of Bridge may keep state that must not move to another host, such as the Arlo
   session of `github.com/llehouerou/oiko-arlo`: its documentation says so.

The Audit log lives in `history.db`: dropping that file drops the trail. The Names of removed
Persons, Kiosks and Programs stay in it for up to a year.

## Expose

Every Oiko needs a Public URL to sign anyone in, even one only ever used at home: an HTTPS origin
with no path, where users reach it, as `{"publicUrl": "https://oiko.example.org"}` in
`data/config.json` or `services.oiko.publicUrl` on NixOS; anything else there stops Oiko from
starting, with the reason. It takes a domain name and a certificate, from
Let's Encrypt through a DNS-01 challenge, which needs no open port, or a Tailscale `ts.net` name;
for LAN-only use, split-horizon DNS points the name at the host from inside the home. Without a
Public URL, Oiko starts and runs the home, but only http://localhost offers sign-in, and anywhere
else only a Program's Token is served.

Oiko serves plain HTTP (`-listen`, `:8080` by default): put a reverse proxy that terminates TLS in
front of it. TLS is all the proxy has to add, since Oiko trusts no forwarded header, but it must
not buffer responses under `/api`: the dashboard follows the home through an event stream,
`/api/updates`. Floods are the proxy's job too: Oiko limits no request rate (ADR 0034). A Program
on the LAN may still call Oiko on its listen address, but a Token sent over plain HTTP can be read
by anyone on the network. With [Caddy](https://caddyserver.com), which gets the certificate and
passes the event stream on as it comes:

```caddyfile
oiko.example.org {
	reverse_proxy localhost:8080
}
```

For DNS-01, Caddy needs your DNS provider's module and a `tls { dns … }` block.

Every API request but signing in needs a Session, signed in on the dashboard, or a Program's Token, sent as
`Authorization: Bearer`; the dashboard's page itself is served to anyone. While Oiko has no Admin,
its log prints at each start a Setup link that claims it (`journalctl -u oiko` on NixOS). The Admin
then invites the household, a Sign-in link for each Person, and creates a Program and its Token for
each other client. A shared screen, such as a wall tablet, signs in as a Kiosk: its sign-in page
offers a QR code, which an Admin scans from a signed-in phone and approves.

The host is the last way back in, for an Admin who lost every Passkey: while Oiko runs,
`oiko -data <dir> sign-in-link` lists the Persons (Name, Access level, id), and
`oiko -data <dir> sign-in-link <id>` prints a Sign-in link for one, valid 15 minutes, whatever
their Access level. It talks to Oiko through `<dir>/sign-in-link.sock`, open only to Oiko's user;
on NixOS, `sudo -u oiko oiko -data /var/lib/oiko sign-in-link`.

## Develop

```sh
make dev                           # API on :8080 + UI with hot reload on http://localhost:5180
make test
make build                         # single ./oiko binary with the UI embedded
go test -run=^$ -bench=. ./internal/zigbee2mqtt   # hot-path latency
scripts/release v0.3.1             # tags a release carrying the built web client (ADR 0018)
```

### Add a type of Bridge

Types written outside Oiko, such as `github.com/llehouerou/oiko-arlo` for Arlo cameras, are
listed in the [catalogue](https://llehouerou.github.io/oiko-catalogue/), with what to paste
to add each one.

A type of Bridge is a Go package of its own implementing `bridge.Bridge`, which registers
itself from `init` (package [`bridge`](bridge/bridge.go), ADR 0017; `oiko-netatmo` is a small
example), keeps its data with [`bridge/store`](bridge/store/store.go), and is tested against
Oiko's own rules with [`bridgetest`](bridge/bridgetest/bridgetest.go).
`oiko-build` builds an Oiko with it, given the version of its module (Go
1.27 needed):

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@v0.3.0 -with example.com/oiko-hue@v1.2.0 -o oiko
```

It builds the Oiko of its own version, or `-oiko v0.3.1`. While developing, give
directories instead: `-oiko ../oiko -with example.com/oiko-hue=../oiko-hue` (a checkout's
web client is there once `make build` has run in it).

Then configure it like any Bridge: `{"bridges": {"hue": {…}}}`.

With Nix, override the flake's package instead, each package with its module's version;
the first `nix build` fails with the `vendorHash` to set:

```nix
services.oiko.package = oiko.packages.${system}.default.override {
  bridges."example.com/oiko-hue" = "v0.1.0";
  vendorHash = "sha256-…";
};
services.oiko.settings.bridges.hue = { … };
```

To list a type in the [catalogue](https://github.com/llehouerou/oiko-catalogue), give its
repository the topic `oiko-bridge` and its module an `oiko-bridge.json` manifest, specified
in the catalogue's README (ADR 0020).
