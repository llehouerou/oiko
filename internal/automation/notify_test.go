package automation

import (
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// A camera's recording Event hands the notify Step the Recording it
// announced; another trigger leaves the Notification without one, and the
// Trace tells why.
func TestANotificationCarriesTheRecordingThatStartedItsRun(t *testing.T) {
	f := fixture()
	f.snap.Devices = append(f.snap.Devices, home.Device{ID: "garden", Name: "Garden", Functions: []home.Function{{Key: "camera", Kind: home.Camera}}})
	var traces []*Trace
	e := New(f, nil, nil, keep(&traces))
	var sent []Notification
	e.NotifyThrough(func(n Notification) { sent = append(sent, n) })
	create(t, e, doc("clips", []string{
		`clip eventTrigger {"target": "device:garden/camera", "capability": "recording", "events": ["person"]}`,
		`btn eventTrigger {"target": "device:switch-alice/button", "capability": "action", "events": ["single"]}`,
		`tell notify {"title": "{name}", "message": "Someone", "video": true}`},
		"clip.out tell.in", "btn.out tell.in"))

	camera := home.TargetDevice("garden", "camera")
	start := time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)
	f.emit(e, home.Update{Kind: home.EventOccurred, Ref: &home.Ref{Target: camera, Capability: bridge.RecordingEvent},
		Value: &home.Value{Data: "person", At: start}})
	f.emit(e, home.Update{Kind: home.EventOccurred, Ref: &home.Ref{Target: camera, Capability: bridge.RecordingEvent},
		Value: &home.Value{Data: "motion", At: start}}) // not one it watches
	if len(sent) != 1 || sent[0].Title != "Garden" || sent[0].Recording == nil || *sent[0].Recording != (RecordingAt{camera, start}) {
		t.Fatalf("sent %+v", sent)
	}
	if r := traces[0].Steps[1]; r.Error != "" || r.Notification.Recording == nil {
		t.Errorf("trace %+v", r)
	}

	press(e, f, "switch-alice", "single")
	if len(sent) != 2 || sent[1].Recording != nil || sent[1].Message != "Someone" {
		t.Fatalf("from a button: %+v", sent)
	}
	if r := traces[1].Steps[1]; r.Error == "" {
		t.Errorf("from a button, the trace tells nothing: %+v", r)
	}
}
