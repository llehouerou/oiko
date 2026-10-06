package history

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/llehouerou/oiko/internal/home"
)

// Series is a series to read: a Ref, and the type of its Capability. A Ref to
// Capability "" is its Target's Availability.
type Series struct {
	Ref       home.Ref
	Type      home.ValueType
	Stateless bool // its points are Events
	Counter   bool // a numeric running total, rising except on reset
}

// Query asks for Series over [From, To], in at most Points points each; with
// Markers, for the Commands and Runs over the range too.
type Query struct {
	From, To time.Time
	Points   int
	Series   []Series
	Markers  bool
}

// Answer is what a Query reads, times in Unix ms. The range starts no
// earlier than the first point recorded of its series, at From. Each series
// reads back as a step function: the point in force at the start comes
// first, even if older. If a series holds more points than the budget, every
// series comes in buckets of size Bucket, cut from local midnight, summing up
// the Values in force from where Oiko knew one until now, Gaps left out.
type Answer struct {
	From   int64   `json:"from"`
	Bucket string  `json:"bucket"` // "" when the points come raw
	Series [][]any `json:"series"` // per Series, in order: Raw points, or buckets
	Gaps   []Gap   `json:"gaps"`
	// The Availability, raw, of every Target involved; a Function's is its
	// Device's.
	Availability map[home.Target][]any `json:"availability"`
	// With Markers, over [From, To], the earliest first: the Commands on the
	// Targets of the Series, and the Runs their Values or Events triggered.
	Commands []home.CommandRecord `json:"commands,omitempty"`
	Runs     []home.RunEnd        `json:"runs,omitempty"`
	// With Markers, the Live views over the range of the cameras of the
	// Devices of the Series, the earliest first.
	LiveViews []LiveView `json:"liveViews,omitempty"`
}

// Raw is a point as recorded, or the Value of a bucket: an enum's state held
// longest, a composite, list or text's last Value.
type Raw struct {
	T int64 `json:"t"`
	V any   `json:"v"`
}

