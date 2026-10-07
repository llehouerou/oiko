// Package dashboard keeps the custom Dashboards in dashboards.json (ADR 0040
// to 0046): shared ones, an Admin's to edit and every Person's to see, and
// personal ones, their owner's alone; each Person's list of those they see;
// and each Kiosk's assigned Dashboard. It alone checks what a Dashboard may
// hold and who may save it; the HTTP layer only names who asks.
package dashboard

import (
	"fmt"
	"log"
	"maps"
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

// seenBy reports whether Person p sees d: every Person a shared one, its
// owner alone a personal one.
func (d Dashboard) seenBy(p string) bool {
	return d.Shared || d.Owner == p
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

// Builtin is the built-in Dashboard's id: the web client derives it, Oiko
// never stores it, and it is in every list.
const Builtin = "builtin"

// Entry is a Dashboard in a Person's list (ADR 0044), by id, and whether
// they hide it from their menu.
type Entry struct {
	ID     string `json:"id"`
	Hidden bool   `json:"hidden,omitempty"`
}

// document is dashboards.json.
type document struct {
	Dashboards []Dashboard        `json:"dashboards"`
	Lists      map[string][]Entry `json:"lists,omitempty"`  // each Person's, as they last saved it, by Person id
	Kiosks     map[string]string  `json:"kiosks,omitempty"` // each Kiosk's shared Dashboard, by Kiosk id; none: the built-in one
}

// Store holds the custom Dashboards, and follows the home to know what they
// may refer to.
type Store struct {
	file    string
	mu      sync.Mutex // under Home's lock: never held while calling Home
	doc     document
	changed map[who]chan struct{} // closed when what an identity is given changes
	known   known
}

// who is an identity told of changes: a Person, an Admin apart, or a Kiosk.
// A Person changing level is not followed: that ends their streams, which
// listen again under their new level.
type who struct {
	kind  access.Kind
	id    string
	admin bool
}

// whoIs is by, as told of changes.
func whoIs(by access.Identity) who {
	return who{by.Kind, by.ID, admin(by)}
}

// admin reports whether by is an Admin Person: a Kiosk or a Program is none,
// whatever its level.
func admin(by access.Identity) bool {
	return by.Kind == access.PersonKind && by.Level.Allows(access.Admin)
}

// known is what exists in the home, as last announced.
type known struct {
	devices     []home.Device
	aggregates  []home.Aggregate
	flags       []home.Flag
	areas       []home.Area
	automations []home.AutomationStatus
}

// Open loads the Dashboards from dir, dropping the personal Dashboards and
// lists of whoever is not one of persons and the assignments of whatever is
// not one of kiosks, by id, and follows h.
func Open(dir string, h *home.Home, persons, kiosks []string) (*Store, error) {
	s := &Store{file: filepath.Join(dir, "dashboards.json"), changed: map[who]chan struct{}{}}
	if err := store.Load(s.file, Format, &s.doc); err != nil {
		return nil, fmt.Errorf("loading %s: %w", s.file, err)
	}
	if s.doc.Lists == nil {
		s.doc.Lists = map[string][]Entry{}
	}
	if s.doc.Kiosks == nil {
		s.doc.Kiosks = map[string]string{}
	}
	s.doc.Dashboards = slices.DeleteFunc(s.doc.Dashboards, func(d Dashboard) bool { return !d.Shared && !slices.Contains(persons, d.Owner) })
	maps.DeleteFunc(s.doc.Lists, func(p string, _ []Entry) bool { return !slices.Contains(persons, p) })
	maps.DeleteFunc(s.doc.Kiosks, func(k, _ string) bool { return !slices.Contains(kiosks, k) })
	// Held across Follow: an Update announced as it returns waits for the
	// snapshot to be kept first, rather than be overwritten by it.
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, _ := h.Follow(s.deliver)
	s.known = known{snap.Devices, snap.Aggregates, snap.Flags, snap.Areas, snap.Automations}
	return s, nil
}

// deliver keeps what exists, and drops from every Dashboard what no longer
// does (ADR 0045): a Device deleted or given up in a Replace, an Aggregate,
// a Flag, an Area or an Automation. Each Update that deletes one is followed
// by the list it is gone from. It runs under Home's lock.
func (s *Store) deliver(u home.Update) {
	switch u.Kind {
	case home.DevicesChanged, home.AggregatesChanged, home.FlagsChanged, home.AreasChanged, home.AutomationsChanged:
	default:
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	ds := slices.Clone(s.doc.Dashboards)
	var changed []Dashboard
	for i, d := range ds {
		if kept, ok := s.known.pruned(d); ok {
			ds[i] = kept
			changed = append(changed, kept)
		}
	}
	if changed == nil {
		return
	}
	// Nothing is recorded (ADR 0045); a failed write is retried by the next
	// one, which drops what is gone again.
	if err := s.write(ds, s.doc.Lists, changed...); err != nil {
		log.Printf("dashboards: %v", err)
	}
}

// Dashboards are those by sees, and a channel closed once they, by's list, or
// for an Admin the Kiosks' assignments change: for a Person the shared ones
// and their own, for a Kiosk the one it is assigned, none for the built-in
// one. A Program has none, and is never told.
func (s *Store) Dashboards(by access.Identity) ([]Dashboard, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := []Dashboard{}
	switch by.Kind {
	case access.PersonKind:
		for _, d := range s.doc.Dashboards {
			if d.seenBy(by.ID) {
				seen = append(seen, d)
			}
		}
	case access.KioskKind:
		if i := s.index(s.doc.Kiosks[by.ID]); i >= 0 {
			seen = append(seen, s.doc.Dashboards[i])
		}
	default:
		return nil, nil
	}
	w := whoIs(by)
	ch := s.changed[w]
	if ch == nil {
		ch = make(chan struct{})
		s.changed[w] = ch
	}
	return seen, ch
}

// Assignments are each Kiosk's assigned Dashboard, by Kiosk id, those showing
// the built-in one left out, for an Admin Person; nil for anyone else.
func (s *Store) Assignments(by access.Identity) map[string]string {
	if !admin(by) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.doc.Kiosks)
}

// Assign assigns Kiosk k Dashboard id, the built-in one or a shared one, for
// an Admin Person (ADR 0045). It grants nothing: no step-up.
func (s *Store) Assign(by access.Identity, k, id string) error {
	if !admin(by) {
		return fmt.Errorf("%w: only an admin assigns a kiosk its dashboard", access.ErrRefused)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kiosks := maps.Clone(s.doc.Kiosks)
	if id == Builtin {
		delete(kiosks, k)
	} else if i := s.index(id); i >= 0 && s.doc.Dashboards[i].Shared {
		kiosks[k] = id
	} else {
		return fmt.Errorf("%w: a kiosk shows the built-in dashboard or a shared one", home.ErrInvalid)
	}
	doc := s.doc
	doc.Kiosks = kiosks
	return s.save(doc, func(w who) bool { return w.admin || w.kind == access.KioskKind && w.id == k })
}

// RemoveKiosk drops removed Kiosk k's assignment (ADR 0045); the Kiosks are
// written first, and an assignment left behind is dropped on load.
func (s *Store) RemoveKiosk(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.doc.Kiosks[k]; !ok {
		return nil
	}
	doc := s.doc
	doc.Kiosks = maps.Clone(s.doc.Kiosks)
	delete(doc.Kiosks, k)
	return s.save(doc, func(w who) bool { return w.admin })
}

// List is by's list (ADR 0044): the Dashboards by sees, the built-in one
// included, in their order, those missing from what by saved at the end,
// shown. A Kiosk and a Program have none.
func (s *Store) List(by access.Identity) []Entry {
	if by.Kind != access.PersonKind {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, _ := reconciled(s.doc.Dashboards, by.ID, s.doc.Lists[by.ID])
	return list
}

// SaveList saves by's list, reconciled (ADR 0046): what by does not see
// dropped, what it misses appended, shown; refused if it shows none.
func (s *Store) SaveList(by access.Identity, list []Entry) error {
	if err := person(by); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, shows := reconciled(s.doc.Dashboards, by.ID, list)
	if !shows {
		return fmt.Errorf("%w: a list shows at least one dashboard", home.ErrInvalid)
	}
	doc := s.doc
	doc.Lists = maps.Clone(s.doc.Lists)
	doc.Lists[by.ID] = list
	return s.save(doc, func(w who) bool { return w.kind == access.PersonKind && w.id == by.ID })
}

// RemovePerson drops removed Person p's personal Dashboards and list (ADR
// 0045); the Persons are written first, and what is left behind is dropped
// on load.
func (s *Store) RemovePerson(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mine := func(d Dashboard) bool { return !d.Shared && d.Owner == p }
	if _, ok := s.doc.Lists[p]; !ok && !slices.ContainsFunc(s.doc.Dashboards, mine) {
		return nil
	}
	lists := maps.Clone(s.doc.Lists)
	delete(lists, p)
	return s.write(slices.DeleteFunc(slices.Clone(s.doc.Dashboards), mine), lists)
}

// reconciled is list as Person p's, ds the Dashboards: each Dashboard p sees
// once, in list's order, then those list misses, shown, the built-in one
// first; and whether it shows any.
func reconciled(ds []Dashboard, p string, list []Entry) ([]Entry, bool) {
	seen := []string{Builtin}
	for _, d := range ds {
		if d.seenBy(p) {
			seen = append(seen, d.ID)
		}
	}
	out := []Entry{}
	in := func(id string) bool { return slices.ContainsFunc(out, func(e Entry) bool { return e.ID == id }) }
	for _, e := range list {
		if slices.Contains(seen, e.ID) && !in(e.ID) {
			out = append(out, e)
		}
	}
	for _, id := range seen {
		if !in(id) {
			out = append(out, Entry{ID: id})
		}
	}
	return out, slices.ContainsFunc(out, func(e Entry) bool { return !e.Hidden })
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
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.valid(d)
	if err != nil {
		return "", err
	}
	return d.ID, s.write(append(slices.Clone(s.doc.Dashboards), d), s.doc.Lists, d)
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
	return s.write(ds, s.doc.Lists, d)
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
	return s.write(slices.Delete(slices.Clone(s.doc.Dashboards), i, i+1), s.doc.Lists, s.doc.Dashboards[i])
}

// person refuses whoever is not a Person: a Kiosk or a Program has no
// Dashboard of its own (ADR 0046).
func person(by access.Identity) error {
	if by.Kind != access.PersonKind {
		return fmt.Errorf("%w: only a signed-in Person has dashboards", access.ErrRefused)
	}
	return nil
}

// index is the index of Dashboard id; -1 if there is none. Callers hold s.mu.
func (s *Store) index(id string) int {
	return slices.IndexFunc(s.doc.Dashboards, func(d Dashboard) bool { return d.ID == id })
}

// editable is the index of Dashboard id, if by may edit it. Callers hold
// s.mu.
func (s *Store) editable(by access.Identity, id string) (int, error) {
	i := s.index(id)
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

// write saves ds, which are then the Dashboards, and lists, the lists of
// Dashboards, and tells whoever sees one of changed: every Person if it is
// shared, its owner otherwise, and the Kiosks it was assigned. Each list
// follows in the same write (ADR 0044): a Dashboard created joins its end,
// shown, one deleted leaves it, and if that leaves none shown, the first one
// left is shown again. A Kiosk whose Dashboard is deleted shows the built-in
// one again (ADR 0045). Callers hold s.mu.
func (s *Store) write(ds []Dashboard, lists map[string][]Entry, changed ...Dashboard) error {
	reconciledLists := map[string][]Entry{}
	for p, l := range lists {
		l, shows := reconciled(ds, p, l)
		if !shows {
			l[0].Hidden = false
		}
		reconciledLists[p] = l
	}
	was := s.doc.Kiosks
	kiosks := maps.Clone(was)
	maps.DeleteFunc(kiosks, func(_, id string) bool {
		return !slices.ContainsFunc(ds, func(d Dashboard) bool { return d.ID == id })
	})
	return s.save(document{ds, reconciledLists, kiosks}, func(w who) bool {
		return slices.ContainsFunc(changed, func(d Dashboard) bool {
			switch w.kind {
			case access.PersonKind:
				return d.seenBy(w.id)
			case access.KioskKind:
				return was[w.id] == d.ID
			}
			return false
		})
	})
}

// save saves doc, which is then dashboards.json, and tells each identity tell
// reports true for. Callers hold s.mu.
func (s *Store) save(doc document, tell func(who) bool) error {
	if err := store.Save(s.file, Format, doc); err != nil {
		return err
	}
	s.doc = doc
	for w, ch := range s.changed {
		if tell(w) {
			close(ch)
			delete(s.changed, w)
		}
	}
	return nil
}

// valid is d as saved (ADR 0045): its Name trimmed, two columns if it has
// none, an id for each own Section without one, and what no longer exists
// dropped; refused unless it holds together. Callers hold s.mu.
func (s *Store) valid(d Dashboard) (Dashboard, error) {
	var err error
	if d.Name, err = home.ValidName(d.Name); err != nil {
		return d, err
	}
	if d.Columns == 0 {
		d.Columns = defaultColumns
	}
	d.Sections = slices.Clone(d.Sections)
	for i, sec := range d.Sections {
		if sec.Area != "" {
			if sec.ID != "" || sec.Name != "" || sec.Icon != "" || sec.Columns != 0 || sec.Tiles != nil {
				return d, fmt.Errorf("%w: an area's section has nothing of its own", home.ErrInvalid)
			}
			continue
		}
		if d.Sections[i], err = own(sec); err != nil {
			return d, err
		}
	}
	d, _ = s.known.pruned(d)
	for _, sec := range d.Sections {
		if sec.Area != "" {
			continue
		}
		if err := home.CheckLayout(sec.Columns, sec.Tiles, func(t Tile) (string, home.Place) {
			if t.Automation != "" {
				return "automation " + t.Automation, t.Place
			}
			return t.Target.Key(), t.Place
		}); err != nil {
			return d, err
		}
	}
	return d, home.CheckLayout(d.Columns, d.Sections, func(sec Section) (string, home.Place) {
		if sec.Area != "" {
			return "area " + string(sec.Area), sec.Place
		}
		return "section " + sec.ID, sec.Place
	})
}

// own is own Section sec as saved: its Name trimmed, an id, and each Tile of
// one Target or Automation.
func own(sec Section) (Section, error) {
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
	for _, t := range sec.Tiles {
		if t.Target.IsZero() == (t.Automation == "") {
			return sec, fmt.Errorf("%w: a tile is a target's or an automation's", home.ErrInvalid)
		}
	}
	return sec, nil
}

// pruned is d without what no longer exists, and whether it held any: the
// Sections of Areas gone, and the Tiles of Targets and Automations gone. An
// own Section left without Tiles stays; a Function whose key its Device no
// longer has stays placed, showing nothing until the key comes back.
func (k known) pruned(d Dashboard) (Dashboard, bool) {
	gone := false
	sections := []Section{}
	for _, sec := range d.Sections {
		if sec.Area != "" && !k.area(sec.Area) {
			gone = true
			continue
		}
		if sec.Area == "" {
			tiles := slices.DeleteFunc(slices.Clone(sec.Tiles), func(t Tile) bool { return !k.target(t.Target) && !k.automation(t.Automation) })
			gone = gone || len(tiles) != len(sec.Tiles)
			sec.Tiles = tiles
		}
		sections = append(sections, sec)
	}
	d.Sections = sections
	return d, gone
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
