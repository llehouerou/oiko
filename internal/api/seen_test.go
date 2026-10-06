package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/access"
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
