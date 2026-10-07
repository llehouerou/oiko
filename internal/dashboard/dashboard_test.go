package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	light := home.Capability{Key: "state", Label: "State", Type: home.Binary, Category: home.Primary, Access: home.Access{Observable: true, Settable: true}}
	port.SyncDevices([]bridge.Device{{NativeAddress: "0x1", Name: "Lamp", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []home.Capability{light}}}}})
	h.SetAutomationStatus([]home.AutomationStatus{{ID: "night", Name: "Night"}})
	snap, _, cancel := h.Subscribe()
	cancel()
	return living{h, area, home.TargetDevice(snap.Devices[0].ID, ""), home.TargetFlag(flag), string(other)}
}

func opened(t *testing.T, dir string, h *home.Home) *Store {
	t.Helper()
	s, err := Open(dir, h)
	if err != nil {
		t.Fatal(err)
	}
	return s
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
