# Install

Oiko is one binary with its web client embedded. Run its prebuilt Docker image, build it with
Go, build it in Docker from the catalogue's Dockerfile, or deploy it with the NixOS module. Then
[configure](configure.md) it, give it a [Public URL](expose.md), and [sign in](sign-in.md).

## Plain binary

`oiko-build` builds an Oiko (Go 1.27 needed):

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -o oiko
./oiko
```

Then open http://localhost:8080. To build it with added types of Bridge, see
[Add a type of Bridge](configure.md#add-a-type-of-bridge). A plain binary
[upgrades itself](upgrade.md#plain-binary).

## Docker

Each release publishes `ghcr.io/llehouerou/oiko`, for `linux/amd64` and `linux/arm64`, with the
built-in types of Bridge, zigbee2mqtt and homekit. It is tagged with the release's version
without its `v`, `0.12.0`, and its minor, `0.12`, which follows the releases that cannot break
it; there is no `latest`. A `compose.yaml`:

```yaml
services:
  oiko:
    image: ghcr.io/llehouerou/oiko:0.12.0
    network_mode: host
    user: "1000:1000"  # your id -u:id -g
    volumes:
      - ./data:/data
      - /etc/localtime:/etc/localtime:ro
    restart: unless-stopped
```

`mkdir data`, write its `config.json` ([Configure](configure.md)), then `docker compose up -d`,
and open http://localhost:8080. Host networking lets HomeKit find accessories over mDNS. `data`
holds Oiko's own configuration and secrets: the user running the container must own it. The
dashboard tells how to [upgrade](upgrade.md#docker).

Other types of Bridge need a build: the [catalogue site](https://llehouerou.github.io/oiko-catalogue/)'s
Docker tab gives a tested `Dockerfile` and `compose.yaml` building an Oiko with the types you
pick. It runs the same way, with the same `/data`, so a home moves from the image to it keeping
its data.

## Flags

`-listen` (`:8080` by default), `-data` (directory holding Oiko's own configuration, `./data`
by default, created `0700` if missing; [back it up](backup.md)), `-config`
(`<data>/config.json` by default), `-version` (what this Oiko is built from: its version and
each type of Bridge, with its module and version; the dashboard shows it too, under Oiko's
logo).

## Commands

`./oiko [flags] <bridge> <command> [args]` runs a command of a Bridge instead, such as
[`homekit pair`](configure.md#homekit). `upgrade` ([Upgrade](upgrade.md#plain-binary)) and
`sign-in-link` ([Sign in](sign-in.md#host-recovery)) are reserved for Oiko's own commands, never
Bridge names.

## NixOS

The flake exports the package and a NixOS module, `nixosModules.default` (`services.oiko`):

- `listen`: the HTTP listen address, `:8080` by default.
- `publicUrl`: the [Public URL](expose.md).
- `mqtt` (required): the broker of the zigbee2mqtt Bridge.
- `settings`: config.json ([Configure](configure.md)); a homekit Bridge is always configured.
- `credentials`: files holding secrets, which the service reads as
  `/run/credentials/oiko.service/<name>`, where `settings` point to them.
- `package`: the Oiko to run, overridden to
  [add types of Bridge](configure.md#add-a-type-of-bridge).

State lives in `/var/lib/oiko`, mode `0700`.
