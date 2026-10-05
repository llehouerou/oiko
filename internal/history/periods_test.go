package history

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// The days around h0, a Monday at 10:00: Monday is [-10h, 14h), Tuesday
// [14h, 38h), Wednesday [38h, 62h).
var (
	monday    = at(-10 * time.Hour)
	tuesday   = at(14 * time.Hour)
	wednesday = at(38 * time.Hour)
	door      = Series{Ref: home.TargetDevice("door", "contact").Ref("contact"), Type: home.Binary}
	reading   = Series{Ref: home.TargetDevice("probe", "temperature").Ref("temperature"), Type: home.Numeric}
	meter     = Series{Ref: home.TargetDevice("plug", "switch").Ref("energy"), Type: home.Numeric, Counter: true}
)

func perDay(t *testing.T, s *Store, now time.Duration, m Measure, x Series) []Period {
	t.Helper()
	written(s)
	a, err := s.periods(PeriodsQuery{From: monday, To: wednesday, Per: "day", Items: []Item{{Series: x, Measure: m}}}, at(now).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return a.Items[0]
}

// setAvailability records Target t's Availability v at d: Follow stamps it now.
func setAvailability(t *testing.T, s *Store, target home.Target, d time.Duration, v string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO series (target, capability) VALUES (?, '')`, target.Key()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO points (series, ts, value) SELECT id, ?, ? FROM series WHERE target = ? AND capability = ''`, at(d).UnixNano(), v, target.Key()); err != nil {
		t.Fatal(err)
	}
}

func hours(h float64) int64 { return int64(h * float64(time.Hour/time.Millisecond)) }

func TestTimeOnCountsOnlineTimeOnOutsideGaps(t *testing.T) {
	s := open(t)
	record(s, state, false, -10*time.Hour)
	record(s, state, true, 0)
	record(s, state, false, 2*time.Hour) // 2 h on
	record(s, state, true, 4*time.Hour)
	record(s, state, false, 8*time.Hour) // 4 h on, 1 h of it offline, 30 min in a Gap
	record(s, state, true, 20*time.Hour)
	record(s, state, false, 21*time.Hour) // Tuesday, 1 h on
	lampDevice := home.TargetDevice("lamp", "")
	setAvailability(t, s, lampDevice, -20*time.Hour, "online")
	setAvailability(t, s, lampDevice, 5*time.Hour, "offline")
	setAvailability(t, s, lampDevice, 6*time.Hour, "online")
	addGap(t, s, 7*time.Hour, 7*time.Hour+30*time.Minute, 0)

	same(t, "time on", perDay(t, s, 60*time.Hour, TimeOn, state), []Period{
		{Start: monday.UnixMilli(), Value: hours(4.5), Incomplete: true},
		{Start: tuesday.UnixMilli(), Value: hours(1)},
	})
}

func TestAPeriodWithNoValueKnownYetIsIncomplete(t *testing.T) {
	s := open(t)
	record(s, state, true, 0)
	same(t, "time on", perDay(t, s, 60*time.Hour, TimeOn, state), []Period{
		{Start: monday.UnixMilli(), Value: hours(14), Incomplete: true},
		{Start: tuesday.UnixMilli(), Value: hours(24)},
	})
}

func TestAPeriodWithNothingKnownNorRecordedHasNoValue(t *testing.T) {
	s := open(t)
	record(s, door, false, -20*time.Hour)
	record(s, door, true, 20*time.Hour) // replayed, inside Tuesday's Gap
	record(s, action, "single", 21*time.Hour)
	record(s, reading, 20.0, -20*time.Hour)
	addGap(t, s, -10*time.Hour, 38*time.Hour, 0) // Monday and Tuesday, whole
	written(s)
	for _, c := range []struct {
		x    Series
		m    Measure
		want []any // Monday's value, Tuesday's
	}{
		{door, TimeOn, []any{nil, int64(0)}}, // Tuesday holds a recorded point
		{door, Count, []any{nil, 1}},
		{action, Count, []any{nil, map[string]int{"single": 1}}},
		{reading, Stats, []any{nil, nil}}, // no time known: no mean
	} {
		got := perDay(t, s, 60*time.Hour, c.m, c.x)
		same(t, string(c.m)+" of "+c.x.Ref.Capability, []any{got[0].Value, got[1].Value}, c.want)
		if !got[0].Incomplete || !got[1].Incomplete {
			t.Errorf("%s of %s: %+v, want both incomplete", c.m, c.x.Ref.Capability, got)
		}
	}
}

