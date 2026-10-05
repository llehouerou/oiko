package automation

import (
	"errors"
	"strings"

	"github.com/llehouerou/oiko/internal/home"
)

// Notification is a message an Automation sends the occupant.
type Notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// notifyStep sends a Notification, then goes on whether or not it could.
// "{name}" in it reads the Name of the Target that started the Run.
// Params: {title, message}. Handles: in → then.
type notifyStep struct{ n Notification }

func parseNotify(s *step, p Notification, _ *Place) error {
	if strings.TrimSpace(p.Title+p.Message) == "" {
		return errors.New("a title or a message is required")
	}
	s.kind = &notifyStep{p}
	return nil
}

func (s *notifyStep) reached(x visit, _ int) {
	name := x.e.nameOf(x.e.trace.Trigger.Target)
	n := Notification{strings.ReplaceAll(s.n.Title, "{name}", name), strings.ReplaceAll(s.n.Message, "{name}", name)}
	r := x.record()
	r.Notification = &n
	if x.e.notify == nil {
		r.Error = "notifications are not configured"
	} else {
		x.e.notify(n)
	}
	x.fire(0)
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
