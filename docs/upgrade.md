# Upgrade

## Update notices

Oiko tells on the dashboard when newer releases of itself or of an added type of Bridge exist,
and never applies them. It asks the Go module proxy a few times a day, following Go's
environment: the first proxy of `GOPROXY` (`proxy.golang.org` by default), none with
`GOPROXY=off`; modules matching `GONOPROXY`, by default `GOPRIVATE`, are never asked about.

It also tells how to apply them for its install, which `OIKO_INSTALL` names: `nixos` (set by
the NixOS module), or unset for a plain binary; an unknown value stops Oiko at start.

## Plain binary

A plain binary upgrades itself: `./oiko upgrade` lists the newer releases of Oiko and of each
added type of Bridge that cannot break it, asks for confirmation (`-y` skips it), rebuilds
Oiko with the same types at those versions (Go needed), and replaces its executable, keeping the
previous one as `oiko.old`. Restart Oiko afterwards; to roll back, `mv oiko.old oiko`.

A release that may break it is only named: upgrade to it by hand, following its release notes.
`oiko upgrade` refuses with NixOS and Docker, where the dashboard tells what to change instead,
and for a development build.

## Releases

Releases follow semantic versioning as Go reads it (ADR 0019). Oiko's own data is migrated on
start by every release, and never broken. The `bridge` contract, the HTTP and WebSocket API,
what Code Steps see and the configuration may break in a minor release while Oiko is in v0, and
only in a major one from v1.0.0; a v0 patch neither breaks nor adds anything. A type of Bridge's
versions are read the same way.
