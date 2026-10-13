package home

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"
)

// PersonID is a Person's id, as the access store keeps it.
type PersonID string

// Person is what Home knows of a Person: whose Presence it derives (ADR
// 0048). The access store owns them.
type Person struct {
	ID   PersonID
	Name string
}

// PresenceSource is a Function bound to a Person's Presence (ADR 0049): read
// through its binary state or main control, true meaning home, it keeps
// counting as home for DepartureDelay seconds once false.
type PresenceSource struct {
	Target         Target `json:"target"`
	DepartureDelay int    `json:"departureDelay"` // in seconds
}

// Presence is a Person's Presence or the Home presence, as the presences
// list shows it: a Person's has its Name, presenceFunction's Kind and
// Capabilities and its Presence sources; the Home presence has
// homePresenceFunction's and its HomeBindings.
type Presence struct {
	ID            PersonID         `json:"id"`
	Name          string           `json:"name"`
	Kind          string           `json:"kind"`
	Capabilities  []Capability     `json:"capabilities"`
	Sources       []PresenceSource `json:"sources,omitzero"` // a Person's: never nil
	*HomeBindings                  // the Home presence's
}

// SavedPresence is a Presence as presences.json keeps it (ADR 0054): its
// Presence sources or HomeBindings, and its Value and manual's, absent until
// something tells. While manual is true, Value is the one a Command forced.
type SavedPresence struct {
	ID            PersonID         `json:"id"`
	Sources       []PresenceSource `json:"sources,omitempty"`
	*HomeBindings                  // the Home presence's
	Value         *Value           `json:"value,omitempty"`
	Manual        *Value           `json:"manual,omitempty"`
}

// person is a Person's Presence as Home holds it.
type person struct {
	name    string
	sources []PresenceSource // never modified: replaced whole
	timer   *time.Timer      // until the earliest departure delay runs out; nil while none runs
}

// presenceFunction is what every Person's Presence does. Callers never
// modify it.
var presenceFunction = Function{Key: "presence", Kind: "presence", Capabilities: []Capability{
	{Key: "home", Label: "Home", Type: Binary, Access: Access{Observable: true, Settable: true}, Category: Primary},
	{Key: "manual", Label: "Set by hand", Type: Binary, Access: Access{Observable: true}, Category: Primary},
}}

func homeRef(id PersonID) Ref   { return TargetPresence(id).Ref("home") }
func manualRef(id PersonID) Ref { return TargetPresence(id).Ref("manual") }

// SetPersons gives Home every Person, as Oiko starts and after each one is
// created, renamed or removed: each has a Presence, under their Name. A new
// one takes what presences.json kept of it, as Oiko starts, as replayed
// state; one whose Person is gone is deleted with its History (ADR 0016),
// what was kept of it dropped, and so is it from every Aggregate. The first
// call announces the Presences even if none changed.
func (h *Home) SetPersons(ps []Person) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := h.recalled != nil
	var fresh []PersonID
	for _, p := range ps {
		isNew, renamed := h.setPerson(p)
		if isNew {
			fresh = append(fresh, p.ID)
		}
		changed = changed || renamed
	}
	removed := false
	for id := range h.presences {
		if !slices.ContainsFunc(ps, func(p Person) bool { return p.ID == id }) {
			h.removePerson(id)
			removed = true
		}
	}
	h.recalled = nil
	if !changed && !removed {
		return nil
	}
	err := h.presencesChanged()
	for _, id := range fresh {
		h.announcePresence(id)
		h.followRule(id, true) // its Presence sources' Values, a Flag's, are replayed state too
	}
	return errors.Join(err, h.rederive(!removed)) // recalled Values are replayed state, for what follows them too
}

// setPerson creates or renames p's Presence, reporting whether it is new and
// whether it changed. A new one takes the Presence sources and Values recalled for it,
// if any. Callers hold h.mu.
func (h *Home) setPerson(p Person) (isNew, changed bool) {
	if known := h.presences[p.ID]; known != nil {
		changed = known.name != p.Name
		known.name = p.Name
		return false, changed
	}
	s := h.recalled[p.ID]
	h.presences[p.ID] = &person{name: p.Name, sources: s.Sources}
	for ref, v := range map[Ref]*Value{homeRef(p.ID): s.Value, manualRef(p.ID): s.Manual} {
		if v != nil {
			h.values[ref] = *v
		}
	}
	return true, true
}

