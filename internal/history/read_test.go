package history

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// h0 is an hour of the past, on the hour.
var h0 = time.Date(2026, 1, 12, 10, 0, 0, 0, time.Local)

func at(d time.Duration) time.Time { return h0.Add(d) }

func msAt(d time.Duration) int64 { return at(d).UnixMilli() }

var (
	brightness = Series{Ref: lamp.Ref("brightness"), Type: home.Numeric}
	state      = Series{Ref: lamp.Ref("state"), Type: home.Binary}
	effect     = Series{Ref: lamp.Ref("effect"), Type: home.Enum}
	color      = Series{Ref: lamp.Ref("color"), Type: home.Composite}
	action     = Series{Ref: home.TargetDevice("remote", "button").Ref("action"), Type: home.Enum, Stateless: true}
)

func record(s *Store, x Series, data any, d time.Duration) {
	kind := home.ValueChanged
	if x.Stateless {
		kind = home.EventOccurred
	}
	s.Follow(value(kind, x.Ref, data, at(d)))
}

func read(t *testing.T, s *Store, from, to time.Duration, points int, series ...Series) Answer {
	t.Helper()
	written(s)
	a, err := s.Points(Query{From: at(from), To: at(to), Points: points, Series: series})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func same(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func TestARangeWithinBudgetComesBackRawFromTheValueInForce(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, 0)
	record(s, brightness, 20.0, time.Hour)
	record(s, brightness, 30.0, 2*time.Hour)
	record(s, brightness, 40.0, 4*time.Hour)
	record(s, state, true, 30*time.Minute)
	record(s, effect, "colorloop", -time.Hour)
	record(s, color, map[string]any{"x": 0.3, "y": 0.4}, 0)
	record(s, action, "single", 0) // before the range: an Event holds nothing
	record(s, action, "double", 90*time.Minute)

	a := read(t, s, 15*time.Minute, 3*time.Hour, 3, brightness, state, effect, color, action)
	same(t, "bucket", a.Bucket, "")
	same(t, "brightness", a.Series[0], []any{Raw{msAt(0), 10.0}, Raw{msAt(time.Hour), 20.0}, Raw{msAt(2 * time.Hour), 30.0}})
	same(t, "state", a.Series[1], []any{Raw{msAt(30 * time.Minute), true}})
	same(t, "effect", a.Series[2], []any{Raw{msAt(-time.Hour), "colorloop"}})
	same(t, "color", a.Series[3], []any{Raw{msAt(0), map[string]any{"x": 0.3, "y": 0.4}}})
	same(t, "action", a.Series[4], []any{Raw{msAt(90 * time.Minute), "double"}})
}

func TestARangeOverBudgetIsBucketedAtTheSmallestNiceSizeForEverySeries(t *testing.T) {
	s := open(t)
	for i := range 180 {
		record(s, brightness, float64(i), time.Duration(i)*time.Minute)
	}
	record(s, state, true, 0)
	// 1m and 5m make too many buckets; 15m makes 12, still too many; 1h fits.
	a := read(t, s, 0, 3*time.Hour, 10, brightness, state)
	same(t, "bucket", a.Bucket, "1h")
	for i, x := range a.Series {
		if len(x) != 3 {
			t.Errorf("series %d: %d buckets, want 3: %+v", i, len(x), x)
		}
	}
	same(t, "state", a.Series[1], []any{Share{msAt(0), 1}, Share{msAt(time.Hour), 1}, Share{msAt(2 * time.Hour), 1}})
}

func TestARangeStartsNoEarlierThanItsSeriesHistory(t *testing.T) {
	s := open(t)
	for i := range 180 {
		record(s, brightness, float64(i), time.Duration(i)*time.Minute)
	}
	record(s, state, true, time.Hour)
	// From a month back, the buckets would be a week; from the first point, an hour.
	a := read(t, s, -30*24*time.Hour, 3*time.Hour, 10, brightness, state)
	same(t, "from", a.From, msAt(0))
	same(t, "bucket", a.Bucket, "1h")
	same(t, "from within the History", read(t, s, time.Hour, 2*time.Hour, 100, brightness).From, msAt(time.Hour))
	same(t, "from before any History", read(t, s, -2*time.Hour, -time.Hour, 100, brightness).From, msAt(-2*time.Hour))
}

func TestEachKindIsBucketedItsOwnWay(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, -30*time.Minute) // in force at the start
	record(s, brightness, 40.0, 45*time.Minute)
	record(s, brightness, 20.0, 50*time.Minute)
	record(s, state, true, 0)
	record(s, state, false, 15*time.Minute)
	record(s, effect, "a", 0)
	record(s, effect, "b", 20*time.Minute)
	record(s, effect, "a", 70*time.Minute)
	record(s, color, map[string]any{"x": 0.1}, 0)
	record(s, color, map[string]any{"x": 0.2}, 30*time.Minute)
	record(s, action, "single", 10*time.Minute)
	record(s, action, "single", 20*time.Minute)
	record(s, action, "double", 30*time.Minute)

	a := read(t, s, 0, 2*time.Hour, 2, brightness, state, effect, color, action)
	same(t, "bucket", a.Bucket, "1h")
	same(t, "brightness", a.Series[0], []any{
		Numeric{msAt(0), (10*45 + 40*5 + 20*10) / 60.0, 10, 40},
		Numeric{msAt(time.Hour), 20, 20, 20},
	})
	same(t, "state", a.Series[1], []any{Share{msAt(0), 0.25}, Share{msAt(time.Hour), 0}})
	same(t, "effect", a.Series[2], []any{Raw{msAt(0), "b"}, Raw{msAt(time.Hour), "a"}})
	same(t, "color", a.Series[3], []any{Raw{msAt(0), map[string]any{"x": 0.2}}, Raw{msAt(time.Hour), map[string]any{"x": 0.2}}})
	same(t, "action", a.Series[4], []any{Counts{msAt(0), map[string]int{"single": 2, "double": 1}}})
}

