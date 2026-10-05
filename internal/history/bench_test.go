package history

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/llehouerou/oiko/internal/home"
)

// The read side's budget, against a synthetic 3-year database of about 12 M
// points over about 300 series: run on the target host with
//
//	OIKO_BENCH_DB=/tmp/oiko-bench.db go test ./internal/history -run '^$' -bench Read
//
// The database is generated at that path the first time, which takes a few
// minutes.

// kind is a Capability of the synthetic home, and how often it changes per day,
// from the 24 h capture of the home's zigbee2mqtt reports (#43).
type kind struct {
	function, capability string // function "": the Device's own
	typ                  home.ValueType
	stateless            bool
	perDay               float64
	tile                 bool // a tile's mini-chart and a Timeline lane draw it
}

var (
	lightKinds = []kind{
		{"light", "state", home.Binary, false, 5, true},
		{"light", "brightness", home.Numeric, false, 1, true},
		{"light", "color_temp", home.Numeric, false, 0.7, false},
		{"light", "power_on_behavior", home.Enum, false, 1.8, false},
		{"light", "effect", home.Enum, false, 0.01, false},
		{"light", "color_temp_startup", home.Numeric, false, 0.01, false},
		{"light", "color_mode", home.Enum, false, 0.01, false},
		{"light", "scene", home.Text, false, 0.01, false},
	}
	colorKinds = []kind{
		{"light", "color_xy", home.Composite, false, 1.3, false},
		{"light", "color_hs", home.Composite, false, 1.3, false},
	}
	plugKinds = []kind{
		{"switch", "state", home.Binary, false, 5, true},
		{"switch", "power", home.Numeric, false, 19, true},
		{"switch", "current", home.Numeric, false, 24, false},
		{"switch", "voltage", home.Numeric, false, 221, false},
		{"switch", "energy", home.Numeric, false, 3.7, false},
		{"switch", "countdown", home.Numeric, false, 0.01, false},
		{"switch", "child_lock", home.Binary, false, 0.01, false},
		{"", "indicator_mode", home.Enum, false, 0.01, false},
		{"", "power_outage_memory", home.Enum, false, 0.01, false},
	}
	motionKinds = []kind{
		{"occupancy", "occupancy", home.Binary, false, 362, true},
		{"illuminance", "illuminance", home.Numeric, false, 278, true},
		{"temperature", "temperature", home.Numeric, false, 26, true},
		{"", "battery", home.Numeric, false, 0.05, false},
		{"", "voltage", home.Numeric, false, 5, false},
	}
	contactKinds = []kind{
		{"contact", "contact", home.Binary, false, 22, true},
		{"tamper", "tamper", home.Binary, false, 0.01, false},
		{"", "battery", home.Numeric, false, 0.05, false},
		{"", "battery_low", home.Binary, false, 0.01, false},
		{"", "voltage", home.Numeric, false, 5, false},
	}
	buttonKinds = []kind{
		{"button", "action", home.Enum, true, 12, true},
		{"", "battery", home.Numeric, false, 0.05, false},
		{"", "action_duration", home.Numeric, false, 1.5, false},
	}
	deviceKinds = []kind{
		{"", "linkquality", home.Numeric, false, 238, false},
		{"", "", home.Enum, false, 0.1, false}, // Availability
	}
)

// syntheticDevice is a Device of the synthetic home, by its kinds.
type syntheticDevice struct {
	id    string
	kinds []kind
}

func syntheticHome() []syntheticDevice {
	var all []syntheticDevice
	add := func(prefix string, n int, kinds func(i int) []kind) {
		for i := range n {
			all = append(all, syntheticDevice{fmt.Sprintf("%s%d", prefix, i), append(kinds(i), deviceKinds...)})
		}
	}
	add("light", 18, func(i int) []kind {
		if i < 3 {
			return append(lightKinds[:len(lightKinds):len(lightKinds)], colorKinds...)
		}
		return lightKinds
	})
	add("plug", 3, func(int) []kind { return plugKinds })
	add("motion", 2, func(int) []kind { return motionKinds })
	add("contact", 5, func(int) []kind { return contactKinds })
	add("button", 6, func(int) []kind { return buttonKinds })
	return all
}

func (k kind) series(device string) Series {
	return Series{Ref: home.TargetDevice(home.DeviceID(device), k.function).Ref(k.capability), Type: k.typ, Stateless: k.stateless}
}

// generate fills a new database at path with 3 years of the synthetic home's
// History, up to now.
func generate(path string) error {
	s, err := Open(path)
	if err != nil {
		return err
	}
	defer s.Close()
	end := time.Now()
	start := end.AddDate(-3, 0, 0)
	r := rand.New(rand.NewPCG(1, 2))
	for _, d := range syntheticHome() {
		for _, k := range d.kinds {
			x := k.series(d.id)
			if err := synthesize(s.db, r, key(x.Ref), k, start, end); err != nil {
				return err
			}
		}
	}
	return nil
}

