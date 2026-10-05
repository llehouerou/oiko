package history

import (
	"fmt"
	"math"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// Measure is what a period sums up of a series.
type Measure string

const (
	TimeOn Measure = "time_on" // a binary Value: ms on while online, outside Gaps
	Count  Measure = "count"   // a binary Value's rising edges, or Events by value
	Stats  Measure = "stats"   // a numeric Value: its Spread while online, outside Gaps
	Energy Measure = "energy"  // a counter: its rise, a drop beyond rounding being a reset
)

// suits tells whether m sums up a series of x's kind.
func (m Measure) suits(x Series) bool {
	switch m {
	case TimeOn:
		return x.Type == home.Binary && !x.Stateless
	case Count:
		return x.Stateless || x.Type == home.Binary
	case Stats:
		return x.Type == home.Numeric && !x.Stateless
	case Energy:
		return x.Type == home.Numeric && !x.Stateless && x.Counter
	}
	return false
}

// Item is a series and the measure to sum up of it. An Aggregate's energy is
// that of its Members, its current members' counters, summed: Detached ones
// too, whose History stops growing yet stays what they used.
type Item struct {
	Series
	Measure Measure
	Members []Series
}

// PeriodsQuery asks for Items per period over [From, To): Per is hour, day,
// week (from Monday) or month in local calendar time, or span for the whole
// range as one period.
type PeriodsQuery struct {
	From, To time.Time
	Per      string
	Items    []Item
}

// Periods are the Items' periods, in order; End is where the last ends, and
// First the first point recorded of any Item, if any, Unix ms.
type Periods struct {
	End   int64      `json:"end"`
	First *int64     `json:"first"`
	Items [][]Period `json:"items"`
}

// Period is what a period, from Start (Unix ms), sums up: up to now for the
// current one, which also tells the previous period's. It is incomplete if a
// Gap, offline or unknown time is part of it, and has no Value if nothing of
// it is known and nothing was recorded in it.
type Period struct {
	Start      int64     `json:"start"`
	Value      any       `json:"value"` // ms on, a count, Events by value, a Spread, energy or nil
	Incomplete bool      `json:"incomplete"`
	Previous   *Previous `json:"previous,omitempty"`
}

// Previous is the previous period's value at equal elapsed time, and in all.
type Previous struct {
	Value any `json:"value"`
	Total any `json:"total"`
}

// Spread is a numeric Value's min, time-weighted mean and max.
type Spread struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	Max  float64 `json:"max"`
}

const maxPeriods = 10000

var calendar = map[string]step{"hour": {d: time.Hour}, "day": {days: 1}, "week": {days: 7}, "month": {months: 1}}

// periodBounds cuts [from, to) into periods per, the first at or before
// from, the last at or after to.
func periodBounds(per string, from, to time.Time) ([]int64, error) {
	if per == "span" {
		return []int64{from.UnixNano(), to.UnixNano()}, nil
	}
	st, ok := calendar[per]
	if !ok {
		return nil, fmt.Errorf("%w: no period %q", home.ErrInvalid, per)
	}
	var bounds []int64
	for t := st.floor(from); ; t = st.next(t) {
		if len(bounds) > maxPeriods {
			return nil, fmt.Errorf("%w: more than %d periods", home.ErrInvalid, maxPeriods)
		}
		bounds = append(bounds, t.UnixNano())
		if !t.Before(to) {
			return bounds, nil
		}
	}
}

// previous is where the period before [start, end) starts, and when as much
// of it has elapsed as of [start, end) at now: the same wall-clock time in a
// calendar period of days.
func previous(per string, start, end, now time.Time) (from, until time.Time) {
	st, ok := calendar[per]
	switch {
	case !ok: // a span
		from = start.Add(-end.Sub(start))
		return from, from.Add(now.Sub(start))
	case st.d > 0:
		from = st.floor(start.Add(-1))
		return from, from.Add(now.Sub(start))
	}
	from = st.floor(start.Add(-1))
	y, m, d := from.Date()
	sy, sm, sd := start.Date()
	ny, nm, nd := now.Date()
	days := int(time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC).Sub(time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	until = time.Date(y, m, d+days, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), from.Location())
	if until.After(start) {
		until = start
	}
	return from, until
}

// Periods reads q.
func (s *Store) Periods(q PeriodsQuery) (Periods, error) {
	return s.periods(q, time.Now().UnixNano())
}

