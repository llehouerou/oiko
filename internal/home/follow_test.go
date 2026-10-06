package home

import (
	"reflect"
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

func TestFollowGetsEveryUpdateWhileObserversAreDropped(t *testing.T) {
	h, _, updates, _ := setup(t) // an observer that never reads
	var got []Update
	snap, cancel := h.Follow(func(u Update) { got = append(got, u) })
	defer cancel()
	for i := range 3 * subscriberBuffer {
		port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "brightness", Data: float64(i % 200)}}, time.Now())
	}
	if len(got) != 3*subscriberBuffer {
		t.Fatalf("followed %d Updates, want %d", len(got), 3*subscriberBuffer)
	}
	for i, u := range got {
		if u.Seq != snap.Seq+uint64(i)+1 {
			t.Fatalf("Update %d has Seq %d, want %d", i, u.Seq, snap.Seq+uint64(i)+1)
		}
	}
	if !closed(updates) {
		t.Error("the lagging observer should have been dropped")
	}

	cancel()
	port(h).Report("0xbulb", []bridge.Reading{{Function: "light", Capability: "brightness", Data: 1.0}}, time.Now())
	if len(got) != 3*subscriberBuffer {
		t.Error("a cancelled follower still gets Updates")
	}
}

// closed empties ch and reports whether it is closed.
func closed(ch <-chan Update) bool {
	for {
		select {
		case _, open := <-ch:
			if !open {
				return true
			}
		default:
			return false
		}
	}
}

func TestAutomationStatusIsInSnapshotAndUpdates(t *testing.T) {
	h, _, updates, _ := setup(t)
	drain(updates)
	list := []AutomationStatus{
		{ID: "a", Name: "A", Status: AutomationEnabled, ManualTriggers: []ManualTrigger{{Step: "go", Name: "Go"}}},
		{ID: "b", Name: "B", Status: AutomationBroken, Reason: "gone"},
	}
	h.SetAutomationStatus(list)
	us := drain(updates)
	if len(us) != 1 || us[0].Kind != AutomationsChanged || !reflect.DeepEqual(us[0].Automations, list) {
		t.Errorf("Updates = %+v", us)
	}
	if got := snapshot(h).Automations; !reflect.DeepEqual(got, list) {
		t.Errorf("Snapshot automations = %+v", got)
	}
}
