package dashboard

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

var (
	alice = access.Identity{Kind: access.PersonKind, ID: "alice", Name: "Alice", Level: access.Guest}
	bob   = access.Identity{Kind: access.PersonKind, ID: "bob", Name: "Bob", Level: access.Admin}
	carol = access.Identity{Kind: access.PersonKind, ID: "carol", Name: "Carol", Level: access.Member}
)

type nopBridge struct{}

func (nopBridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

// living is a home with an Area, a Device in it, a Flag and an Automation.
type living struct {
	h          *home.Home
	port       *home.Port
	area       home.AreaID
	lamp, flag home.Target
	otherArea  string
}

func aHome(t *testing.T) living {
	t.Helper()
	h := home.New(nil)
	area, _ := h.CreateArea("Living room", "sofa")
	other, _ := h.CreateArea("Office", "")
	flag, _ := h.CreateFlag("Away")
	port := h.Attach("z2m", nopBridge{})
	port.SyncDevices([]bridge.Device{lampAt("0x1")})
	h.SetAutomationStatus([]home.AutomationStatus{{ID: "night", Name: "Night"}})
	snap, _, cancel := h.Subscribe()
	cancel()
	return living{h, port, area, home.TargetDevice(snap.Devices[0].ID, ""), home.TargetFlag(flag), string(other)}
}

// lampAt is a lamp at Native Address address.
func lampAt(address string) bridge.Device {
	light := home.Capability{Key: "state", Label: "State", Type: home.Binary, Category: home.Primary, Access: home.Access{Observable: true, Settable: true}}
	return bridge.Device{NativeAddress: address, Name: "Lamp", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []home.Capability{light}}}}
}

// persons and kiosks are the Persons and Kiosks the tests' Dashboards know at
// load.
var (
	persons = []string{"alice", "bob", "carol"}
	kiosks  = []string{"hall", "kitchen"}
)

