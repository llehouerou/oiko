# A Presence is a Target Oiko derives for each Person, computed by a rule from a table

Every Person has a Presence: whether they are home or away, a binary Function Oiko derives from the Person, with no Device or Bridge, as an Area Aggregate is derived from its Area (ADR 0013). Its key is `presence:<person-id>`, its kind `presence`, its one Capability `home` (true when home), with the state Role. No Value means unknown, as everywhere: a Person with nothing bound, or of whom nothing has been told since they were created, has none. It is always online, has no Area, and shows under its Person's Name. Oiko computes its Value from the sources bound to the Person by a rule taken from a table of rules compiled into Oiko, like an Aggregate's; a by-hand override comes on top. v1 ships one plain rule, set by a few declarative parameters. A later rule, statistical or learned, is a new entry in the table, with parameters of its own per source, picked per Person. Nothing about weights or statistics is designed now.

We chose this because presence must be as easy to describe as an Aggregate in the plain case, yet open to a computation far richer than any or all later, without changing what Automations, Tiles and the History see.

## Considered Options

- **An Aggregate with a new rule**: no new kind of Target, but an Aggregate's members share one kind, and a Person's sources mix kinds (a Flag, a contact, an occupancy sensor, a phone seen by a router). A departure delay and an override do not fit an Aggregate's rule either.
- **A Flag kept by an Automation**: nothing new in the model, but the rule lives in a graph an Admin builds by hand for each Person, the override fights the Automation, the sources are not bound to the Person in any visible way, and a statistical rule is out of reach.
- **Pointing at any binary Function**: a Presence would be whichever Function an Admin designates, and complex logic would live elsewhere. The most general option, but the plain case then takes two objects per Person.
- **A Starlark rule**: arbitrary logic without a release, but statistics over the History are awkward in Starlark, and every Presence becomes code to review.
- **Availability derived from the sources**, as an Aggregate's is: it mixes the sources' reachability into the Person's state and needs an exception for the override. The rule decides what an offline or silent source means, and a statistical rule has to own that anyway.
- **An explicit unknown Value, or Values open to zones**: both clash with "no Value means unknown" and lose the binary triggers, conditions and any/all rules. Zones need geofencing, which is out of scope.

## Consequences

- Target gets a fourth kind beside Device, Aggregate and Flag: Home, the API, Starlark, the History and the web client learn `presence:`. Adding presence is a minor release (ADR 0019).
- Persons live in the access store, Targets in Home. Home holds one Presence per Person it is told of: it learns of each Person created or removed, as the Dashboards learn of a removed Person (ADR 0045).
- Removing a Person deletes their Presence and its History, like any deleted Target (ADR 0016): Automations that refer to it become broken, and Tiles of it go.
- A Presence may be a member of an Aggregate of Presences ("any child home"), as Flags may be members of an Aggregate of Flags.
- A rule that also estimates a probability may add a second, numeric Capability beside `home`, without changing what reads `home`.
- The Step that fakes occupancy while the home is empty stays the Presence simulation, never "Presence" alone.
