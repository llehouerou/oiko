package home

import (
	"cmp"
	"fmt"
	"log"
	"maps"
	"reflect"
	"slices"
	"time"
	"uuid"
)

type AggregateID string

// BinaryRule derives a binary Value from the members' Values.
type BinaryRule string

const (
	Any BinaryRule = "any" // true as soon as one member is
	All BinaryRule = "all" // true only when every member is
)

// NumericRule derives a numeric Value from the members' Values.
type NumericRule string

const (
	Mean NumericRule = "mean"
	Min  NumericRule = "min"
	Max  NumericRule = "max"
	Sum  NumericRule = "sum"
)

// Aggregate is a Function computed by Oiko from member Functions of the same
// kind: Functions of Devices, Flags and nested Aggregates. Home treats an
// *Aggregate as immutable once stored.
type Aggregate struct {
	ID   AggregateID `json:"id"`
	Name string      `json:"name"`
	Icon string      `json:"icon,omitempty"` // "" for the dashboard's default
	Area AreaID      `json:"area,omitempty"` // "" for none
	// An Area Aggregate: Oiko derives it from its Area, members and Name
	// included, and never saves it (ADR 0013).
	Derived bool        `json:"derived,omitempty"`
	Members []Target    `json:"members"`
	Binary  BinaryRule  `json:"binary"`
	Numeric NumericRule `json:"numeric"`
	// Derived from the members, never saved: their kind and the Capabilities
	// they all share.
	Kind         string       `json:"kind,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// CreateAggregate defines a new Aggregate and returns its ID. Empty rules
// default to any and mean.
func (h *Home) CreateAggregate(name string, members []Target, binary BinaryRule, numeric NumericRule) (AggregateID, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := AggregateID(uuid.NewV7().String())
	if err := h.define(id, name, members, binary, numeric); err != nil {
		return "", err
	}
	return id, h.aggregatesChanged()
}

// EditAggregate replaces the Name, members and rules of Aggregate id, under
// the same rules as CreateAggregate. A refused edit leaves it unchanged.
func (h *Home) EditAggregate(id AggregateID, name string, members []Target, binary BinaryRule, numeric NumericRule) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.handMade(id); err != nil {
		return err
	}
	if err := h.define(id, name, members, binary, numeric); err != nil {
		return err
	}
	return h.aggregatesChanged()
}

// define validates an Aggregate definition and stores it, underived. A
// refused one leaves the Aggregates unchanged. Callers hold h.mu.
func (h *Home) define(id AggregateID, name string, members []Target, binary BinaryRule, numeric NumericRule) error {
	name, err := ValidName(name)
	if err != nil {
		return err
	}
	binary, numeric = cmp.Or(binary, Any), cmp.Or(numeric, Mean)
	if binary != Any && binary != All {
		return fmt.Errorf("%w: the binary rule is any or all", ErrInvalid)
	}
	if !slices.Contains([]NumericRule{Mean, Min, Max, Sum}, numeric) {
		return fmt.Errorf("%w: the numeric rule is mean, min, max or sum", ErrInvalid)
	}
	if len(members) == 0 {
		return fmt.Errorf("%w: an aggregate needs at least one member", ErrInvalid)
	}
	a := &Aggregate{ID: id, Name: name, Binary: binary, Numeric: numeric}
	if old := h.aggregates[id]; old != nil {
		a.Icon, a.Area = old.Icon, old.Area
	}
	for _, m := range members {
		if id := m.Aggregate(); id != "" {
			if h.aggregates[id] == nil {
				return fmt.Errorf("%w: unknown member aggregate %s", ErrNotFound, id)
			}
		} else if _, ok := h.member(m); !ok {
			return fmt.Errorf("%w: unknown member %s", ErrNotFound, m)
		}
		if !slices.Contains(a.Members, m) {
			a.Members = append(a.Members, m)
		}
	}

	old := h.aggregates[id]
	h.aggregates[id] = a
	if err := h.checkNesting(); err != nil {
		if old != nil {
			h.aggregates[id] = old
		} else {
			delete(h.aggregates, id)
		}
		return err
	}
	return nil
}

// checkNesting refuses an Aggregate that contains itself, or whose members
// resolve to Functions of different kinds. Callers hold h.mu.
func (h *Home) checkNesting() error {
	for _, a := range h.aggregates {
		fns, nested := h.resolve(a)
		if nested[a.ID] {
			return fmt.Errorf("%w: an aggregate cannot contain itself", ErrInvalid)
		}
		kind := ""
		for _, m := range fns {
			d, ok := h.member(m)
			if !ok {
				continue
			}
			if kind != "" && d.kind() != kind {
				return fmt.Errorf("%w: members must all be of the same kind", ErrInvalid)
			}
			kind = d.kind()
		}
	}
	return nil
}

// resolve returns the Functions a's members resolve to through nested
// Aggregates, each once, in order; and the Aggregates nested in a, directly
// or not.
func (h *Home) resolve(a *Aggregate) (fns []Target, nested map[AggregateID]bool) {
	nested = map[AggregateID]bool{}
	seen := map[Target]bool{}
	var walk func([]Target)
	walk = func(ms []Target) {
		for _, m := range ms {
			switch n := h.aggregates[m.Aggregate()]; {
			case m.Aggregate() == "" && !seen[m]:
				seen[m] = true
				fns = append(fns, m)
			case n != nil && !nested[n.ID]:
				nested[n.ID] = true
				walk(n.Members)
			}
		}
	}
	walk(a.Members)
	return fns, nested
}

// Members are the Functions Aggregate t resolves to now, each once, those
// of Detached Devices included; none if t is not an Aggregate.
func (h *Home) Members(t Target) []Target {
	h.mu.Lock()
	defer h.mu.Unlock()
	a, ok := h.aggregates[t.Aggregate()]
	if !ok {
		return nil
	}
	ms, _ := h.resolve(a)
	return slices.DeleteFunc(ms, func(m Target) bool { _, ok := h.member(m); return !ok })
}

// DeleteAggregate forgets an Aggregate, its Values and its History, and
// removes it from every Aggregate containing it.
func (h *Home) DeleteAggregate(id AggregateID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.handMade(id); err != nil {
		return err
	}
	delete(h.aggregates, id)
	h.commands.abandon(TargetAggregate(id)) // its member Commands carry on unrelayed
	h.emit(Update{Kind: TargetDeleted, Target: TargetAggregate(id)})
	h.dropMembers(func(m Target) bool { return m == TargetAggregate(id) })
	return h.aggregatesChanged()
}

// handMade refuses Aggregate id unless it exists and is an Admin's own,
// not an Area Aggregate. Callers hold h.mu.
func (h *Home) handMade(id AggregateID) error {
	a, ok := h.aggregates[id]
	switch {
	case !ok:
		return ErrNotFound
	case a.Derived:
		return fmt.Errorf("%w: an area aggregate follows its area", ErrInvalid)
	}
	return nil
}

// dropMembers removes the members drop selects from every Aggregate,
// reporting whether it removed any. Callers hold h.mu.
func (h *Home) dropMembers(drop func(Target) bool) bool {
	dropped := false
	for id, a := range h.aggregates {
		if ms := slices.DeleteFunc(slices.Clone(a.Members), drop); len(ms) != len(a.Members) {
			d := *a
			d.Members = ms
			h.aggregates[id] = &d
			dropped = true
		}
	}
	return dropped
}

// aggregatesChanged re-derives the Aggregates, saves and announces them, and
// brings their Values and Availability in line. Callers hold h.mu.
func (h *Home) aggregatesChanged() error {
	h.deriveAll()
	var err error
	if h.saveAggregates != nil {
		list := slices.DeleteFunc(h.aggregateList(), func(a Aggregate) bool { return a.Derived })
		for i := range list {
			list[i].Kind, list[i].Capabilities = "", nil
		}
		if err = h.saveAggregates(list); err != nil {
			log.Printf("home: saving aggregates: %v", err)
		}
	}
	h.emit(Update{Kind: AggregatesChanged, Aggregates: h.aggregateList()})
	h.syncAggregateValues()
	h.syncAggregateAvailability()
	return err
}

// deriveAll rebuilds the Area Aggregates, then re-derives every Aggregate,
// reporting whether any changed, came or went. Callers hold h.mu.
func (h *Home) deriveAll() bool {
	before := maps.Clone(h.aggregates)
	h.syncAreaAggregates()
	changed := len(before) != len(h.aggregates)
	for id, a := range h.aggregates {
		d := *a
		h.derive(&d)
		if old := before[id]; old != nil && reflect.DeepEqual(d, *old) {
			h.aggregates[id] = old
		} else {
			h.aggregates[id] = &d
			changed = true
		}
	}
	return changed
}

// rederive follows a change of the Devices: members whose Device was deleted
// are dropped, then derived Capabilities, Values and Availability are
// recomputed and announced where they differ. Callers hold h.mu.
func (h *Home) rederive() error {
	if h.dropMembers(func(m Target) bool { return m.Device() != "" && h.devices[m.Device()] == nil }) {
		return h.aggregatesChanged()
	}
	if h.deriveAll() {
		h.emit(Update{Kind: AggregatesChanged, Aggregates: h.aggregateList()})
	}
	h.syncAggregateValues()
	h.syncAggregateAvailability()
	return nil
}

// derive sets a's kind and Capabilities from the Functions it resolves to:
// the Capabilities every one has with the same key and type, Stateless ones
// excluded. Each is settable only if every one's is, within the bounds and
// options every one accepts.
func (h *Home) derive(a *Aggregate) {
	a.Kind, a.Capabilities = "", nil
	var fns []direct
	ms, _ := h.resolve(a)
	for _, m := range ms {
		if d, ok := h.member(m); ok {
			fns = append(fns, d)
		}
	}
	if len(fns) == 0 {
		return
	}
	a.Kind = fns[0].kind()
next:
	for _, c := range fns[0].capabilities() {
		if c.Stateless {
			continue
		}
		c.Access = Access{Observable: true, Settable: c.Access.Settable}
		c.Options = slices.Clone(c.Options)
		for _, f := range fns[1:] {
			caps := f.capabilities()
			i := slices.IndexFunc(caps, func(o Capability) bool { return o.Key == c.Key && o.Type == c.Type })
			if i < 0 {
				continue next
			}
			o := caps[i]
			c.Access.Settable = c.Access.Settable && o.Access.Settable
			c.Counter = c.Counter && o.Counter
			if o.Min != nil && (c.Min == nil || *o.Min > *c.Min) {
				c.Min = o.Min
			}
			if o.Max != nil && (c.Max == nil || *o.Max < *c.Max) {
				c.Max = o.Max
			}
			c.Options = slices.DeleteFunc(c.Options, func(s string) bool { return !slices.Contains(o.Options, s) })
		}
		if c.Min != nil && c.Max != nil && *c.Min > *c.Max || c.Type == Enum && len(c.Options) == 0 {
			c.Access.Settable = false // no value every member accepts
		}
		a.Capabilities = append(a.Capabilities, c)
	}
}

// member resolves m, a Function of a Device or a Flag; false if it is
// neither, such as a Device itself or a nested Aggregate. Callers hold h.mu.
func (h *Home) member(m Target) (direct, bool) {
	if m.Device() != "" && m.Function() == "" {
		return nil, false
	}
	return h.lookupDirect(m)
}

// countedMembers returns the Functions a resolves to that contribute to its
// Values and Availability: Flags, and Functions of attached Devices.
func (h *Home) countedMembers(a *Aggregate) []direct {
	ms, _ := h.resolve(a)
	var counted []direct
	for _, m := range ms {
		if d, ok := h.member(m); ok && d.attached() {
			counted = append(counted, d)
		}
	}
	return counted
}

// availabilityOf is online if any counted member is, otherwise offline if any
// is, otherwise unknown.
func (h *Home) availabilityOf(a *Aggregate) Availability {
	av := Unknown
	for _, d := range h.countedMembers(a) {
		switch d.availability() {
		case Online:
			return Online
		case Offline:
			av = Offline
		}
	}
	return av
}

// syncAggregateAvailability recomputes every Aggregate's Availability,
// announcing those that changed. Callers hold h.mu.
func (h *Home) syncAggregateAvailability() {
	old := h.aggregateAvailability
	h.aggregateAvailability = map[AggregateID]Availability{}
	for _, a := range h.aggregateList() {
		av := h.availabilityOf(&a)
		h.aggregateAvailability[a.ID] = av
		if av != cmp.Or(old[a.ID], Unknown) {
			h.emit(Update{Kind: AvailabilityChanged, Target: TargetAggregate(a.ID), Availability: av})
		}
	}
}

// aggregateValue derives Capability key of a from the Values of the counted
// Functions it resolves to, ignoring those without one. Only binary and
// numeric Capabilities have a Value. A settable numeric one (a light's
// brightness) counts only the members switched on, all of them if none is:
// an off light's brightness is not what the room shows. The Value's time is
// the latest member's; a binary one's Since comes from its members'
// (heldSince), a numeric one's is left to held.
func (h *Home) aggregateValue(a *Aggregate, key string) (Value, bool) {
	i := slices.IndexFunc(a.Capabilities, func(c Capability) bool { return c.Key == key })
	if i < 0 {
		return Value{}, false
	}
	typ := a.Capabilities[i].Type
	adjusts := typ == Numeric && a.Capabilities[i].Access.Settable
	var at time.Time
	var bs []Value
	var fs, fsOn []float64
	for _, d := range h.countedMembers(a) {
		v, ok := h.values[d.target().Ref(key)]
		_, isBool := v.Data.(bool)
		f, isNum := v.Data.(float64)
		switch {
		case ok && typ == Binary && isBool:
			bs = append(bs, v)
		case ok && typ == Numeric && isNum:
			fs = append(fs, f)
			if adjusts && h.values[d.target().Ref("state")].Data != false {
				fsOn = append(fsOn, f)
			}
		default:
			continue
		}
		if v.At.After(at) {
			at = v.At
		}
	}
	if len(fsOn) > 0 {
		fs = fsOn
	}
	var data any
	var since time.Time
	switch {
	case len(bs) > 0:
		on := slices.ContainsFunc(bs, func(v Value) bool { return v.Data == true })
		if a.Binary == All {
			on = !slices.ContainsFunc(bs, func(v Value) bool { return v.Data == false })
		}
		data, since = on, heldSince(bs, on, (a.Binary == All) != on)
	case len(fs) > 0 && a.Numeric == Min:
		data = slices.Min(fs)
	case len(fs) > 0 && a.Numeric == Max:
		data = slices.Max(fs)
	case len(fs) > 0:
		sum := 0.0
		for _, f := range fs {
			sum += f
		}
		if a.Numeric != Sum {
			sum /= float64(len(fs))
		}
		data = sum
	default:
		return Value{}, false
	}
	return Value{Data: data, At: at, Since: since}, true
}

// heldSince is since when a binary Aggregate has been on or off, from its members'
// Values: with any, on since the first of those on took theirs, as they all
// overlap up to now, and off since the last went off; with all, the other way
// round. earliest says which.
func heldSince(members []Value, on, earliest bool) time.Time {
	var since time.Time
	found := false
	for _, v := range members {
		if v.Data == on && (!found || v.Since.Before(since) == earliest) {
			since, found = v.Since, true
		}
	}
	return since
}

// held gives v, an Aggregate Value just derived, a Since if it has none: the
// old one's if its Data is the same, otherwise its time.
func held(v, old Value, had bool) Value {
	if v.Since.IsZero() {
		v.Since = v.At
		if had && Equal(old.Data, v.Data) {
			v.Since = old.Since
		}
	}
	return v
}

// syncAggregateValues recomputes every Aggregate Value from the members'
// current Values, announcing those that changed, and as a refresh those that
// held for another time: a member joined or left. A Capability of a remaining
// Aggregate that loses its Value is announced as a change without Value.
// Callers hold h.mu.
func (h *Home) syncAggregateValues() {
	old := map[Ref]Value{}
	for ref, v := range h.values {
		if ref.Target.Aggregate() != "" {
			old[ref] = v
			delete(h.values, ref)
		}
	}
	for _, a := range h.aggregateList() {
		for _, c := range a.Capabilities {
			ref := TargetAggregate(a.ID).Ref(c.Key)
			prev, had := old[ref]
			v, ok := h.aggregateValue(&a, c.Key)
			v = held(v, prev, had)
			switch {
			case !ok && had:
				h.emit(Update{Kind: ValueChanged, Ref: &ref})
			case !ok:
			case had && Equal(prev.Data, v.Data) && prev.Since.Equal(v.Since):
				h.values[ref] = prev
			case had && Equal(prev.Data, v.Data):
				prev.Since = v.Since
				h.values[ref] = prev
				h.emit(Update{Kind: ValueRefreshed, Ref: &ref, Value: &prev})
			default:
				h.values[ref] = v
				h.emit(Update{Kind: ValueChanged, Ref: &ref, Value: &v, Initial: !had})
			}
		}
	}
}

// recorded is a member Value just recorded, and whether it is its first.
type recorded struct {
	ref   Ref
	first bool
}

// reportToAggregates updates the Aggregate Values that the member Values just
// recorded feed, with the report's time; a member's state feeds the settable
// numeric ones too (aggregateValue). A change moved only by members' first
// Values is Initial, as theirs are. Callers hold h.mu.
func (h *Home) reportToAggregates(rs []recorded, at time.Time) {
	type touch struct {
		ref     Ref
		initial bool
	}
	var touched []touch
	for _, a := range h.aggregateList() {
		ms := h.countedMembers(&a)
		for _, r := range rs {
			if !slices.ContainsFunc(ms, func(d direct) bool { return d.target() == r.ref.Target }) {
				continue
			}
			keys := []string{r.ref.Capability}
			if r.ref.Capability == "state" {
				for _, c := range a.Capabilities {
					if c.Type == Numeric && c.Access.Settable {
						keys = append(keys, c.Key)
					}
				}
			}
			for _, key := range keys {
				ref := TargetAggregate(a.ID).Ref(key)
				if i := slices.IndexFunc(touched, func(t touch) bool { return t.ref == ref }); i >= 0 {
					touched[i].initial = touched[i].initial && r.first
				} else {
					touched = append(touched, touch{ref, r.first})
				}
			}
		}
	}
	for _, t := range touched {
		ref := t.ref
		v, ok := h.aggregateValue(h.aggregates[ref.Target.Aggregate()], ref.Capability)
		if !ok {
			continue
		}
		v.At = at
		kind := ValueChanged
		old, had := h.values[ref]
		v = held(v, old, had)
		if had && Equal(old.Data, v.Data) {
			kind = ValueRefreshed
		}
		h.values[ref] = v
		h.emit(Update{Kind: kind, Ref: &ref, Value: &v, Initial: t.initial || !had})
	}
}

func (h *Home) aggregateList() []Aggregate {
	list := make([]Aggregate, 0, len(h.aggregates))
	for _, a := range h.aggregates {
		list = append(list, *a)
	}
	slices.SortFunc(list, func(a, b Aggregate) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return list
}

// aggregateTarget is Aggregate a. Its Commands are relayed to every counted
// Function it resolves to, once, as Commands of their own with the same
// origin. An Aggregate Command is confirmed once every member Command is;
// failed, timed out or superseded as soon as one member Command is; superseded
// by a newer Command on the same Aggregate.
type aggregateTarget struct {
	h *Home
	a *Aggregate
}

// accept checks values against a's Capabilities, then against every counted
// member's.
func (x aggregateTarget) accept(values map[string]any) error {
	for k := range values {
		if !slices.ContainsFunc(x.a.Capabilities, func(c Capability) bool { return c.Key == k }) {
			return ErrNotFound
		}
	}
	members := x.h.countedMembers(x.a)
	for _, d := range members {
		if err := d.accept(values); err != nil {
			return err
		}
	}
	if len(members) == 0 {
		return fmt.Errorf("%w: no member to command", ErrInvalid)
	}
	return nil
}

// carry resolves a toggle as a Function's: against the pending Command on the
// Aggregate or, failing one, its own Value (with any, one member on turns all
// off). Then it relays the Command; one that only adjusts (a brightness, no
// state) goes to the members switched on, all of them if none is, so it turns
// none on.
func (x aggregateTarget) carry(req Request, key string) string {
	h, t := x.h, TargetAggregate(x.a.ID)
	if key != "" {
		req.Values = toggled(req.Values, key, h.commands.isOn(t, key, h.values[t.Ref(key)].Data))
	}
	members := h.countedMembers(x.a)
	on := slices.DeleteFunc(slices.Clone(members), func(d direct) bool { return h.switchedOff(d.target()) })
	if len(on) > 0 && x.adjusts(req.Values) {
		members = on
	}
	ac := h.commands.start(t, req, nil, 0) // its members time out
	ac.waiting = len(members)
	for _, d := range members {
		h.deliver(d, req, ac)
	}
	return ac.id
}

// adjusts is whether values set no state and no setting (Config) of a.
func (x aggregateTarget) adjusts(values map[string]any) bool {
	if _, ok := values["state"]; ok {
		return false
	}
	for k := range values {
		if !slices.ContainsFunc(x.a.Capabilities, func(c Capability) bool { return c.Key == k && c.Category != Config }) {
			return false
		}
	}
	return true
}

// switchedOff is whether Function t's state is off, as a toggle sees it: as
// the pending Command asks or, failing one, as its Value is. Unknown is not
// off. Callers hold h.mu.
func (h *Home) switchedOff(t Target) bool {
	if v, ok := h.commands.asking(t)["state"]; ok {
		return v == false
	}
	return h.values[t.Ref("state")].Data == false
}
