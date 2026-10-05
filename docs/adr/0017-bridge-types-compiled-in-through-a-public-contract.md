# Types of Bridge compiled in, through a public contract

Anyone can add a type of Bridge to Oiko without changing it: a type is a Go package that registers a `bridge.Module` from `init`, and a build of Oiko is a main package that imports such packages for their side effect and calls `oiko.Main`. The four built-in types (zigbee2mqtt, homekit, arlo, netatmo) register the same way and are always compiled in. The module is `github.com/llehouerou/oiko`, so that builds and types outside the repository import it; Oiko's own packages stay internal.

The contract is the `bridge` package, which depends on the standard library only: a `Bridge` runs and sends Commands, and feeds Oiko through a `Port` (`SyncDevices`, `SetOnline`, `SetAvailability`, `Report`, `Replayed`), the methods Home already exposed to its Bridges. It also holds the terms a Bridge describes its Devices in. `Capability`, its types and categories, and `Availability` are Home's own, through aliases. A described `Device` and `Function` carry only what a Bridge knows: identity, Name, Icon and Areas stay Oiko's.

The configuration names each Bridge under `bridges`; a Bridge's type is its section's `type`, or else its name, so several Bridges may share a type. Each gets its section as it is and a data directory of its own, `<data>/<name>`. A type may add commands, run as `oiko <bridge> <command>` while Oiko is not serving: pairing a HomeKit accessory is `oiko homekit pair <code>`.

## Considered Options

- **Go's `plugin` package**: loads a type without rebuilding, but the plugin and Oiko must be built with the same Go version and the very same version of every package they share, cgo, on Linux or macOS only, and nothing ever unloads. Ecosystems built on it break with each release.
- **Bridges as separate processes** (HashiCorp's go-plugin over gRPC, or JSON over stdio or a WebSocket): added without rebuilding, isolated, written in any language, Python's device libraries included. Costs a versioned protocol and processes to supervise. It remains possible as one more type of Bridge relaying the same Port, without changing this contract.
- **WebAssembly** (wazero, Extism): sandboxed and loaded at runtime, but a Bridge needs long-lived sockets, mDNS, UDP, TLS and goroutines, which WASI barely offers: the host would have to expose a network stack.
- **The contract as a Go module of its own**: types would not depend on Oiko's version. Like Caddy, one module instead: a type importing `bridge` compiles nothing else of Oiko, and a contract change is an Oiko release anyway.

## Consequences

- Adding a type means rebuilding Oiko: `oiko-build`, on a release that carries its web client (ADR 0018). The flake's package builds one by override: `bridges` lists the packages to import with their modules' versions, and `go get` adds them to go.mod while the modules are fetched, then again offline while building.
- A type runs in Oiko's process with its full trust: a panic stops Oiko.
- A type that names its Functions' kinds and Capabilities' keys as the built-in ones do gets their Tiles (ADR 0014). Those names belong to the contract as much as the Go types.
- A Bridge's name is recorded on its Devices: renaming it in the configuration makes them new Devices.
- `Register` records the package calling it, so that Oiko tells each type's module and version from the build information Go records in its executable: its Build. A type need not declare where it comes from, and cannot misstate it.
- State kept by the built-in Bridges moved to their data directories: `homekit/pairings.json`, `homekit/accessories.json`, `arlo/session.json`, `netatmo/token.json`.
