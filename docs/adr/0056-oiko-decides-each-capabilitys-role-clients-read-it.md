# Oiko decides each Capability's Role, and clients read it

ADR 0014 put the rule that tells a Capability's Role in the web client. Since ADR 0049, Oiko needs part of it too: a Presence source is read through its main control or its state, so Home kept its own copy of the main-control and state keys, with a test holding both to the guide's Roles table. Two copies of one rule disagree as soon as one of them learns a key or a kind and the other does not. Since this ADR, Oiko alone decides every Role (`internal/home/roles.go`) and announces them: each Function, each Device for its own Capabilities, each Aggregate, Flag and Presence carries `roles`, the Role of each Capability by key (`control`, `adjustment`, `state`, `reading`, `event`, `setting`, `health`, `diagnostic`). A Capability with no Role, such as a light's `color_xy`, is absent. The web client sorts Capabilities by what it is told and decides nothing.

We chose this because Oiko already needs the Roles, and the rule lives best with what needs it first. A Program then reads the same Roles as the web client, rather than redoing the rule.

## Considered Options

- **Keep two copies, held together by a shared test**: no API change, but every new key or kind is still written twice, in two languages.
- **A `role` field on each Capability**: easier to read, but a Capability is the `bridge` contract's type (ADR 0017). A Bridge would see a field it must not set, or Home would need a type of its own wrapping it, along with every place that builds one.
- **Roles saved with the Devices**: derived data in `devices.json` goes stale as soon as a release changes the rule, so it would be recomputed on every load anyway.
- **Oiko also deciding what a Function's chart shows and the reading order**: those are the web client's choices of how to show a Function, and Oiko needs neither.

## Consequences

- `roles` is computed each time Oiko announces a Device, Aggregate, Flag or Presence, and is never saved: no store changes format.
- The field is added to the HTTP and WebSocket API, so it ships in a minor release (ADR 0019). Removing it, or changing what a Role means, is breaking.
- The keys that shape Tiles stay part of the `bridge` contract (ADR 0019). `roles_test.go` holds the guide's Roles table to Oiko's lists; the web client's tests check only the reading order, which stays its own.
- A rule change reaches every client at once, and a client reading `roles` never needs updating for a new key or kind.