func TestPeriodsTellWhereTheirHistoryStarts(t *testing.T) {
	s := open(t)
	record(s, state, true, 0)
	record(s, state, false, time.Hour)
	written(s)
	a, err := s.periods(PeriodsQuery{From: monday, To: wednesday, Per: "day", Items: []Item{{Series: state, Measure: TimeOn}}}, at(60*time.Hour).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if a.First == nil || *a.First != msAt(0) {
		t.Errorf("first = %v, want the first point", a.First)
	}
	if a, _ := s.periods(PeriodsQuery{From: monday, To: wednesday, Per: "day", Items: []Item{{Series: door, Measure: TimeOn}}}, at(60*time.Hour).UnixNano()); a.First != nil {
		t.Errorf("first = %v, want none before any point", *a.First)
	}
}

func TestOpeningsAreRisingEdgesReplayedOnesIncluded(t *testing.T) {
	s := open(t)
	record(s, door, false, -10*time.Hour)
	record(s, door, true, time.Hour)
	record(s, door, false, 2*time.Hour)
	record(s, door, true, 3*time.Hour)
	record(s, door, false, 4*time.Hour)
	record(s, door, true, 5*time.Hour+30*time.Minute) // replayed, stamped inside the Gap
	record(s, door, false, 7*time.Hour)
	record(s, door, true, 14*time.Hour) // on Tuesday's first ns
	addGap(t, s, 5*time.Hour, 6*time.Hour, 0)

	same(t, "openings", perDay(t, s, 60*time.Hour, Count, door), []Period{
		{Start: monday.UnixMilli(), Value: 3, Incomplete: true},
		{Start: tuesday.UnixMilli(), Value: 1},
	})
}

func TestPressesAreCountedByEventValue(t *testing.T) {
	s := open(t)
	record(s, action, "single", time.Hour)
	record(s, action, "single", 2*time.Hour)
	record(s, action, "double", 3*time.Hour)
	record(s, action, "hold", 20*time.Hour)
	written(s)
	a, err := s.periods(PeriodsQuery{From: monday, To: at(62 * time.Hour), Per: "day", Items: []Item{{Series: action, Measure: Count}}}, at(70*time.Hour).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "presses", a.Items[0], []Period{
		{Start: monday.UnixMilli(), Value: map[string]int{"single": 2, "double": 1}},
		{Start: tuesday.UnixMilli(), Value: map[string]int{"hold": 1}},
		{Start: wednesday.UnixMilli(), Value: map[string]int{}},
	})
	same(t, "end", a.End, at(62*time.Hour).UnixMilli())
}

func TestTheMeanIsTimeWeightedOverStepsAndMinMaxAreTheRecordedExtremes(t *testing.T) {
	s := open(t)
	record(s, reading, 20.0, -10*time.Hour)
	record(s, reading, 22.0, -4*time.Hour)
	record(s, reading, 18.0, 2*time.Hour)
	record(s, reading, 30.0, 3*time.Hour) // replayed inside the Gap: recorded, yet held over no known time
	record(s, reading, 18.0, 4*time.Hour) // 1 of its 4 h in the Gap
	record(s, reading, 20.0, 8*time.Hour)
	addGap(t, s, 2*time.Hour, 5*time.Hour, 0)

	same(t, "stats", perDay(t, s, 60*time.Hour, Stats, reading), []Period{
		{Start: monday.UnixMilli(), Value: Spread{Min: 18, Mean: (20*6 + 22*6 + 18*3 + 20*6) / 21.0, Max: 30}, Incomplete: true},
		{Start: tuesday.UnixMilli(), Value: Spread{Min: 20, Mean: 20, Max: 20}},
	})
	empty := open(t)
	same(t, "nothing known", perDay(t, empty, 60*time.Hour, Stats, reading)[0], Period{Start: monday.UnixMilli(), Incomplete: true})
}

func TestTheCurrentPeriodIsComparedWithThePreviousAtEqualElapsedTime(t *testing.T) {
	s := open(t)
	record(s, state, false, -10*time.Hour)
	record(s, state, true, 0)
	record(s, state, false, 4*time.Hour) // Monday: 2 h on by 12:00, 4 h in all
	record(s, state, true, 20*time.Hour) // Tuesday from 06:00, still on at 12:00

	same(t, "per day", perDay(t, s, 26*time.Hour, TimeOn, state), []Period{
		{Start: monday.UnixMilli(), Value: hours(4)},
		{Start: tuesday.UnixMilli(), Value: hours(6), Previous: &Previous{Value: hours(2), Total: hours(4)}},
	})
}

func TestPeriodsAreLocalCalendarPeriods(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	local := func(month time.Month, day, hour int) int64 {
		return time.Date(2026, month, day, hour, 0, 0, 0, paris).UnixNano()
	}
	for _, c := range []struct {
		per      string
		from, to time.Time
		want     []int64
	}{
		// Clocks go forward on 29 March 2026 and back on 25 October: 23 h and 25 h days.
		{"day", time.Date(2026, 3, 28, 15, 0, 0, 0, paris), time.Date(2026, 3, 29, 12, 0, 0, 0, paris), []int64{local(3, 28, 0), local(3, 29, 0), local(3, 30, 0)}},
		{"day", time.Date(2026, 10, 25, 1, 0, 0, 0, paris), time.Date(2026, 10, 25, 2, 0, 0, 0, paris), []int64{local(10, 25, 0), local(10, 26, 0)}},
		// 1 April 2026 is a Wednesday: its week starts on Monday 30 March.
		{"week", time.Date(2026, 4, 1, 9, 0, 0, 0, paris), time.Date(2026, 4, 7, 9, 0, 0, 0, paris), []int64{local(3, 30, 0), local(4, 6, 0), local(4, 13, 0)}},
		{"month", time.Date(2026, 3, 15, 9, 0, 0, 0, paris), time.Date(2026, 4, 1, 0, 0, 0, 0, paris), []int64{local(3, 1, 0), local(4, 1, 0)}},
		{"hour", time.Date(2026, 3, 29, 1, 30, 0, 0, paris), time.Date(2026, 3, 29, 3, 30, 0, 0, paris), []int64{local(3, 29, 1), local(3, 29, 3), local(3, 29, 4)}},
		{"span", time.Date(2026, 3, 29, 1, 30, 0, 0, paris), time.Date(2026, 3, 29, 3, 30, 0, 0, paris), []int64{time.Date(2026, 3, 29, 1, 30, 0, 0, paris).UnixNano(), local(3, 29, 3) + int64(30*time.Minute)}},
	} {
		bounds, err := periodBounds(c.per, c.from, c.to)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(bounds, c.want) {
			t.Errorf("%s from %v to %v: bounds %v, want %v", c.per, c.from, c.to, bounds, c.want)
		}
	}
	if d := time.Duration(local(3, 30, 0) - local(3, 29, 0)); d != 23*time.Hour {
		t.Errorf("29 March lasts %v", d)
	}
	if d := time.Duration(local(10, 26, 0) - local(10, 25, 0)); d != 25*time.Hour {
		t.Errorf("25 October lasts %v", d)
	}
}

func TestThePreviousPeriodEndsAtTheSameWallClockTime(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	date := func(month time.Month, day, hour int) time.Time {
		return time.Date(2026, month, day, hour, 0, 0, 0, paris)
	}
	for _, c := range []struct {
		per                 string
		start, end, now     time.Time
		wantPrev, wantUntil time.Time
	}{
		{"day", date(3, 30, 0), date(3, 31, 0), date(3, 30, 14), date(3, 29, 0), date(3, 29, 14)}, // after a 23 h day: still 14:00
		{"week", date(4, 6, 0), date(4, 13, 0), date(4, 8, 9), date(3, 30, 0), date(4, 1, 9)},
		{"month", date(3, 1, 0), date(4, 1, 0), date(3, 31, 9), date(2, 1, 0), date(3, 1, 0)}, // February is over by then
		{"hour", date(3, 30, 14), date(3, 30, 15), date(3, 30, 14).Add(20 * time.Minute), date(3, 30, 13), date(3, 30, 13).Add(20 * time.Minute)},
		{"span", date(3, 30, 6), date(3, 30, 18), date(3, 30, 8), date(3, 29, 18), date(3, 29, 20)},
	} {
		prev, until := previous(c.per, c.start, c.end, c.now)
		if !prev.Equal(c.wantPrev) || !until.Equal(c.wantUntil) {
			t.Errorf("%s at %v: previous from %v until %v, want %v until %v", c.per, c.now, prev, until, c.wantPrev, c.wantUntil)
		}
	}
}

func TestAMeasureMustSuitItsSeries(t *testing.T) {
	s := open(t)
	plugs := Series{Ref: home.TargetAggregate("plugs").Ref("energy"), Type: home.Numeric, Counter: true}
	for _, it := range []Item{
		{Series: brightness, Measure: TimeOn}, {Series: state, Measure: Stats}, {Series: action, Measure: TimeOn}, {Series: brightness, Measure: Count}, {Series: brightness, Measure: Energy},
		{Series: plugs, Measure: Energy, Members: []Series{}}, {Series: plugs, Measure: Stats, Members: []Series{meter}}, {Series: plugs, Measure: Energy, Members: []Series{reading}},
	} {
		_, err := s.Periods(PeriodsQuery{From: monday, To: tuesday, Per: "day", Items: []Item{it}})
		if !errors.Is(err, home.ErrInvalid) {
			t.Errorf("%s of %s: %v, want invalid", it.Measure, it.Ref.Capability, err)
		}
	}
	if _, err := s.Periods(PeriodsQuery{From: monday, To: tuesday, Per: "year"}); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("per year: %v, want invalid", err)
	}
	if _, err := s.Periods(PeriodsQuery{From: monday, To: monday.AddDate(2, 0, 0), Per: "hour"}); !errors.Is(err, home.ErrInvalid) {
		t.Errorf("two years per hour: %v, want too many", err)
	}
}

