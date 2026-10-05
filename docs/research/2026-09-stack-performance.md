# Research: performance- and responsiveness-oriented stack (September 2026)

Goal #1 of the project: maximum responsiveness, backend and frontend.
Every claim below links to its primary source.

## Go backend

- **Go 1.27** was released on 2026-08-19 (1.27.1 on 2026-09-01).
  [release history](https://go.dev/doc/devel/release)
- `encoding/json` is now backed by the v2 implementation; `encoding/json/v2` and
  `encoding/json/jsontext` are GA. "Marshal performance is broadly at parity […] while
  unmarshal performance is significantly faster." v2 rejects invalid UTF-8 and duplicate
  names. [Go 1.27 notes](https://go.dev/doc/go1.27)
  → No third-party JSON library needed to parse the zigbee2mqtt stream.
- Allocations under 80 bytes up to 30% cheaper; `goroutineleak` profile GA; new `uuid`
  package; `httptest.NewTestServer` + `testing/synctest` for in-memory network tests.
  [Go 1.27 notes](https://go.dev/doc/go1.27)
- **Green Tea** GC enabled by default since Go 1.26. [Go 1.26 notes](https://go.dev/doc/go1.26)
- HTTP/3: not in `net/http` as of 1.27. [Go 1.27 notes](https://go.dev/doc/go1.27)

## MQTT

- **eclipse/paho.golang** (`autopaho`): official Eclipse MQTT v5 client, handles
  reconnection and resubscription. [repo](https://github.com/eclipse-paho/paho.golang)
- **mochi-mqtt/server v2**: embeddable MQTT v5/3.1.1 broker in Go, MIT license.
  Would replace Mosquitto and deliver messages in-process (no TCP hop).
  [repo](https://github.com/mochi-mqtt/server)

## zigbee2mqtt (hardware source of truth)

- `zigbee2mqtt/bridge/devices` (retained): device list with `ieee_address`,
  `friendly_name`, `power_source`, `interview_state`, `endpoints`, `definition.exposes`.
  `bridge/groups` (retained): groups, members, scenes. `bridge/event`: join / leave /
  interview. Requests via `bridge/request/+` → `bridge/response/+`.
  [MQTT topics](https://www.zigbee2mqtt.io/guide/usage/mqtt_topics_and_messages.html)
- **Exposes**: generic types `binary`, `numeric`, `enum`, `text`, `composite`, `list`;
  specific types `light`, `switch`, `fan`, `cover`, `lock`, `climate` that group generics
  in `features`, with an optional `endpoint` (e.g. a double switch).
  `access` = bitmask: 1 published in state, 2 `/set`, 4 `/get`.
  `category` = `config` | `diagnostic` | absent (regular use).
  [Exposes](https://www.zigbee2mqtt.io/guide/usage/exposes.html)
- 2.0: `bridge/state` is always JSON `{"state":"online"}`; "action sensors" (buttons) are
  now meant as **triggers**, not states; `illuminance_lux` → `illuminance`; some devices
  changed their `action` values.
  [2.0 breaking changes](https://github.com/Koenkk/zigbee2mqtt/discussions/24198)
  → Exposed values can change between z2m versions: the model must read them from
  `exposes`, never hard-code them.

## Lessons from the Home Assistant model

- Opaque `device_id`s in automations break when a device is replaced; the community
  recommends `entity_id`s, which then have to be renamed by hand to "transfer" identity.
  Since ~2026.6 `entity_id`s are prefixed with the area, so moving a device also breaks
  references.
  [HA community cookbook](https://community.home-assistant.io/t/why-and-how-to-avoid-device-ids-in-automations-and-scripts/605517/1)
- `unavailable` / `unknown` are mixed into state values (strings); buttons have no state,
  so HA stores a timestamp as the "state". (same source)
- Sub-devices: a double switch driving two rooms forced HA to add per-entity areas, then
  sub-devices.
  [architecture #1036](https://github.com/home-assistant/architecture/discussions/1036)

## Frontend

- **Datastar 1.0.x**: SSE-first hypermedia. The backend "patches" the DOM (morph by
  default) and signals; no npm dependency; official Go SDK
  (`github.com/starfederation/datastar-go/datastar`, Go ≥ 1.24).
  Recommends CQRS: one long-lived SSE for reads, short POSTs for writes; "fat morph";
  no optimistic UI, indicators instead.
  [Getting started](https://data-star.dev/guide/getting_started),
  [Tao of Datastar](https://data-star.dev/guide/the_tao_of_datastar)
- **Brotli over SSE**: a compressed stream shares its context; a 32 KB → 263 KB window
  raised the ratio from 30:1 to 150–250:1 and cut server CPU by 4–8× in an extreme case.
  zstd is unavailable on Safari/iOS.
  [Anders Murphy](https://andersmurphy.com/2025/04/15/why-you-should-use-brotli-sse.html)
- **htmx 4.0.0** released on 2026-08-28: internal `fetch()`, built-in morph
  (`innerMorph`/`outerMorph`), `<hx-partial>`, rewritten `hx-sse` extension, `hx-live`
  for local reactivity. Stays tagged `next` on npm until early 2027.
  [announcement](https://four.htmx.org/announcements/2026-08-28-htmx-4.0.0-is-released)
- SPA alternatives: Solid 2.0 (RC), React Compiler 1.0 stable. They require a client-side
  model duplicated in TS and a build chain.
  [Solid 2.0 RC](https://www.solidjs.com/blog/solid-2-0-rc-the-big-reveal),
  [React Compiler 1.0](https://react.dev/blog/2025/10/07/react-compiler-1)
- **templ**: typed, compiled HTML rendering in Go; release v0.3.1020 (2026-05).
  [release](https://github.com/a-h/templ/releases/tag/v0.3.1020)
- **Tailwind v4**: Oxide engine, standalone CLI (no Node required).
  [blog](https://tailwindcss.com/blog/tailwindcss-v4)

### Datastar risks (follow-up research)

- 1.0.0 only published on 2026-04-16; API breakages throughout the RC phase.
  [release](https://github.com/starfederation/datastar/releases/tag/v1.0.0),
  [considerations](https://github.com/alvarolm/datastar-resources/blob/main/docs/considerations.md)
- Features that were free in beta (`replace-url`, `scroll-into-view`, `persist`…) moved to
  Pro (commercial license, forbidden in an open-source project).
  [account](https://drshapeless.com/blog/posts/htmx,-datastar,-greedy-developer.html),
  [Pro](https://data-star.dev/pro)
- Expressions evaluated via `Function()` → CSP `unsafe-eval` required, manual escaping.
  [considerations](https://github.com/alvarolm/datastar-resources/blob/main/docs/considerations.md)
- Third-party JS libraries integrate through web components (attributes in, events out)
  and `data-ignore`. [example](https://data-star.dev/examples/web_component)
- HTTP/1.1 limit of 6 connections per origin across all tabs: each SSE holds one
  (applies to any SSE client, not only Datastar). HTTP/2 lifts the limit.
  (same source)
- Graph editor: xyflow is maintained for React (`@xyflow/react` 12.11) and Svelte
  (`@xyflow/svelte` 1.6). [releases](https://github.com/xyflow/xyflow/releases)

## Storage

- Go SQLite driver benchmarks (2026-03, go1.26): cgo wrappers and `mattn` are ~2× faster
  at inserts than `modernc` (pure Go) and `ncruces` (WASM); on large reads `ncruces` ≈ cgo,
  `modernc` ~2× slower.
  [go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench)
  → Irrelevant for a registry of a few hundred rows; revisit for history (frequent writes).