func (s *Store) periods(q PeriodsQuery, now int64) (Periods, error) {
	for _, it := range q.Items {
		if it.Members != nil && (it.Measure != Energy || len(it.Members) == 0) {
			return Periods{}, fmt.Errorf("%w: members sum up energy, and there must be some", home.ErrInvalid)
		}
		for _, x := range append([]Series{it.Series}, it.Members...) {
			if !it.Measure.suits(x) {
				return Periods{}, fmt.Errorf("%w: no %q of %s %q", home.ErrInvalid, it.Measure, x.Ref.Target, x.Ref.Capability)
			}
		}
	}
	bounds, err := periodBounds(q.Per, q.From.In(time.Local), q.To.In(time.Local))
	if err != nil {
		return Periods{}, err
	}
	n := len(bounds) - 1
	lo, hi := bounds[0], bounds[n]-1
	current := -1 // the period now is in, if any
	var prev, until int64
	for k := range n {
		if bounds[k] <= now && now < bounds[k+1] {
			current = k
			f, u := previous(q.Per, time.Unix(0, bounds[k]).In(time.Local), time.Unix(0, bounds[k+1]).In(time.Local), time.Unix(0, now).In(time.Local))
			prev, until = f.UnixNano(), u.UnixNano()
			lo = min(lo, prev)
		}
	}
	ids, err := s.seriesIDs()
	if err != nil {
		return Periods{}, err
	}
	gaps, err := s.gaps(lo, hi)
	if err != nil {
		return Periods{}, err
	}
	out := Periods{End: ms(bounds[n]), Items: make([][]Period, len(q.Items))}
	var series []Series
	for _, it := range q.Items {
		series = append(series, it.parts()...)
	}
	if first, err := s.first(ids, series); err != nil {
		return Periods{}, err
	} else if first != math.MaxInt64 {
		out.First = new(ms(first))
	}
	blank := map[home.Target][][2]int64{} // per Target whose Availability counts
	// periodsOf sums up series x as it.
	periodsOf := func(it Item, x Series) ([]Period, error) {
		it.Series = x
		t := availabilityOf(x.Ref.Target)
		if _, ok := blank[t]; !ok {
			ps, err := s.read(ids[seriesKey{t.Key(), ""}], true, lo-1, hi, -1)
			if err != nil {
				return nil, err
			}
			blank[t] = blanks(gaps, offline(decoded(Series{Type: home.Enum}, ps))...)
		}
		// From the point in force just before lo, so that one at lo may be an edge.
		ps, err := s.read(ids[key(x.Ref)], !x.Stateless, lo-1, hi, -1)
		if err != nil {
			return nil, err
		}
		if it.Measure == Energy { // and the reading after hi, to spread the rise up to it
			next, err := s.read(ids[key(x.Ref)], false, hi+1, math.MaxInt64, 1)
			if err != nil {
				return nil, err
			}
			ps = append(ps, next...)
		}
		// Up to now: a reading stamped ahead of the clock has not come yet.
		ps = upTo(decoded(x, ps), now)
		periods := sum(it, ps, bounds, now, blank[t])
		if current >= 0 {
			periods[current].Previous = &Previous{
				// Known as the current period is: from the readings until then.
				Value: sum(it, upTo(ps, until), []int64{prev, until}, now, blank[t])[0].Value,
				Total: sum(it, ps, []int64{prev, bounds[current]}, now, blank[t])[0].Value,
			}
		}
		return periods, nil
	}
	for i, it := range q.Items {
		for _, x := range it.parts() {
			periods, err := periodsOf(it, x)
			if err != nil {
				return Periods{}, err
			}
			out.Items[i] = summed(out.Items[i], periods)
		}
	}
	return out, nil
}

// parts are the series it sums up: its Members' if any, else its own.
func (it Item) parts() []Series {
	if it.Members != nil {
		return it.Members
	}
	return []Series{it.Series}
}

// upTo is the points ps, the earliest first, stamped at or before t.
func upTo(ps []stored, t int64) []stored {
	n := len(ps)
	for n > 0 && ps[n-1].ts > t {
		n--
	}
	return ps[:n]
}

// summed adds the energy of periods b to that of periods a: a period has
// none only if neither has, and is incomplete if either is.
func summed(a, b []Period) []Period {
	if a == nil {
		return b
	}
	plus := func(x, y any) any {
		if x == nil {
			return y
		}
		if y == nil {
			return x
		}
		return x.(float64) + y.(float64)
	}
	for k, p := range b {
		a[k].Value = plus(a[k].Value, p.Value)
		a[k].Incomplete = a[k].Incomplete || p.Incomplete
		if p.Previous != nil { // the same period is current in a
			a[k].Previous.Value = plus(a[k].Previous.Value, p.Previous.Value)
			a[k].Previous.Total = plus(a[k].Previous.Total, p.Previous.Total)
		}
	}
	return a
}

// offline is the spans of availability, the earliest first, where a Target
// was not online. Before its first, nothing is recorded, not even unknown:
// the writer ignores the Follow Snapshot, so a Device that never changed
// since counts as online.
func offline(availability []stored) [][2]int64 {
	var out [][2]int64
	for i, p := range availability {
		if p.value == string(home.Online) {
			continue
		}
		end := int64(math.MaxInt64)
		if i+1 < len(availability) {
			end = availability[i+1].ts
		}
		out = append(out, [2]int64{p.ts, end})
	}
	return out
}