// kWh checks the energy of periods, to within rounding.
func kWh(t *testing.T, what string, got []Period, want []float64, incomplete []bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %+v, want %v", what, got, want)
	}
	for k, p := range got {
		if v, ok := p.Value.(float64); !ok || math.Abs(v-want[k]) > 1e-9 || p.Incomplete != incomplete[k] {
			t.Errorf("%s: period %d = %+v, want %v kWh, incomplete %v", what, k, p, want[k], incomplete[k])
		}
	}
}

func TestEnergyIsTheCountersRiseSpreadLinearlyAcrossPeriods(t *testing.T) {
	s := open(t)
	record(s, meter, 10.0, -12*time.Hour) // Sunday 22:00
	record(s, meter, 11.0, 0)             // 1 kWh over 12 h: 2 h of it on Sunday
	record(s, meter, 12.0, 12*time.Hour)
	record(s, meter, 12.5, 16*time.Hour) // across midnight: 2 h each side
	record(s, meter, 13.0, 20*time.Hour)
	kWh(t, "energy", perDay(t, s, 60*time.Hour, Energy, meter), []float64{1.0*10/12 + 1 + 0.25, 0.25 + 0.5}, []bool{false, false})
}

func TestACounterDropIsAResetCountingTheNewValueFromZero(t *testing.T) {
	s := open(t)
	record(s, meter, 5.0, -10*time.Hour)
	record(s, meter, 6.0, 0)
	record(s, meter, 0.5, 2*time.Hour) // reset, then 0.5 kWh used
	record(s, meter, 1.5, 4*time.Hour)
	record(s, meter, 0.25, 20*time.Hour) // Replaced by fresh hardware: 0.25 kWh over 16 h
	kWh(t, "energy", perDay(t, s, 60*time.Hour, Energy, meter), []float64{2.5 + 0.25*10/16, 0.25 * 6 / 16}, []bool{false, false})
}

