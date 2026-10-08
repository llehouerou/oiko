package automation

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// quietHome is a fakeHome that only counts Commands, allocating nothing.
type quietHome struct {
	fakeHome
	n int
}

func (q *quietHome) Command(home.Target, home.Request) (string, error) {
	q.n++
	return "", nil
}

func (q *quietHome) EndRun(home.RunEnd) {}

// loaded is a quiet home whose engine runs the office flow after n copies of
// the Bedroom Alice one, which the office button never triggers.
func loaded(t testing.TB, n int) (*Engine, *quietHome) {
	t.Helper()
	q := &quietHome{fakeHome: *fixture()}
	docs := copies(t, "flow1-bedroom-alice.json", n)
	docs = append(docs, doc("office",
		[]string{pressSingle, isNight, cmd("lamp", `"device:lamp-office/light"`)},
		"single.out is-night.in", "is-night.true lamp.in", "is-night.false lamp.in"))
	e := New(q, docs, nil, nil)
	q.deliver(home.Update{Kind: home.ValueChanged, Ref: &nightRef, Value: &home.Value{Data: true}})
	settle(e)
	e.now = spaced()
	return e, q
}

// spaced is a clock whose readings are a second apart, which keeps the
// runaway guard out of the way of many Runs.
func spaced() func() time.Time {
	t := time.Now()
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

// copies decodes n distinct copies of testdata/file.
func copies(t testing.TB, file string, n int) []Document {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	docs := make([]Document, n)
	for i := range docs {
		if err := json.Unmarshal(data, &docs[i]); err != nil {
			t.Fatal(err)
		}
	}
	return docs
}

var singlePress = home.Update{Kind: home.EventOccurred,
	Ref:   &home.Ref{Target: home.TargetDevice("button-office", "button"), Capability: "action"},
	Value: &home.Value{Data: "single"}}

// report delivers one press and applies it, as the engine goroutine would.
func report(e *Engine, q *quietHome) {
	q.deliver(singlePress)
	e.drain()
}

func TestRunAllocatesNothing(t *testing.T) {
	e, q := loaded(t, 0)
	if allocs := testing.AllocsPerRun(1000, func() { report(e, q) }); allocs != 0 {
		t.Errorf("%v allocations per Run", allocs)
	}
	if q.n != 1001 {
		t.Errorf("%d Commands for 1001 Runs", q.n)
	}
}

func TestHeapPerLoadedAutomation(t *testing.T) {
	for _, file := range []string{"flow1-bedroom-alice.json", "flow5a-veranda.json"} { // the most Steps, the most state
		const n = 500
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		e := New(fixture(), copies(t, file, n), nil, nil)
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(e)
		if per := (int64(after.HeapAlloc) - int64(before.HeapAlloc)) / n; per > 20<<10 {
			t.Errorf("%s: %d bytes of heap per Automation, want at most 20 KiB", file, per)
		}
	}
}

func TestReportCostDoesNotDependOnLoadedAutomations(t *testing.T) {
	e0, q0 := loaded(t, 0)
	e500, q500 := loaded(t, 500)
	allocs0 := testing.AllocsPerRun(100, func() { report(e0, q0) })
	allocs500 := testing.AllocsPerRun(100, func() { report(e500, q500) })
	if allocs0 != allocs500 {
		t.Errorf("allocations per report: %v with 0 Automations, %v with 500", allocs0, allocs500)
	}
	// Rounds alternate between the two engines, so a busy machine (other test
	// packages run alongside) slows both alike, and each keeps its best round.
	time0, time500 := time.Hour, time.Hour
	round := func(e *Engine, q *quietHome) time.Duration {
		start := time.Now()
		for range 2000 {
			report(e, q)
		}
		return time.Since(start)
	}
	for range 10 {
		time0 = min(time0, round(e0, q0))
		time500 = min(time500, round(e500, q500))
	}
	// Generous: the point is the absence of a per-Automation factor.
	if time500 > 2*time0 {
		t.Errorf("2000 reports: %v with 0 Automations, %v with 500", time0, time500)
	}
}
