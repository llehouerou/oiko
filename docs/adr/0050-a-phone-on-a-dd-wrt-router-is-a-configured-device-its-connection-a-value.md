# A phone on a DD-WRT router is a configured Device, its connection a Value

Presence (ADR 0048, ADR 0049) reads whether a phone is on the home's network through an external type of Bridge, `ddwrt`, in its own repository, `oiko-ddwrt` (ADR 0021: one router family is one product line). It polls the router's HTTP status page `Status_Wireless.live.asp` every 30 s with Basic Auth, as Home Assistant does: the page draws on the build's own driver, so one client covers every chipset. A phone counts as connected while it is in the page's `active_wireless` list, the radio's associations; the ARP table and the DHCP leases are not read.

Its Devices are the phones the configuration lists, and only those: `phones` maps a key of the Admin's choosing to the phone's `name` and its `mac` on this network. The key is the Device's Native Address, so a phone whose MAC changes (iOS 18's rotation, Android's non-persistent randomization) is re-bound by editing its `mac` and restarting: the same Device, its History and its Presence source bindings stay. Each phone has one Function of kind `network` with one Capability, `connected` (binary, observable, primary: a state), true while associated. The section also holds the router's `url` (http, or https with a certificate that verifies), a `username`, and a `passwordFile`, so that the password stays out of the configuration; the Bridge keeps nothing in its data directory. A command, `oiko <bridge> clients`, prints the clients the router sees now, with their MACs and lease host names, for the Admin to find a phone's MAC.

The Bridge is online while the status page answers, offline on a timeout, an error or a 401 (logged as a credentials error); each phone's Availability is online while the Bridge is. Its first good poll is its Replay: every configured phone, then a Report of `connected` for each, then Replayed. Every later poll reports every phone, a refresh when nothing changed. A home with several DD-WRT access points configures one Bridge for each: a phone is a Device on each, all bound as Presence sources, and the Presence rule's "any" covers roaming.

## Considered Options

- **SSH and the router's Linux tools**: closer to covering OpenWrt too, but the association tool differs by chipset (`wl`, `wl_atheros`, `iw`), and the router needs Oiko's SSH key.
- **One generic router type with a driver per family**: one configuration shape, but a plug-in layer inside a type of Bridge for families nobody uses yet; each family becomes its own type when one is wanted.
- **ARP or ping from Oiko's host**: any router, but an idle phone stops answering and flaps.
- **Every client the router sees as a Device**: no MAC to type, but every guest's phone, TV and rotated MAC would pile up as Devices, and the Bridge would have to remember each to keep listing it once gone.
- **The MAC as Native Address**, like an IEEE address: a new MAC would detach the Device, and keeping its History and bindings would need a Replace on top of the edit.
- **ARP or the leases as well as association**: catches wired devices, which presence does not need, and lags departures.
- **A signal strength Capability**: useful for coverage, but it changes on every poll, a History row each time.
- **A configurable poll interval**: nobody needs it yet; departures are smoothed by the Presence source's departure delay, not by the poll.
- **One Bridge polling several access points**: one Device per phone, but the Bridge's online state is ambiguous while one access point is down.

## Consequences

- Renaming a phone's key in the configuration detaches its Device; a Replace hands the new key its identity.
- A phone the router never saw still has a Device, not connected: tracking starts from the configuration, not from the network.
- The `connected` Value means "associated now", never "seen recently": the ~15 min departure delay a phone needs is set on its Presence source (ADR 0049).
- `network` is a kind Oiko does not treat apart: how its Tile and History show it is the web client's to decide.
- `clients` only reads the router, so it is harmless while Oiko serves, though `bridge.Module` describes Commands as run while it does not.