func addGap(t *testing.T, s *Store, from, to time.Duration, lost int) {
	t.Helper()
	if _, err := s.db.Exec(insertGap, at(from).UnixNano(), at(to).UnixNano(), lost); err != nil {
		t.Fatal(err)
	}
}

func TestABucketLeavesGapTimeOut(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, 0)
	record(s, brightness, 30.0, 20*time.Minute) // replayed, stamped inside the Gap
	record(s, brightness, 20.0, 50*time.Minute)
	record(s, brightness, 25.0, 150*time.Minute)
	record(s, brightness, 20.0, 165*time.Minute) // over budget: 4 points in range for 3
	record(s, state, true, 0)
	record(s, action, "single", 15*time.Minute) // replayed inside the Gap: it happened
	addGap(t, s, 10*time.Minute, 30*time.Minute, 0)
	addGap(t, s, 25*time.Minute, 40*time.Minute, 2) // dropped points, overlapping
	addGap(t, s, time.Hour, 2*time.Hour, 0)         // a whole bucket

	a := read(t, s, 0, 3*time.Hour, 3, brightness, state, action)
	same(t, "bucket", a.Bucket, "1h")
	same(t, "brightness", a.Series[0], []any{
		Numeric{msAt(0), (10*10 + 30*10 + 20*10) / 30.0, 10, 30},
		Numeric{msAt(2 * time.Hour), (20*45 + 25*15) / 60.0, 20, 25}, // after a Gap, the last Value holds again
	})
	same(t, "state", a.Series[1], []any{Share{msAt(0), 1}, Share{msAt(2 * time.Hour), 1}})
	same(t, "action", a.Series[2], []any{Counts{msAt(0), map[string]int{"single": 1}}})
}
func TestABucketEndsAtNow(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, 0)
	record(s, brightness, 20.0, 10*time.Minute)
	record(s, brightness, 30.0, 30*time.Minute) // stamped ahead of now
	written(s)
	a, err := s.points(Query{From: at(0), To: at(59 * time.Minute), Points: 1, Series: []Series{brightness}}, at(20*time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "buckets", a.Series[0], []any{Numeric{msAt(0), 15, 10, 20}})
}

func TestDayStepsStartAtLocalMidnightAcrossDST(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	// Clocks go forward on 29 March 2026: that day lasts 23 h.
	from := time.Date(2026, 3, 27, 15, 0, 0, 0, paris)
	step, bounds := cut(from, from.Add(4*24*time.Hour), 6)
	same(t, "step", step, "1d")
	var got []time.Time
	for _, b := range bounds {
		got = append(got, time.Unix(0, b).In(paris))
	}
	var want []time.Time
	for d := 27; d <= 32; d++ { // to 1 April
		want = append(want, time.Date(2026, 3, d, 0, 0, 0, 0, paris))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bounds = %v, want local midnights %v", got, want)
	}
	if d := got[3].Sub(got[2]); d != 23*time.Hour {
		t.Errorf("29 March lasts %v, want 23h", d)
	}
}

func TestStepsUnderADayAreCutFromLocalMidnight(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	from := time.Date(2026, 1, 12, 7, 30, 0, 0, paris)
	step, bounds := cut(from, from.Add(24*time.Hour), 5)
	same(t, "step", step, "6h")
	if first := time.Unix(0, bounds[0]).In(paris); first != time.Date(2026, 1, 12, 6, 0, 0, 0, paris) {
		t.Errorf("first bound = %v, want 06:00 local", first)
	}
}

func TestGapsAndTheAvailabilityOfEveryTargetAreIncluded(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, 0)
	written(s)
	device := home.TargetDevice("lamp", "")
	for _, p := range []struct {
		d time.Duration
		v string
	}{{-time.Hour, "online"}, {30 * time.Minute, "offline"}, {5 * time.Hour, "online"}} {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO series (target, capability) VALUES (?, '')`, device.Key()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO points (series, ts, value) SELECT id, ?, ? FROM series WHERE target = ?`, at(p.d).UnixNano(), p.v, device.Key()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(insertGap, at(time.Hour).UnixNano(), at(90*time.Minute).UnixNano(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(insertGap, at(-3*time.Hour).UnixNano(), at(-2*time.Hour).UnixNano(), 0); err != nil {
		t.Fatal(err)
	}

	a := read(t, s, 0, 2*time.Hour, 10, brightness, Series{Ref: device.Ref("")})
	same(t, "gaps", a.Gaps, []Gap{{msAt(time.Hour), msAt(90 * time.Minute), 3}})
	same(t, "availability", a.Availability, map[home.Target][]any{
		device: {Raw{msAt(-time.Hour), "online"}, Raw{msAt(30 * time.Minute), "offline"}},
	})
	same(t, "the Device's Availability as a series", a.Series[1], a.Availability[device])
}

func TestAnswerTimesAreUnixMilliseconds(t *testing.T) {
	s := open(t)
	record(s, brightness, 10.0, 0)
	data, err := json.Marshal(read(t, s, 0, time.Hour, 10, brightness))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"from":` + jsonInt(msAt(0)) + `,"bucket":"","series":[[{"t":` + jsonInt(msAt(0)) + `,"v":10}]],"gaps":[],"availability":{"device:lamp":[]}}`
	same(t, "answer", string(data), want)
}

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }
