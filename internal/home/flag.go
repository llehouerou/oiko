package home

import (
	"cmp"
	"errors"
	"slices"
	"time"

	"uuid"
)

type FlagID string

// Flag is a binary Function held by Oiko itself, with no Device. Its Kind and
// Capabilities are always flagFunction's; its Value is held with the others.
type Flag struct {
	ID           FlagID       `json:"id"`
	Name         string       `json:"name"`
	Area         AreaID       `json:"area,omitempty"` // "" for none
	Kind         string       `json:"kind"`
	Capabilities []Capability `json:"capabilities"`
}

// SavedFlag is a Flag as persisted: its definition and its Value.
type SavedFlag struct {
	ID    FlagID `json:"id"`
	Name  string `json:"name"`
	Area  AreaID `json:"area,omitempty"`
	Value Value  `json:"value"`
}

// flagFunction is what every Flag does. Callers never modify it.
var flagFunction = Function{Key: "flag", Kind: "flag", Capabilities: []Capability{
	{Key: "on", Label: "On", Type: Binary, Access: Access{Observable: true, Settable: true}, Category: Primary},
}}

func flagRef(id FlagID) Ref { return TargetFlag(id).Ref("on") }

// CreateFlag defines a new Flag, off, and returns its ID.
func (h *Home) CreateFlag(name string) (FlagID, error) {
	name, err := ValidName(name)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id := FlagID(uuid.NewV7().String())
	h.flags[id] = Flag{ID: id, Name: name}
	now := time.Now()
	ref, v := flagRef(id), Value{Data: false, At: now, Since: now}
	h.values[ref] = v
	err = h.flagsChanged()
	h.emit(Update{Kind: ValueChanged, Ref: &ref, Value: &v, Initial: true})
	h.emit(Update{Kind: AvailabilityChanged, Target: TargetFlag(id), Availability: Online})
	return id, err
}

// RenameFlag sets a Flag's Name.
func (h *Home) RenameFlag(id FlagID, name string) error {
	name, err := ValidName(name)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f, ok := h.flags[id]
	if !ok {
		return ErrNotFound
	}
	f.Name = name
	h.flags[id] = f
	return h.flagsChanged()
}

// DeleteFlag forgets a Flag, its Value and its History, and removes it from
// every Aggregate containing it.
func (h *Home) DeleteFlag(id FlagID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.flags[id]; !ok {
		return ErrNotFound
	}
	delete(h.flags, id)
	delete(h.values, flagRef(id))
	h.emit(Update{Kind: TargetDeleted, Target: TargetFlag(id)})
	err := h.flagsChanged()
	if h.dropMembers(func(m Target) bool { return m == TargetFlag(id) }) {
		err = errors.Join(err, h.aggregatesChanged())
	}
	return err
}

// flagTarget is Flag id. Its Commands need no Bridge: each is applied and
// confirmed at once.
type flagTarget struct {
	h  *Home
	id FlagID
}

func (x flagTarget) target() Target           { return TargetFlag(x.id) }
func (flagTarget) kind() string               { return flagFunction.Kind }
func (flagTarget) capabilities() []Capability { return flagFunction.Capabilities }
func (flagTarget) attached() bool             { return true }
func (flagTarget) availability() Availability { return Online }
func (flagTarget) accept(values map[string]any) error {
	return checkValues(flagFunction.Capabilities, values)
}
func (x flagTarget) carry(req Request, key string) string { return x.h.issue(x, req, key) }
func (flagTarget) timeout() time.Duration                 { return 0 }

func (x flagTarget) transmit(req Request) {
	x.h.setFlag(x.id, req.Values)
	x.h.confirm(x.target())
}

// setFlag records the Values of a Command on Flag id, as a Device's report
// would, and saves them. Callers hold h.mu.
func (h *Home) setFlag(id FlagID, values map[string]any) {
	at := time.Now()
	var rs []recorded
	for k, data := range values {
		ref := TargetFlag(id).Ref(k)
		rs = append(rs, recorded{ref, h.record(ref, data, at)})
	}
	h.reportToAggregates(rs, at)
	h.saveFlagList()
}

// flagsChanged saves the Flags and announces them. Callers hold h.mu.
func (h *Home) flagsChanged() error {
	err := h.saveFlagList()
	h.emit(Update{Kind: FlagsChanged, Flags: h.flagList()})
	return err
}

// saveFlagList saves every Flag with its Value. Callers hold h.mu.
func (h *Home) saveFlagList() error {
	list := make([]SavedFlag, 0, len(h.flags))
	for _, f := range h.flagList() {
		list = append(list, SavedFlag{ID: f.ID, Name: f.Name, Area: f.Area, Value: h.values[flagRef(f.ID)]})
	}
	return h.save(flagsFile, flagsFormat, list)
}

func (h *Home) flagList() []Flag {
	list := make([]Flag, 0, len(h.flags))
	for _, f := range h.flags {
		f.Kind, f.Capabilities = flagFunction.Kind, flagFunction.Capabilities
		list = append(list, f)
	}
	slices.SortFunc(list, func(a, b Flag) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return list
}
