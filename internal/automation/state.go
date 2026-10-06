package automation

import (
	"bytes"
	"encoding/json"
	"log"
	"path/filepath"
	"slices"
	"time"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// writeDelay coalesces writes of the Step state: at most one per writeDelay,
// and none unless it changed.
const writeDelay = time.Second

// State is what an Automation remembers at runtime, as automation-state.json
// keeps it by Automation id: when it became runaway, and its Steps' state.
type State struct {
	Runaway time.Time   `json:"runawaySince,omitzero"`
	Steps   []StepState `json:"steps,omitempty"`
}

// StepState is what a Step remembers between Runs: its pending deadline, a
// cooldown's latest passes, the latest occurrence a catch-up trigger fired,
// or when it started from, whether a presence simulation is enabled, with its
// switches to come and the end of the window they are drawn in, and a Code
// Step's state.
type StepState struct {
	Step      string          `json:"step"`
	Kind      string          `json:"kind"`
	Deadline  time.Time       `json:"deadline,omitzero"`
	Passed    []Passed        `json:"passed,omitempty"`
	Last      time.Time       `json:"last,omitzero"`
	Enabled   bool            `json:"enabled,omitempty"`
	Schedule  []Switch        `json:"schedule,omitempty"`
	WindowEnd time.Time       `json:"windowEnd,omitzero"`
	State     json.RawMessage `json:"state,omitempty"`
}

// Passed is when a cooldown last let a Run through, for Runs triggered by
// Target (zero unless the cooldown is per target).
type Passed struct {
	Target home.Target `json:"target,omitzero"`
	At     time.Time   `json:"at"`
}

// State returns the state of Automation id's Steps that remember any.
func (e *Engine) State(id string) ([]StepState, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.find(id)
	if i < 0 {
		return nil, home.ErrNotFound
	}
	return append([]StepState{}, e.autos[i].stepStates()...), nil
}

func (a *automation) stepStates() []StepState {
	var list []StepState
	for j, s := range a.steps {
		r, ok := s.kind.(remembering)
		if !ok {
			continue
		}
		st := StepState{Step: a.doc.Steps[j].ID, Kind: a.doc.Steps[j].Kind}
		if t, ok := s.kind.(scheduled); ok && t.deadline().index >= 0 {
			st.Deadline = t.deadline().at
		}
		r.save(&st)
		if !st.Deadline.IsZero() || st.Passed != nil || !st.Last.IsZero() || st.Enabled || st.State != nil {
			list = append(list, st)
		}
	}
	return list
}

// states returns what every Automation remembers, by id. Callers hold e.mu.
func (e *Engine) states() map[string]State {
	m := map[string]State{}
	for _, a := range e.autos {
		if s := (State{Runaway: a.runaway, Steps: a.stepStates()}); !s.Runaway.IsZero() || s.Steps != nil {
			m[a.doc.ID] = s
		}
	}
	return m
}

// restore hands saved over to a's Steps of the same id and kind, whatever
// their params, if a is live. Callers hold e.mu.
func (e *Engine) restore(a *automation, saved []StepState) {
	if !a.indexed {
		return
	}
	for _, s := range saved {
		i := slices.IndexFunc(a.doc.Steps, func(o Step) bool { return o.ID == s.Step })
		if i < 0 || a.doc.Steps[i].Kind != s.Kind {
			continue
		}
		if r, ok := a.steps[i].kind.(remembering); ok {
			r.restore(e, s)
		}
	}
}

// touch notes a possible change of the Step state, to be written a little
// later. Callers hold e.mu.
func (e *Engine) touch() {
	if e.dirty || e.dir == "" {
		return
	}
	e.dirty = true
	if e.flush == nil {
		e.flush = time.AfterFunc(writeDelay, e.Flush)
	} else {
		e.flush.Reset(writeDelay)
	}
}

// Flush writes the Step state now, if it changed since it was last written
// or loaded. Call it on shutdown, once nothing changes it any more.
func (e *Engine) Flush() {
	e.writing.Lock()
	defer e.writing.Unlock()
	e.mu.Lock()
	if !e.dirty {
		e.mu.Unlock()
		return
	}
	e.dirty = false
	s := e.states()
	e.mu.Unlock()
	data, err := json.Marshal(s)
	if err == nil && bytes.Equal(data, e.written) { // such as a deadline restored as it was
		return
	}
	if err == nil {
		err = store.Save(filepath.Join(e.dir, stateFile), stateFormat, s)
	}
	if err != nil {
		log.Printf("automation: saving the Step state: %v", err)
		return
	}
	e.written = data
}
