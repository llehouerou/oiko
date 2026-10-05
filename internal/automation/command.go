package automation

import (
	"errors"
	"slices"

	"github.com/llehouerou/oiko/internal/home"
)

// commandStep issues one Command per target, then goes on. Params: {targets,
// values, transition}. Handles: in → then.
type commandStep struct {
	targets []home.Target
	req     home.Request
}

type commandParams struct {
	Targets    []home.Target  `json:"targets"`
	Values     map[string]any `json:"values"`
	Transition float64        `json:"transition"`
}

func parseCommand(s *step, p commandParams, _ *Place) error {
	if len(p.Targets) == 0 || slices.ContainsFunc(p.Targets, home.Target.IsZero) {
		return errors.New("no targets")
	}
	if len(p.Values) == 0 || p.Transition < 0 {
		return errors.New("a command needs values and a transition of 0 or more")
	}
	s.kind = &commandStep{p.Targets, home.Request{Values: p.Values, Transition: seconds(p.Transition)}}
	s.targets = p.Targets
	return nil
}

func (s *commandStep) reached(x visit, _ int) {
	for _, t := range s.targets {
		x.issue(t, s.req)
	}
	x.fire(0)
}
