package home

// Targets is every Target of the home as a follower of it last heard: each
// Device, each of its Functions, each Aggregate, Flag and Presence. Build it
// from a Snapshot, then Apply each later Update. Apply never changes what a
// copy holds, so a copy may be read while the original follows.
type Targets struct {
	devices    []Device
	aggregates []Aggregate
	flags      []Flag
	presences  []Presence
	known      map[Target]known
}

// known is what Targets keeps of a Target: its Name, its Device's for a
// Function, and the Function it is, if it is one.
type known struct {
	name     string
	function Function
}

// TargetsIn is the Targets of s.
func TargetsIn(s Snapshot) Targets {
	ts := Targets{devices: s.Devices, aggregates: s.Aggregates, flags: s.Flags, presences: s.Presences}
	ts.index()
	return ts
}

// Apply follows u, reporting whether it changed the Targets.
func (ts *Targets) Apply(u Update) bool {
	switch u.Kind {
	case DevicesChanged:
		ts.devices = u.Devices
	case AggregatesChanged:
		ts.aggregates = u.Aggregates
	case FlagsChanged:
		ts.flags = u.Flags
	case PresencesChanged:
		ts.presences = u.Presences
	default:
		return false
	}
	ts.index()
	return true
}

// index rebuilds known, anew: a copy keeps the one it holds.
func (ts *Targets) index() {
	ts.known = map[Target]known{}
	for _, d := range ts.devices {
		ts.known[TargetDevice(d.ID, "")] = known{name: d.Name}
		for _, f := range d.Functions {
			ts.known[TargetDevice(d.ID, f.Key)] = known{d.Name, f}
		}
	}
	for _, a := range ts.aggregates {
		ts.known[TargetAggregate(a.ID)] = known{name: a.Name}
	}
	for _, f := range ts.flags {
		ts.known[TargetFlag(f.ID)] = known{name: f.Name}
	}
	for _, p := range ts.presences {
		ts.known[TargetPresence(p.ID)] = known{name: p.Name}
	}
}

// Has reports whether t exists.
func (ts Targets) Has(t Target) bool {
	_, ok := ts.known[t]
	return ok
}

// Name is the Name of t, its Device's for a Function; "" if there is no t.
func (ts Targets) Name(t Target) string { return ts.known[t].name }

// Function is the Function t is, if t is an existing one.
func (ts Targets) Function(t Target) (Function, bool) {
	k, ok := ts.known[t]
	return k.function, ok && t.Function() != ""
}
