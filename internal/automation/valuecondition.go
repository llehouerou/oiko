package automation

import "github.com/llehouerou/oiko/internal/home"

// valueConditionStep fires true or false whether the last known Value of its
// Capability satisfies its comparison, and neither when there is none.
// Params: {target, capability, op, value}. Handles: in → true, false.
type valueConditionStep struct {
	ref home.Ref
	cmp comparison
}

func parseValueCondition(s *step, p compared, _ *Place) error {
	ref, err := p.ref()
	if err != nil {
		return err
	}
	cmp, err := p.comparison()
	if err != nil {
		return err
	}
	s.kind, s.targets = &valueConditionStep{ref, cmp}, []home.Target{ref.Target}
	return nil
}

func (s *valueConditionStep) reached(x visit, _ int) {
	v, known := x.e.values[s.ref]
	r := x.record()
	r.Read, r.At, r.Unknown = v.Data, v.At, !known
	switch {
	case !known:
	case s.cmp.holds(v.Data):
		x.fire(0)
	default:
		x.fire(1)
	}
}
