package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// Document is an Automation as authored and stored in automations.json: a
// graph of Steps joined by edges between named handles. It maps 1:1 onto the
// editor's nodes and edges.
type Document struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Steps   []Step `json:"steps"`
	Edges   []Edge `json:"edges"`
}

// Step is one box of the graph. Params depend on Kind.
type Step struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Params   json.RawMessage `json:"params"`
	Position Position        `json:"position"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Edge means "then": reaching handle From fires handle To.
type Edge struct {
	From Port `json:"from"`
	To   Port `json:"to"`
}

type Port struct {
	Step   string `json:"step"`
	Handle string `json:"handle"`
}

// step is a compiled Step, at the same index as its Step in the Document.
type step struct {
	kind    stepKind
	targets []home.Target // that it refers to
	outputs []string      // handle names, shared with kinds unless declared
	next    [][]link      // by output handle, in edge order
}

// link is where an edge leads: a Step, its input handle, and the slot of
// that input among every input of the Automation.
type link struct{ step, in, slot int }

// compile builds the Steps of doc and wires its edges, or returns why doc is
// broken whatever the home holds. Sun triggers need place.
func compile(doc Document, place *Place) ([]step, int, problem) {
	bad := func(step, format string, a ...any) ([]step, int, problem) {
		return nil, 0, problem{step, fmt.Sprintf(format, a...)}
	}
	steps := make([]step, 0, len(doc.Steps))
	byID := map[string]int{}
	firstSlot := make([]int, len(doc.Steps))
	slots := 0
	for i, s := range doc.Steps {
		k, ok := kinds[s.Kind]
		if !ok {
			return bad(s.ID, "step %q: unknown kind %q", s.ID, s.Kind)
		}
		if _, dup := byID[s.ID]; dup {
			return bad(s.ID, "step %q: duplicate id", s.ID)
		}
		byID[s.ID] = i
		st := step{outputs: k.outputs}
		if err := k.parse(&st, s.Params, place); err != nil {
			return bad(s.ID, "step %q: %v", s.ID, err)
		}
		st.next = make([][]link, len(st.outputs))
		steps = append(steps, st)
		firstSlot[i] = slots
		slots += len(k.inputs)
	}
	for _, e := range doc.Edges {
		from, okFrom := byID[e.From.Step]
		to, okTo := byID[e.To.Step]
		if !okFrom || !okTo {
			return bad("", "edge %s → %s: unknown step", e.From.Step, e.To.Step)
		}
		out := slices.Index(steps[from].outputs, e.From.Handle)
		if out < 0 {
			return bad(e.From.Step, "step %q has no output %q", e.From.Step, e.From.Handle)
		}
		k := kinds[doc.Steps[to].Kind]
		if len(k.inputs) == 0 {
			return bad(e.To.Step, "edge into trigger %q", e.To.Step)
		}
		in := slices.Index(k.inputs, e.To.Handle)
		if in < 0 {
			return bad(e.To.Step, "step %q has no input %q", e.To.Step, e.To.Handle)
		}
		steps[from].next[out] = append(steps[from].next[out], link{to, in, firstSlot[to] + in})
	}
	return steps, slots, problem{}
}

// watched is what a trigger or a condition watches: a Capability of a
// Target.
type watched struct {
	Target     home.Target `json:"target"`
	Capability string      `json:"capability"`
}

func (w watched) ref() (home.Ref, error) {
	if w.Target.IsZero() || w.Capability == "" {
		return home.Ref{}, errors.New("a target and a capability are required")
	}
	return w.Target.Ref(w.Capability), nil
}

// compared is what a value trigger or condition compares its Value with.
type compared struct {
	watched
	Op    string `json:"op"`
	Value any    `json:"value"`
}

func (c compared) comparison() (comparison, error) {
	cmp := comparison{c.Op, c.Value}
	return cmp, cmp.check()
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// comparison holds for a Value: op is eq, ne, lt, le, gt or ge.
type comparison struct {
	op    string
	value any
}

func (c comparison) check() error {
	switch c.op {
	case "eq", "ne":
		return nil
	case "lt", "le", "gt", "ge":
		if _, ok := c.value.(float64); !ok {
			return fmt.Errorf("%s compares with a number", c.op)
		}
		return nil
	}
	return fmt.Errorf("unknown comparison %q", c.op)
}

func (c comparison) holds(v any) bool {
	switch c.op {
	case "eq":
		return home.Equal(v, c.value)
	case "ne":
		return !home.Equal(v, c.value)
	}
	x, ok := v.(float64)
	if !ok {
		return false
	}
	y := c.value.(float64)
	switch c.op {
	case "lt":
		return x < y
	case "le":
		return x <= y
	case "gt":
		return x > y
	}
	return x >= y
}
