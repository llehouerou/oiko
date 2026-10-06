package home

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"

	"uuid"
)

type AreaID string

type areaKind struct {
	kind, label string // label names the Aggregate after its Area's Name
	binary      BinaryRule
	numeric     NumericRule
}

// areaKinds are the kinds every Area aggregates, with their rules (ADR 0013).
// A room's climate is its mean, but its CO2 its worst corner: airing is for
// the highest.
var areaKinds = []areaKind{
	{"light", "lights", Any, Mean},
	{"occupancy", "occupancy", Any, Mean},
	{"contact", "doors", Any, Mean},
	{"temperature", "temperature", Any, Mean},
	{"humidity", "humidity", Any, Mean},
	{"co2", "CO2", Any, Max},
}

// areaAggregateID is the identity of the Area Aggregate of area and kind,
// whether it exists or not.
func areaAggregateID(area AreaID, kind string) AggregateID {
	return AggregateID(string(area) + "." + kind)
}

// areaAggregates are the Targets the Area Aggregates of area have, whether
// they exist now or not.
func areaAggregates(area AreaID) []Target {
	var ts []Target
	for _, k := range areaKinds {
		ts = append(ts, TargetAggregate(areaAggregateID(area, k.kind)))
	}
	return ts
}

// syncAreaAggregates rebuilds, underived, one Area Aggregate per Area and
// aggregated kind of which the Area holds at least one Function. Callers
// hold h.mu.
func (h *Home) syncAreaAggregates() {
	maps.DeleteFunc(h.aggregates, func(_ AggregateID, a *Aggregate) bool { return a.Derived })
	devices := h.deviceList() // in a steady order, so the members are
	for _, area := range h.areas {
		for _, k := range areaKinds {
			var members []Target
			for _, d := range devices {
				for _, f := range d.Functions {
					if f.Kind == k.kind && cmp.Or(f.Area, d.Area) == area.ID {
						members = append(members, TargetDevice(d.ID, f.Key))
					}
				}
			}
			if len(members) > 0 {
				id := areaAggregateID(area.ID, k.kind)
				h.aggregates[id] = &Aggregate{ID: id, Name: area.Name + " " + k.label, Area: area.ID, Derived: true,
					Members: members, Binary: k.binary, Numeric: k.numeric}
			}
		}
	}
}

// Area is a room or zone of the home. Areas are flat, in the order an
// Admin set. A Device, one of its Functions, a Flag or an Aggregate is assigned
// one by its own Area field; a Function without one is in its Device's.
type Area struct {
	ID   AreaID `json:"id"`
	Name string `json:"name"`
	// How the dashboard shows it: the tiles it folds away (a Device's, a
	// Flag's or an Aggregate's), the kinds whose Area Aggregate its header
	// leaves out, and its Layout.
	Hidden           []Target    `json:"hidden,omitempty"`
	HiddenAggregates []string    `json:"hiddenAggregates,omitempty"`
	Columns          int         `json:"columns,omitempty"` // 0 lets the dashboard pick
	Layout           []Placement `json:"layout,omitempty"`
}

// Placement is where a tile sits in its Area's grid: its first cell, from 0,
// and how many columns and rows it spans (0 rows is one). A row is as tall
// as its tallest tile.
type Placement struct {
	Tile   Target `json:"tile"`
	Col    int    `json:"col"`
	Row    int    `json:"row"`
	Width  int    `json:"width"`
	Height int    `json:"height,omitempty"`
}

// maxColumns bounds an Area's grid: wider would not fit a screen.
const maxColumns = 6

// CreateArea defines a new Area, last in the order, and returns its ID.
func (h *Home) CreateArea(name string) (AreaID, error) {
	name, err := ValidName(name)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id := AreaID(uuid.NewV7().String())
	h.areas = append(h.areas, Area{ID: id, Name: name})
	return id, h.areasChanged()
}

// RenameArea sets an Area's Name.
func (h *Home) RenameArea(id AreaID, name string) error {
	name, err := ValidName(name)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	i := h.area(id)
	if i < 0 {
		return ErrNotFound
	}
	h.areas[i].Name = name
	return h.areasChanged()
}

