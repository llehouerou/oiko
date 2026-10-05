package automation

import (
	"slices"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

var gate = home.TargetDevice("gate", "")

func availability(e *Engine, f *fakeHome, a home.Availability) {
	f.emit(e, home.Update{Kind: home.AvailabilityChanged, Target: gate, Availability: a})
}

// 6d's camera watchdog: a camera not online for 15 minutes is reported once,
// by its Name.
func TestAvailabilityTriggerHeldForNotifies(t *testing.T) {
	f := fixture()
	f.snap.Devices = append(f.snap.Devices, home.Device{ID: "gate", Name: "Gate"})
	f.snap.Availability = map[home.Target]home.Availability{gate: home.Online}
	var traces []*Trace
	e := New(f, nil, nil, nil, nil, nil, keep(&traces))
	c := &clock{at(15, 20, 0)}
	e.now = func() time.Time { return c.t }
	var sent []Notification
	e.NotifyThrough(func(n Notification) { sent = append(sent, n) })
	create(t, e, doc("watchdog", []string{
		`down availabilityTrigger {"target": "device:gate", "op": "ne", "availability": "online", "heldFor": 900}`,
		`tell notify {"title": "Camera down", "message": "{name} silent for 15 min"}`},
		"down.out tell.in"))

	availability(e, f, home.Offline)
	c.advance(e, at(15, 20, 14))
	availability(e, f, home.Online)  // back in time: nothing
	availability(e, f, home.Unknown) // Arlo unreachable: due at 20:29
	c.advance(e, at(15, 20, 20))
	availability(e, f, home.Offline) // known again, still not online: the deadline holds
	c.advance(e, at(15, 20, 30))
	want := []Notification{{"Camera down", "Gate silent for 15 min"}}
	if !slices.Equal(sent, want) {
		t.Fatalf("sent %q, want %q", sent, want)
	}
	if r := traces[0].Steps[1]; *r.Notification != want[0] || r.Error != "" || traces[0].Trigger.Value != "offline" {
		t.Errorf("trace: %+v, triggered by %v", r, traces[0].Trigger.Value)
	}
	c.advance(e, at(15, 22, 0))
	if len(sent) != 1 {
		t.Errorf("reported again: %q", sent)
	}
}

// Leaving unknown is the Availability becoming known, not a change; and
// without a way to send it, a Notification fails its Run.
func TestAvailabilityBecomingKnownFiresNothing(t *testing.T) {
	f := fixture()
	f.snap.Devices = append(f.snap.Devices, home.Device{ID: "gate", Name: "Gate"})
	var traces []*Trace
	e := New(f, nil, nil, nil, nil, nil, keep(&traces))
	create(t, e, doc("offline", []string{
		`off availabilityTrigger {"target": "device:gate", "op": "eq", "availability": "offline"}`,
		`tell notify {"title": "{name} offline"}`},
		"off.out tell.in"))

	availability(e, f, home.Offline) // from unknown
	if len(traces) != 0 {
		t.Fatalf("fired on becoming known")
	}
	availability(e, f, home.Online)
	availability(e, f, home.Offline)
	if len(traces) != 1 || f.runs[0].Outcome != home.RunError || traces[0].Steps[1].Error == "" {
		t.Fatalf("runs %+v", f.runs)
	}
	if n := traces[0].Steps[1].Notification; n == nil || n.Title != "Gate offline" {
		t.Errorf("notification %+v", n)
	}
}
