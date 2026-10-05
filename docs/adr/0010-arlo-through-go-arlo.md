# Arlo through go-arlo, Oiko's first cloud Bridge

Oiko follows the Arlo cameras through a type of Bridge of their own built on `github.com/llehouerou/go-arlo`, a client of Arlo's cloud API ported from pyaarlo, rather than through Home Assistant's aarlo integration. The type lives outside Oiko, in `github.com/llehouerou/oiko-arlo`, and is added to a build like any other (ADR 0017): few homes have Arlo cameras, and it is a real type exercising the `bridge` contract from outside. Arlo has no local API, so this Bridge is the first to depend on a vendor's cloud: while the internet or Arlo is down, the Bridge is offline and its Devices' Availability unknown, and a Command to them is refused.

Oiko logs in as a dedicated account the owner of the cameras granted access to, never as the owner. Arlo rate limits auth attempts with a long cooldown and rotates its trusted-browser cookie on every login, so the session lives in one file in the data directory that only one Oiko instance ever uses: it is never copied, and a second instance pairs its own at the cost of one email two-factor, read over IMAP. go-arlo alone decides when to log in again; Oiko never retries around it.

The location's alarm mode is not a Device in Arlo. It is carried by the base station, which reports the mode it applies: a `mode` Capability of an `arming` Function, whose options are `standby`, `armHome` and `armAway`. An owner's custom mode is reported as `custom` and shown, but cannot be commanded. Cameras are motion sensors: an `occupancy` Function, like the Zigbee ones, and a diagnostic `battery` Capability.

## Considered Options

- **Home Assistant's aarlo integration**: works today, but Oiko would depend on a second home automation platform, the one it replaces, to relay three devices.
- **A Device for the location**: matches Arlo's model, but a Device is physical hardware; this one would have no Native Address of its own and no Availability to report.
- **Built into Oiko**: what it first was; every build carries a client few homes use, and no type outside Oiko exercises the contract.
