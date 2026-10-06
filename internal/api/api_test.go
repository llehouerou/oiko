package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
	"github.com/llehouerou/oiko/internal/release"
)

// static is a built web client.
var static = fstest.MapFS{
	"index.html":          {Data: []byte(`<!doctype html><script type="module" src="/assets/index-abc.js"></script>`)},
	"favicon.svg":         {Data: []byte("<svg></svg>")},
	"assets/index-abc.js": {Data: []byte("console.log(1)")},
}

// handler is Oiko's API over a Home without Devices, with its engine and
// history running, serving static, without a Public URL.
func handler(t *testing.T) (*home.Home, http.Handler) {
	h, _, hdl := handlerAt(t, nil)
	return h, hdl
}

// handlerAt is handler with Public URL public, and its access store.
func handlerAt(t *testing.T, public *url.URL) (*home.Home, *access.Store, http.Handler) {
	t.Helper()
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	acc, err := access.Open(t.TempDir(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	h := home.New(nil, nil, nil, nil, nil, nil, nil, nil, store.Command)
	e := automation.New(h, nil, nil, nil, nil, nil, store.Record)
	h.Follow(store.Follow)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go e.Run(ctx)
	go store.Run(ctx)
	return h, acc, Handler(h, e, store, acc, build.Build{}, "binary", release.New(build.Build{}), nil, public, static)
}

// server serves handler(t), and does requests on it.
func server(t *testing.T) (*home.Home, func(method, path, body string) *http.Response) {
	t.Helper()
	h, hdl := handler(t)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	return h, func(method, path, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
}

func TestAutomationEndpoints(t *testing.T) {
	h, do := server(t)
	list := func() []automation.Document {
		var docs []automation.Document
		json.NewDecoder(do("GET", "/api/automations", "").Body).Decode(&docs)
		return docs
	}
	status := func() []home.AutomationStatus {
		snap, _, cancel := h.Subscribe()
		cancel()
		return snap.Automations
	}

	resp := do("POST", "/api/automations", `{"name": "Night", "enabled": true}`)
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)
	if resp.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("create: %d %+v", resp.StatusCode, created)
	}
	if docs := list(); len(docs) != 1 || docs[0].ID != created.ID || docs[0].Name != "Night" {
		t.Errorf("list = %+v", docs)
	}
	if s := status(); len(s) != 1 || s[0] != (home.AutomationStatus{ID: created.ID, Status: home.AutomationEnabled}) {
		t.Errorf("status = %+v", s)
	}

	if resp := do("PUT", "/api/automations/"+created.ID, `{"id": "ignored", "name": "Night", "enabled": false}`); resp.StatusCode != http.StatusNoContent {
		t.Errorf("replace: %d", resp.StatusCode)
	}
	if docs := list(); len(docs) != 1 || docs[0].ID != created.ID || docs[0].Name != "Night" || docs[0].Enabled {
		t.Errorf("list after replace = %+v", docs)
	}
	if s := status(); len(s) != 1 || s[0].Status != home.AutomationDisabled {
		t.Errorf("status after replace = %+v", s)
	}

	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/api/automations", `{"name": " "}`, http.StatusBadRequest},
		{"POST", "/api/automations", `{"name": `, http.StatusBadRequest},
		{"PUT", "/api/automations/unknown", `{"name": "x"}`, http.StatusNotFound},
		{"DELETE", "/api/automations/unknown", "", http.StatusNotFound},
		{"GET", "/api/automations/" + created.ID + "/state", "", http.StatusOK},
		{"GET", "/api/automations/unknown/state", "", http.StatusNotFound},
		{"DELETE", "/api/automations/" + created.ID, "", http.StatusNoContent},
	} {
		if resp := do(c.method, c.path, c.body); resp.StatusCode != c.want {
			t.Errorf("%s %s %s: %d, want %d", c.method, c.path, c.body, resp.StatusCode, c.want)
		}
	}
	if docs := list(); len(docs) != 0 {
		t.Errorf("list after delete = %+v", docs)
	}
}

// eventually retries get until it succeeds, for up to 5 s.
func eventually[T any](t *testing.T, what string, get func() (T, bool)) T {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		v, ok := get()
		if ok {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %+v", what, v)
		}
	}
}

