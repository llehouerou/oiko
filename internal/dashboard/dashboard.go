// Package dashboard keeps the custom Dashboards in dashboards.json (ADR 0040
// to 0046): shared ones, an Admin's to edit and every Person's to see, and
// personal ones, their owner's alone. It alone checks what a Dashboard may
// hold and who may save it; the HTTP layer only names who asks.
package dashboard

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

// Format is dashboards.json's; format 1.
var Format store.Format

// defaultColumns are a Dashboard's when it is not given any.
const defaultColumns = 2

// Dashboard is a custom Dashboard: shared or whose it is, fixed at creation,
// its Name, and its Sections on a Layout of its columns.
type Dashboard struct {
	ID       string    `json:"id"`
	Shared   bool      `json:"shared,omitempty"`
	Owner    string    `json:"owner,omitempty"` // the Person whose personal Dashboard it is
	Name     string    `json:"name"`
	Columns  int       `json:"columns"`
	Sections []Section `json:"sections"`
}

// EditableBy reports whether by may edit d (ADR 0041): an Admin a shared one,
// its owner a personal one; a Kiosk or a Program none.
func (d Dashboard) EditableBy(by access.Identity) bool {
	if by.Kind != access.PersonKind {
		return false
	}
	if d.Shared {
		return by.Level.Allows(access.Admin)
	}
	return d.Owner == by.ID
}

// Section is an Area's, by its id, which shows what the built-in Dashboard
// shows of it, or one of the Dashboard's own: an optional Name and Icon, and
// its Tiles on a Layout of its columns. Its id names an own Section among the
// others, for its fold. Either sits at its place on the Dashboard's Layout.
type Section struct {
	Area    home.AreaID `json:"area,omitempty"`
	ID      string      `json:"id,omitempty"`
	Name    string      `json:"name,omitempty"`
	Icon    string      `json:"icon,omitempty"`
	Columns int         `json:"columns,omitempty"`
	Tiles   []Tile      `json:"tiles,omitempty"`
	home.Place
}

// Tile is a Tile placed in an own Section: a Target's (a Device, one of its
// Functions, an Aggregate or a Flag) or an Automation's, at its place on the
// Section's Layout.
type Tile struct {
	Target     home.Target `json:"target,omitzero"`
	Automation string      `json:"automation,omitempty"`
	home.Place
}

// document is dashboards.json.
type document struct {
	Dashboards []Dashboard `json:"dashboards"`
}

// Store holds the custom Dashboards, and follows the home to know what they
// may refer to.
type Store struct {
	file    string
	mu      sync.Mutex
	doc     document
	changed map[string]chan struct{} // closed when the Dashboards a Person sees change, by Person id

	kmu   sync.Mutex // under Home's lock: never held while calling Home
	known known
}

// known is what exists in the home, as last announced.
type known struct {
	devices     []home.Device
	aggregates  []home.Aggregate
	flags       []home.Flag
	areas       []home.Area
	automations []home.AutomationStatus
}

// Open loads the Dashboards from dir, and follows h.
func Open(dir string, h *home.Home) (*Store, error) {
	s := &Store{file: filepath.Join(dir, "dashboards.json"), changed: map[string]chan struct{}{}}
	if err := store.Load(s.file, Format, &s.doc); err != nil {
		return nil, fmt.Errorf("loading %s: %w", s.file, err)
	}
	// Held across Follow: an Update announced as it returns waits for the
	// snapshot to be kept first, rather than be overwritten by it.
	s.kmu.Lock()
	defer s.kmu.Unlock()
	snap, _ := h.Follow(s.deliver)
	s.known = known{snap.Devices, snap.Aggregates, snap.Flags, snap.Areas, snap.Automations}
	return s, nil
}

// deliver keeps what exists. It runs under Home's lock.
func (s *Store) deliver(u home.Update) {
	s.kmu.Lock()
	defer s.kmu.Unlock()
	switch u.Kind {
	case home.DevicesChanged:
		s.known.devices = u.Devices
	case home.AggregatesChanged:
		s.known.aggregates = u.Aggregates
	case home.FlagsChanged:
		s.known.flags = u.Flags
	case home.AreasChanged:
		s.known.areas = u.Areas
	case home.AutomationsChanged:
		s.known.automations = u.Automations
	}
}

