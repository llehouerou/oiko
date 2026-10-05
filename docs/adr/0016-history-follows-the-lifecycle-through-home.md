# The History follows a Replace or a Delete through Home's Updates

Home announces every Delete and Replace it applies as an Update, like any other change: `TargetDeleted` (a Device, an Aggregate or a Flag; once per Area Aggregate when an Area is deleted, whether that Aggregate exists now or not) and `DeviceReplaced` (the kept Device, and the Device whose hardware it took). It emits them once the change is applied in memory, before saving, so a save that fails afterwards still has the History follow what Home now holds.

The History receives them through `Home.Follow`, like Values. Its hand-off is a queue under a mutex instead of a channel: past its bound it still drops a point, a Trace or a Command (counted lost, a lost point lands in a Gap), but it always appends a Delete or a Replace. So `Follow` never waits under Home's lock, nothing lifecycle is ever dropped, and each lands after every point handed over before it. The API no longer touches the History on a Delete or a Replace.

This replaces the Lifecycle hand-off of ADR 0011, where the API handler queued the change after Home accepted it and waited for room. That put the decision in every handler, read from Home's error modes, and a Delete was already lost there once.

## Considered Options

- **Keep the hand-off in the API**: nothing new in Home, but each new handler that deletes must remember it, and must tell an applied change from a refused one by its error.
- **A second, unbounded channel for lifecycle changes**: never drops either, but loses the order between a Delete and the points queued before it, which the writer would then have to restore.
- **Have `Follow` block for lifecycle Updates**: one channel kept, but a full buffer would stall Home's lock and every observer behind it.

## Consequences

- The queue is unbounded only for lifecycle changes, which are rare and occupant-driven.
- Every observer of Home's Updates, the web included, sees `deleted` and `replaced`; those not interested ignore them.
