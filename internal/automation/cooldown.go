package automation

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// cooldownStep lets a Run through at most once per duration, per triggering
// target or not. Params: {duration, perTarget}. Handles: in → out.
type cooldownStep struct {
	duration  time.Duration
	perTarget bool
	passed    map[home.Target]time.Time // its latest pass, by triggering target (zero unless per target)
}

type cooldownParams struct {
	Duration  float64 `json:"duration"`
	PerTarget bool    `json:"perTarget"`
}

func parseCooldown(s *step, p cooldownParams, _ *Place) error {
	if p.Duration <= 0 {
		return errors.New("duration must be more than 0")
	}
	s.kind = &cooldownStep{duration: seconds(p.Duration), perTarget: p.PerTarget}
	return nil
}

func (s *cooldownStep) reached(x visit, _ int) {
	var key home.Target
	if s.perTarget {
		key = x.e.trace.Trigger.Target
	}
	now, r := x.now(), x.record()
	if last, ok := s.passed[key]; ok && now.Before(last.Add(s.duration)) {
		r.Action, r.Until = "block", last.Add(s.duration)
		return
	}
	if s.passed == nil {
		s.passed = map[home.Target]time.Time{}
	}
	s.passed[key] = now
	x.e.touch()
	r.Action, r.Until = "pass", now.Add(s.duration)
	x.fire(0)
}

func (s *cooldownStep) save(st *StepState) {
	for t, at := range s.passed {
		st.Passed = append(st.Passed, Passed{t, at})
	}
	slices.SortFunc(st.Passed, func(x, y Passed) int { return strings.Compare(x.Target.Key(), y.Target.Key()) })
}

func (s *cooldownStep) restore(_ *Engine, st StepState) {
	for _, p := range st.Passed {
		if s.passed == nil {
			s.passed = map[home.Target]time.Time{}
		}
		s.passed[p.Target] = p.At
	}
}

func (s *cooldownStep) forget() { s.passed = nil }
