# Built in: only the type of Bridge Oiko's core is shaped around

A type of Bridge is built into Oiko only when Oiko's core is shaped around it and most installations need it. That is zigbee2mqtt alone: `commandTimeout` sits above zigbee2mqtt's delivery timeout, `maxTransition` is the Zigbee transition ceiling, and the send spacing exists for the radio network; its tests and benchmarks exercise Home's hot path in this repository. Every other type is external, in a repository of its own, added through `oiko-build` or the flake's `bridges` (ADR 0017) and listed in the catalogue (ADR 0020), as Arlo already is (ADR 0010).

netatmo and homekit therefore leave Oiko before v1.0.0: after it, removing a built-in type, its configuration and its NixOS defaults is a breaking release (ADR 0019). Both keep their data in versioned stores (`token.json`, `pairings.json`, `accessories.json`) through `internal/store`, which an external type cannot import: the store becomes part of the public contract first. A type that moves keeps its name, so its data directory, `<data>/<name>`, stays where it is.

## Considered Options

- **Every type external, zigbee2mqtt included**: the purest core, but the stock binary and Docker image would control nothing, and the code Oiko's timings are tuned for would be tested outside the repository that tunes them.
- **Keep homekit built in**: local and standard, it would let a stock binary follow Wi-Fi accessories. But it is read-only, used for one kind of sensor so far, and brings HAP, mDNS and SRP into every build.
- **Keep the current built-ins**: no move, no work; Oiko's repository keeps owning protocols and dependencies few installations use, and each rides Oiko's release cadence.

## Consequences

- A type unused by the configuration never cost anything at runtime: what moves out is code, dependencies and release work, not load.
- While the contract still changes in minor releases, each change means a release of every external type before an installation using it can upgrade. The moves come after the contract's open questions are settled, so that they are paid once.
- An installation with only zigbee2mqtt runs the released binary as is; any other type means a build (`oiko-build`, the flake's `bridges`, or the catalogue's Dockerfile).