// announcePresence announces a new Presence, once listed: its recalled
// Values, as replayed state, and its Availability. Callers hold h.mu.
func (h *Home) announcePresence(id PersonID) {
	for _, ref := range []Ref{homeRef(id), manualRef(id)} {
		if v, ok := h.values[ref]; ok {
			h.emit(Update{Kind: ValueChanged, Ref: &ref, Value: &v, Initial: true})
		}
	}
	h.emit(Update{Kind: AvailabilityChanged, Target: TargetPresence(id), Availability: Online})
}

// removePerson deletes Person id's Presence. Callers hold h.mu.
func (h *Home) removePerson(id PersonID) {
	if t := h.presences[id].timer; t != nil {
		t.Stop()
	}
	delete(h.presences, id)
	delete(h.values, homeRef(id))
	delete(h.values, manualRef(id))
	h.emit(Update{Kind: TargetDeleted, Target: TargetPresence(id)})
}

// SetPresenceSources binds Presence sources to Person id's Presence, in place of
// those it had (ADR 0054). Each must be a Function, Aggregate or Flag with a
// binary state or main control, other than a Presence or an Aggregate of
// them, and its delay not
// negative; one listed twice is kept once, as first listed. The Presence then
// follows them: an override ends if they agree with it.
func (h *Home) SetPresenceSources(id PersonID, sources []PresenceSource) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.presences[id]
	if p == nil {
		return ErrNotFound
	}
	kept := []PresenceSource{}
	for _, s := range sources {
		if err := h.checkSource(s); err != nil {
			return err
		}
		if !slices.ContainsFunc(kept, func(k PresenceSource) bool { return k.Target == s.Target }) {
			kept = append(kept, s)
		}
	}
	p.sources = kept
	err := h.presencesChanged()
	h.followAll(false)
	return err
}

// checkSource refuses what cannot be a Presence source. Callers hold h.mu.
func (h *Home) checkSource(s PresenceSource) error {
	if s.DepartureDelay < 0 {
		return fmt.Errorf("%w: a departure delay cannot be negative", ErrInvalid)
	}
	return h.checkBinding(s.Target)
}

// checkBinding refuses what cannot be bound to a Presence or the Home
// presence (ADR 0054). Callers hold h.mu.
func (h *Home) checkBinding(t Target) error {
	switch {
	case t.IsZero():
		return fmt.Errorf("%w: a binding names its target", ErrInvalid)
	case t.Presence() != "" || h.aggregates[t.Aggregate()] != nil && h.aggregates[t.Aggregate()].Kind == presenceFunction.Kind:
		return fmt.Errorf("%w: a presence, or an aggregate of them, cannot be bound to a presence", ErrInvalid) // none feeds what follows a Presence (propagate.go)
	case t.Device() != "" && t.Function() == "":
		return fmt.Errorf("%w: a binding is a function, not a device", ErrInvalid)
	}
	if _, known := h.lookup(t); !known {
		return fmt.Errorf("%w: unknown target %s", ErrNotFound, t)
	}
	if _, ok := h.sourceKey(t); !ok {
		return fmt.Errorf("%w: %s has no binary state or main control", ErrInvalid, t)
	}
	return nil
}

// dropSources unbinds the Targets drop selects from every Presence and the
// Home presence, once they are deleted. Callers hold h.mu.
func (h *Home) dropSources(drop func(Target) bool) error {
	dropped := false
	for _, p := range h.presences {
		if ss := slices.DeleteFunc(slices.Clone(p.sources), func(s PresenceSource) bool { return drop(s.Target) }); len(ss) != len(p.sources) {
			p.sources = ss
			dropped = true
		}
	}
	if boxed := h.dropFromBox(drop); !dropped && !boxed {
		return nil
	}
	err := h.presencesChanged()
	h.followAll(false)
	return err
}

// reportToPresences brings in line the Presences whose Presence sources'
// Values were just recorded. A change moved only by replayed Values is
// replayed state, as theirs are (ADR 0054). Callers hold h.mu.
func (h *Home) reportToPresences(rs []recorded) {
	for id, p := range h.presences {
		fed, initial := false, true
		for _, s := range p.sources {
			f, i := h.fed(s.Target, rs)
			fed, initial = fed || f, initial && (!f || i)
		}
		if fed {
			h.followRule(id, initial)
		}
	}
}

