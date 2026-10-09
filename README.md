# Oiko

Self-hosted home automation in one small binary: your devices, automations and dashboards,
local and in real time.

> Status: v0, built and maintained by one person. Minor releases may break the configuration,
> the API or the Bridge contract; Oiko's own data is always migrated
> ([releases](docs/upgrade.md#releases), ADR 0019).

![Oiko's built-in Dashboard, dark, one Section per Area: Living room, Kitchen, Bedroom and Office. Each Section's header shows its temperature, humidity, motion or door state and its lights; below are Tiles for lamps, plugs, climate sensors, motion sensors and door and window contacts with their current readings.](docs/images/dashboard.png)

## What it does

- **Devices**: zigbee2mqtt and HomeKit built in; any other system through a type of Bridge
  (a plugin) from the [catalogue](https://llehouerou.github.io/oiko-catalogue/), or
  [one you write](docs/write-a-bridge.md).
- **Automations**: a visual editor, Starlark Code Steps, Telegram Notifications.
- **Dashboards for the household**: personal and shared ones, wall tablets as Kiosks.
- **Safe on the internet**: Passkeys and never a password, three Access levels, an Audit log.

History, Cameras, Programs and the rest are defined in the [Glossary](GLOSSARY.md).

## Quickstart

A plain binary, built with Go 1.27:

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -o oiko
./oiko
```

Open http://localhost:8080 and claim it with the Setup link printed in the log.

On NixOS, add the flake's module to a system:

```nix
inputs.oiko.url = "github:llehouerou/oiko";

# among the system's modules:
inputs.oiko.nixosModules.default
{
  services.oiko.enable = true;
  services.oiko.mqtt = "mqtt://localhost:1883";    # the broker zigbee2mqtt publishes to
  services.oiko.publicUrl = "https://oiko.example.org";
}
```

Reaching Oiko from anywhere but this machine needs a Public URL: see [Expose](docs/expose.md).
[Install](docs/install.md) has the rest: flags, commands and the NixOS module's options.

## How it compares

Oiko is one binary with its types of Bridge compiled in: no add-on runtime, no YAML, no database
server. Home Assistant and openHAB support far more devices: choose them for breadth. Homebridge
bridges devices into Apple Home; Oiko is the controller itself. Vendor apps are cloud silos, one
per brand.

## Documentation

[Install](docs/install.md) · [Configure](docs/configure.md) · [Expose](docs/expose.md) ·
[Sign in](docs/sign-in.md) · [Back up](docs/backup.md) · [Upgrade](docs/upgrade.md) ·
[Write a type of Bridge](docs/write-a-bridge.md) · [Glossary](GLOSSARY.md) ·
[Decisions (ADRs)](docs/adr)

## Help, contributing, security, license

- Bugs: [Issues](https://github.com/llehouerou/oiko/issues). Questions and ideas:
  [Discussions](https://github.com/llehouerou/oiko/discussions).
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md) and the [code of conduct](CODE_OF_CONDUCT.md).
- License: Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).
