package automation

import (
	"errors"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// timerStep fires duration after start; a start while it runs restarts it or
// keeps it, and cancel stops it without firing. Params: {duration, reentry}.
// Handles: start, cancel → fired.
type timerStep struct {
	duration time.Duration
	restart  bool
	due      // while it runs
}

type timerParams struct {
	Duration float64 `json:"duration"`
	Reentry  string  `json:"reentry"`
}

func parseTimer(s *step, p timerParams, _ *Place) error {
	if p.Duration <= 0 {
		return errors.New("duration must be more than 0")
	}
	if p.Reentry != "restart" && p.Reentry != "keep" {
		return errors.New(`reentry must be "restart" or "keep"`)
	}
	s.kind = &timerStep{duration: seconds(p.Duration), restart: p.Reentry == "restart"}
	return nil
}

func (s *timerStep) reached(x visit, in int) {
	r := x.record()
	switch {
	case in == 1:
		x.e.unschedule(&s.due)
		r.Action = "cancel"
	case s.index < 0:
		x.e.schedule(&s.due, x.now().Add(s.duration))
		r.Action, r.Until = "start", s.at
	case s.restart:
		x.e.schedule(&s.due, x.now().Add(s.duration))
		r.Action, r.Until = "restart", s.at
	default:
		r.Action, r.Until = "keep", s.at
	}
}

func (s *timerStep) arm(*Engine) {}

func (s *timerStep) expired(*Engine) (home.Trigger, int, bool) {
	return home.Trigger{Time: s.at}, 0, true
}

func (s *timerStep) save(*StepState) {}

func (s *timerStep) restore(e *Engine, st StepState) {
	if !st.Deadline.IsZero() {
		e.schedule(&s.due, st.Deadline)
	}
}

func (s *timerStep) forget() {}