func TestTraceAndCommandHistoryEndpoints(t *testing.T) {
	h, do := server(t)
	x, _ := h.CreateFlag("X")
	y, _ := h.CreateFlag("Y")
	resp := do("POST", "/api/automations", fmt.Sprintf(`{"name": "X sets Y", "enabled": true, "steps": [
		{"id": "x-on", "kind": "valueTrigger", "params": {"target": "flag:%s", "capability": "on", "op": "eq", "value": true}},
		{"id": "set-y", "kind": "command", "params": {"targets": ["flag:%s"], "values": {"on": true}}}
	], "edges": [{"from": {"step": "x-on", "handle": "out"}, "to": {"step": "set-y", "handle": "in"}}]}`, x, y))
	var auto struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&auto)

	if resp := do("POST", "/api/commands", fmt.Sprintf(`{"target": "flag:%s", "values": {"on": true}}`, x)); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("command: %d", resp.StatusCode)
	}
	commands := func(flag home.FlagID) func() ([]home.CommandRecord, bool) {
		return func() ([]home.CommandRecord, bool) {
			var cs []home.CommandRecord
			json.NewDecoder(do("GET", "/api/commands?target="+home.TargetFlag(flag).Key(), "").Body).Decode(&cs)
			return cs, len(cs) == 1 && cs[0].Status == home.Confirmed
		}
	}
	if c := eventually(t, "commands on X", commands(x)); c[0].Origin != (home.Origin{}) || c[0].Values["on"] != true {
		t.Errorf("command on X = %+v, want from the api", c[0])
	}

	runs := eventually(t, "runs", func() ([]home.RunEnd, bool) {
		var runs []home.RunEnd
		json.NewDecoder(do("GET", "/api/automations/"+auto.ID+"/runs", "").Body).Decode(&runs)
		return runs, len(runs) == 1
	})
	if r := runs[0]; r.Automation != auto.ID || r.Outcome != home.RunActed {
		t.Errorf("run = %+v", r)
	}
	resp = do("GET", "/api/runs/"+runs[0].Run.String(), "")
	var trace automation.Trace
	json.NewDecoder(resp.Body).Decode(&trace)
	if resp.StatusCode != http.StatusOK || trace.Run != runs[0].Run || len(trace.Steps) != 2 || len(trace.Steps[1].Commands) != 1 {
		t.Fatalf("trace: %d %+v", resp.StatusCode, trace)
	}
	if c := eventually(t, "commands on Y", commands(y)); c[0].ID != trace.Steps[1].Commands[0].ID ||
		c[0].Origin != (home.Origin{Automation: auto.ID, Step: "set-y", Run: trace.Run}) {
		t.Errorf("command on Y = %+v, want the trace's", c[0])
	}

	if resp := do("GET", "/api/runs/unknown", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown run: %d", resp.StatusCode)
	}
	var lost struct{ Lost *uint64 }
	if json.NewDecoder(do("GET", "/api/lost-entries", "").Body).Decode(&lost); lost.Lost == nil || *lost.Lost != 0 {
		t.Errorf("lost = %v", lost.Lost)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	h, do := server(t)
	flag, err := h.CreateFlag("Away")
	if err != nil {
		t.Fatal(err)
	}
	target := home.TargetFlag(flag)
	if resp := do("POST", "/api/commands", fmt.Sprintf(`{"target": %q, "values": {"on": true}}`, target)); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("command: %d", resp.StatusCode)
	}
	now := time.Now()
	query := func(points int, refs string) *http.Response {
		return do("POST", "/api/history", fmt.Sprintf(`{"from": %d, "to": %d, "points": %d, "refs": [%s]}`,
			now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), points, refs))
	}
	type point struct {
		T int64
		V any
	}
	var answer struct {
		Bucket       string
		Series       [][]point
		Gaps         []any
		Availability map[string][]point
	}
	eventually(t, "the flag's history", func() (any, bool) {
		resp := query(100, fmt.Sprintf(`{"target": %q, "capability": "on"}, {"target": %q, "capability": ""}`, target, target))
		answer.Series = nil
		json.NewDecoder(resp.Body).Decode(&answer)
		return answer, resp.StatusCode == http.StatusOK && len(answer.Series) == 2 && len(answer.Series[0]) == 2
	})
	if on := answer.Series[0]; on[0].V != false || on[1].V != true || on[0].T < now.Add(-time.Minute).UnixMilli() || on[1].T > time.Now().UnixMilli() {
		t.Errorf("on = %+v, want off then on, in ms", on)
	}
	if a := answer.Availability[target.Key()]; len(a) != 1 || a[0].V != "online" || answer.Bucket != "" || answer.Gaps == nil {
		t.Errorf("answer = %+v, want raw, with the flag online and the gaps", answer)
	}
	if s := answer.Series[1]; len(s) != 1 || s[0].V != "online" {
		t.Errorf("availability series = %+v", s)
	}
	var plain map[string]any
	json.NewDecoder(query(100, fmt.Sprintf(`{"target": %q, "capability": "on"}`, target)).Body).Decode(&plain)
	if _, ok := plain["commands"]; ok || plain["runs"] != nil {
		t.Errorf("answer = %v, want no markers unasked", plain)
	}
	var marked struct {
		Commands []struct{ Origin, Status string }
	}
	eventually(t, "the flag's commands", func() (any, bool) {
		resp := do("POST", "/api/history", fmt.Sprintf(`{"from": %d, "to": %d, "points": 100, "markers": true, "refs": [{"target": %q, "capability": "on"}]}`,
			now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), target))
		json.NewDecoder(resp.Body).Decode(&marked)
		return marked, len(marked.Commands) == 1 && marked.Commands[0].Status == "confirmed"
	})
	if marked.Commands[0].Origin != "api" {
		t.Errorf("commands = %+v, want the API's", marked.Commands)
	}

	for _, c := range []struct {
		points int
		refs   string
		want   int
	}{
		{100, fmt.Sprintf(`{"target": %q, "capability": "brightness"}`, target), http.StatusNotFound},
		{100, `{"target": "flag:unknown", "capability": "on"}`, http.StatusNotFound},
		{100, `{"target": "nonsense", "capability": "on"}`, http.StatusBadRequest},
		{0, fmt.Sprintf(`{"target": %q, "capability": "on"}`, target), http.StatusBadRequest},
		{1e9, fmt.Sprintf(`{"target": %q, "capability": "on"}`, target), http.StatusBadRequest},
	} {
		if resp := query(c.points, c.refs); resp.StatusCode != c.want {
			t.Errorf("%d points of %s: %d, want %d", c.points, c.refs, resp.StatusCode, c.want)
		}
	}
	for _, body := range []string{
		`{"from": 2, "to": 1, "points": 10, "refs": []}`,
		`{"from": 0, "to": 9300000000000000, "points": 10, "refs": []}`, // past what Unix ns hold
	} {
		if resp := do("POST", "/api/history", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want refused", body, resp.StatusCode)
		}
	}
}

