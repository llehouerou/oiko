package automation_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
)

type nopBridge struct{}

func (nopBridge) Send(context.Context, string, string, map[string]any, time.Duration) error {
	return nil
}

func TestBurstOf1000ReportsLosesNoTrace(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := home.New(store.Command)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.Replayed()
	z.SyncDevices([]bridge.Device{
		{NativeAddress: "0xlamp", Functions: []bridge.Function{{Key: "light", Kind: "light", Capabilities: []home.Capability{
			{Key: "state", Type: home.Binary, Access: home.Access{Observable: true, Settable: true}},
		}}}},
		{NativeAddress: "0xremote", Functions: []bridge.Function{{Key: "button", Kind: "button", Capabilities: []home.Capability{
			{Key: "action", Type: home.Enum, Stateless: true, Access: home.Access{Observable: true}},
		}}}},
	})
	var mu sync.Mutex
	var runs []uuid.UUID
	snap, unfollow := h.Follow(func(u home.Update) {
		if u.Kind == home.RunEnded {
			mu.Lock()
			runs = append(runs, u.Run.Run)
			mu.Unlock()
		}
	})
	defer unfollow()
	ids := map[string]home.DeviceID{}
	for _, d := range snap.Devices {
		ids[d.NativeAddress] = d.ID
	}
	e := automation.New(h, []automation.Document{{Name: "press", Enabled: true,
		Steps: []automation.Step{
			{ID: "press", Kind: "eventTrigger", Params: json.RawMessage(`{"target": "device:` + string(ids["0xremote"]) + `/button", "capability": "action", "events": ["single"]}`)},
			{ID: "on", Kind: "command", Params: json.RawMessage(`{"targets": ["device:` + string(ids["0xlamp"]) + `/light"], "values": {"state": true}}`)},
		},
		Edges: []automation.Edge{{From: automation.Port{Step: "press", Handle: "out"}, To: automation.Port{Step: "on", Handle: "in"}}},
	}}, nil, nil, nil, nil, store.Record)
	automation.SpaceRuns(e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)
	writerCtx, stopWriter := context.WithCancel(context.Background())
	writerDone := make(chan struct{})
	go func() {
		store.Run(writerCtx)
		close(writerDone)
	}()

	for range 1000 {
		z.Report("0xremote", []bridge.Reading{{Function: "button", Capability: "action", Data: "single"}}, time.Now())
	}
	ended := func() []uuid.UUID {
		mu.Lock()
		defer mu.Unlock()
		return runs
	}
	for deadline := time.Now().Add(10 * time.Second); len(ended()) < 1000 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	stopWriter()
	<-writerDone

	if n := store.Lost(); n != 0 {
		t.Errorf("%d entries lost", n)
	}
	if n := len(ended()); n != 1000 {
		t.Fatalf("%d Runs for 1000 presses", n)
	}
	for _, run := range ended() {
		if _, err := store.Trace(run.String()); err != nil {
			t.Fatalf("trace of run %s: %v", run, err)
		}
	}
}