// Dashboards are those by sees, the shared ones and their own, and a channel
// closed once they change. A Kiosk and a Program have none, and are never
// told.
func (s *Store) Dashboards(by access.Identity) ([]Dashboard, <-chan struct{}) {
	if by.Kind != access.PersonKind {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := []Dashboard{}
	for _, d := range s.doc.Dashboards {
		if d.Shared || d.Owner == by.ID {
			seen = append(seen, d)
		}
	}
	ch := s.changed[by.ID]
	if ch == nil {
		ch = make(chan struct{})
		s.changed[by.ID] = ch
	}
	return seen, ch
}

// Create adds d, shared if it says so, which only an Admin creates, or by's
// personal Dashboard, and answers its id.
func (s *Store) Create(by access.Identity, d Dashboard) (string, error) {
	if err := person(by); err != nil {
		return "", err
	}
	d.ID, d.Owner = uuid.NewV7().String(), ""
	if !d.Shared {
		d.Owner = by.ID
	}
	if !d.EditableBy(by) {
		return "", fmt.Errorf("%w: only an admin creates a shared dashboard", access.ErrRefused)
	}
	d, err := s.valid(d)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return d.ID, s.write(append(slices.Clone(s.doc.Dashboards), d), d)
}

// Save replaces Dashboard id with d, whole, for whoever edits it; shared or
// personal it stays.
func (s *Store) Save(by access.Identity, id string, d Dashboard) error {
	if err := person(by); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.editable(by, id)
	if err != nil {
		return err
	}
	if d, err = s.valid(d); err != nil {
		return err
	}
	was := s.doc.Dashboards[i]
	d.ID, d.Shared, d.Owner = id, was.Shared, was.Owner
	ds := slices.Clone(s.doc.Dashboards)
	ds[i] = d
	return s.write(ds, d)
}

// Delete deletes Dashboard id, for whoever edits it.
func (s *Store) Delete(by access.Identity, id string) error {
	if err := person(by); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.editable(by, id)
	if err != nil {
		return err
	}
	return s.write(slices.Delete(slices.Clone(s.doc.Dashboards), i, i+1), s.doc.Dashboards[i])
}

// person refuses whoever is not a Person: a Kiosk or a Program has no
// Dashboard of its own (ADR 0046).
func person(by access.Identity) error {
	if by.Kind != access.PersonKind {
		return fmt.Errorf("%w: only a signed-in Person has dashboards", access.ErrRefused)
	}
	return nil
}

// editable is the index of Dashboard id, if by may edit it. Callers hold
// s.mu.
func (s *Store) editable(by access.Identity, id string) (int, error) {
	i := slices.IndexFunc(s.doc.Dashboards, func(d Dashboard) bool { return d.ID == id })
	if i < 0 {
		return 0, fmt.Errorf("dashboard %q: %w", id, home.ErrNotFound)
	}
	if d := s.doc.Dashboards[i]; !d.EditableBy(by) {
		if d.Shared {
			return 0, fmt.Errorf("%w: only an admin edits a shared dashboard", access.ErrRefused)
		}
		return 0, fmt.Errorf("%w: only its owner edits a personal dashboard", access.ErrRefused)
	}
	return i, nil
}

// write saves ds, which are then the Dashboards, and tells whoever sees
// changed: every Person if it is shared, its owner otherwise. Callers hold
// s.mu.
func (s *Store) write(ds []Dashboard, changed Dashboard) error {
	if err := store.Save(s.file, Format, document{ds}); err != nil {
		return err
	}
	s.doc.Dashboards = ds
	for p, ch := range s.changed {
		if changed.Shared || p == changed.Owner {
			close(ch)
			delete(s.changed, p)
		}
	}
	return nil
}

// valid is d as saved (ADR 0045): its Name trimmed, two columns if it has
// none, an id for each own Section without one, and what no longer exists
// dropped; refused unless it holds together.
func (s *Store) valid(d Dashboard) (Dashboard, error) {
	s.kmu.Lock()
	k := s.known
	s.kmu.Unlock()
	var err error
	if d.Name, err = home.ValidName(d.Name); err != nil {
		return d, err
	}
	if d.Columns == 0 {
		d.Columns = defaultColumns
	}
	sections := []Section{}
	for _, sec := range d.Sections {
		if sec.Area != "" {
			if sec.ID != "" || sec.Name != "" || sec.Icon != "" || sec.Columns != 0 || sec.Tiles != nil {
				return d, fmt.Errorf("%w: an area's section has nothing of its own", home.ErrInvalid)
			}
			if k.area(sec.Area) {
				sections = append(sections, sec)
			}
			continue
		}
		if sec, err = k.own(sec); err != nil {
			return d, err
		}
		sections = append(sections, sec)
	}
	d.Sections = sections
	return d, home.CheckLayout(d.Columns, d.Sections, func(sec Section) (string, home.Place) {
		if sec.Area != "" {
			return "area " + string(sec.Area), sec.Place
		}
		return "section " + sec.ID, sec.Place
	})
}

// own is own Section sec as saved: its Name trimmed, an id, and its Tiles of
// what still exists, each of one Target or Automation, on its Layout.
func (k known) own(sec Section) (Section, error) {
	var err error
	if sec.Name != "" {
		if sec.Name, err = home.ValidName(sec.Name); err != nil {
			return sec, err
		}
	}
	if err := home.ValidIcon(sec.Icon); err != nil {
		return sec, err
	}
	if sec.ID == "" {
		sec.ID = uuid.NewV7().String()
	}
	tiles := []Tile{}
	for _, t := range sec.Tiles {
		if t.Target.IsZero() == (t.Automation == "") {
			return sec, fmt.Errorf("%w: a tile is a target's or an automation's", home.ErrInvalid)
		}
		if k.target(t.Target) || k.automation(t.Automation) {
			tiles = append(tiles, t)
		}
	}
	sec.Tiles = tiles
	return sec, home.CheckLayout(sec.Columns, sec.Tiles, func(t Tile) (string, home.Place) {
		if t.Automation != "" {
			return "automation " + t.Automation, t.Place
		}
		return t.Target.Key(), t.Place
	})
}

func (k known) area(id home.AreaID) bool {
	return slices.ContainsFunc(k.areas, func(a home.Area) bool { return a.ID == id })
}

func (k known) automation(id string) bool {
	return id != "" && slices.ContainsFunc(k.automations, func(a home.AutomationStatus) bool { return a.ID == id })
}

// target reports whether t exists: a Function only as its Device, whose keys
// come and go, and an Area Aggregate as its Area, whose members do.
func (k known) target(t home.Target) bool {
	switch {
	case t.Device() != "":
		return slices.ContainsFunc(k.devices, func(d home.Device) bool { return d.ID == t.Device() })
	case t.Flag() != "":
		return slices.ContainsFunc(k.flags, func(f home.Flag) bool { return f.ID == t.Flag() })
	case t.Aggregate() != "":
		if area, ok := home.AreaAggregate(t.Aggregate()); ok {
			return k.area(area)
		}
		return slices.ContainsFunc(k.aggregates, func(a home.Aggregate) bool { return a.ID == t.Aggregate() })
	}
	return false
}