// Numeric is a bucket of a numeric Value: its time-weighted mean, min and max.
type Numeric struct {
	T    int64   `json:"t"`
	Mean float64 `json:"mean"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

// Share is a bucket of a binary Value: the share of time it was on.
type Share struct {
	T  int64   `json:"t"`
	On float64 `json:"on"`
}

// Counts is a bucket of Events: how many of each.
type Counts struct {
	T      int64          `json:"t"`
	Counts map[string]int `json:"counts"`
}

// Gap is a span where Oiko recorded nothing: it was stopped, or Lost points.
type Gap struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Lost  int64 `json:"lost"`
}

// Points reads q.
func (s *Store) Points(q Query) (Answer, error) { return s.points(q, time.Now().UnixNano()) }

func (s *Store) points(q Query, now int64) (Answer, error) {
	ids, err := s.seriesIDs()
	if err != nil {
		return Answer{}, err
	}
	a := Answer{Series: make([][]any, len(q.Series)), Availability: map[home.Target][]any{}}
	lo, hi := q.From.UnixNano(), q.To.UnixNano()
	if first, err := s.first(ids, q.Series); err != nil {
		return Answer{}, err
	} else if first > lo && first < hi {
		lo = first
	}
	a.From = ms(lo)
	read := make([][]stored, len(q.Series))
	err = parallel(len(q.Series), func(i int) (err error) {
		x := q.Series[i]
		read[i], err = s.read(ids[key(x.Ref)], !x.Stateless, lo, hi, q.Points+1)
		return err
	})
	if err != nil {
		return Answer{}, err
	}
	bucketed := false
	for i, x := range q.Series {
		bucketed = bucketed || count(read[i], lo, x.Stateless) > q.Points
	}
	var bounds []int64
	if bucketed {
		a.Bucket, bounds = cut(time.Unix(0, lo).In(time.Local), q.To.In(time.Local), q.Points)
		lo, hi = bounds[0], bounds[len(bounds)-1]-1
	}
	gaps, err := s.gaps(lo, hi)
	if err != nil {
		return Answer{}, err
	}
	if bucketed {
		// Reading rows is the cost: each series is read in as many spans of
		// time as there are cores, at once.
		k := runtime.GOMAXPROCS(0)
		span := (hi-lo)/int64(k) + 1
		parts := make([][]stored, len(q.Series)*k)
		err = parallel(len(parts), func(c int) (err error) {
			x, part := q.Series[c/k], int64(c%k)
			from := lo + part*span
			parts[c], err = s.read(ids[key(x.Ref)], !x.Stateless && part == 0, from, min(from+span-1, hi), -1)
			return err
		})
		if err != nil {
			return Answer{}, err
		}
		hs := blanks(gaps)
		for i, x := range q.Series {
			a.Series[i] = bucket(x, decoded(x, slices.Concat(parts[i*k:(i+1)*k]...)), bounds, now, hs)
		}
	} else {
		for i, x := range q.Series {
			a.Series[i] = raw(decoded(x, read[i]))
		}
	}
	for _, x := range q.Series {
		t := availabilityOf(x.Ref.Target)
		if _, ok := a.Availability[t]; ok {
			continue
		}
		ps, err := s.read(ids[seriesKey{t.Key(), ""}], true, lo, hi, -1)
		if err != nil {
			return Answer{}, err
		}
		a.Availability[t] = raw(decoded(Series{Type: home.Enum}, ps))
	}
	a.Gaps = make([]Gap, len(gaps))
	for i, g := range gaps {
		a.Gaps[i] = Gap{ms(g.Start), ms(g.End), g.Lost}
	}
	if q.Markers {
		if a.Commands, a.Runs, err = s.markers(q); err != nil {
			return Answer{}, err
		}
		if a.LiveViews, err = s.liveViews(q); err != nil {
			return Answer{}, err
		}
	}
	return a, nil
}

// availabilityOf is the Target whose Availability t has: a Function's is its
// Device's.
func availabilityOf(t home.Target) home.Target {
	if t.Function() != "" {
		return home.TargetDevice(t.Device(), "")
	}
	return t
}

// markers reads, over q's range, the Commands on the Targets of q's Series,
// and the Runs their Values or Events triggered.
func (s *Store) markers(q Query) ([]home.CommandRecord, []home.RunEnd, error) {
	args := []any{q.From.UnixNano(), q.To.UnixNano()}
	seen := map[home.Target]bool{}
	for _, x := range q.Series {
		if !seen[x.Ref.Target] {
			seen[x.Ref.Target] = true
			args = append(args, x.Ref.Target.Key())
		}
	}
	in := strings.TrimPrefix(strings.Repeat(",?", len(seen)), ",")
	cs, err := commands(s.db.Query(`SELECT command, status FROM commands WHERE time BETWEEN ? AND ? AND target IN (`+in+`) ORDER BY time`, args...))
	if err != nil {
		return nil, nil, err
	}
	rs, err := runs(s.db.Query(`SELECT trace FROM runs WHERE time BETWEEN ? AND ? AND trigger_target IN (`+in+`) ORDER BY time`, args...))
	return cs, rs, err
}

// liveViews reads the Live views over q's range of the cameras of the
// Devices of q's Series.
func (s *Store) liveViews(q Query) ([]LiveView, error) {
	args := []any{q.To.UnixNano(), q.From.UnixNano()}
	seen := map[home.Target]bool{}
	for _, x := range q.Series {
		if d := x.Ref.Target.Device(); d != "" && !seen[home.TargetDevice(d, "")] {
			seen[home.TargetDevice(d, "")] = true
			args = append(args, home.TargetDevice(d, "").Key())
		}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	in := strings.TrimPrefix(strings.Repeat(",?", len(seen)), ",")
	rows, err := s.db.Query(`SELECT target, start, "end", origin FROM live_views WHERE start <= ? AND "end" >= ? AND device IN (`+in+`) ORDER BY start`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vs []LiveView
	for rows.Next() {
		var target, origin string
		var start, end int64
		if err := rows.Scan(&target, &start, &end, &origin); err != nil {
			return nil, err
		}
		v := LiveView{Start: time.Unix(0, start), End: time.Unix(0, end)}
		if v.Target, err = home.ParseTarget(target); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(origin), &v.Origin); err != nil {
			return nil, err
		}
		vs = append(vs, v)
	}
	return vs, rows.Err()
}

func key(r home.Ref) seriesKey {
	if r.Capability == "" {
		return seriesKey{availabilityOf(r.Target).Key(), ""}
	}
	return seriesKey{r.Target.Key(), r.Capability}
}

// seriesIDs reads the row id of every series.
func (s *Store) seriesIDs() (map[seriesKey]int64, error) {
	rows, err := s.db.Query(`SELECT id, target, capability FROM series`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[seriesKey]int64{}
	for rows.Next() {
		var id int64
		var k seriesKey
		if err := rows.Scan(&id, &k.target, &k.capability); err != nil {
			return nil, err
		}
		ids[k] = id
	}
	return ids, rows.Err()
}

// first is the time of the first point recorded of any of xs, or MaxInt64.
func (s *Store) first(ids map[seriesKey]int64, xs []Series) (int64, error) {
	first := int64(math.MaxInt64)
	for _, x := range xs {
		var ts int64
		switch err := s.db.QueryRow(`SELECT ts FROM points WHERE series = ? ORDER BY ts LIMIT 1`, ids[key(x.Ref)]).Scan(&ts); {
		case err == nil:
			first = min(first, ts)
		case !errors.Is(err, sql.ErrNoRows):
			return 0, err
		}
	}
	return first, nil
}

// read reads the points of series id in [lo, hi], at most limit (-1: no
// limit), after the point in force at lo if inForce.
func (s *Store) read(id int64, inForce bool, lo, hi int64, limit int) ([]stored, error) {
	var ps []stored
	if id == 0 { // nothing recorded yet
		return ps, nil
	}
	after := lo - 1
	if inForce {
		var p stored
		switch err := s.db.QueryRow(`SELECT ts, value FROM points WHERE series = ? AND ts <= ? ORDER BY ts DESC LIMIT 1`, id, lo).Scan(&p.ts, &p.value); {
		case err == nil:
			ps = append(ps, p)
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		after = lo
	}
	rows, err := s.db.Query(`SELECT ts, value FROM points WHERE series = ? AND ts > ? AND ts <= ? ORDER BY ts LIMIT ?`, id, after, hi, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p stored
		if err := rows.Scan(&p.ts, &p.value); err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, rows.Err()
}

// count is how many of ps lie in the range starting at lo, the point in force
// at lo aside.
func count(ps []stored, lo int64, events bool) int {
	if !events && len(ps) > 0 && ps[0].ts <= lo {
		return len(ps) - 1
	}
	return len(ps)
}

// typeOf is the type of x's Values: an Availability is an enum.
func typeOf(x Series) home.ValueType {
	if x.Ref.Capability == "" {
		return home.Enum
	}
	return x.Type
}

// decoded decodes ps as x's Values, skipping those that are not of its type:
// recorded when the Capability had another.
func decoded(x Series, ps []stored) []stored {
	typ := typeOf(x)
	out := ps[:0]
	for _, p := range ps {
		if v, err := decode(typ, p.value); err == nil {
			out = append(out, stored{p.ts, v})
		}
	}
	return out
}

// decode reads back a value stored for a Capability of type t.
func decode(t home.ValueType, v any) (any, error) {
	switch t {
	case home.Binary:
		if i, ok := v.(int64); ok {
			return i != 0, nil
		}
	case home.Numeric:
		if f, ok := v.(float64); ok {
			return f, nil
		}
	case home.Enum, home.Text:
		if s, ok := v.(string); ok {
			return s, nil
		}
	case home.Composite, home.List:
		if s, ok := v.(string); ok {
			var data any
			err := json.Unmarshal([]byte(s), &data)
			return data, err
		}
	}
	return nil, fmt.Errorf("bad %s value %#v", t, v)
}

func ms(ns int64) int64 { return ns / int64(time.Millisecond) }

// stored is a point as stored, or once decoded.
type stored struct {
	ts    int64
	value any
}

func raw(ps []stored) []any {
	out := make([]any, len(ps))
	for i, p := range ps {
		out[i] = Raw{ms(p.ts), p.value}
	}
	return out
}

// segment is a Value held for d ns within a bucket.
type segment struct {
	v any
	d int64
}

// tally counts the Events ps, the earliest first, by value in each period
// between bounds.
func tally(ps []stored, bounds []int64) []map[string]int {
	n := len(bounds) - 1
	counts := make([]map[string]int, n)
	for j := range counts {
		counts[j] = map[string]int{}
	}
	j := 0
	for _, p := range ps {
		for j < n && bounds[j+1] <= p.ts {
			j++
		}
		if j == n {
			break
		}
		if p.ts >= bounds[j] {
			counts[j][fmt.Sprint(p.value)]++
		}
	}
	return counts
}

// bucket sums up x's points ps in the buckets between bounds, up to now:
// Values hold outside gaps, disjoint spans the earliest first, and Events
// count wherever they are.
func bucket(x Series, ps []stored, bounds []int64, now int64, gaps [][2]int64) []any {
	out := []any{}
	if x.Stateless {
		for j, c := range tally(ps, bounds) {
			if len(c) > 0 {
				out = append(out, Counts{ms(bounds[j]), c})
			}
		}
		return out
	}
	for j, segs := range hold(ps, bounds, now, gaps) {
		if len(segs) > 0 {
			out = append(out, summary(typeOf(x), ms(bounds[j]), segs))
		}
	}
	return out
}

// hold cuts what each of ps holds, until the next one or now, into the
// periods between bounds, left out of blank (disjoint spans, the earliest
// first): the segments of each period, in increasing time.
func hold(ps []stored, bounds []int64, now int64, blank [][2]int64) [][]segment {
	n := len(bounds) - 1
	held := make([][]segment, n)
	horizon := min(bounds[n], now)
	j, h := 0, 0
	// add holds v over [from, to), split at bounds, in increasing time.
	add := func(v any, from, to int64) {
		for j < n-1 && bounds[j+1] <= from {
			j++
		}
		for k := j; k < n && bounds[k] < to; k++ {
			if d := min(to, bounds[k+1]) - max(from, bounds[k]); d > 0 {
				held[k] = append(held[k], segment{v, d})
			}
		}
	}
	for i, p := range ps {
		start, end := max(p.ts, bounds[0]), horizon
		if i+1 < len(ps) {
			end = min(ps[i+1].ts, horizon)
		}
		for h < len(blank) && blank[h][1] <= start {
			h++
		}
		for k := h; start < end; k++ { // the Value holds outside blank spans
			if k == len(blank) || blank[k][0] >= end {
				add(p.value, start, end)
				break
			}
			add(p.value, start, blank[k][0])
			start = max(start, blank[k][1])
		}
	}
	return held
}

// summary sums up the Values held in one bucket starting at t: their
// time-weighted mean, min and max for a numeric Value; the share on for a
// binary one; the state held longest for an enum; else the last Value.
func summary(typ home.ValueType, t int64, segs []segment) any {
	switch typ {
	case home.Numeric:
		b := Numeric{T: t, Min: math.Inf(1), Max: math.Inf(-1)}
		var sum, d float64
		for _, s := range segs {
			v := s.v.(float64)
			sum += v * float64(s.d)
			d += float64(s.d)
			b.Min, b.Max = min(b.Min, v), max(b.Max, v)
		}
		b.Mean = sum / d
		return b
	case home.Binary:
		var on, d int64
		for _, s := range segs {
			if s.v.(bool) {
				on += s.d
			}
			d += s.d
		}
		return Share{t, float64(on) / float64(d)}
	case home.Enum:
		held := map[string]int64{}
		var longest string
		for _, s := range segs {
			v := s.v.(string)
			held[v] += s.d
			if c := cmp.Compare(held[v], held[longest]); c > 0 || c == 0 && v < longest {
				longest = v
			}
		}
		return Raw{t, longest}
	}
	return Raw{t, segs[len(segs)-1].v}
}

// parallel runs f for 0 to n-1, as many at once as there are cores, and
// returns the first error.
func parallel(n int, f func(i int) error) error {
	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))
	for i := range n {
		g.Go(func() error { return f(i) })
	}
	return g.Wait()
}

// gaps reads the Gaps overlapping [lo, hi], times in ns, the earliest first.
func (s *Store) gaps(lo, hi int64) ([]Gap, error) {
	rows, err := s.db.Query(`SELECT start, "end", lost FROM gaps WHERE "end" >= ? AND start <= ? ORDER BY start`, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var gs []Gap
	for rows.Next() {
		var g Gap
		if err := rows.Scan(&g.Start, &g.End, &g.Lost); err != nil {
			return nil, err
		}
		gs = append(gs, g)
	}
	return gs, rows.Err()
}

// blanks merges gaps and more spans [start, end) where nothing is known
// into disjoint spans, the earliest first.
func blanks(gaps []Gap, more ...[2]int64) [][2]int64 {
	spans := more
	for _, g := range gaps {
		spans = append(spans, [2]int64{g.Start, g.End})
	}
	slices.SortFunc(spans, func(a, b [2]int64) int { return cmp.Compare(a[0], b[0]) })
	var out [][2]int64
	for _, s := range spans {
		if n := len(out); n > 0 && s[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], s[1])
		} else {
			out = append(out, s)
		}
	}
	return out
}

// step is a nice bucket size. Under a day, steps are cut from each local
// midnight; a day or more, they start at local midnight, a week on Monday, a
// month on the 1st.
type step struct {
	label        string
	d            time.Duration
	days, months int
}

var steps = []step{
	{label: "1m", d: time.Minute}, {label: "5m", d: 5 * time.Minute}, {label: "15m", d: 15 * time.Minute},
	{label: "1h", d: time.Hour}, {label: "6h", d: 6 * time.Hour},
	{label: "1d", days: 1}, {label: "1w", days: 7}, {label: "1mo", months: 1},
}

// floor is the boundary at or before t.
func (st step) floor(t time.Time) time.Time {
	y, m, d := t.Date()
	switch {
	case st.months > 0:
		return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
	case st.days == 7:
		d -= (int(t.Weekday()) + 6) % 7
	}
	midnight := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	if st.d == 0 {
		return midnight
	}
	return midnight.Add(t.Sub(midnight).Truncate(st.d))
}

// next is the boundary after boundary b.
func (st step) next(b time.Time) time.Time {
	y, m, d := b.Date()
	if st.d == 0 {
		return time.Date(y, m+time.Month(st.months), d+st.days, 0, 0, 0, 0, b.Location())
	}
	midnight := time.Date(y, m, d+1, 0, 0, 0, 0, b.Location())
	if n := b.Add(st.d); n.Before(midnight) {
		return n
	}
	return midnight
}

// cut cuts [from, to) into buckets at the smallest step that makes at most
// budget of them, or else the largest. It returns the step and the bounds of
// the buckets, the first at or before from, the last at or after to.
func cut(from, to time.Time, budget int) (string, []int64) {
	for i, st := range steps {
		largest := i == len(steps)-1
		var bounds []int64
		for t := st.floor(from); ; t = st.next(t) {
			bounds = append(bounds, t.UnixNano())
			if !t.Before(to) || !largest && len(bounds) > budget+1 {
				break
			}
		}
		if largest || len(bounds) <= budget+1 {
			return st.label, bounds
		}
	}
	panic("unreachable")
}
