# A Presence is set by hand by a Command that holds until its rule agrees

A Presence's `home` Capability is settable: anyone who may issue a Command (a Guest, a Member, an Admin, a Kiosk, a Program, a Run of an Automation) sets a Person's Presence by hand with a Command on it, as on any Target. The Value it sets is an override: it holds while the Presence's rule (ADR 0049) computes another Value or none, and ends the moment the rule computes the same one, after which the Presence follows its rule again. Nothing else ends it: no duration, no limit. A Command setting the Value the rule already computes clears the override. While one holds, a second binary Capability, `manual`, observable and never settable, is true. The override is remembered across restarts, as a Flag's Value is.

We chose this because the two cases it is for are each closed by the rule itself: Alice forces "away" when her phone stays home, and the override ends when the phone leaves with her or when she presses "home" on her return; she forces "home" when her phone is dead, and it ends once the phone reconnects. Being a plain Command, the override gets its Origin and its place in the History (ADR 0031), and reaches Automations, Programs and Aggregates of Presences with nothing new.

## Considered Options

- **An override setting with its own API**: the Value would stay read-only, but the override would have no Origin and no History, and no Automation could set it without a new Step.
- **A Flag per Person bound as a Presence source**: nothing new in the model, but a source only ever counts toward home, so it cannot force "away" when the phone is left at home.
- **Until cleared by hand**: easy to predict, but easy to forget, and the Presence then silently stops following its sources, the complaint about openHAB's Items.
- **A duration chosen with each override**: a defined end every time, at the cost of a parameter no other Command has and a picker in the web client.
- **Until any Presence source changes**: ends sooner, but a flapping source would cancel it at random, even one that leaves the rule's Value unchanged.
- **A fixed cap (24 h)**: bounds a forgotten "home" that keeps "when the last one leaves" from firing, but a Person with no Presence source, set only by hand, would lose their Value every day. It can be added later without breaking anything.
- **Only the Person and Admins may set it**: more private, but it is a permission on one kind of Target that Oiko has nowhere else, and it shuts out Kiosks, Programs and Automations. Who may see a Presence bounds who may set it anyway.
- **The rule's own Value as a read-only Capability instead of `manual`**: richer ("away by hand, the sources say home"), but it exposes the rule's internals, which a later statistical rule would have to fit into the same shape.
- **Overrides cleared on restart**: simpler, but a Presence set only by hand would lose its Value with every restart.

## Consequences

- `home` is no longer only a state, as ADR 0048 had it: as a settable binary Capability it is the Presence's main control, so its Tile shows a control.
- A Person with no Presence source, a child whose Presence an Admin keeps, is set only by hand: the rule never speaks, so the override never ends.
- A Command on a Presence is confirmed at once, as one on a Flag. Toggle works as on any binary Capability, and a Command on an Aggregate of Presences ("all children home") is relayed to each member.
- An Admin rebinding a Person's Presence sources ends the override if the rule then agrees with it.
- Oiko remembers the forced Value and whether it holds: the rule alone no longer gives the Presence's Value after a restart.
- The Home presence still has no override (ADR 0051); a Person's override reaches it through their Presence.
