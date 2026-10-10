# A Presence is seen like any Target, with no privacy rule of its own

A Person's Presence (ADR 0048) and the Home presence (ADR 0051) follow the Access levels (ADR 0023) exactly as any Target does. Everyone who observes the home sees them as they are now: every Guest, Member and Admin, every Kiosk and every Program. Members and Admins read their History. That History is kept indefinitely like any Target's, until the Person is removed, which deletes it (ADR 0048). A Person sees their own Presence as anyone else sees it: a Guest reads no past, their own included. Nothing about presence goes into the Audit log. An override is a Command, with its Origin, in the History and among the Commands issued (ADR 0052). Binding Presence sources, Entrance doors or Signs of life is editing the home.

We chose this because presence reveals nothing a Guest does not already observe: the doors, the lights, the motion and a phone's `network` Function (ADR 0050) already show who comes and goes. The household's habits over time are what ADR 0023 created the Member level to guard, and the front door's contact already keeps them indefinitely. "Who is home" on a shared Dashboard or a wall Kiosk is one of presence's main uses. No peer gives presence a privacy model of its own either: it follows each system's general visibility.

## Considered Options

- **Members and up only**: Guests, Guest Kiosks and Guest Programs see no Presence. It would be the first exception by kind of Target to "a Guest observes the home as it is now", while the Guest still sees the sensors the Presence is derived from. A temporary guest could not see who is home, nor override a Presence (ADR 0052).
- **Each Person their own, Admins all**: the snapshot and the event stream filtered per Person, a first in Oiko. It kills a family "who is home" Tile, and Kiosks and Programs fit neither side.
- **A shorter retention for a Presence's History**: a retention by kind of Target that Oiko has nowhere else, while the sensors it is derived from keep the same past indefinitely. Automations comparing with past weeks would lose it.
- **A Person reads their own Presence's History whatever their level**: a per-Person exception in History reads, for a past a Guest has no use managing.
- **Overrides of someone else's Presence in the Audit log**: they would duplicate the Commands issued and widen the Audit log beyond access.

## Consequences

- Every Person's Name becomes visible to everyone who observes the home, through their Presence, although only Admins list Persons.
- Removing a Person is the only way to erase their Presence's History.
- Restricting who sees a Presence later removes something the API returns, so it waits for a breaking release (ADR 0019).
