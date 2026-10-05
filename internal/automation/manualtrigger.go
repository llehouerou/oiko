package automation

import (
	"fmt"
	"slices"

	"github.com/llehouerou/oiko/internal/home"
)

// manualTriggerStep starts a Run when the occupant asks for it, from the
// dashboard or the API, through Engine.Trigger. Params: {}. Handles: → out.
type manualTriggerStep struct{}

func parseManualTrigger(s *step, _ struct{}, _ *Place) error {
	s.kind = &manualTriggerStep{}
	return nil
}

// Trigger starts a Run of Automation id from its Manual trigger step, at
// once, and returns how it ended. Only an enabled Automation, neither broken
// nor runaway, runs.
func (e *Engine) Trigger(id, step string) (home.RunEnd, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.find(id)
	if i < 0 {
		return home.RunEnd{}, home.ErrNotFound
	}
	a := e.autos[i]
	j := slices.IndexFunc(a.doc.Steps, func(s Step) bool { return s.ID == step })
	switch {
	case j < 0:
		return home.RunEnd{}, fmt.Errorf("%w: no step %q", home.ErrNotFound, step)
	case a.doc.Steps[j].Kind != manualTrigger:
		return home.RunEnd{}, fmt.Errorf("%w: step %q is not a manual trigger", home.ErrInvalid, step)
	}
	if s := a.status(); s.Status != home.AutomationEnabled {
		return home.RunEnd{}, fmt.Errorf("%w: it is %s", home.ErrNotRunning, s.Status)
	}
	end, ok := e.run(a, j, 0, home.Trigger{Time: e.now()})
	if !ok {
		return home.RunEnd{}, fmt.Errorf("%w: it is %s", home.ErrNotRunning, home.AutomationRunaway)
	}
	return end, nil
}
