# Presence configuration lives in Home, saved whole per Presence and sent in a presences list

What presence needs Oiko to keep, it keeps in one new Home document, `presences.json`, format 1, through `bridge/store`, as Flags are kept in `flags.json` (ADR 0003): one entry per Person, by id, holding their Presence sources with each one's departure delay, the override (its forced Value and `manual`) and the Presence's last Value with its At and Since; and one entry `home` holding the Entrance doors, the Signs of life, the exit grace and the Home presence's last Value. It is rewritten on every change, a few times a day. An Admin, Person or Program, saves one entry's bindings whole with `PUT /api/presences/{id}`, `{id}` being a Person's id or `home`, as in the key `presence:<id>`: `{"sources": [{"target": "device:…/network", "departureDelay": 900}]}` for a Person, `{"entranceDoors": [...], "signsOfLife": [...], "exitGrace": 300}` for the home, durations in seconds as a Command's `transition` (a delay 0 and a grace 300 when absent). A binding names a Target only: Oiko reads its binary state or main-control Capability (ADR 0049). The snapshot of `GET /api/updates` carries a `presences` list, each entry with its id, its Name (its Person's, or "Home"), kind `presence`, its Capabilities `home` and `manual`, and its bindings, as an Aggregate carries its members; a `presences` Update resends the whole list whenever a Person is created, renamed or removed or a binding is saved or loses a deleted Target.

We chose this because a Presence is a Target, and Home owns Targets: it alone can check a binding and drop a deleted Target from it, as it does for an Aggregate's members, and keeping the last Values with the bindings, as `flags.json` does, lets a Presence keep its Value across a restart while its sources are still silent.

## Considered Options

- **Bindings in `persons.json`**: stored next to the Person, but the access store does not know Targets, so it cannot check them, and deleting a Device would rewrite an identity document (the objection ADR 0045 makes for Dashboards). The Home presence has no Person to hang on.
- **Bindings and state in two documents**, as `automation-state.json` is apart from `automations.json`: that split is for state written every second; an override or a last Value changes a few times a day.
- **Recalling a Presence's last Value from the History**: one less thing saved, but it holds only while `history.db` does, which an operator may drop to reclaim space.
- **Fields on `PUT /api/persons/{id}`, or `PUT /api/persons/{id}/presence` and `PUT /api/home-presence`**: the request goes through the access module, which does not keep the data, and the Home presence needs a path of its own.
- **Presences in the `flags` list**: no new Update kind, but a client cannot tell them apart, and a Flag's Area and rename do not apply.
- **Bindings read with a `GET /api/presences` for Admins only**: two paths to the same data, refused for Dashboards by ADR 0046; Guests already see an Aggregate's members.
- **A binding naming its Capability**, as a value trigger does: explicit for a Function with two binary Capabilities, but ADR 0049 fixes the Capability read, and the Admin makes one more choice.
- **Durations as strings (`"15m"`)**: easier to read in the file, but no other field of the API takes them.

## Consequences

- A binding saved is refused when a Target does not exist, is not a Function, has no binary state or main-control Capability, or is a Presence or the Home presence; when a delay or the grace is negative; or when Signs of life are bound with no Entrance door (ADR 0051). A Target listed twice is kept once. A Target deleted later is unbound at once and the list resent; a Detached Device or a vanished Function stays bound (ADR 0049).
- The access store gives Home the Persons at start and tells it of each Person created, renamed or removed. An entry left for a Person gone, after a crash between two document writes, is dropped on load (ADR 0032, ADR 0045). A Person with no entry has a Presence with no binding and no Value.
- There is no `POST` or `DELETE`: Oiko derives every Presence (ADR 0048, ADR 0051).
- Guests receive the `presences` Update and list (ADR 0053).
- Starlark, the History API and the Command history take `presence:<id>` and `presence:home` as any Target key; the History records `home` and `manual` as any Capability.
- Everything here is added, nothing changed: presence ships in a minor release (ADR 0019). A later rule's per-source parameters (ADR 0049) are new optional fields beside `departureDelay`.
