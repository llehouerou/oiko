# SQLite for Traces and Command history

_Amended by [ADR 0011](0011-history-one-row-per-change.md): Commands are kept indefinitely, and the History joins this database._

Traces of Automation Runs and the history of Commands live in an SQLite database in the data directory, accessed through a pure-Go driver (`modernc.org/sqlite`, no cgo). This is the first workload that appends continuously and is queried by range (per Automation, per Function, by time), which ADR 0003 set apart from configuration. Configuration stays in JSON documents. Keeping Traces in memory would break the budget of 20 KiB of heap per Automation from [Performance budget for automations](https://github.com/llehouerou/oiko/issues/13), lose the Traces from before a crash, and let a chatty Automation wipe its own history. A single writer goroutine owns the database and writes in batches. The hot path (Home for Commands, the engine for Traces) hands it entries through a bounded buffer and never waits on the disk. Entries older than 30 days are purged. Value history, when it comes, joins the same database.

## Considered Options

- **In-memory rings per Automation** (Home Assistant keeps 5 Traces by default): no dependency, but volatile, heap-bound, and a chatty Automation overwrites its own history within minutes.
- **JSON Lines files per day**: no dependency, but every query per Function or per Automation reads whole files, which amounts to an ad-hoc index.
- **mattn/go-sqlite3**: faster, but cgo complicates the build and cross-compilation for a volume of a few hundred rows a day.