// sum sums up item x's points ps in each period between bounds, up to now,
// blanks left out: Events and rising edges count wherever they are.
func sum(x Item, ps []stored, bounds []int64, now int64, blanks [][2]int64) []Period {
	n := len(bounds) - 1
	out := make([]Period, n)
	known := make([]int64, n) // time known of each period
	if x.Stateless {
		for k, c := range tally(ps, bounds) {
			out[k].Value = c
			known[k] = max(0, min(bounds[k+1], now)-bounds[k]) - covered(blanks, bounds[k], min(bounds[k+1], now))
		}
	} else {
		for k, segs := range hold(ps, bounds, now, blanks) {
			var on int64
			for _, s := range segs {
				known[k] += s.d
				if s.v == true {
					on += s.d
				}
			}
			switch x.Measure {
			case TimeOn:
				out[k].Value = ms(on)
			case Stats:
				if len(segs) > 0 { // else no mean, though a point may be recorded
					b := summary(home.Numeric, 0, segs).(Numeric)
					out[k].Value = Spread{b.Min, b.Mean, b.Max}
				}
			case Count:
				out[k].Value = 0
			case Energy:
				out[k].Value = 0.0
			}
		}
		switch x.Measure {
		case Count:
			edges(ps, bounds, out)
		case Stats:
			extremes(ps, bounds, now, out)
		}
	}
	recorded := make([]bool, n)
	if x.Measure == Energy {
		recorded = spread(ps, bounds, out)
	}
	k := 0
	for _, p := range ps {
		for k < n && bounds[k+1] <= p.ts {
			k++
		}
		if k == n || p.ts >= now {
			break
		}
		recorded[k] = recorded[k] || p.ts >= bounds[k]
	}
	for k := range out {
		out[k].Start = ms(bounds[k])
		out[k].Incomplete = known[k] < min(bounds[k+1], now)-bounds[k]
		if known[k] == 0 && !recorded[k] {
			out[k].Value = nil
		}
	}
	return out
}

// edges counts in each period between bounds the rising edges of the binary
// points ps, the earliest first, into out's Values.
func edges(ps []stored, bounds []int64, out []Period) {
	k := 0
	for i := 1; i < len(ps); i++ {
		p := ps[i]
		for k < len(out) && bounds[k+1] <= p.ts {
			k++
		}
		if k == len(out) {
			return
		}
		if p.ts >= bounds[k] && p.value == true && ps[i-1].value == false {
			out[k].Value = out[k].Value.(int) + 1
		}
	}
}

// resetDrop is the share of a counter's highest reading it must lose to have
// been reset: a smaller drop is the device rounding its reading.
const resetDrop = 0.01

// spread spreads the rise of the counter readings ps, the earliest first,
// between each two of them linearly over the time between them, into the
// periods between bounds, adding it to out's Values. The rise is from the
// highest reading so far, so a drop within rounding uses nothing; a larger
// drop is a reset: the new reading has risen from zero. It tells which
// periods it reached.
// ponytail: the highest reading starts at the first of ps, so a drop within
// rounding just before it counts its rise again; read further back if that
// ever shows.
func spread(ps []stored, bounds []int64, out []Period) []bool {
	n := len(bounds) - 1
	reached := make([]bool, n)
	k := 0
	var high float64 // the highest reading since the last reset
	for i := 1; i < len(ps); i++ {
		a, b := ps[i-1], ps[i]
		if i == 1 {
			high = a.value.(float64)
		}
		var rise float64
		switch v := b.value.(float64); {
		case v < high*(1-resetDrop):
			rise, high = v, v
		case v > high:
			rise, high = v-high, v
		}
		for k < n && bounds[k+1] <= a.ts {
			k++
		}
		for j := k; j < n && bounds[j] < b.ts; j++ {
			if d := min(b.ts, bounds[j+1]) - max(a.ts, bounds[j]); d > 0 {
				v, _ := out[j].Value.(float64)
				out[j].Value = v + rise*float64(d)/float64(b.ts-a.ts)
				reached[j] = true
			}
		}
	}
	return reached
}

// extremes widens each period's Spread in out to the points ps recorded in
// it before now, blank time or not: min and max are of the recorded points.
func extremes(ps []stored, bounds []int64, now int64, out []Period) {
	k := 0
	for _, p := range ps {
		for k < len(out) && bounds[k+1] <= p.ts {
			k++
		}
		if k == len(out) || p.ts >= now {
			return
		}
		if s, ok := out[k].Value.(Spread); ok && p.ts >= bounds[k] {
			v := p.value.(float64)
			s.Min, s.Max = min(s.Min, v), max(s.Max, v)
			out[k].Value = s
		}
	}
}

// covered is how much of [from, to) blanks cover.
func covered(blanks [][2]int64, from, to int64) int64 {
	var d int64
	for _, b := range blanks {
		d += max(0, min(b[1], to)-max(b[0], from))
	}
	return d
}
