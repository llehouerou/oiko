# Configuration stored as JSON documents

Oiko keeps what it alone owns (Device identity and Names, and later Areas and automations) as JSON documents in a data directory, one file per concern, each rewritten atomically (temporary file, fsync, rename) on every change. This data is small (tens to hundreds of records), changes rarely and is always loaded whole at startup, so a database adds a dependency and a schema for no benefit, while plain files stay readable and trivially backed up. Home Assistant stores its registries the same way (`.storage/`).

Device Values are not stored: zigbee2mqtt replays them (`retain`, `last_seen`). A Flag's Value is the exception: no Bridge replays it, so `flags.json` keeps it with its time next to the Flag's definition. It is rewritten on every change, which happens a few times a day. Value history, when it comes, is a different workload (frequent appends, range queries) and will get its own storage.

Automations' runtime state (Step state and the runaway mark) is the other exception: `automation-state.json`, kept apart from `automations.json`. It can change every second, so its writes are coalesced to at most one per second after a change, off the engine's hot path, and flushed on shutdown; a crash loses at most about a second of it.

## Considered Options

- **SQLite**: transactions and queries, one store for everything including future history, but a driver dependency and a schema to maintain for a handful of rarely written records.
