package automation

import (
	"fmt"
	"slices"

	"github.com/llehouerou/oiko/internal/home"
)

// manualTriggerStep starts a Run when a Person, a Kiosk or a Program asks
// for it, from the dashboard or the API, through Engine.Trigger. Params: {}. Handles: → out.
type manualTriggerStep struct{}

func parseManualTrigger(s *step, _ struct{}, _ *Place) error {
	s.kind = &manualTriggerStep{}
	return nil
}

// Trigger starts a Run of Automation id from its Manual trigger step, at
// once, on behalf of by, and returns how it ended. Only an enabled
// Automation, neither broken nor runaway, runs.
func (e *Engine) Trigger(id, step string, by home.Origin) (home.RunEnd, error) {
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
	end, ok := e.run(a, j, 0, home.Trigger{Time: e.now(), By: &by})
	if !ok {
		return home.RunEnd{}, fmt.Errorf("%w: it is %s", home.ErrNotRunning, home.AutomationRunaway)
	}
	return end, nil
}

// Tile is what a Guest sees of an Automation (ADR 0023): its Name, its
// status and its Manual triggers, never how it is built.
type Tile struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Status         home.AutomationState `json:"status"`
	ManualTriggers []ManualTrigger      `json:"manualTriggers"`
}

// ManualTrigger is a Manual trigger Step, by its id and Name.
type ManualTrigger struct {
	Step string `json:"step"`
	Name string `json:"name"`
}

// Tiles returns every Automation's Tile, in document order.
func (e *Engine) Tiles() []Tile {
	e.mu.Lock()
	defer e.mu.Unlock()
	tiles := make([]Tile, len(e.autos))
	for i, a := range e.autos {
		tiles[i] = Tile{ID: a.doc.ID, Name: a.doc.Name, Status: a.status().Status, ManualTriggers: []ManualTrigger{}}
		for _, s := range a.doc.Steps {
			if s.Kind == manualTrigger {
				tiles[i].ManualTriggers = append(tiles[i].ManualTriggers, ManualTrigger{s.ID, s.Name})
			}
		}
	}
	return tiles
}
