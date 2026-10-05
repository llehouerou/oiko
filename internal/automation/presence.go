package automation

import (
	"errors"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// presenceSimulationStep, while enabled, switches on and off at random every
// day, in blocks drawn within its window [from, to). The day's schedule is
// its state. Params: {from, to, minBlocks, maxBlocks, minDuration,
// maxDuration}. Handles: enable, disable → on, off.
type presenceSimulationStep struct {
	window
	blocks    presence
	enabled   bool
	schedule  []Switch  // its switches to come, in order; never modified in place
	windowEnd time.Time // the end of the window they are drawn in
	due                 // its next switch, or the end of its window
}

type presenceParams struct {
	windowParams
	MinBlocks   int     `json:"minBlocks"`
	MaxBlocks   int     `json:"maxBlocks"`
	MinDuration float64 `json:"minDuration"`
	MaxDuration float64 `json:"maxDuration"`
}

func parsePresenceSimulation(s *step, p presenceParams, _ *Place) error {
	w, err := parseWindow(p.From, p.To)
	if err != nil {
		return err
	}
	k := &presenceSimulationStep{window: w}
	if err := k.blocks.parse(p, w); err != nil {
		return err
	}
	s.kind = k
	return nil
}

func (s *presenceSimulationStep) reached(x visit, in int) {
	now, r := x.now(), x.record()
	switch {
	case in == 0 && !s.enabled:
		s.enabled = true
		s.draw(x.e, now)
		r.Action, r.Until = "enable", s.at
	case in == 1 && s.enabled:
		lit := s.contains(now) || len(s.schedule) > 0 && !s.schedule[0].On // in the window, or a block drawn in an earlier one is on
		x.e.unschedule(&s.due)
		s.forget()
		x.e.touch()
		r.Action = "disable"
		if lit {
			x.fire(1)
		}
	}
}

func (s *presenceSimulationStep) arm(*Engine) {}

// expired fires the latest switch due, from on or off.
func (s *presenceSimulationStep) expired(e *Engine) (home.Trigger, int, bool) {
	sw, skipped, ok := s.switchDue(e)
	if !ok {
		return home.Trigger{}, 0, false
	}
	out := 0
	if !sw.On {
		out = 1
	}
	return home.Trigger{Time: sw.At, Skipped: skipped}, out, true
}

func (s *presenceSimulationStep) save(st *StepState) {
	st.Enabled, st.Schedule, st.WindowEnd = s.enabled, s.schedule, s.windowEnd
}

// restore resumes an enabled simulation: the switches due while Oiko was
// down collapse into the latest, due in its place.
func (s *presenceSimulationStep) restore(e *Engine, st StepState) {
	if !st.Enabled {
		return
	}
	s.enabled, s.schedule, s.windowEnd = true, st.Schedule, st.WindowEnd
	at := s.next()
	for _, sw := range s.schedule {
		if !sw.At.After(e.now()) {
			at = sw.At
		}
	}
	e.schedule(&s.due, at)
}

func (s *presenceSimulationStep) forget() {
	s.enabled, s.schedule, s.windowEnd = false, nil, time.Time{}
}

// next is when it is next due: at its next switch, or at the end of its
// window, when it draws the next one.
func (s *presenceSimulationStep) next() time.Time {
	if len(s.schedule) > 0 {
		return s.schedule[0].At
	}
	return s.windowEnd
}

// draw draws its switches for the rest of the window under way at t, or else
// for the next one.
func (s *presenceSimulationStep) draw(e *Engine, t time.Time) {
	from, to := s.around(t)
	if from.Before(t) {
		from = t
	}
	s.schedule, s.windowEnd = s.blocks.draw(e.rand, from, to), to
	e.schedule(&s.due, s.next())
}

// switchDue takes the switches that are due and returns the latest, with how
// many it skips. Once its window is over, it draws the next one.
func (s *presenceSimulationStep) switchDue(e *Engine) (sw Switch, skipped int, ok bool) {
	now := e.now()
	n := 0
	for n < len(s.schedule) && !s.schedule[n].At.After(now) {
		n++
	}
	if n > 0 {
		sw, skipped, ok = s.schedule[n-1], n-1, true
		s.schedule = s.schedule[n:]
	}
	if len(s.schedule) == 0 && !s.windowEnd.After(now) {
		s.draw(e, now)
	} else {
		e.schedule(&s.due, s.next())
	}
	return sw, skipped, ok
}

// presence is how a presence simulation draws its blocks: a number of them
// in [minBlocks, maxBlocks], each lasting [minDuration, maxDuration].
type presence struct {
	minBlocks, maxBlocks     int
	minDuration, maxDuration time.Duration
}

// parse reads q's blocks, which must fit window w.
func (p *presence) parse(q presenceParams, w window) error {
	if q.MinBlocks < 1 || q.MaxBlocks < q.MinBlocks {
		return errors.New("minBlocks must be 1 or more, and maxBlocks at least minBlocks")
	}
	if q.MinDuration <= 0 || q.MaxDuration < q.MinDuration {
		return errors.New("minDuration must be more than 0, and maxDuration at least minDuration")
	}
	*p = presence{q.MinBlocks, q.MaxBlocks, seconds(q.MinDuration), seconds(q.MaxDuration)}
	if length := time.Duration((w.to-w.from+86400)%86400) * time.Second; time.Duration(p.minBlocks)*p.minDuration > length {
		return errors.New("minBlocks of minDuration don't fit the window")
	}
	return nil
}

// Switch is when a presence simulation turns the lights on or off.
type Switch struct {
	At time.Time `json:"at"`
	On bool      `json:"on"`
}

// draw returns the switches of blocks drawn at random within [from, to), in
// order. Blocks that don't fit are dropped, and touching ones merge.
func (p *presence) draw(r *rand.Rand, from, to time.Time) []Switch {
	n := p.minBlocks + r.IntN(p.maxBlocks-p.minBlocks+1)
	var lengths []time.Duration
	free := to.Sub(from)
	for range n {
		l := p.minDuration + time.Duration(r.Int64N(int64(p.maxDuration-p.minDuration)+1))
		if l > free {
			break
		}
		lengths, free = append(lengths, l), free-l
	}
	offsets := make([]time.Duration, len(lengths)) // of each block, less the blocks before it
	for i := range offsets {
		offsets[i] = time.Duration(r.Int64N(int64(free) + 1))
	}
	slices.Sort(offsets)
	var (
		s    []Switch
		busy time.Duration // the blocks before
	)
	for i, l := range lengths {
		start := from.Add(offsets[i] + busy)
		if len(s) > 0 && s[len(s)-1].At.Equal(start) {
			s = s[:len(s)-1]
		} else {
			s = append(s, Switch{start, true})
		}
		busy += l
		s = append(s, Switch{start.Add(l), false})
	}
	return s
}

// around returns the occurrence of w under way at t, or else the next one.
func (w window) around(t time.Time) (from, to time.Time) {
	y, m, d := t.Date()
	for i := -1; ; i++ { // from the day before: w may wrap midnight
		from = time.Date(y, m, d+i, 0, 0, w.from, 0, t.Location())
		to = time.Date(y, m, d+i, 0, 0, w.to, 0, t.Location())
		if w.to < w.from {
			to = time.Date(y, m, d+i+1, 0, 0, w.to, 0, t.Location())
		}
		if t.Before(to) {
			return from, to
		}
	}
}
