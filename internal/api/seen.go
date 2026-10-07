package api

import (
	"slices"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/dashboard"
	"github.com/llehouerou/oiko/internal/home"
)

// What each Access level sees of the home (ADR 0031): a Member and an Admin
// see all of it; a Guest only what it can press, never who did what.

// snapshotFor is snap as level l sees it: of the Automations, a Guest sees
// only their Tiles, those with a Manual trigger.
func snapshotFor(l access.Level, snap home.Snapshot) home.Snapshot {
	if !l.Allows(access.Member) {
		snap.Automations = pressable(snap.Automations)
	}
	return snap
}

// updateFor is u as level l sees it, false if not at all. A Guest sees only
// the kinds listed here, so that one added later stays hidden until decided,
// and whatever their kind: the Automations as their Tiles, no Run, a Command
// without its Origin.
func updateFor(l access.Level, u home.Update) (any, bool) {
	if l.Allows(access.Member) {
		return u, true
	}
	switch u.Kind {
	case home.DevicesChanged, home.ValueChanged, home.ValueRefreshed, home.EventOccurred,
		home.AvailabilityChanged, home.BridgeChanged, home.CommandChanged, home.AggregatesChanged,
		home.FlagsChanged, home.AreasChanged, home.AutomationsChanged, home.TargetDeleted, home.DeviceReplaced:
	default: // home.RunEnded, and any kind not decided yet
		return nil, false
	}
	if u.Automations != nil {
		u.Automations = pressable(u.Automations)
	}
	u.Run = nil
	if u.Command == nil {
		return u, true
	}
	type command struct {
		home.CommandState
		Origin *struct{} `json:"origin,omitempty"` // hides the Origin: never set
	}
	return struct {
		home.Update
		Command command `json:"command"`
	}{u, command{CommandState: *u.Command}}, true
}

// runEndFor is end as level l sees it: a Guest who started a Run learns how
// it ended, not who started it.
func runEndFor(l access.Level, end home.RunEnd) home.RunEnd {
	if !l.Allows(access.Member) {
		end.Trigger.By = nil
	}
	return end
}

// dashboardsFor is ds as by sees them (ADR 0041): those by edits whole, so
// that saving one never erases what by no longer sees; from the others, the
// Tiles by's level may not see are left out without a trace, an own Section
// left empty kept, its cells empty. automations are all of the home's.
func dashboardsFor(by access.Identity, ds []dashboard.Dashboard, automations []home.AutomationStatus) []dashboard.Dashboard {
	if by.Level.Allows(access.Member) {
		return ds
	}
	seen := map[string]bool{}
	for _, a := range pressable(automations) {
		seen[a.ID] = true
	}
	ds = slices.Clone(ds)
	for i, d := range ds {
		if d.EditableBy(by) {
			continue
		}
		d.Sections = slices.Clone(d.Sections)
		for j, sec := range d.Sections {
			d.Sections[j].Tiles = slices.DeleteFunc(slices.Clone(sec.Tiles), func(t dashboard.Tile) bool {
				return t.Automation != "" && !seen[t.Automation]
			})
		}
		ds[i] = d
	}
	return ds
}

// pressable is what a Guest sees of the Automations: their Tiles, those with
// a Manual trigger only, and their state without its cause.
func pressable(list []home.AutomationStatus) []home.AutomationStatus {
	tiles := []home.AutomationStatus{}
	for _, s := range list {
		if len(s.ManualTriggers) > 0 {
			tiles = append(tiles, home.AutomationStatus{ID: s.ID, Name: s.Name, Status: s.Status, ManualTriggers: s.ManualTriggers})
		}
	}
	return tiles
}
