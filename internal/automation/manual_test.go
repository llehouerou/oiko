package automation

import (
	"errors"
	"testing"

	"github.com/llehouerou/oiko/internal/home"
)

func TestManualTriggerRunsALiveAutomationOnly(t *testing.T) {
	f := fixture()
	e := New(f, nil, nil, nil, nil, nil, nil)
	d := doc("lamps", []string{`previous manualTrigger {}`, cmd("set", `"flag:a"`), pressSingle}, "previous.out set.in")
	id := create(t, e, d)

	end, err := e.Trigger(id, "previous")
	if err != nil {
		t.Fatal(err)
	}
	expect(t, f, "run", "flag a map[on:true] 0s")
	if end.Outcome != home.RunActed || end.Commands != 1 || end.Trigger.Step != "previous" || end.Trigger.Kind != manualTrigger {
		t.Errorf("run ended %+v", end)
	}

	for _, c := range []struct {
		automation, step string
		want             error
	}{
		{"nope", "previous", home.ErrNotFound},
		{id, "nope", home.ErrNotFound},
		{id, "single", home.ErrInvalid},
	} {
		if _, err := e.Trigger(c.automation, c.step); !errors.Is(err, c.want) {
			t.Errorf("%s %s: %v, want %v", c.automation, c.step, err, c.want)
		}
	}

	d.Enabled = false
	if err := e.Replace(id, d); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Trigger(id, "previous"); !errors.Is(err, home.ErrNotRunning) {
		t.Errorf("disabled: %v, want %v", err, home.ErrNotRunning)
	}
	expect(t, f, "while disabled")
}