// synthesize records a series of k's changes from start to end, at random times.
func synthesize(db *sql.DB, r *rand.Rand, sk seriesKey, k kind, start, end time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`INSERT INTO series (target, capability) VALUES (?, ?) RETURNING id`, sk.target, sk.capability).Scan(&id); err != nil {
		return err
	}
	insert, err := tx.Prepare(`INSERT INTO points (series, ts, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	mean := float64(24*time.Hour) / k.perDay
	level := 100.0
	for i, ts := 0, start.UnixNano(); ts < end.UnixNano(); i, ts = i+1, ts+1+int64(r.ExpFloat64()*mean) {
		var v any
		switch {
		case k.capability == "":
			v = []string{"online", "offline"}[i%2]
		case k.stateless:
			v = []string{"single", "double", "hold"}[r.IntN(3)]
		case k.typ == home.Binary:
			v = int64(1 - i%2)
		case k.typ == home.Numeric:
			level = math.Abs(level + r.NormFloat64()*5 + 0.5)
			v = math.Round(level*100) / 100
		case k.typ == home.Composite:
			v = fmt.Sprintf(`{"x":%.4f,"y":%.4f}`, r.Float64(), r.Float64())
		default:
			v = fmt.Sprintf("option%d", i%4)
		}
		if _, err := insert.Exec(id, ts, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// bench opens the synthetic database, generating it first if need be.
func bench(b *testing.B) *Store {
	path := os.Getenv("OIKO_BENCH_DB")
	if path == "" {
		b.Skip("OIKO_BENCH_DB is not set")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		b.Logf("generating %s", path)
		if err := generate(path); err != nil {
			b.Fatal(err)
		}
	}
	s, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	var points, series int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM points), (SELECT count(*) FROM series)`).Scan(&points, &series); err != nil {
		b.Fatal(err)
	}
	b.Logf("%d points in %d series", points, series)
	return s
}

// tiles is the series every tile draws, or every Timeline lane.
func tiles() []Series {
	var all []Series
	for _, d := range syntheticHome() {
		for _, k := range d.kinds {
			if k.tile {
				all = append(all, k.series(d.id))
			}
		}
	}
	return all
}

func benchRead(b *testing.B, q Query, want string) {
	s := bench(b)
	var a Answer
	for b.Loop() {
		var err error
		if a, err = s.Points(q); err != nil {
			b.Fatal(err)
		}
	}
	if a.Bucket != want {
		b.Errorf("bucket %q, want %q", a.Bucket, want)
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/op")
}

// One Function's year, bucketed, under 100 ms: a plug's switch, the busiest.
func BenchmarkReadOneFunctionsYear(b *testing.B) {
	var series []Series
	for _, k := range plugKinds {
		if k.function == "switch" {
			series = append(series, k.series("plug0"))
		}
	}
	now := time.Now()
	benchRead(b, Query{From: now.AddDate(-1, 0, 0), To: now, Points: 1000, Series: series}, "1d")
}

// The 24 h mini-series of every tile, under 200 ms.
func BenchmarkReadEveryTiles24h(b *testing.B) {
	now := time.Now()
	benchRead(b, Query{From: now.Add(-24 * time.Hour), To: now, Points: 200, Series: tiles()}, "15m")
}

// A Timeline week of every Function, under 300 ms.
func BenchmarkReadATimelineWeek(b *testing.B) {
	now := time.Now()
	benchRead(b, Query{From: now.AddDate(0, 0, -7), To: now, Points: 1500, Markers: true, Series: tiles()}, "15m")
}

// The summaries of a Timeline week, each lane's over the span, alongside its
// lanes: within the same 300 ms.
func BenchmarkReadATimelineWeeksSummaries(b *testing.B) {
	s := bench(b)
	now := time.Now()
	// A week through now, which also reads the previous week's at equal elapsed time.
	q := PeriodsQuery{From: now.AddDate(0, 0, -6), To: now.AddDate(0, 0, 1), Per: "span"}
	for _, x := range tiles() {
		m := Stats
		if x.Stateless || x.Type == home.Binary {
			m = Count
		}
		q.Items = append(q.Items, Item{Series: x, Measure: m})
	}
	for b.Loop() {
		if _, err := s.Periods(q); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/op")
}

// A plug's energy per day over a month, under 50 ms.
func BenchmarkReadEnergyPerDayOverAMonth(b *testing.B) {
	s := bench(b)
	var x Series
	for _, k := range plugKinds {
		if k.capability == "energy" {
			x = k.series("plug0")
		}
	}
	x.Counter = true // its synthetic readings wander: each drop is a reset
	now := time.Now()
	q := PeriodsQuery{From: now.AddDate(0, -1, 0), To: now, Per: "day", Items: []Item{{Series: x, Measure: Energy}}}
	for b.Loop() {
		if _, err := s.Periods(q); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/op")
}