// fed is whether rs, Values just recorded, feed what t is read through, and
// whether only by replayed Values. Callers hold h.mu.
func (h *Home) fed(t Target, rs []recorded) (fed, initial bool) {
	initial = true
	for _, r := range rs {
		if h.feeds(t, r.ref) {
			fed, initial = true, initial && r.initial
		}
	}
	return fed, initial
}

// followRule sets Person id's Presence to what its rule computes now, unless
// an override holds; it ends the override once the rule agrees with it. It
// has the rule computed again when the earliest departure delay runs out.
// initial marks a change as replayed state. Callers hold h.mu.
func (h *Home) followRule(id PersonID, initial bool) {
	p := h.presences[id]
	now := time.Now()
	home, ok, next := h.rule(p.sources, now)
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if !next.IsZero() {
		p.timer = time.AfterFunc(next.Sub(now), func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.presences[id] == p { // not removed since
				h.followRule(id, false)
			}
		})
	}
	if !ok {
		return
	}
	cur, had := h.values[homeRef(id)]
	manual := h.values[manualRef(id)].Data == true
	if manual && cur.Data != home {
		return // the override holds
	}
	var rs []recorded
	if !had || cur.Data != home {
		rs = append(rs, recorded{homeRef(id), h.recordAs(homeRef(id), home, now, initial)})
	}
	if m, had := h.values[manualRef(id)]; !had || m.Data != false {
		rs = append(rs, recorded{manualRef(id), h.recordAs(manualRef(id), false, now, initial)})
	}
	if len(rs) > 0 {
		h.followValues(rs, now)
		h.savePresences()
	}
}

// presenceTarget is Person id's Presence. A Command on it is an override
// (ADR 0052), applied and confirmed at once.
type presenceTarget struct {
	h  *Home
	id PersonID
}

func (x presenceTarget) target() Target           { return TargetPresence(x.id) }
func (presenceTarget) kind() string               { return presenceFunction.Kind }
func (presenceTarget) capabilities() []Capability { return presenceFunction.Capabilities }
func (presenceTarget) attached() bool             { return true }
func (presenceTarget) availability() Availability { return Online }
func (presenceTarget) accept(values map[string]any) error {
	return checkValues(presenceFunction.Capabilities, values)
}
func (x presenceTarget) carry(req Request, key string) string { return x.h.issue(x, req, key) }
func (presenceTarget) timeout() time.Duration                 { return 0 }

// transmit forces home to the Value asked for: an override, unless the rule
// computes that Value already. With no Presence source heard, nothing ends it but
// the next Command.
func (x presenceTarget) transmit(req Request) {
	h, at := x.h, time.Now()
	forced := req.Values["home"]
	ruled, ok, _ := h.rule(h.presences[x.id].sources, at)
	rs := []recorded{
		{homeRef(x.id), h.record(homeRef(x.id), forced, at)},
		{manualRef(x.id), h.record(manualRef(x.id), !ok || ruled != forced, at)},
	}
	h.followValues(rs, at)
	h.savePresences()
	h.confirm(x.target())
}

// presencesChanged saves the Presences and announces them. Callers hold h.mu.
func (h *Home) presencesChanged() error {
	err := h.savePresences()
	h.emit(Update{Kind: PresencesChanged, Presences: h.presenceList()})
	return err
}

// savePresences saves every Presence with its bindings and Values. Callers
// hold h.mu.
func (h *Home) savePresences() error {
	list := []SavedPresence{}
	for _, p := range h.presenceList() {
		s := SavedPresence{ID: p.ID, Sources: p.Sources, HomeBindings: p.HomeBindings}
		if v, ok := h.values[homeRef(p.ID)]; ok {
			s.Value = &v
		}
		if v, ok := h.values[manualRef(p.ID)]; ok {
			s.Manual = &v
		}
		list = append(list, s)
	}
	return h.save(presencesFile, presencesFormat, list)
}

// presenceList is the Home presence, then every Person's Presence by Name.
func (h *Home) presenceList() []Presence {
	list := make([]Presence, 0, len(h.presences))
	for id, p := range h.presences {
		list = append(list, Presence{ID: id, Name: p.name, Kind: presenceFunction.Kind, Capabilities: presenceFunction.Capabilities,
			Sources: append([]PresenceSource{}, p.sources...)})
	}
	slices.SortFunc(list, func(a, b Presence) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return append([]Presence{h.homePresence()}, list...)
}