func opened(t *testing.T, dir string, h *home.Home) *Store {
	t.Helper()
	s, err := Open(dir, h, persons, kiosks)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// list is what Person p's list holds, as "id" or "-id" when hidden.
func list(s *Store, p access.Identity) []string {
	var ids []string
	for _, e := range s.List(p) {
		if e.Hidden {
			ids = append(ids, "-"+e.ID)
		} else {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// parse is a Dashboard from its JSON, as the web client sends it.
func parse(t *testing.T, doc string) Dashboard {
	t.Helper()
	var d Dashboard
	if err := json.Unmarshal([]byte(doc), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAPersonCreatesSavesAndDeletesTheirOwn(t *testing.T) {
	l := aHome(t)
	dir := t.TempDir()
	s := opened(t, dir, l.h)
	mine, changed := s.Dashboards(alice)
	if len(mine) != 0 {
		t.Fatalf("before any: %+v", mine)
	}
	id, err := s.Create(alice, parse(t, `{"name": " Evening ", "sections": [
		{"area": "`+string(l.area)+`", "col": 0, "row": 0, "width": 1},
		{"name": "Favourites", "icon": "sofa", "columns": 2, "col": 1, "row": 0, "width": 1, "height": 2, "tiles": [
			{"target": "`+l.lamp.Key()+`", "col": 0, "row": 0, "width": 2},
			{"automation": "night", "col": 0, "row": 1, "width": 1}
		]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Error("creating one does not tell its owner")
	}
	mine, changed = s.Dashboards(alice)
	if len(mine) != 1 || mine[0].ID != id || mine[0].Owner != "alice" || mine[0].Name != "Evening" || mine[0].Columns != 2 {
		t.Fatalf("created: %+v", mine)
	}
	own := mine[0].Sections[1]
	if own.ID == "" || own.Name != "Favourites" || len(own.Tiles) != 2 || own.Tiles[0].Target != l.lamp {
		t.Errorf("its own Section: %+v", own)
	}
	if theirs, _ := s.Dashboards(bob); len(theirs) != 0 {
		t.Errorf("Bob sees %+v", theirs)
	}

	// Saved whole: its own Section keeps its id, and a restart keeps it all.
	saved := mine[0]
	saved.Name, saved.Columns = "Night", 3
	if err := s.Save(alice, id, saved); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Error("saving one does not tell its owner")
	}
	if again, _ := opened(t, dir, l.h).Dashboards(alice); !reflect.DeepEqual(again, []Dashboard{saved}) {
		t.Errorf("after a restart:\n got %+v\nwant %+v", again, []Dashboard{saved})
	}

	// Another Person, an Admin included, neither saves nor deletes it.
	if err := s.Save(bob, id, Dashboard{}); !errors.Is(err, access.ErrRefused) { // refused before told what is invalid
		t.Errorf("Bob saving it: %v", err)
	}
	if err := s.Delete(bob, id); !errors.Is(err, access.ErrRefused) {
		t.Errorf("Bob deleting it: %v", err)
	}
	if err := s.Delete(alice, "nope"); !errors.Is(err, home.ErrNotFound) {
		t.Errorf("deleting none: %v", err)
	}
	if err := s.Delete(alice, id); err != nil {
		t.Fatal(err)
	}
	if mine, _ := opened(t, dir, l.h).Dashboards(alice); len(mine) != 0 {
		t.Errorf("after deleting it: %+v", mine)
	}
}

func TestAnAdminKeepsSharedDashboardsAndEveryPersonSeesThem(t *testing.T) {
	l := aHome(t)
	dir := t.TempDir()
	s := opened(t, dir, l.h)
	told := map[string]<-chan struct{}{}
	listen := func() {
		for _, p := range []access.Identity{alice, bob, carol} {
			_, told[p.Name] = s.Dashboards(p)
		}
	}
	everyoneTold := func(what string) {
		t.Helper()
		for name, ch := range told {
			select {
			case <-ch:
			default:
				t.Errorf("%s: %s is not told", what, name)
			}
		}
		listen()
	}
	listen()

	// A Member or a Guest creates none; an Admin does.
	for _, by := range []access.Identity{alice, carol} {
		if _, err := s.Create(by, Dashboard{Shared: true, Name: "Evening"}); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s creating one: %v", by.Level, err)
		}
	}
	id, err := s.Create(bob, Dashboard{Shared: true, Owner: "bob", Name: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	everyoneTold("creating one")
	for _, p := range []access.Identity{alice, bob, carol} {
		if ds, _ := s.Dashboards(p); len(ds) != 1 || ds[0].ID != id || !ds[0].Shared || ds[0].Owner != "" {
			t.Errorf("%s sees %+v", p.Name, ds)
		}
	}

	// A Member or a Guest neither saves nor deletes it; an Admin saves it,
	// shared still, whatever the document says.
	for _, by := range []access.Identity{alice, carol} {
		if err := s.Save(by, id, Dashboard{Name: "Mine"}); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s saving it: %v", by.Level, err)
		}
		if err := s.Delete(by, id); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s deleting it: %v", by.Level, err)
		}
	}
	if err := s.Save(bob, id, Dashboard{Name: "Night"}); err != nil {
		t.Fatal(err)
	}
	everyoneTold("saving one")
	if ds, _ := opened(t, dir, l.h).Dashboards(alice); len(ds) != 1 || !ds[0].Shared || ds[0].Owner != "" || ds[0].Name != "Night" {
		t.Errorf("after a restart: %+v", ds)
	}

	// Personal ones stay their owner's, beside the shared ones.
	if _, err := s.Create(alice, Dashboard{Name: "Alice's"}); err != nil {
		t.Fatal(err)
	}
	if ds, _ := s.Dashboards(carol); len(ds) != 1 {
		t.Errorf("Carol sees %+v", ds)
	}
	listen()
	if err := s.Delete(bob, id); err != nil {
		t.Fatal(err)
	}
	everyoneTold("deleting one")
	if ds, _ := s.Dashboards(bob); len(ds) != 0 {
		t.Errorf("after deleting it: %+v", ds)
	}
}

func TestWhoEditsADashboard(t *testing.T) {
	shared, alices := Dashboard{Shared: true}, Dashboard{Owner: "alice"}
	kiosk := access.Identity{Kind: access.KioskKind, ID: "hall", Level: access.Admin}
	for _, c := range []struct {
		d    Dashboard
		by   access.Identity
		want bool
	}{
		{shared, bob, true}, {shared, carol, false}, {shared, alice, false}, {shared, kiosk, false},
		{alices, alice, true}, {alices, bob, false}, {alices, carol, false},
	} {
		if got := c.d.EditableBy(c.by); got != c.want {
			t.Errorf("%+v by %s: %v, want %v", c.d, c.by.ID, got, c.want)
		}
	}
}

func TestOnlyAPersonHasDashboards(t *testing.T) {
	s := opened(t, t.TempDir(), home.New(nil))
	for _, by := range []access.Identity{
		{Kind: access.KioskKind, ID: "hall", Level: access.Member},
		{Kind: access.ProgramKind, ID: "script", Level: access.Admin},
	} {
		if _, err := s.Create(by, Dashboard{Name: "Wall"}); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s creating one: %v", by.Kind, err)
		}
	}
}

func TestASaveIsRefusedUnlessItHoldsTogether(t *testing.T) {
	l := aHome(t)
	s := opened(t, t.TempDir(), l.h)
	area := `"area": "` + string(l.area) + `"`
	lamp := `"target": "` + l.lamp.Key() + `"`
	for name, doc := range map[string]string{
		"no name":               `{"name": " "}`,
		"a name too long":       `{"name": "` + strings.Repeat("x", 101) + `"}`,
		"too many columns":      `{"name": "E", "columns": 7}`,
		"an Area's with a name": `{"name": "E", "sections": [{` + area + `, "name": "Mine", "col": 0, "row": 0, "width": 1}]}`,
		"an Area twice":         `{"name": "E", "sections": [{` + area + `, "col": 0, "row": 0, "width": 1}, {` + area + `, "col": 1, "row": 0, "width": 1}]}`,
		"a Section outside":     `{"name": "E", "sections": [{` + area + `, "col": 1, "row": 0, "width": 2}]}`,
		"overlapping Sections":  `{"name": "E", "sections": [{` + area + `, "col": 0, "row": 0, "width": 1, "height": 2}, {"columns": 1, "col": 0, "row": 1, "width": 1}]}`,
		"an unplaced Section":   `{"name": "E", "sections": [{` + area + `}]}`,
		"a bad Icon":            `{"name": "E", "sections": [{"icon": "Sofa!", "columns": 1, "col": 0, "row": 0, "width": 1}]}`,
		"a Section name long":   `{"name": "E", "sections": [{"name": "` + strings.Repeat("x", 101) + `", "columns": 1, "col": 0, "row": 0, "width": 1}]}`,
		"no columns of its own": `{"name": "E", "sections": [{"col": 0, "row": 0, "width": 1}]}`,
		"a Tile twice":          `{"name": "E", "sections": [{"columns": 2, "col": 0, "row": 0, "width": 1, "tiles": [{` + lamp + `, "col": 0, "row": 0, "width": 1}, {` + lamp + `, "col": 1, "row": 0, "width": 1}]}]}`,
		"a Function twice":      `{"name": "E", "sections": [{"columns": 2, "col": 0, "row": 0, "width": 1, "tiles": [{"target": "` + l.lamp.Key() + `/light", "col": 0, "row": 0, "width": 1}, {"target": "` + l.lamp.Key() + `/light", "col": 1, "row": 0, "width": 1}]}]}`,
		"overlapping Tiles":     `{"name": "E", "sections": [{"columns": 2, "col": 0, "row": 0, "width": 1, "tiles": [{` + lamp + `, "col": 0, "row": 0, "width": 2}, {"automation": "night", "col": 1, "row": 0, "width": 1}]}]}`,
		"too many of its own":   `{"name": "E", "sections": [{"columns": 7, "col": 0, "row": 0, "width": 1}]}`,
		"a Tile outside":        `{"name": "E", "sections": [{"columns": 1, "col": 0, "row": 0, "width": 1, "tiles": [{` + lamp + `, "col": 0, "row": 0, "width": 2}]}]}`,
		"a Tile of nothing":     `{"name": "E", "sections": [{"columns": 1, "col": 0, "row": 0, "width": 1, "tiles": [{"col": 0, "row": 0, "width": 1}]}]}`,
		"a Tile of two things":  `{"name": "E", "sections": [{"columns": 1, "col": 0, "row": 0, "width": 1, "tiles": [{` + lamp + `, "automation": "night", "col": 0, "row": 0, "width": 1}]}]}`,
		"two own with one id":   `{"name": "E", "sections": [{"id": "s", "columns": 1, "col": 0, "row": 0, "width": 1}, {"id": "s", "columns": 1, "col": 1, "row": 0, "width": 1}]}`,
	} {
		if _, err := s.Create(alice, parse(t, doc)); !errors.Is(err, home.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
}

func TestADeviceAndOneOfItsFunctionsAreTwoTiles(t *testing.T) {
	l := aHome(t)
	dir := t.TempDir()
	s := opened(t, dir, l.h)
	// The key is not checked against the Device: one it has, one it may have later.
	id, err := s.Create(alice, parse(t, `{"name": "E", "sections": [{"columns": 3, "col": 0, "row": 0, "width": 1, "tiles": [
		{"target": "`+l.lamp.Key()+`", "col": 0, "row": 0, "width": 1},
		{"target": "`+l.lamp.Key()+`/light", "col": 1, "row": 0, "width": 1},
		{"target": "`+l.lamp.Key()+`/switch/l2", "col": 2, "row": 0, "width": 1}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mine, _ := opened(t, dir, l.h).Dashboards(alice) // kept across a restart
	var kept []string
	for _, tile := range mine[0].Sections[0].Tiles {
		kept = append(kept, tile.Target.Key())
	}
	want := []string{l.lamp.Key(), l.lamp.Key() + "/light", l.lamp.Key() + "/switch/l2"}
	if mine[0].ID != id || !reflect.DeepEqual(kept, want) {
		t.Errorf("its Tiles kept: %v, want %v", kept, want)
	}
}

func TestWhatIsGoneIsDroppedOnASave(t *testing.T) {
	l := aHome(t)
	s := opened(t, t.TempDir(), l.h)
	other := home.AreaID(l.otherArea)
	if err := l.h.DeleteArea(other); err != nil {
		t.Fatal(err)
	}
	gone := map[string]string{
		"area":       `{"area": "` + l.otherArea + `", "col": 1, "row": 0, "width": 1}`,
		"device":     `{"target": "device:gone", "col": 0, "row": 0, "width": 1}`,
		"aggregate":  `{"target": "aggregate:gone", "col": 0, "row": 1, "width": 1}`,
		"flag":       `{"target": "flag:gone", "col": 0, "row": 2, "width": 1}`,
		"automation": `{"automation": "gone", "col": 0, "row": 3, "width": 1}`,
		"area agg.":  `{"target": "aggregate:` + l.otherArea + `.light", "col": 0, "row": 4, "width": 1}`,
	}
	id, err := s.Create(alice, parse(t, `{"name": "E", "sections": [
		{"area": "`+string(l.area)+`", "col": 0, "row": 0, "width": 1}, `+gone["area"]+`,
		{"columns": 2, "col": 0, "row": 1, "width": 2, "tiles": [
			`+gone["device"]+`, `+gone["aggregate"]+`, `+gone["flag"]+`, `+gone["automation"]+`, `+gone["area agg."]+`,
			{"target": "`+l.lamp.Key()+`/light", "col": 1, "row": 0, "width": 1},
			{"target": "`+l.flag.Key()+`", "col": 1, "row": 1, "width": 1},
			{"target": "aggregate:`+string(l.area)+`.light", "col": 1, "row": 2, "width": 1},
			{"automation": "night", "col": 1, "row": 3, "width": 1}
		]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	mine, _ := s.Dashboards(alice)
	d := mine[0]
	if d.ID != id || len(d.Sections) != 2 || d.Sections[0].Area != l.area {
		t.Fatalf("its Sections: %+v", d.Sections)
	}
	var kept []string
	for _, tile := range d.Sections[1].Tiles {
		kept = append(kept, tile.Target.Key()+tile.Automation)
	}
	want := []string{l.lamp.Key() + "/light", l.flag.Key(), "aggregate:" + string(l.area) + ".light", "night"}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("its Tiles kept: %v, want %v", kept, want)
	}
}

func TestEachPersonOrdersTheDashboardsTheySee(t *testing.T) {
	dir := t.TempDir()
	s := opened(t, dir, home.New(nil))
	want := func(p access.Identity, ids ...string) {
		t.Helper()
		if got := list(s, p); !slices.Equal(got, ids) {
			t.Errorf("%s's list: %v, want %v", p.Name, got, ids)
		}
	}
	create := func(by access.Identity, d Dashboard) string {
		t.Helper()
		id, err := s.Create(by, d)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// A new Person's starts with the built-in Dashboard; one created later
	// joins the end of every list it belongs to, shown.
	want(alice, Builtin)
	shared := create(bob, Dashboard{Shared: true, Name: "Evening"})
	alices := create(alice, Dashboard{Name: "Alice's"})
	carols := create(carol, Dashboard{Name: "Carol's"})
	want(alice, Builtin, shared, alices)
	want(carol, Builtin, shared, carols)

	// A list saved is reconciled: what Alice does not see dropped, what she
	// misses appended, shown; she alone is told.
	_, aliceTold := s.Dashboards(alice)
	_, carolTold := s.Dashboards(carol)
	if err := s.SaveList(alice, []Entry{{ID: shared}, {ID: carols}, {ID: "gone"}, {ID: Builtin, Hidden: true}, {ID: shared, Hidden: true}}); err != nil {
		t.Fatal(err)
	}
	want(alice, shared, "-"+Builtin, alices)
	want(carol, Builtin, shared, carols)
	select {
	case <-aliceTold:
	default:
		t.Error("saving her list does not tell Alice")
	}
	select {
	case <-carolTold:
		t.Error("Alice saving her list tells Carol")
	default:
	}

	// Only a list with none shown is refused.
	if err := s.SaveList(alice, []Entry{{ID: shared, Hidden: true}, {ID: Builtin, Hidden: true}, {ID: alices, Hidden: true}}); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("none shown: %v", err)
	}
	if err := s.SaveList(alice, []Entry{{ID: shared}, {ID: Builtin, Hidden: true}, {ID: alices, Hidden: true}}); err != nil {
		t.Fatal(err)
	}

	// A deletion that leaves none shown shows the first one left again.
	if err := s.Delete(bob, shared); err != nil {
		t.Fatal(err)
	}
	want(alice, Builtin, "-"+alices)
	// and keeps it shown once another joins the end.
	later := create(alice, Dashboard{Name: "Later"})
	want(alice, Builtin, "-"+alices, later)
	if got := list(opened(t, dir, home.New(nil)), alice); !slices.Equal(got, []string{Builtin, "-" + alices, later}) {
		t.Errorf("Alice's list after a restart: %v", got)
	}

	// A Kiosk and a Program have no list.
	for _, by := range []access.Identity{
		{Kind: access.KioskKind, ID: "hall", Level: access.Member},
		{Kind: access.ProgramKind, ID: "script", Level: access.Admin},
	} {
		if err := s.SaveList(by, []Entry{{ID: Builtin}}); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s saving a list: %v", by.Kind, err)
		}
	}
}

func TestARemovedPersonTakesTheirDashboardsAndListWithThem(t *testing.T) {
	dir := t.TempDir()
	s := opened(t, dir, home.New(nil))
	shared, err := s.Create(bob, Dashboard{Shared: true, Name: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []access.Identity{alice, carol} {
		if _, err := s.Create(p, Dashboard{Name: p.Name + "'s"}); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveList(p, []Entry{{ID: shared}, {ID: Builtin, Hidden: true}}); err != nil {
			t.Fatal(err)
		}
	}
	// names are the Names of the Dashboards p sees.
	names := func(s *Store, p access.Identity) []string {
		var ns []string
		for _, d := range must(s.Dashboards(p)) {
			ns = append(ns, d.Name)
		}
		return ns
	}
	if err := s.RemovePerson("alice"); err != nil {
		t.Fatal(err)
	}
	reopened := opened(t, dir, home.New(nil))
	if got := list(reopened, alice); !slices.Equal(got, []string{Builtin, shared}) {
		t.Errorf("a removed Person's list: %v", got)
	}
	if got := names(reopened, alice); !slices.Equal(got, []string{"Evening"}) {
		t.Errorf("a removed Person's Dashboards: %v", got)
	}
	if got := names(reopened, carol); !slices.Equal(got, []string{"Evening", "Carol's"}) {
		t.Errorf("another Person's Dashboards: %v", got)
	}

	// Carol, removed while her Dashboards and list were not, loses them on load.
	again, err := Open(dir, home.New(nil), []string{"bob"}, kiosks)
	if err != nil {
		t.Fatal(err)
	}
	if got := list(again, carol); !slices.Equal(got, []string{Builtin, shared}) {
		t.Errorf("a list of a Person no longer known: %v", got)
	}
	if got := names(again, carol); !slices.Equal(got, []string{"Evening"}) {
		t.Errorf("the Dashboards of a Person no longer known: %v", got)
	}
}

// must is ds, without the channel Dashboards answers with them.
func must(ds []Dashboard, _ <-chan struct{}) []Dashboard { return ds }

// held is what Dashboard d holds: each Area's Section as "area:<id>", and
// each own Section as "<id>:" then its Tiles' keys.
func held(d Dashboard) []string {
	var h []string
	for _, sec := range d.Sections {
		if sec.Area != "" {
			h = append(h, "area:"+string(sec.Area))
			continue
		}
		h = append(h, sec.ID+":")
		for _, t := range sec.Tiles {
			h = append(h, t.Target.Key()+t.Automation)
		}
	}
	return h
}

func TestWhatIsDeletedLeavesEveryDashboard(t *testing.T) {
	l := aHome(t)
	dir := t.TempDir()
	s := opened(t, dir, l.h)
	l.port.SyncDevices([]bridge.Device{lampAt("0x1"), lampAt("0x2")})
	var fresh home.Target // the second lamp
	snap, _, cancel := l.h.Subscribe()
	cancel()
	for _, d := range snap.Devices {
		if d.ID != l.lamp.Device() {
			fresh = home.TargetDevice(d.ID, "")
		}
	}
	groupID, err := l.h.CreateAggregate("Lamps", []home.Target{home.TargetDevice(l.lamp.Device(), "light")}, home.Any, home.Mean)
	if err != nil {
		t.Fatal(err)
	}
	group := home.TargetAggregate(groupID)
	officeLights := "aggregate:" + l.otherArea + ".light"

	// Alice's own Dashboard and a shared one hold the same, the shared one
	// shown by a Kiosk.
	doc := `{"name": "E", "sections": [
		{"area": "` + l.otherArea + `", "col": 1, "row": 0, "width": 1},
		{"id": "all", "columns": 4, "col": 0, "row": 0, "width": 1, "tiles": [
			{"target": "` + l.lamp.Key() + `", "col": 0, "row": 0, "width": 1},
			{"target": "` + l.lamp.Key() + `/light", "col": 1, "row": 0, "width": 1},
			{"target": "` + l.lamp.Key() + `/gone", "col": 2, "row": 0, "width": 1},
			{"target": "` + fresh.Key() + `", "col": 3, "row": 0, "width": 1},
			{"target": "` + group.Key() + `", "col": 0, "row": 1, "width": 1},
			{"target": "` + officeLights + `", "col": 1, "row": 1, "width": 1},
			{"automation": "night", "col": 2, "row": 1, "width": 1}]},
		{"id": "flag", "columns": 1, "col": 0, "row": 1, "width": 1, "tiles": [
			{"target": "` + l.flag.Key() + `", "col": 0, "row": 0, "width": 1}]}]}`
	alices, err := s.Create(alice, parse(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := s.Create(bob, parse(t, strings.Replace(doc, `{"name"`, `{"shared": true, "name"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Assign(bob, "hall", shared); err != nil {
		t.Fatal(err)
	}
	mine := func(s *Store, id string) Dashboard {
		for _, d := range must(s.Dashboards(bob)) {
			if d.ID == id {
				return d
			}
		}
		for _, d := range must(s.Dashboards(alice)) {
			if d.ID == id {
				return d
			}
		}
		t.Fatalf("no dashboard %s", id)
		return Dashboard{}
	}
	// after checks that each deletion left both Dashboards holding want,
	// across a restart too, and told their Persons and the Kiosk.
	after := func(what string, del func() error, want ...string) {
		t.Helper()
		told := map[string]<-chan struct{}{}
		for _, by := range []access.Identity{alice, bob, hall} {
			_, told[by.Name] = s.Dashboards(by)
		}
		if err := del(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		for _, id := range []string{alices, shared} {
			if got := held(mine(s, id)); !slices.Equal(got, want) {
				t.Errorf("%s:\n got %v\nwant %v", what, got, want)
			}
			if got := held(mine(opened(t, dir, home.New(nil)), id)); !slices.Equal(got, want) {
				t.Errorf("%s, after a restart: %v", what, got)
			}
		}
		for name, ch := range told {
			select {
			case <-ch:
			default:
				t.Errorf("%s: %s is not told", what, name)
			}
		}
	}
	all := []string{"area:" + l.otherArea, "all:", l.lamp.Key(), l.lamp.Key() + "/light", l.lamp.Key() + "/gone", fresh.Key(), group.Key(), officeLights, "night", "flag:", l.flag.Key()}
	if got := held(mine(s, alices)); !slices.Equal(got, all) {
		t.Fatalf("before any deletion:\n got %v\nwant %v", got, all)
	}
	without := func(gone ...string) []string {
		all = slices.DeleteFunc(all, func(h string) bool { return slices.Contains(gone, h) })
		return all
	}

	// A Replace drops the Device given up, and leaves the kept one alone,
	// Detached, its Function's key gone included.
	l.port.SyncDevices([]bridge.Device{lampAt("0x2")})
	after("a Replace", func() error { return l.h.Replace(l.lamp.Device(), fresh.Device()) }, without(fresh.Key())...)
	l.port.SyncDevices(nil)
	after("a Device deleted", func() error { return l.h.Delete(l.lamp.Device()) },
		without(l.lamp.Key(), l.lamp.Key()+"/light", l.lamp.Key()+"/gone")...)
	after("an Aggregate deleted", func() error { return l.h.DeleteAggregate(groupID) }, without(group.Key())...)
	// An own Section left without Tiles stays.
	after("a Flag deleted", func() error { return l.h.DeleteFlag(l.flag.Flag()) }, without(l.flag.Key())...)
	after("an Area deleted", func() error { return l.h.DeleteArea(home.AreaID(l.otherArea)) }, without("area:"+l.otherArea, officeLights)...)
	after("an Automation deleted", func() error { l.h.SetAutomationStatus(nil); return nil }, without("night")...)
}

var (
	hall    = access.Identity{Kind: access.KioskKind, ID: "hall", Name: "Hall tablet", Level: access.Guest}
	kitchen = access.Identity{Kind: access.KioskKind, ID: "kitchen", Name: "Kitchen tablet", Level: access.Member}
)

// shows is the id of the Dashboard Kiosk k shows: its own, or the built-in one.
func shows(s *Store, k access.Identity) string {
	switch ds, _ := s.Dashboards(k); len(ds) {
	case 0:
		return Builtin
	case 1:
		return ds[0].ID
	default:
		return fmt.Sprintf("%d dashboards", len(ds))
	}
}

func TestAnAdminAssignsAKioskItsDashboard(t *testing.T) {
	dir := t.TempDir()
	s := opened(t, dir, home.New(nil))
	evening, err := s.Create(bob, Dashboard{Shared: true, Name: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	bobs, err := s.Create(bob, Dashboard{Name: "Bob's"})
	if err != nil {
		t.Fatal(err)
	}
	want := func(s *Store, assigned map[string]string) {
		t.Helper()
		if got := s.Assignments(bob); !maps.Equal(got, assigned) {
			t.Errorf("the assignments: %v, want %v", got, assigned)
		}
		for _, k := range []access.Identity{hall, kitchen} {
			if got, want := shows(s, k), cmp.Or(assigned[k.ID], Builtin); got != want {
				t.Errorf("%s shows %s, want %s", k.Name, got, want)
			}
		}
	}
	want(s, map[string]string{})

	// Only an Admin assigns one, and only the built-in one or a shared one.
	for _, by := range []access.Identity{alice, carol, hall, {Kind: access.ProgramKind, ID: "script", Level: access.Admin}} {
		if err := s.Assign(by, "hall", evening); !errors.Is(err, access.ErrRefused) {
			t.Errorf("a %s %s assigning one: %v", by.Level, by.Kind, err)
		}
	}
	for _, id := range []string{bobs, "gone"} {
		if err := s.Assign(bob, "hall", id); !errors.Is(err, home.ErrInvalid) {
			t.Errorf("assigning %s: %v", id, err)
		}
	}

	// Several Kiosks share one; the Kiosk and every Admin are told.
	_, hallTold := s.Dashboards(hall)
	_, bobTold := s.Dashboards(bob)
	_, carolTold := s.Dashboards(carol)
	for _, k := range kiosks {
		if err := s.Assign(bob, k, evening); err != nil {
			t.Fatal(err)
		}
	}
	for name, ch := range map[string]<-chan struct{}{"the Kiosk": hallTold, "an Admin": bobTold, "a Member": carolTold} {
		select {
		case <-ch:
			if name == "a Member" {
				t.Errorf("%s is told", name)
			}
		default:
			if name != "a Member" {
				t.Errorf("%s is not told", name)
			}
		}
	}
	want(s, map[string]string{"hall": evening, "kitchen": evening})

	// Its Dashboard edited reaches the Kiosk.
	_, hallTold = s.Dashboards(hall)
	if err := s.Save(bob, evening, Dashboard{Name: "Night"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-hallTold:
	default:
		t.Error("an edit does not tell the Kiosk")
	}
	if ds, _ := s.Dashboards(hall); ds[0].Name != "Night" {
		t.Errorf("the Hall tablet shows %+v", ds)
	}

	// Assigned the built-in one again, a Kiosk has none of its own.
	if err := s.Assign(bob, "hall", Builtin); err != nil {
		t.Fatal(err)
	}
	want(s, map[string]string{"kitchen": evening})
	want(opened(t, dir, home.New(nil)), map[string]string{"kitchen": evening})

	// Deleted, a shared Dashboard sends its Kiosks back to the built-in one.
	_, kitchenTold := s.Dashboards(kitchen)
	if err := s.Delete(bob, evening); err != nil {
		t.Fatal(err)
	}
	select {
	case <-kitchenTold:
	default:
		t.Error("deleting its Dashboard does not tell the Kiosk")
	}
	want(s, map[string]string{})
	want(opened(t, dir, home.New(nil)), map[string]string{})

	// Only an Admin learns the assignments.
	for _, by := range []access.Identity{alice, carol, hall} {
		if got := s.Assignments(by); got != nil {
			t.Errorf("a %s %s gets %v", by.Level, by.Kind, got)
		}
	}
}

func TestARemovedKioskTakesItsAssignmentWithIt(t *testing.T) {
	dir := t.TempDir()
	s := opened(t, dir, home.New(nil))
	evening, err := s.Create(bob, Dashboard{Shared: true, Name: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range kiosks {
		if err := s.Assign(bob, k, evening); err != nil {
			t.Fatal(err)
		}
	}
	_, bobTold := s.Dashboards(bob)
	if err := s.RemoveKiosk("hall"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-bobTold:
	default:
		t.Error("removing a Kiosk does not tell an Admin")
	}
	if got := opened(t, dir, home.New(nil)).Assignments(bob); !maps.Equal(got, map[string]string{"kitchen": evening}) {
		t.Errorf("once the Hall tablet is removed: %v", got)
	}

	// The Kitchen tablet, removed while its assignment was not, loses it on load.
	again, err := Open(dir, home.New(nil), persons, []string{"hall"})
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Assignments(bob); len(got) != 0 {
		t.Errorf("an assignment of a Kiosk no longer known: %v", got)
	}
}
