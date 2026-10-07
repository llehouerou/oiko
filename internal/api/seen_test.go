package api

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/dashboard"
	"github.com/llehouerou/oiko/internal/home"
)

func TestEachLevelSeesTheUpdatesADR0031Allows(t *testing.T) {
	alice := home.Origin{Person: "p1"}
	tiles := []home.AutomationStatus{
		{ID: "a1", Name: "Leave", Status: home.AutomationBroken, Reason: "step \"set\" is broken", Step: "set", ManualTriggers: []home.ManualTrigger{{Step: "go", Name: "Go"}}},
		{ID: "a2", Name: "Night", Status: home.AutomationEnabled},
	}
	shown := []home.UpdateKind{home.DevicesChanged, home.ValueChanged, home.ValueRefreshed, home.EventOccurred,
		home.AvailabilityChanged, home.BridgeChanged, home.CommandChanged, home.AggregatesChanged,
		home.FlagsChanged, home.AreasChanged, home.AutomationsChanged, home.TargetDeleted, home.DeviceReplaced}
	hidden := []home.UpdateKind{home.RunEnded, "made-up"}

	for _, l := range allLevels {
		guest := l == access.Guest
		for _, kind := range append(shown, hidden...) {
			u := home.Update{Seq: 1, Kind: kind, Automations: tiles,
				Command: &home.CommandState{ID: "c1", Status: home.Pending, Origin: alice},
				Run:     &home.RunEnd{Automation: "a1", Trigger: home.Trigger{By: &alice}}}
			v, ok := updateFor(l, u)
			if want := !guest || !strings.Contains(" run made-up ", " "+string(kind)+" "); ok != want {
				t.Errorf("a %s, a %q Update: seen %v, want %v", l, kind, ok, want)
				continue
			}
			if !ok {
				continue
			}
			b, _ := json.Marshal(v)
			if got := strings.Contains(string(b), `"person":"p1"`); got == guest {
				t.Errorf("a %s, a %q Update: %s", l, kind, b)
			}
			if got := strings.Contains(string(b), "Night") || strings.Contains(string(b), "is broken"); got == guest {
				t.Errorf("a %s, a %q Update, its Automations: %s", l, kind, b)
			}
		}
	}
}

func TestAGuestSeesTheTilesOfThePressableAutomationsAndNoOneWhoStartedARun(t *testing.T) {
	alice := home.Origin{Person: "p1"}
	snap := home.Snapshot{Automations: []home.AutomationStatus{
		{ID: "a1", Name: "Leave", Status: home.AutomationRunaway, Reason: "ran away", ManualTriggers: []home.ManualTrigger{{Step: "go", Name: "Go"}}},
		{ID: "a2", Name: "Night", Status: home.AutomationEnabled},
	}}
	end := home.RunEnd{Automation: "a1", Outcome: home.RunActed, Trigger: home.Trigger{Step: "go", By: &alice}}
	for _, l := range allLevels {
		guest := l == access.Guest
		got := snapshotFor(l, snap).Automations
		if guest && (len(got) != 1 || got[0].Name != "Leave" || got[0].Reason != "" || got[0].Status != home.AutomationRunaway) ||
			!guest && len(got) != 2 {
			t.Errorf("a %s's snapshot: %+v", l, got)
		}
		if by := runEndFor(l, end).Trigger.By; (by == nil) != guest {
			t.Errorf("a %s, the Run's end: by %v", l, by)
		}
	}
	if snap.Automations[0].Reason == "" {
		t.Error("the Home's snapshot was redacted in place")
	}
}

func TestADashboardIsFilteredForWhoeverCannotEditIt(t *testing.T) {
	automations := []home.AutomationStatus{
		{ID: "leave", Name: "Leave", ManualTriggers: []home.ManualTrigger{{Step: "go", Name: "Go"}}},
		{ID: "night", Name: "Night"},
	}
	place := home.Place{Width: 1}
	sections := func() []dashboard.Section {
		return []dashboard.Section{
			{ID: "mixed", Columns: 2, Place: place, Tiles: []dashboard.Tile{
				{Target: home.TargetFlag("away"), Place: place},
				{Automation: "leave", Place: home.Place{Col: 1, Width: 1}},
				{Automation: "night", Place: home.Place{Row: 1, Width: 1}},
			}},
			{ID: "night only", Columns: 1, Place: home.Place{Col: 1, Width: 1}, Tiles: []dashboard.Tile{{Automation: "night", Place: place}}},
		}
	}
	ds := []dashboard.Dashboard{
		{ID: "shared", Shared: true, Name: "Evening", Columns: 2, Sections: sections()},
		{ID: "carols", Owner: "carol", Name: "Mine", Columns: 2, Sections: sections()},
	}
	whole := sections()
	tiles := func(d dashboard.Dashboard) (keys []string) {
		for _, sec := range d.Sections {
			keys = append(keys, sec.ID+":")
			for _, t := range sec.Tiles {
				keys = append(keys, t.Target.Key()+t.Automation)
			}
		}
		return keys
	}
	all := tiles(ds[0])
	for _, c := range []struct {
		by                access.Identity
		shared, ownedByIt []string
	}{
		// From a shared Dashboard, a Guest gets the Tiles of the Automations it
		// may press only, an own Section left empty kept; its own comes whole.
		{access.Identity{Kind: access.PersonKind, ID: "carol", Level: access.Guest}, []string{"mixed:", "flag:away", "leave", "night only:"}, all},
		{access.Identity{Kind: access.PersonKind, ID: "bob", Level: access.Member}, all, all},
		{access.Identity{Kind: access.PersonKind, ID: "alice", Level: access.Admin}, all, all},
	} {
		got := dashboardsFor(c.by, ds, automations)
		if s := tiles(got[0]); !slices.Equal(s, c.shared) {
			t.Errorf("a %s, a shared Dashboard: %v, want %v", c.by.Level, s, c.shared)
		}
		if s := tiles(got[1]); !slices.Equal(s, c.ownedByIt) {
			t.Errorf("a %s, Carol's: %v, want %v", c.by.Level, s, c.ownedByIt)
		}
	}
	if !reflect.DeepEqual(ds[0].Sections, whole) {
		t.Error("the Dashboards were filtered in place")
	}
	if dashboardsFor(access.Identity{Kind: access.KioskKind, Level: access.Guest}, nil, automations) != nil {
		t.Error("none became some")
	}
}
