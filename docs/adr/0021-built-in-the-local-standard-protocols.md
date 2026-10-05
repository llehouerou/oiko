# Built in: the local, standard protocols most homes meet

A type of Bridge is built into Oiko when it speaks a local, standard protocol most installations meet: zigbee2mqtt for the Zigbee radio network, around which Oiko's core is shaped (`commandTimeout` sits above zigbee2mqtt's delivery timeout, `maxTransition` is the Zigbee transition ceiling, the send spacing exists for the radio network), and homekit for the Wi-Fi accessories that speak HomeKit on the local network. A type reaching a vendor's cloud, or one product line, is external, in a repository of its own, added through `oiko-build` or the flake's `bridges` (ADR 0017) and listed in the catalogue (ADR 0020), as Arlo already is (ADR 0010).

netatmo, a vendor cloud for one product line, therefore leaves Oiko before v1.0.0: after it, removing a built-in type and its configuration is a breaking release (ADR 0019). It keeps its token in a versioned store, so Oiko's store became part of the public contract first, as `bridge/store`. It keeps its name, so its data directory, `<data>/netatmo`, stays where it is.

## Considered Options

- **Only zigbee2mqtt built in**: the smallest core, the one Oiko's timings are tuned for. But HomeKit is as much a local standard as Zigbee, and a stock binary should follow both without a build; its HAP, mDNS and SRP dependencies are the price.
- **Every type external, zigbee2mqtt included**: the purest core, but the stock binary and Docker image would control nothing, and the code Oiko's timings are tuned for would be tested outside the repository that tunes them.
- **Keep netatmo built in**: no move, no work; Oiko's repository keeps owning a vendor's API and OAuth for the installations that have its stations, and each of its fixes rides Oiko's release cadence.

## Consequences

- A type unused by the configuration never cost anything at runtime: what moves out is code, dependencies and release work, not load.
- An installation with only Zigbee and HomeKit devices runs the released binary as is; any other type means a build (`oiko-build`, the flake's `bridges`, or the catalogue's Dockerfile).
- A new protocol becomes built in only if it is local, standard, and met by most installations, Matter or Thread for instance; amending this ADR says so.
