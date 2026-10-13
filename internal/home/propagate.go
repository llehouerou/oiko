package home

import "time"

// How what Oiko derives follows what it derives from: the Aggregates, the
// Presences, the Home presence's box and the Home presence. Every change
// reaches them through followValues or followAll, which keep their order: an
// Aggregate may be a Presence source, so the Aggregates follow first; a
// Presence may be an Aggregate's member, so its own Values come back through
// followValues; neither a Presence nor an Aggregate of them is ever a
// Presence source or bound to the Home presence (checkBinding), and the Home
// presence feeds nothing, so the chain ends there.

// recorded is a Value just recorded, and whether it was announced as
// replayed state: its first, or one recorded as replayed.
type recorded struct {
	ref     Ref
	initial bool
}

// followValues brings in line what the Values rs, just recorded at at, feed.
// A change moved only by replayed Values is replayed state, as theirs are
// (ADR 0054). Callers hold h.mu.
func (h *Home) followValues(rs []recorded, at time.Time) {
	h.reportToAggregates(rs, at)
	h.reportToPresences(rs)
	h.reportToBox(rs)
	initial := true
	for _, r := range rs {
		initial = initial && r.initial
	}
	h.followHome(initial)
}

// followAll brings everything derived in line with the home as it is now,
// after what it derives from changed shape: Availability, members, bindings,
// Devices or Persons. A change is replayed state if initial, or until the
// home is known, that is while its Bridges replay: a Bridge may report its
// retained state before it says it is online. Callers hold h.mu.
func (h *Home) followAll(initial bool) {
	h.syncAggregateValues()
	h.syncAggregateAvailability()
	h.checkKnown()
	select {
	case <-h.known:
	default:
		initial = true
	}
	for id := range h.presences {
		h.followRule(id, initial)
	}
	h.followHome(initial)
}
