package automation

import (
	"errors"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// valueTriggerStep starts a Run when its Capability's Value comes to satisfy
// its comparison and has held it for heldFor. A change away cancels a
// held-for, and it is checked again when it comes due and when it is
// restored. The engine indexes it by what it watches. Params: {target,
// capability, op, value, heldFor}. Handles: → out.
type valueTriggerStep struct {
	ref     home.Ref
	cmp     comparison
	heldFor time.Duration
	due     // while held for
}

type valueTriggerParams struct {
	compared
	HeldFor float64 `json:"heldFor"`
}

func parseValueTrigger(s *step, p valueTriggerParams, _ *Place) error {
	ref, err := p.ref()
	if err != nil {
		return err
	}
	if p.HeldFor < 0 {
		return errors.New("heldFor must be 0 or more")
	}
	cmp, err := p.comparison()
	if err != nil {
		return err
	}
	s.kind, s.targets = &valueTriggerStep{ref: ref, cmp: cmp, heldFor: seconds(p.HeldFor)}, []home.Target{ref.Target}
	return nil
}

// changed follows a change of the Value it watches, from old to now (nil once
// gone), for trigger t. A first Value fires nothing, and neither does
// replayed retained state. Callers hold e.mu.
func (s *valueTriggerStep) changed(e *Engine, t trigger, old home.Value, first bool, now *home.Value) {
	switch {
	case now == nil || !s.cmp.holds(now.Data):
		e.unschedule(&s.due)
	case first || s.cmp.holds(old.Data): // no change into it
	case s.heldFor > 0:
		e.schedule(&s.due, e.now().Add(s.heldFor))
	default:
		e.run(t.a, t.step, 0, observed(s.ref, *now))
	}
}

func (s *valueTriggerStep) arm(*Engine) {}

// expired fires once held for long enough, if it still holds.
func (s *valueTriggerStep) expired(e *Engine) (home.Trigger, int, bool) {
	v, ok := e.values[s.ref]
	if !ok || !s.cmp.holds(v.Data) {
		return home.Trigger{}, 0, false
	}
	tg := observed(s.ref, v)
	tg.Time = s.at
	return tg, 0, true
}

func (s *valueTriggerStep) save(*StepState) {}

// restore resumes a held-for, unless the Value no longer satisfies the
// comparison.
func (s *valueTriggerStep) restore(e *Engine, st StepState) {
	if s.heldFor == 0 || st.Deadline.IsZero() {
		return
	}
	if v, known := e.values[s.ref]; !known || s.cmp.holds(v.Data) {
		e.schedule(&s.due, st.Deadline)
	}
}

func (s *valueTriggerStep) forget() {}
