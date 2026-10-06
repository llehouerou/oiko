package automation

import (
	"errors"
	"strings"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// Notification is a message an Automation sends the home's Telegram chat,
// never a Person, with the video of a Recording when it carries one.
type Notification struct {
	Title     string       `json:"title"`
	Message   string       `json:"message"`
	Recording *RecordingAt `json:"recording,omitempty"`
}

// RecordingAt is the Recording of camera Function Camera that started at
// Start, as the Event announcing it tells (ADR 0039).
type RecordingAt struct {
	Camera home.Target `json:"camera"`
	Start  time.Time   `json:"start"`
}

// notifyStep sends a Notification, then goes on whether or not it could.
// "{name}" in it reads the Name of the Target that started the Run; with
// video, it carries the Recording whose Event started the Run.
// Params: {title, message, video}. Handles: in → then.
type notifyStep struct{ p notifyParams }

type notifyParams struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Video   bool   `json:"video,omitempty"`
}

func parseNotify(s *step, p notifyParams, _ *Place) error {
	if strings.TrimSpace(p.Title+p.Message) == "" {
		return errors.New("a title or a message is required")
	}
	s.kind = &notifyStep{p}
	return nil
}

func (s *notifyStep) reached(x visit, _ int) {
	trigger := x.e.trace.Trigger
	name := x.e.nameOf(trigger.Target)
	n := Notification{Title: strings.ReplaceAll(s.p.Title, "{name}", name), Message: strings.ReplaceAll(s.p.Message, "{name}", name)}
	r := x.record()
	if s.p.Video {
		if trigger.Capability == bridge.RecordingEvent && x.e.isCamera(trigger.Target) {
			n.Recording = &RecordingAt{trigger.Target, trigger.Time}
		} else {
			r.Error = "no video: the Run did not start from a camera's recording Event"
		}
	}
	r.Notification = &n
	if x.e.notify == nil {
		r.Error = "notifications are not configured"
	} else {
		x.e.notify(n)
	}
	x.fire(0)
}

// isCamera reports whether t is a camera Function.
func (e *Engine) isCamera(t home.Target) bool {
	for _, d := range e.devices {
		if d.ID == t.Device() {
			for _, f := range d.Functions {
				if f.Key == t.Function() {
					return f.Kind == home.Camera
				}
			}
		}
	}
	return false
}

// NotifyThrough has Notifications handed to send, which must return at once.
// Without it, a notify Step fails. Call it before Run.
func (e *Engine) NotifyThrough(send func(Notification)) { e.notify = send }

// nameOf is the Name of t, of its Device for a Function; "" for none.
func (e *Engine) nameOf(t home.Target) string {
	for _, d := range e.devices {
		if d.ID == t.Device() {
			return d.Name
		}
	}
	for _, a := range e.aggregates {
		if a.ID == t.Aggregate() {
			return a.Name
		}
	}
	for _, f := range e.flags {
		if f.ID == t.Flag() {
			return f.Name
		}
	}
	return ""
}
