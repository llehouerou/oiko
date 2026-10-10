# The Home presence is any Presence, or someone seen inside since an Entrance door closed

Oiko derives one Home presence for the home: whether someone is home, Persons or not. It is a Target like a Presence (ADR 0048): key `presence:home`, kind `presence`, one binary Capability `home` with the state Role. It always exists, is always online, has no Area, no override and the fixed Name "Home", and it belongs to no Aggregate of Presences. It is home while any Person's Presence is home or while the box holds someone, and away otherwise. Presences with no Value are ignored, and so are offline or silent doors and Signs of life. It has no Value until a Presence or a Sign of life first tells. The box follows the "Wasp in a Box" pattern over two lists an Admin binds, each holding any binary Function except a Presence or the Home presence. In the Entrance doors list, true means open. In the Signs of life list, true means someone is there: an occupancy sensor, an Area's occupancy Aggregate, a "Guests" Flag. A Sign of life turning true puts someone in the box at once. An Entrance door closing at T empties it at T plus the exit grace (5 min unless set), unless a Sign of life turned true after T or is still true at that moment. Nothing else empties it. Signs of life with no Entrance door bound are refused. After a restart the box is recalled as the Home presence's last Value.

We chose this because not everyone in the home is a Person: a guest, a cleaner or a child without a phone must keep the home occupied. Doors and motion together tell this where neither does alone. A guest asleep for hours stays home, and a Person leaving empties the box, since no motion follows their door closing.

## Considered Options

- **Just "any Person home"** (every peer's default): a guest alone at home leaves it empty, so the heating drops and the alarm arms around them.
- **An Aggregate of Presences plus a Flag kept by Automations**: nothing new in the model, but every home rebuilds the inference by hand, out of sight (the objection ADR 0048 makes to a Flag-based Presence).
- **The Presence rule over occupancy sources** (any source home, each with a departure delay, ADR 0049): reuses everything, but a sleeping guest goes away after the delay, doors cannot take part, and a leaver's motion keeps the home occupied for the whole delay.
- **Emptying the box when a door opens**: simpler, but fetching the mail makes the home empty for a moment and fires every "home empty" Automation.
- **A long-quiet limit** (empty after N hours with no Sign of life): it would cover a pet or an unbound door, but it sends a long sleeper away. A stuck box ends the next time anyone goes through an Entrance door. The limit can be added later as a parameter without changing the rule.
- **Picking the Persons who count**: an Aggregate of Presences already says "any of these Persons"; someone home means any Person.
- **Rebuilding the box from the History after a restart**: more exact after a long downtime, but no other rule depends on History queries. Recalling the last Value errs toward occupied, and events missed while Oiko was down are lost either way.
- **An own override**: binding a Flag as a Sign of life already says "we have guests". Forcing "away" is not offered.

## Consequences

- Home, the API and the web client learn `presence:home` beside each Person's `presence:<id>`. Person ids are UUIDs, so the key cannot collide.
- The Home presence's Value may change with no Sign of life or door reporting, when an exit grace runs out.
- A pet's motion or an unbound door can keep the home occupied until someone next goes through an Entrance door. The Admin's choice of Signs of life is the defence.
- Who sees the Home presence and its History is decided together with the Persons' Presences.
