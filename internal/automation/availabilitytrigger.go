package automation

import (
	"errors"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// availabilityTriggerStep starts a Run when a Target's Availability comes to
// be, or not to be, one state and has stayed so for heldFor. Its target owns
// an Availability: a Device, an Aggregate or a Flag. Leaving unknown is the
// Availability becoming known, not a change: it fires nothing, as a first
// Value doesn't. Params: {target, op (eq or ne), availability, heldFor}.
// Handles: → out.
type availabilityTriggerStep struct {
	target  home.Target
	cmp     comparison
	heldFor time.Duration
	due     // while held for
}

type availabilityTriggerParams struct {
	Target       home.Target       `json:"target"`
	Op           string            `json:"op"`
	Availability home.Availability `json:"availability"`
	HeldFor      float64           `json:"heldFor"`
}

func parseAvailabilityTrigger(s *step, p availabilityTriggerParams, _ *Place) error {
	switch {
	case p.Target.IsZero():
		return errors.New("a target is required")
	case p.Target.Function() != "":
		return errors.New("a Function's Availability is its Device's: watch the Device")
	case p.Op != "eq" && p.Op != "ne":
		return errors.New("op must be eq or ne")
	case p.Availability != home.Online && p.Availability != home.Offline && p.Availability != home.Unknown:
		return errors.New("availability must be online, offline or unknown")
	case p.HeldFor < 0:
		return errors.New("heldFor must be 0 or more")
	}
	s.kind = &availabilityTriggerStep{target: p.Target, cmp: comparison{p.Op, string(p.Availability)}, heldFor: seconds(p.HeldFor)}
	s.targets = []home.Target{p.Target}
	return nil
}

func (s *availabilityTriggerStep) holds(a home.Availability) bool { return s.cmp.holds(string(a)) }

// changed follows its target's Availability from old to now, for trigger t.
// Callers hold e.mu.
func (s *availabilityTriggerStep) changed(e *Engine, t trigger, old, now home.Availability) {
	switch {
	case !s.holds(now):
		e.unschedule(&s.due)
	case old == home.Unknown || s.holds(old): // becoming known, or no change into it
	case s.heldFor > 0:
		e.schedule(&s.due, e.now().Add(s.heldFor))
	default:
		e.run(t.a, t.step, 0, s.observed(now, e.now()))
	}
}

func (s *availabilityTriggerStep) observed(a home.Availability, at time.Time) home.Trigger {
	return home.Trigger{Target: s.target, Value: string(a), Time: at}
}

func (s *availabilityTriggerStep) arm(*Engine) {}

// expired fires once held for long enough, if it still holds.
func (s *availabilityTriggerStep) expired(e *Engine) (home.Trigger, int, bool) {
	a := e.availabilityOf(s.target)
	if !s.holds(a) {
		return home.Trigger{}, 0, false
	}
	return s.observed(a, s.at), 0, true
}

func (s *availabilityTriggerStep) save(*StepState) {}

// restore resumes a held-for, unless the Availability no longer holds: at
// startup it is mostly unknown still, and checked again when it comes due.
func (s *availabilityTriggerStep) restore(e *Engine, st StepState) {
	if a := e.availabilityOf(s.target); s.heldFor > 0 && !st.Deadline.IsZero() && (a == home.Unknown || s.holds(a)) {
		e.schedule(&s.due, st.Deadline)
	}
}

func (s *availabilityTriggerStep) forget() {}
