# What a release promises: data migrated, contracts broken only where the version allows

Until Oiko is public it keeps no backward compatibility. It goes public (#85) at `v0.3.0` and stays in v0 while what others write against settles; releasing `v1.0.0` is a decision of its own, taken once it has. From going public on, a release follows semantic versioning as Go reads it, so that someone updating without reading release notes keeps a working home: during v0 a minor release may break what others write against and a patch neither breaks nor adds anything; from `v1.0.0` only a major release may break, and additions come in minor ones. A release that may break is called breaking below: a minor during v0, a major after.

Data Oiko owns (its JSON documents, ADR 0003, and `history.db`, ADR 0006) carries a format version per store. On starting, Oiko migrates each store forward to its own format, in any release, v0 included, without a human. Before migrating a store it keeps a copy named after the format it leaves (`devices.json.v3`, `history.db.v2` through `VACUUM INTO`). An Oiko finding a format newer than its own refuses to start and names the copy to restore, so a rollback, such as `nixos-rebuild switch --rollback`, loses only what changed since the update. A breaking release may drop the migrations older than the last release before it: Oiko then refuses to start and says which release to go through first.

What others write against breaks only in a breaking release:

- The `bridge` contract (ADR 0017): its Go API, `bridge/bridgetest` included, and the kinds of Function and keys of Capability that shape Tiles (ADR 0014). Adding a method to an interface a type implements (`Bridge`, `Module`) breaks it; adding one to `Port` does not. `scripts/release` runs `gorelease` against the previous release before tagging and refuses a version the changes to the Go API do not allow; `gorelease` judges nothing during v0, so the script itself refuses a v0 patch whose API changed at all.
- The HTTP and WebSocket API, for programs other than the web client, which ships in the same executable and never lags.
- What a Code Step's Starlark sees: `run(trigger, state)`, the shape of `trigger` and `state`, and the functions Oiko provides. A Step's params are data, migrated.
- The configuration, which a human or Nix writes and Oiko cannot migrate. A removed or renamed key fails Oiko's start, as an unknown key already does, in a Bridge's section too when its type decodes it with `Env.Decode`, as the built-in types do; the breaking release's notes list each one. No alias keeps an old key working.

These share Oiko's version: they live in its module. A major release from `v2` on changes its module path (`github.com/llehouerou/oiko/v2`), so every type of Bridge must follow. The version of Oiko a type of Bridge's `go.mod` requires is therefore enough to tell which Oiko it builds with (ADR 0020): during v0, the releases of that minor.

A type of Bridge owns its data directory and its format. The built-in types follow the policy for Oiko's data; it is recommended to other types, not enforced, and the contract provides nothing for it. Oiko reads a type's versions as its own: during v0, a type's minor release may break its configuration.

Nothing is ever applied without a human, neither Oiko nor a type of Bridge: a type runs with Oiko's full trust, so a compromised release of it is a compromised home, and a hub restarting on its own is a risk of its own. Oiko tells that newer versions exist (#79); a human applies them. `oiko upgrade` (#84) takes Oiko and each type to their newest release that cannot break them, the newest patch of their minor during v0 and the newest release of their major after, and only points to a breaking one and its release notes.

Only the latest release gets fixes; nothing is backported.

## Considered Options

- **`v1.0.0` on going public**: what this ADR first said; a public promise on contracts still moving, each change to them costing a major.
- **No promise during v0, data included**: plain semantic versioning, the most freedom, but a public user updating could lose their home's configuration.
- **Breaking data formats only in a breaking release, with an upgrade guide**: no migration code, but the human must act, and few read a guide before bumping a flake input.
- **Data formats frozen between breaking releases, and migrated**: safest, but every feature touching a store waits for one.
- **Reversible migrations**: a transparent rollback, at twice the migration code, with inverses rarely exercised.
- **A contract version of its own** (`bridge.ContractVersion`): breaks the contract without a breaking release of Oiko, but two versions to manage, against ADR 0017's single module.
- **Deprecated configuration keys accepted for a while**: gentler, but the very compatibility layer this policy avoids elsewhere.
- **Patches applied without a human**, Oiko's alone or the types' too: needs a scheduled, unattended path and an automatic rollback, and exposes the home to the supply chain of code running with full trust.

## Consequences

- Each store has a format version before going public, and `history.db` a `user_version`.
- A migration keeps a copy of its store once, the size of `history.db` included.
- Upgrading across several breaking releases goes through the last release before each.
- The README states this policy when Oiko goes public.
