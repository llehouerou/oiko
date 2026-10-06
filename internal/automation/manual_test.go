package automation

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/llehouerou/oiko/internal/home"
)

func TestManualTriggerRunsALiveAutomationOnly(t *testing.T) {
	f := fixture()
	var traces []*Trace
	e := New(f, nil, nil, func(tr *Trace) { traces = append(traces, tr) })
	d := doc("lamps", []string{`previous manualTrigger {}`, cmd("set", `"flag:a"`), pressSingle}, "previous.out set.in")
	id := create(t, e, d)
	alice := home.Origin{Person: "alice"}

	end, err := e.Trigger(id, "previous", alice)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, f, "run", "flag a map[on:true] 0s")
	if end.Outcome != home.RunActed || end.Commands != 1 || end.Trigger.Step != "previous" || end.Trigger.Kind != manualTrigger {
		t.Errorf("run ended %+v", end)
	}
	// Who started it is on its trigger; its Commands are the Run's.
	if end.Trigger.By == nil || *end.Trigger.By != alice {
		t.Errorf("run ended started by %v, want %v", end.Trigger.By, alice)
	}
	if len(traces) != 1 || traces[0].Trigger.By == nil || *traces[0].Trigger.By != alice {
		t.Errorf("traces = %+v, want one started by %v", traces, alice)
	}
	if want := (home.Origin{Automation: id, Step: "set", Run: end.Run}); len(f.origins) != 1 || f.origins[0] != want {
		t.Errorf("origins = %+v, want %+v", f.origins, want)
	}

	for _, c := range []struct {
		automation, step string
		want             error
	}{
		{"nope", "previous", home.ErrNotFound},
		{id, "nope", home.ErrNotFound},
		{id, "single", home.ErrInvalid},
	} {
		if _, err := e.Trigger(c.automation, c.step, alice); !errors.Is(err, c.want) {
			t.Errorf("%s %s: %v, want %v", c.automation, c.step, err, c.want)
		}
	}

	d.Enabled = false
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Trigger(id, "previous", alice); !errors.Is(err, home.ErrNotRunning) {
		t.Errorf("disabled: %v, want %v", err, home.ErrNotRunning)
	}
	expect(t, f, "while disabled")
}

func TestAStatusIsTheAutomationsTileAndFollowsItsEdits(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil)
	d := doc("lamps", []string{pressSingle, cmd("set", `"flag:a"`)}, "single.out set.in")
	id := create(t, e, d)
	if s := statusOf(f, id); s.Name != "lamps" || s.ManualTriggers != nil {
		t.Errorf("status = %+v, want lamps without Manual triggers", s)
	}

	// Neither a rename nor a Manual trigger changes how it runs; both change its Tile.
	d.Name = "Lamps"
	d.Steps = append(d.Steps, Step{ID: "go", Kind: manualTrigger, Name: "Go", Params: json.RawMessage(`{}`)})
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	want := home.AutomationStatus{ID: id, Name: "Lamps", Status: home.AutomationEnabled, ManualTriggers: []home.ManualTrigger{{Step: "go", Name: "Go"}}}
	if s := statusOf(f, id); !reflect.DeepEqual(s, want) {
		t.Errorf("status = %+v, want %+v", s, want)
	}
}
