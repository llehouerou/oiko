package home

import (
	"slices"
	"time"
)

// The v1 entry of the Presence rule table (ADR 0048, 0049): any, a departure
// delay per Presence source. A later rule is a new entry, its parameters
// new optional fields of PresenceSource.

// rule is what any computes of Presence sources now: home while one counts as
// home, away once none does; not ok while none is heard, when the Presence
// keeps its last Value. A Presence source counts while true, and until its departure
// delay has passed since its Value's Since. next is when the earliest delay
// still running runs out, zero for none. Callers hold h.mu.
func (h *Home) rule(sources []PresenceSource, now time.Time) (home, ok bool, next time.Time) {
	for _, s := range sources {
		v, heard := h.sourceValue(s.Target)
		if !heard {
			continue
		}
		ok = true
		until := v.Since.Add(time.Duration(s.DepartureDelay) * time.Second)
		switch {
		case v.Data == true:
			home = true
		case now.Before(until):
			home = true
			if next.IsZero() || until.Before(next) {
				next = until
			}
		}
	}
	return home, ok, next
}

// sourceValue is Presence source t's binary Value, if it is heard: it has one, it
// has not vanished and is not a Detached Device's, nor one whose Device or
// Bridge is offline. Callers hold h.mu.
func (h *Home) sourceValue(t Target) (Value, bool) {
	key, ok := h.sourceKey(t)
	if !ok || !h.reachable(t) {
		return Value{}, false
	}
	v, ok := h.values[t.Ref(key)]
	if _, binary := v.Data.(bool); !ok || !binary {
		return Value{}, false
	}
	return v, true
}

// reachable is whether t, a Function, a Flag or an Aggregate, speaks for
// now: an attached Device's Function while its Device and Bridge are not
// offline, an Aggregate while one of its counted members is reachable.
// Callers hold h.mu.
func (h *Home) reachable(t Target) bool {
	if a := h.aggregates[t.Aggregate()]; a != nil {
		return slices.ContainsFunc(h.countedMembers(a), func(d direct) bool { return h.reachable(d.target()) })
	}
	d, ok := h.lookupDirect(t)
	if !ok || !d.attached() {
		return false
	}
	if id := t.Device(); id != "" {
		return h.online(h.devices[id].Bridge) && h.availability[id] != Offline
	}
	return true
}

// fed is whether rs, Values just recorded, feed what Presence source t is
// read through, directly or as a member of the Aggregate t is, and whether
// only by replayed Values. Callers hold h.mu.
func (h *Home) fed(t Target, rs []recorded) (fed, initial bool) {
	key, ok := h.sourceKey(t)
	if !ok {
		return false, true
	}
	from := []Target{t}
	if a := h.aggregates[t.Aggregate()]; a != nil {
		from, _ = h.resolve(a)
	}
	initial = true
	for _, r := range rs {
		if r.ref.Capability == key && slices.Contains(from, r.ref.Target) {
			fed, initial = true, initial && r.initial
		}
	}
	return fed, initial
}

// sourceKey is the Capability Presence source t is read through; false if t is
// unknown now, a Device itself or a Presence, or has neither. Callers hold
// h.mu.
func (h *Home) sourceKey(t Target) (string, bool) {
	if a := h.aggregates[t.Aggregate()]; a != nil {
		return readCapability(a.Kind, a.Capabilities)
	}
	if t.Presence() != "" || t.Device() != "" && t.Function() == "" {
		return "", false
	}
	d, ok := h.lookupDirect(t)
	if !ok {
		return "", false
	}
	return readCapability(d.kind(), d.capabilities())
}
