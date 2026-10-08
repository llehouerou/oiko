# Configure

Oiko's configuration is `data/config.json` (`-config` names another file; on NixOS,
`services.oiko.settings` writes it). The [Public URL](expose.md) is set there too.

## Bridges

Oiko learns about devices through Bridges, each configured by name under `bridges` in
`data/config.json`, and keeping its state in `data/<its name>/`. A key a Bridge does not
know stops Oiko from starting. Two types are built in (ADR 0021): zigbee2mqtt and homekit.
Others are [added](#add-a-type-of-bridge) from the catalogue.

A Bridge's type is its name, unless its section says `"type"`: two zigbee2mqtt instances
are `"zigbee2mqtt": {…}` and `"garage": {"type": "zigbee2mqtt", …}`. A Bridge's name is
recorded on its Devices: renaming it makes them new Devices.

## zigbee2mqtt

One [zigbee2mqtt](https://www.zigbee2mqtt.io) instance, through an existing MQTT broker.
Enable zigbee2mqtt's `availability` option, or every Device's Availability stays unknown.

```json
{"bridges": {"zigbee2mqtt": {"broker": "mqtt://user:password@broker:1883"}}}
```

`"baseTopic"` defaults to `zigbee2mqtt`. On NixOS, `services.oiko.mqtt` sets the broker.

## homekit

Oiko as a HomeKit controller for Wi-Fi accessories that speak HomeKit on the local network
(read-only for now: occupancy and light sensors, such as the Aqara FP2). Configure it as
`"homekit": {}`, pair an accessory while it waits to be paired (not paired with Apple Home),
then restart Oiko:

```sh
./oiko homekit pair 123-45-678     # its setup code; -id <device id> if several wait
```

On NixOS, pair it with
`sudo -u oiko oiko -data /var/lib/oiko -config /etc/oiko/config.json homekit pair <code>`,
then restart `oiko`.

The pairing, with Oiko's private key, is kept in `data/homekit/pairings.json`.

## Add a type of Bridge

Types written outside Oiko are listed in the
[catalogue](https://llehouerou.github.io/oiko-catalogue/), with what to paste to add each one.

`oiko-build` builds an Oiko with it, given the version of its module (Go 1.27 needed):

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@v0.3.0 -with example.com/oiko-hue@v1.2.0 -o oiko
```

It builds the Oiko of its own version, or `-oiko v0.3.1`.

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

## Location

Sun triggers need the home's location, written by hand in `data/config.json`:
`{"location": {"latitude": 48.86, "longitude": 2.35}}`. Times of day follow the host timezone.

## Notifications

Automations send Notifications to one Telegram chat through a bot:
`{"telegram": {"tokenFile": "…", "chatIdFile": "…"}}` in `data/config.json`.
