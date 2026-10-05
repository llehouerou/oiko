package automation

import (
	"errors"

	"github.com/llehouerou/oiko/internal/home"
)

// eventTriggerStep starts a Run when its Capability emits an Event in its
// set. The engine indexes it by what it watches. Params: {target,
// capability, events}. Handles: → out.
type eventTriggerStep struct {
	ref    home.Ref
	events []string
}

type eventTriggerParams struct {
	watched
	Events []string `json:"events"`
}

func parseEventTrigger(s *step, p eventTriggerParams, _ *Place) error {
	ref, err := p.ref()
	if err != nil {
		return err
	}
	if len(p.Events) == 0 {
		return errors.New("no events")
	}
	s.kind, s.targets = &eventTriggerStep{ref, p.Events}, []home.Target{ref.Target}
	return nil
}
