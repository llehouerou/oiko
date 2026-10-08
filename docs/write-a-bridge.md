# Write a type of Bridge

A type of Bridge (a plugin) connects Oiko to a system it doesn't know: Oiko is built with it,
and a Bridge of that type is then [configured](configure.md#bridges) like any other.

## The package

A type of Bridge is a Go package of its own implementing `bridge.Bridge`, which registers itself
from `init` (package [`bridge`](../bridge/bridge.go), ADR 0017; `oiko-netatmo` is a small
example), keeps its data with
[`bridge/store`](../bridge/store/store.go), and is tested against Oiko's own rules with
[`bridgetest`](../bridge/bridgetest/bridgetest.go). A type with cameras also implements
`bridge.Cameras`, for their Picture and Live view (ADR 0036), and `bridge.Recordings` when their
system keeps Recordings (ADR 0038).

## Build Oiko with it

`oiko-build` builds an Oiko with a type given the version of its module
([Add a type of Bridge](configure.md#add-a-type-of-bridge)). While developing, give directories
instead: `-oiko ../oiko -with example.com/oiko-hue=../oiko-hue` (a checkout's web client is
there once `make build` has run in it).

## Publish

To list a type in the [catalogue](https://github.com/llehouerou/oiko-catalogue), give its
repository the topic `oiko-bridge` and its module an `oiko-bridge.json` manifest, specified in
the catalogue's README (ADR 0020).
