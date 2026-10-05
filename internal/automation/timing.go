package automation

import (
	"cmp"
	"container/heap"
	"errors"
	"fmt"
	"time"
)

// Place is the home's location, which sets the sun's times.
type Place struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Times of day are in seconds since midnight, on the wall clock.
func parseClock(s string) (int, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return clockOf(t), nil
		}
	}
	return 0, fmt.Errorf("bad time of day %q", s)
}

func clockOf(t time.Time) int { return t.Hour()*3600 + t.Minute()*60 + t.Second() }

// window is a time-of-day window [from, to), wrapping midnight if to < from.
type window struct{ from, to int }

func parseWindow(from, to string) (window, error) {
	f, err := parseClock(from)
	if err != nil {
		return window{}, err
	}
	t, err := parseClock(to)
	if err != nil {
		return window{}, err
	}
	if f == t {
		return window{}, errors.New("an empty window")
	}
	return window{f, t}, nil
}

func (w window) contains(t time.Time) bool {
	c := clockOf(t)
	if w.from < w.to {
		return w.from <= c && c < w.to
	}
	return c >= w.from || c < w.to
}

// due is a scheduled Step's pending deadline: a timer's, a held-for's, a
// time or sun trigger's next occurrence, or a presence simulation's next
// switch or draw. Its kind embeds it.
type due struct {
	a     *automation
	step  int
	at    time.Time
	index int // in the heap, or -1 when not pending
}

func (d *due) deadline() *due { return d }

// deadlines is the timer heap, earliest first, then in document order.
type deadlines []*due

func (h deadlines) Len() int { return len(h) }
func (h deadlines) Less(i, j int) bool {
	x, y := h[i], h[j]
	return cmp.Or(x.at.Compare(y.at), cmp.Compare(x.a.order, y.a.order), cmp.Compare(x.step, y.step)) < 0
}
func (h deadlines) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}
func (h *deadlines) Push(x any) {
	d := x.(*due)
	d.index = len(*h)
	*h = append(*h, d)
}
func (h *deadlines) Pop() any {
	old := *h
	d := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	d.index = -1
	return d
}

// schedule sets d to come due at at. Callers hold e.mu.
func (e *Engine) schedule(d *due, at time.Time) {
	e.touch()
	d.at = at
	if d.index < 0 {
		heap.Push(&e.pending, d)
	} else {
		heap.Fix(&e.pending, d.index)
	}
	if d.index == 0 { // earlier than the engine goroutine may be waiting for
		e.poke()
	}
}

// unschedule cancels d, if pending. Callers hold e.mu.
func (e *Engine) unschedule(d *due) {
	if d.index >= 0 {
		e.touch()
		heap.Remove(&e.pending, d.index)
	}
}

// tick runs what has come due, in order, and returns when the next deadline
// is, or the zero time if there is none. Callers hold e.mu.
func (e *Engine) tick() time.Time {
	if len(e.pending) == 0 {
		return time.Time{}
	}
	now := e.now()
	for len(e.pending) > 0 && !e.pending[0].at.After(now) {
		e.expire(heap.Pop(&e.pending).(*due))
	}
	if len(e.pending) == 0 {
		return time.Time{}
	}
	return e.pending[0].at
}

// expire starts the Run d's deadline stands for, if its kind says so; one
// due before the clock started is a catch-up. Callers hold e.mu.
func (e *Engine) expire(d *due) {
	e.touch()
	tg, out, ok := d.a.steps[d.step].kind.(scheduled).expired(e)
	if !ok {
		return
	}
	tg.CatchUp = tg.Time.Before(e.started)
	e.run(d.a, d.step, out, tg)
}
