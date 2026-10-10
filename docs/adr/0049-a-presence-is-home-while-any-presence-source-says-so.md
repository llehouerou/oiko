# A Presence is home while any Presence source says so, each with its own departure delay

The v1 rule of the Presence table (ADR 0048) reads the Presence sources an Admin binds to a Person: any binary Function, through its state or main-control Capability, except a Presence; true always means home, with no polarity setting. The Presence is home while at least one source counts as home, away once none does. A source counts as home while its Value is true, and keeps counting until its own departure delay (zero unless set) has passed since that Value's Since, so an arrival is immediate and a phone dozing off the Wi-Fi is not a departure. A source with no Value, or whose Device is offline, is ignored; when no source counts, the Presence keeps its last Value, and has none until a source first speaks. Sources follow the rules of an Aggregate's members: a Detached Device or a vanished Function stays bound but is ignored, a deleted Target is unbound.

We chose this because it depends only on the sources' current Values and the clock: the same answer after a restart, with no timer to keep, since Since is already recalled from the History.

## Considered Options

- **Latest change wins** (Home Assistant's person): follows whichever source changed last, so a phone dropping off the Wi-Fi sends Alice away while a Flag still says she is home, and the answer depends on the order of events rather than on the state.
- **One departure delay per Presence**: simpler to set, but a geofence Flag's explicit departure would wait as long as a dozing phone.
- **The delay in the router type of Bridge**: the Presence rule would have no timing, but the phone's Function would then mean "seen recently", not "connected", for everything that reads it.
- **An offline source counted as away**: a router reboot would send everyone away and fire every "when the last one leaves".
- **A stale Value counted whatever the Availability**: a router that is down would keep everyone home forever.
- **A polarity per source**: its only use is a source an Admin can name the other way round ("Alice home", not "Alice away").

## Consequences

- A type of Bridge reporting whether a phone is connected must report it as a Value, with the phone's Device online while the Bridge reaches the router, never through Availability: an offline source is ignored, not away.
- A Presence's Value may change with no source reporting, when a departure delay runs out.
- Per-source parameters are where a later statistical rule puts its own (a reliability, a prior), beside or instead of the departure delay.
