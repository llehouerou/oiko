# Contributing

## Develop

```sh
direnv allow                       # or `nix develop`: Go 1.27, Node 24, Mosquitto, make
make dev                           # API on :8080 + UI with hot reload on http://localhost:5180
make test
make build                         # single ./oiko binary with the UI embedded
./oiko
go test -run=^$ -bench=. ./internal/zigbee2mqtt   # hot-path latency
```

## Nix hashes

After changing `web/package-lock.json` or `go.sum`, update `npmDepsHash` or `vendorHash` in
`nix/package.nix` (`nix build` prints the new one).

## Release

```sh
scripts/release v0.3.1             # tags a release carrying the built web client (ADR 0018)
```

Releases follow [the compatibility rules](docs/upgrade.md#releases) (ADR 0019).