func TestPeriodsEndpoint(t *testing.T) {
	h, do := server(t)
	flag, err := h.CreateFlag("Away")
	if err != nil {
		t.Fatal(err)
	}
	target := home.TargetFlag(flag)
	now := time.Now()
	query := func(per, measure string, ref string) *http.Response {
		return do("POST", "/api/history/periods", fmt.Sprintf(`{"from": %d, "to": %d, "per": %q, "items": [{"ref": %s, "measure": %q}]}`,
			now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), per, ref, measure))
	}
	on := fmt.Sprintf(`{"target": %q, "capability": "on"}`, target)
	var answer struct {
		End   int64
		First *int64
		Items [][]struct {
			Start      int64
			Value      any
			Incomplete bool
			Previous   *struct{ Value, Total any }
		}
	}
	resp := query("span", "time_on", on)
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("%d: %v", resp.StatusCode, err)
	}
	if len(answer.Items) != 1 || len(answer.Items[0]) != 1 || answer.End != now.Add(time.Hour).UnixMilli() || answer.First == nil {
		t.Fatalf("answer = %+v, want one period over the span", answer)
	}
	if p := answer.Items[0][0]; p.Start != now.Add(-time.Hour).UnixMilli() || p.Value != 0.0 || !p.Incomplete || p.Previous == nil {
		t.Errorf("period = %+v, want none of it on, unknown before the flag, compared with the span before", p)
	}
	for _, c := range []struct {
		per, measure, ref string
		want              int
	}{
		{"year", "time_on", on, http.StatusBadRequest},
		{"day", "stats", on, http.StatusBadRequest},
		{"day", "energy", on, http.StatusBadRequest},
		{"day", "count", `{"target": "flag:unknown", "capability": "on"}`, http.StatusNotFound},
	} {
		if resp := query(c.per, c.measure, c.ref); resp.StatusCode != c.want {
			t.Errorf("%s per %s of %s: %d, want %d", c.measure, c.per, c.ref, resp.StatusCode, c.want)
		}
	}
	if resp := do("POST", "/api/history/periods", `{"from": 2, "to": 1, "per": "day", "items": []}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("from after to: %d, want refused", resp.StatusCode)
	}
}

type nopBridge struct{}

func (nopBridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

func TestAnAggregatesEnergyIsItsMembersCountersSummed(t *testing.T) {
	h, do := server(t)
	port := h.Attach("z2m", nopBridge{})
	port.SetOnline(true)
	plug := func(address string) bridge.Device {
		return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: []home.Capability{
			{Key: "energy", Type: home.Numeric, Unit: "kWh", Category: home.Primary, Access: home.Access{Observable: true}, Counter: true}}}}}
	}
	port.SyncDevices([]bridge.Device{plug("0x1"), plug("0x2")})
	snap, _, cancel := h.Subscribe()
	cancel()
	var members []home.Target
	for _, d := range snap.Devices {
		members = append(members, home.TargetDevice(d.ID, "switch"))
	}
	id, err := h.CreateAggregate("Plugs", members, "", home.Sum)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	energy := func(kWh float64) []bridge.Reading {
		return []bridge.Reading{{Function: "switch", Capability: "energy", Data: kWh}}
	}
	// The Aggregate's own sum leaps by 100 kWh as 0x2 first reports.
	port.Report("0x1", energy(10), now.Add(-50*time.Minute))
	port.Report("0x2", energy(100), now.Add(-40*time.Minute))
	port.Report("0x1", energy(11), now.Add(-20*time.Minute))
	port.Report("0x2", energy(102), now.Add(-10*time.Minute))
	body := fmt.Sprintf(`{"from": %d, "to": %d, "per": "span", "items": [{"ref": {"target": %q, "capability": "energy"}, "measure": "energy"}]}`,
		now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), home.TargetAggregate(id))
	eventually(t, "the plugs' energy", func() (any, bool) {
		var answer struct{ Items [][]struct{ Value any } }
		json.NewDecoder(do("POST", "/api/history/periods", body).Body).Decode(&answer)
		return answer, len(answer.Items) == 1 && len(answer.Items[0]) == 1 && answer.Items[0][0].Value == 3.0
	})
}
