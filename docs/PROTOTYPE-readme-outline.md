# PROTOTYPE — README outline and docs split (throwaway, wayfinder ticket #86)

Rough outline to react to, not final text. Inputs: the pitch (#83), the peers'
patterns (#81: README as a short visual lobby linking out), Apache-2.0 (#85),
SECURITY (#84).

## 1. The new README (the lobby)

```
# Oiko                                     [logo? → fog: Visuals]

Self-hosted home automation in one small binary: your devices, automations
and dashboards, local and in real time.

> Status: v0, built and maintained by one person. Minor releases may break the
> configuration, the API or the Bridge contract; Oiko's own data is always
> migrated (→ docs/upgrade.md#releases).

[screenshot of a Dashboard, made-up data → fog: Visuals]

## What it does
- **Devices** — zigbee2mqtt and HomeKit built in; any other system through a
  type of Bridge (a plugin) from the [catalogue], or [one you write].
- **Automations** — a visual editor, Starlark Code Steps, Telegram Notifications.
- **Dashboards for the household** — personal and shared, wall tablets as Kiosks.
- **Safe on the internet** — Passkeys and never a password, three Access levels,
  an Audit log.
(History, Cameras, Programs and the rest: → docs/)

## Quickstart
Plain binary (Go 1.27):
    go run github.com/llehouerou/oiko/cmd/oiko-build@latest -o oiko
    ./oiko
Open http://localhost:8080, claim it with the Setup link printed in the log.
NixOS: 6-line flake snippet (inputs.oiko + services.oiko.enable + publicUrl).
Reaching it from anywhere but this machine needs a Public URL → docs/expose.md.

## How it compares
4 sentences from #83 (one binary, no add-on runtime/YAML/DB server; HA and
openHAB for breadth; Homebridge feeds Apple Home, Oiko is the controller;
vendor apps are cloud silos).

## Documentation
Install · Configure · Expose · Sign in · Back up · Upgrade ·
Write a type of Bridge · Glossary · Decisions (ADRs)

## Help, contributing, security, license
Questions → (channel: #89) · Bugs/features → Issues · CONTRIBUTING.md ·
SECURITY.md · Apache-2.0 (LICENSE, NOTICE)
```

Roughly 80 lines, against 200 today; every fact moves to a page below.

## 2. The docs it splits into

| File | Holds | Reader |
|---|---|---|
| `docs/install.md` | plain binary via `oiko-build`, flags, Bridge commands, NixOS module, `OIKO_INSTALL` | self-hoster |
| `docs/configure.md` | `config.json`, Bridges (name, type, rename), zigbee2mqtt, homekit, adding a type from the catalogue (`oiko-build -with`, Nix override), location, Telegram | self-hoster |
| `docs/expose.md` | Public URL, TLS proxy, Caddy, DNS-01, LAN-only, no rate limit | self-hoster |
| `docs/sign-in.md` | Sessions, Tokens, Setup link, invites, Kiosks, host recovery `sign-in-link` | self-hoster / Admin |
| `docs/backup.md` | what to copy, restore, Gap, Audit log retention | self-hoster |
| `docs/upgrade.md` | update notices, `GOPROXY`, `oiko upgrade`, rollback, releases & compatibility | self-hoster |
| `docs/write-a-bridge.md` | the guide (#88) | Bridge author |
| `CONTRIBUTING.md` | develop, test, bench, release, Nix hashes (#89) | core contributor |

## 3. Every fact of today's README, and where it goes

| Today (README line) | Fact | New home |
|---|---|---|
| 3 | tagline | README, replaced by #83's description |
| 4 | GLOSSARY, ADR links | README › Documentation |
| 6–8 | Bridges under `bridges`, state in `data/<name>/`, unknown key stops start, two built in | configure › Bridges |
| 10–18 | zigbee2mqtt: broker, `availability`, `baseTopic` | configure › zigbee2mqtt |
| 20–29 | homekit: read-only, pairing, `pairings.json` | configure › homekit |
| 31–33 | Arlo and Netatmo as examples | dropped from README (#83); the catalogue lists them |
| 35–37 | name vs `type`, renaming makes new Devices | configure › Bridges |
| 39–40 | Telegram | configure › Notifications |
| 44–48 | `direnv` / `make build` / `./oiko` | CONTRIBUTING; README quickstart uses `oiko-build` |
| 50 | flags `-listen -data -config -version`, `0700`, back it up | install › Flags |
| 51–52 | `oiko <bridge> <command>`, reserved names | install › Commands |
| 54 | sign-in needs a Public URL | README quickstart → expose |
| 56–57 | location, host timezone | configure › Location |
| 59–65 | update notices, `GOPROXY`/`GONOPROXY`, `OIKO_INSTALL` | upgrade › Update notices (`docker` value: see Q3) |
| 67–73 | `oiko upgrade`, `oiko.old`, refusals | upgrade › Plain binary |
| 75–79 | semver, migrations, what may break | upgrade › Releases; status line links it |
| 83–89 | NixOS module options, `/var/lib/oiko`, homekit pairing on NixOS | install › NixOS (pairing: configure › homekit) |
| 90–91 | `npmDepsHash` / `vendorHash` | CONTRIBUTING |
| 93–108 | backup, restore, Bridges' own state, Audit log | backup |
| 112–135 | Public URL, proxy, streams, rate limits, Caddy, DNS-01 | expose |
| 139–150 | Sessions, Tokens, Setup link, Kiosk QR, `sign-in-link` | sign-in |
| 154–160 | `make dev/test/build`, bench, `scripts/release` | CONTRIBUTING |
| 164–166 | catalogue | configure › Add a type of Bridge |
| 168–173 | a type is a Go package, `bridge`, `store`, `bridgetest`, Cameras, Recordings | write-a-bridge |
| 174–185 | `oiko-build -with`, `-oiko`, local directories | configure (users); local dirs → write-a-bridge |
| 187–196 | Nix override with `bridges` and `vendorHash` | configure › Add a type of Bridge |
| 198–200 | listing: topic + `oiko-bridge.json` | write-a-bridge › Publish |

## 4. Settled with the user

- Quickstart builds with Go: `oiko-build@latest -o oiko`; no prebuilt binaries
  (the same command takes `-with` later, and `oiko upgrade` needs Go anyway).
- User docs are flat `docs/*.md`, next to `adr/`, `agents/`, `research/`.
- `OIKO_INSTALL=docker` is not documented until a Dockerfile exists.
- One page per topic: the 7 files above.
- Quickstart comes before "How it compares".
