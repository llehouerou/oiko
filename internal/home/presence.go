package home

import (
	"cmp"
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

// Presence is a Person's Presence, as the presences list shows it: its
// Name is its Person's. Its Kind and Capabilities are presenceFunction's.
type Presence struct {
	ID           PersonID     `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Capabilities []Capability `json:"capabilities"`
}

// SavedPresence is a Presence as presences.json keeps it (ADR 0054): its
// Value and manual's, absent until something tells. While manual is true,
// Value is the one a Command forced.
type SavedPresence struct {
	ID     PersonID `json:"id"`
	Value  *Value   `json:"value,omitempty"`
	Manual *Value   `json:"manual,omitempty"`
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
// state; one whose Person is gone is deleted with its History (ADR 0016), and
// what was kept of it dropped. The first call announces the Presences even if
// none changed.
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
	for id := range h.presences {
		if !slices.ContainsFunc(ps, func(p Person) bool { return p.ID == id }) {
			h.removePerson(id)
			changed = true
		}
	}
	h.recalled = nil
	if !changed {
		return nil
	}
	err := h.presencesChanged()
	for _, id := range fresh {
		h.announcePresence(id)
	}
	return err
}

// setPerson creates or renames p's Presence, reporting whether it is new and
// whether it changed. A new one takes the Values recalled for it, if any.
// Callers hold h.mu.
func (h *Home) setPerson(p Person) (isNew, changed bool) {
	name, known := h.presences[p.ID]
	h.presences[p.ID] = p.Name
	if known {
		return false, name != p.Name
	}
	if s, ok := h.recalled[p.ID]; ok {
		for ref, v := range map[Ref]*Value{homeRef(p.ID): s.Value, manualRef(p.ID): s.Manual} {
			if v != nil {
				h.values[ref] = *v
			}
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
	delete(h.presences, id)
	delete(h.values, homeRef(id))
	delete(h.values, manualRef(id))
	h.emit(Update{Kind: TargetDeleted, Target: TargetPresence(id)})
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

// transmit forces home to the Value asked for. With no Presence source yet,
// nothing ends the override: manual holds until the next Command.
func (x presenceTarget) transmit(req Request) {
	h, at := x.h, time.Now()
	rs := []recorded{
		{homeRef(x.id), h.record(homeRef(x.id), req.Values["home"], at)},
		{manualRef(x.id), h.record(manualRef(x.id), true, at)},
	}
	h.reportToAggregates(rs, at)
	h.savePresences()
	h.confirm(x.target())
}

// presencesChanged saves the Presences and announces them. Callers hold h.mu.
func (h *Home) presencesChanged() error {
	err := h.savePresences()
	h.emit(Update{Kind: PresencesChanged, Presences: h.presenceList()})
	return err
}

// savePresences saves every Presence with its Values. Callers hold h.mu.
func (h *Home) savePresences() error {
	list := []SavedPresence{}
	for _, p := range h.presenceList() {
		s := SavedPresence{ID: p.ID}
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

// presenceList is every Presence, by Name.
func (h *Home) presenceList() []Presence {
	list := make([]Presence, 0, len(h.presences))
	for id, name := range h.presences {
		list = append(list, Presence{ID: id, Name: name, Kind: presenceFunction.Kind, Capabilities: presenceFunction.Capabilities})
	}
	slices.SortFunc(list, func(a, b Presence) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return list
}
