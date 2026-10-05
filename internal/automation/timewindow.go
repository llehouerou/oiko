package automation

// timeWindowStep fires true or false whether the Run's time of day is within
// its window [from, to), which may wrap midnight: the handle fired is the
// verdict. Params: {from, to}. Handles: in → true, false.
type timeWindowStep struct{ window }

type windowParams struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func parseTimeWindow(s *step, p windowParams, _ *Place) error {
	w, err := parseWindow(p.From, p.To)
	if err != nil {
		return err
	}
	s.kind = &timeWindowStep{w}
	return nil
}

func (s *timeWindowStep) reached(x visit, _ int) {
	if s.contains(x.now()) {
		x.fire(0)
	} else {
		x.fire(1)
	}
}