// SetAreaDisplay sets which tiles the dashboard folds away in Area id, and
// which kinds of its Area Aggregates its header leaves out.
func (h *Home) SetAreaDisplay(id AreaID, hidden []Target, hiddenAggregates []string) error {
	for _, k := range hiddenAggregates {
		if !slices.ContainsFunc(areaKinds, func(a areaKind) bool { return a.kind == k }) {
			return fmt.Errorf("%w: an area aggregates no %q", ErrInvalid, k)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	i := h.area(id)
	if i < 0 {
		return ErrNotFound
	}
	h.areas[i].Hidden, h.areas[i].HiddenAggregates = hidden, hiddenAggregates
	return h.areasChanged()
}

// SetAreaLayout sets the grid of Area id: its columns and where its tiles
// sit, each within the grid, apart from the others. A tile it does not place
// is the dashboard's to place.
func (h *Home) SetAreaLayout(id AreaID, columns int, layout []Placement) error {
	if columns < 1 || columns > maxColumns {
		return fmt.Errorf("%w: an area has 1 to %d columns", ErrInvalid, maxColumns)
	}
	taken := map[[2]int]bool{}
	for i, p := range layout {
		if p.Col < 0 || p.Row < 0 || p.Width < 1 || p.Height < 0 || p.Col+p.Width > columns {
			return fmt.Errorf("%w: %s lies outside the grid", ErrInvalid, p.Tile)
		}
		if slices.ContainsFunc(layout[:i], func(q Placement) bool { return q.Tile == p.Tile }) {
			return fmt.Errorf("%w: %s is placed twice", ErrInvalid, p.Tile)
		}
		for r := p.Row; r < p.Row+max(p.Height, 1); r++ {
			for c := p.Col; c < p.Col+p.Width; c++ {
				if taken[[2]int{c, r}] {
					return fmt.Errorf("%w: %s overlaps another tile", ErrInvalid, p.Tile)
				}
				taken[[2]int{c, r}] = true
			}
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	i := h.area(id)
	if i < 0 {
		return ErrNotFound
	}
	h.areas[i].Columns, h.areas[i].Layout = columns, layout
	return h.areasChanged()
}

// OrderAreas puts the Areas in the order of ids, which lists each of them
// once.
func (h *Home) OrderAreas(ids []AreaID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ordered := make([]Area, 0, len(ids))
	for _, id := range ids {
		i := h.area(id)
		if i < 0 || slices.ContainsFunc(ordered, func(a Area) bool { return a.ID == id }) {
			return fmt.Errorf("%w: an order lists every Area once", ErrInvalid)
		}
		ordered = append(ordered, h.areas[i])
	}
	if len(ordered) != len(h.areas) {
		return fmt.Errorf("%w: an order lists every Area once", ErrInvalid)
	}
	h.areas = ordered
	return h.areasChanged()
}

// DeleteArea forgets an Area: what it held is left without one. Its Area
// Aggregates go, and their History, whether they exist now or not.
func (h *Home) DeleteArea(id AreaID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	i := h.area(id)
	if i < 0 {
		return ErrNotFound
	}
	h.areas = slices.Delete(h.areas, i, i+1)
	for _, t := range areaAggregates(id) {
		h.emit(Update{Kind: TargetDeleted, Target: t})
	}
	err := h.areasChanged()
	if h.dropMembers(func(m Target) bool { return slices.Contains(areaAggregates(id), m) }) {
		err = errors.Join(err, h.aggregatesChanged())
	}

	devices := false
	for did, d := range h.devices {
		if changed, ok := d.without(id); ok {
			h.devices[did] = changed
			devices = true
		}
	}
	if devices {
		err = errors.Join(err, h.registryChanged())
	}
	flags := false
	for fid, f := range h.flags {
		if f.Area == id {
			f.Area = ""
			h.flags[fid] = f
			flags = true
		}
	}
	if flags {
		err = errors.Join(err, h.flagsChanged())
	}
	aggregates := false
	for aid, a := range h.aggregates {
		if a.Area == id {
			changed := *a
			changed.Area = ""
			h.aggregates[aid] = &changed
			aggregates = true
		}
	}
	if aggregates {
		err = errors.Join(err, h.aggregatesChanged())
	}
	return err
}

// without is d with no trace of Area id, and whether it had any.
func (d *Device) without(id AreaID) (*Device, bool) {
	changed := *d
	changed.Functions = slices.Clone(d.Functions)
	had := false
	if changed.Area == id {
		changed.Area, had = "", true
	}
	for i := range changed.Functions {
		if changed.Functions[i].Area == id {
			changed.Functions[i].Area, had = "", true
		}
	}
	return &changed, had
}

// SetArea assigns Area area to t: a Device, one of its Functions, a Flag or
// an Aggregate. "" leaves it without one, or a Function in its Device's.
func (h *Home) SetArea(t Target, area AreaID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if area != "" && h.area(area) < 0 {
		return fmt.Errorf("%w: unknown area %s", ErrNotFound, area)
	}
	switch {
	case t.Device() != "":
		d, ok := h.devices[t.Device()]
		if !ok {
			return ErrNotFound
		}
		changed := *d
		if fn := t.Function(); fn == "" {
			changed.Area = area
		} else {
			changed.Functions = slices.Clone(d.Functions)
			f := changed.function(fn)
			if f == nil {
				return ErrNotFound
			}
			f.Area = area
		}
		h.devices[t.Device()] = &changed
		return h.registryChanged()
	case t.Flag() != "":
		f, ok := h.flags[t.Flag()]
		if !ok {
			return ErrNotFound
		}
		f.Area = area
		h.flags[t.Flag()] = f
		return h.flagsChanged()
	case t.Aggregate() != "":
		if err := h.handMade(t.Aggregate()); err != nil {
			return err
		}
		changed := *h.aggregates[t.Aggregate()]
		changed.Area = area
		h.aggregates[t.Aggregate()] = &changed
		return h.aggregatesChanged()
	}
	return fmt.Errorf("%w: a device, a function, a flag or an aggregate has an area", ErrInvalid)
}

// area is the index of Area id in the order, -1 if none. Callers hold h.mu.
func (h *Home) area(id AreaID) int {
	return slices.IndexFunc(h.areas, func(a Area) bool { return a.ID == id })
}

// areasChanged saves the Areas and announces them, then brings the Area
// Aggregates in line. Callers hold h.mu.
func (h *Home) areasChanged() error {
	var err error
	if h.saveAreas != nil {
		if err = h.saveAreas(h.areas); err != nil {
			log.Printf("home: saving areas: %v", err)
		}
	}
	h.emit(Update{Kind: AreasChanged, Areas: slices.Clone(h.areas)})
	return errors.Join(err, h.rederive())
}