func TestADropWithinRoundingIsNoReset(t *testing.T) {
	s := open(t)
	record(s, meter, 100.0, -10*time.Hour)
	record(s, meter, 102.0, 0)
	record(s, meter, 101.5, 2*time.Hour)  // under 1 %: rounding, nothing used
	record(s, meter, 103.0, 4*time.Hour)  // 1 kWh from the 102 reached
	record(s, meter, 102.2, 20*time.Hour) // 0.8 % under 103
	record(s, meter, 102.5, 22*time.Hour) // 103 is not reached again: nothing used
	record(s, meter, 0.5, 30*time.Hour)   // a reset
	kWh(t, "energy", perDay(t, s, 60*time.Hour, Energy, meter), []float64{3, 0.5}, []bool{false, false})
}
func TestEnergyAcrossAGapKeepsItsTotalAndMarksThePeriodIncomplete(t *testing.T) {
	s := open(t)
	record(s, meter, 10.0, -12*time.Hour)
	record(s, meter, 14.0, 20*time.Hour) // 4 kWh over 32 h, 6 of them on Tuesday
	addGap(t, s, 12*time.Hour, 16*time.Hour, 0)
	kWh(t, "energy", perDay(t, s, 60*time.Hour, Energy, meter), []float64{4.0 * 24 / 32, 4.0 * 6 / 32}, []bool{true, true})
	all, err := s.periods(PeriodsQuery{From: monday, To: wednesday, Per: "span", Items: []Item{{Series: meter, Measure: Energy}}}, at(60*time.Hour).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	kWh(t, "over both days", all.Items[0], []float64{4.0 * 30 / 32}, []bool{true})
}

func TestTodaysEnergySoFarIsComparedWithWhatWasKnownAtTheSameTime(t *testing.T) {
	s := open(t)
	record(s, meter, 10.0, -10*time.Hour) // Monday 00:00
	record(s, meter, 11.0, 0)
	record(s, meter, 13.0, 4*time.Hour)  // 12:00, the same time as now, falls within this one
	record(s, meter, 14.0, 20*time.Hour) // Tuesday 06:00: 6 h of its 16 h on Tuesday
	record(s, meter, 20.0, 27*time.Hour) // stamped ahead of the clock
	got := perDay(t, s, 26*time.Hour, Energy, meter)
	kWh(t, "energy", got, []float64{3 + 1.0*10/16, 1.0 * 6 / 16}, []bool{false, false})
	if p := got[1].Previous; p == nil || p.Value != 1.0 || math.Abs(p.Total.(float64)-(3+1.0*10/16)) > 1e-9 {
		t.Errorf("previous = %+v, want 1 kWh by 12:00, as read then, of 3.625", p)
	}
}
func TestAnAggregatesEnergyIsItsMembersSummed(t *testing.T) {
	s := open(t)
	other := Series{Ref: home.TargetDevice("plug2", "switch").Ref("energy"), Type: home.Numeric, Counter: true}
	plugs := Series{Ref: home.TargetAggregate("plugs").Ref("energy"), Type: home.Numeric, Counter: true}
	record(s, meter, 10.0, -10*time.Hour)
	record(s, meter, 11.0, 14*time.Hour)
	record(s, meter, 12.0, 38*time.Hour)
	record(s, other, 500.0, 20*time.Hour) // added to the Aggregate on Tuesday
	record(s, other, 501.0, 38*time.Hour)
	// Its own sum leaps by 500 kWh as the member joins: not a use of energy.
	record(s, plugs, 10.0, -10*time.Hour)
	record(s, plugs, 511.0, 20*time.Hour)
	record(s, plugs, 513.0, 38*time.Hour)
	written(s)
	a, err := s.periods(PeriodsQuery{From: monday, To: wednesday, Per: "day", Items: []Item{{Series: plugs, Measure: Energy, Members: []Series{meter, other}}}}, at(60*time.Hour).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	// Before its first reading, the new member's energy is unknown.
	kWh(t, "energy", a.Items[0], []float64{1, 2}, []bool{true, true})
	if a.First == nil || *a.First != msAt(-10*time.Hour) {
		t.Errorf("first = %v, want the members' first point", a.First)
	}
}
