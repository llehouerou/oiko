# oiko-plug

Smart plugs on the local network for [Oiko](https://github.com/llehouerou/oiko): the `plug`
type of Bridge, added to an Oiko build like any other. It polls each plug's HTTP API every
10 seconds, and switches a plug through the same API.

This plug is made up: it is the first type of Oiko's guide,
[Write a type of Bridge](https://github.com/llehouerou/oiko/blob/main/docs/write-a-bridge.md),
and this README is the shape a type's README takes.

What it follows, for each plug:

- whether it is on, a `switch` Function that can be switched on and off;
- its power draw, in W;
- whether it answers (its Availability). A plug that doesn't answer reports nothing until
  it is back.

## Configure

The Bridge's section of Oiko's
[configuration](https://github.com/llehouerou/oiko/blob/main/docs/configure.md#bridges),
under `bridges`:

```json
{"bridges": {"plug": {
  "plugs": {"Desk lamp": "http://192.0.2.10", "Kettle": "http://192.0.2.11"}
}}}
```

- `plugs`: each plug's name and the base URL of its API, at least one. The name is the
  Device's address in Oiko: renaming a plug makes it a new Device.

The plugs need no secret. A type that does takes it as a `…File` key naming a file that
holds it, so the secret stays out of the configuration.

Any other key stops Oiko from starting.

## Add it to Oiko

Build an Oiko with this type (Go 1.27 needed):

```sh
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -with example.com/oiko-plug@latest -o oiko
./oiko -version   # lists the plug type, with its module and version
```

On NixOS, override Oiko's package with this module's version:

```nix
services.oiko.package = oiko.packages.${system}.default.override {
  bridges."example.com/oiko-plug" = "<version>"; # its latest tag
  vendorHash = "sha256-…"; # the first nix build prints it
};
services.oiko.settings.bridges.plug = {
  plugs."Desk lamp" = "http://192.0.2.10";
};
```

Its entry in the [catalogue](https://llehouerou.github.io/oiko-catalogue/#plug) gives both,
with the latest versions (a made-up plug has no entry: the link shows where a real type's is).

## Versions

Versions follow Oiko's
([ADR 0019](https://github.com/llehouerou/oiko/blob/main/docs/adr/0019-release-and-compatibility-policy.md)):
during v0, a patch release neither breaks nor adds anything, and a minor release may break
the configuration, its notes listing what changed. Each version's `go.mod` names the
minimum Oiko it needs.

## Develop

```sh
go test ./...
go run github.com/llehouerou/oiko/cmd/oiko-build@latest -with example.com/oiko-plug=. -o oiko
```

The tests run against a fake plug in memory, with no network and no real clock.

## License

Apache-2.0. This sample is part of Oiko, under
[its license](https://github.com/llehouerou/oiko/blob/main/LICENSE); a type's own repository
holds its `LICENSE` file.
