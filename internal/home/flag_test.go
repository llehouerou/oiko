package home

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// flags is a Home without a Bridge, whose saved Flags are captured as they
// would be on disk.
func flags(t *testing.T) (*Home, <-chan Update, *[]SavedFlag) {
	t.Helper()
	saved := &[]SavedFlag{}
	h := newHome(&fakeBridge{}, nil, nil, nil, nil, nil, func(f []SavedFlag) error { *saved = f; return nil }, nil)
	_, updates, cancel := h.Subscribe()
	t.Cleanup(cancel)
	return h, updates, saved
}

func flagValue(h *Home, id FlagID) (Value, bool) {
	ref := TargetFlag(id).Ref("on")
	i := slices.IndexFunc(snapshot(h).Values, func(rv RefValue) bool { return rv.Ref == ref })
	if i < 0 {
		return Value{}, false
	}
	return snapshot(h).Values[i].Value, true
}

func TestNewFlagIsOffAndOnline(t *testing.T) {
	h, updates, saved := flags(t)
	id, err := h.CreateFlag("  Night ")
	if err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if ks := kinds(us); !slices.Equal(ks, []UpdateKind{FlagsChanged, ValueChanged, AvailabilityChanged}) {
		t.Fatalf("creation kinds = %v", ks)
	}
	if f := us[0].Flags; len(f) != 1 || f[0].ID != id || f[0].Name != "Night" || f[0].Kind != "flag" || len(f[0].Capabilities) != 1 || f[0].Capabilities[0].Key != "on" {
		t.Errorf("announced %+v", us[0].Flags)
	}
	if u := us[2]; u.Target != TargetFlag(id) || u.Availability != Online {
		t.Errorf("availability update = %+v, want the Flag online", u)
	}
	s := snapshot(h)
	if len(s.Flags) != 1 || s.Availability[TargetFlag(id)] != Online {
		t.Fatalf("snapshot flags %+v, availability %+v", s.Flags, s.Availability)
	}
	if v, ok := flagValue(h, id); !ok || v.Data != false {
		t.Errorf("value = %+v, %v; want off", v, ok)
	}
	if len(*saved) != 1 || (*saved)[0].ID != id || (*saved)[0].Value.Data != false {
		t.Errorf("saved = %+v", *saved)
	}

	for name, bad := range map[string]string{"blank": " ", "long": strings.Repeat("x", 101)} {
		if _, err := h.CreateFlag(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s name: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestFlagCommandIsConfirmedAtOnceWithoutBridge(t *testing.T) {
	h, updates, saved := flags(t)
	id, _ := h.CreateFlag("Night")
	drain(updates)

	for _, step := range []struct {
		set  any
		want bool
	}{{true, true}, {"toggle", false}, {"toggle", true}} {
		want := step.want
		values := map[string]any{"on": step.set}
		if _, err := h.Command(TargetFlag(id), Request{Values: values}); err != nil {
			t.Fatal(err)
		}
		us := drain(updates)
		if ks := kinds(us); !slices.Equal(ks, []UpdateKind{CommandChanged, ValueChanged, CommandChanged}) {
			t.Fatalf("kinds = %v", ks)
		}
		if ss := commandStatuses(us); ss[0] != Pending || ss[1] != Confirmed || us[0].Command.Target != TargetFlag(id) {
			t.Errorf("command updates = %+v", us)
		}
		if u := us[1]; *u.Ref != TargetFlag(id).Ref("on") || u.Value.Data != want {
			t.Errorf("value update = %+v, want %v", u, want)
		}
		if (*saved)[0].Value.Data != want {
			t.Errorf("saved %+v, want %v", *saved, want)
		}
	}

	port(h).SetOnline(true)
	port(h).SetOnline(false)
	drain(updates)
	if _, err := h.Command(TargetFlag(id), Request{Values: map[string]any{"on": "toggle"}}); err != nil {
		t.Fatalf("while the Bridge is offline: %v", err)
	}
	if v, _ := flagValue(h, id); v.Data != false {
		t.Errorf("toggled value = %v, want off", v.Data)
	}
	for name, values := range map[string]map[string]any{"unknown capability": {"brightness": 1.0}, "not a bool": {"on": 1.0}} {
		if _, err := h.Command(TargetFlag(id), Request{Values: values}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := h.Command(TargetFlag("nope"), Request{Values: map[string]any{"on": true}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown flag: err = %v, want ErrNotFound", err)
	}
}

func TestFlagSurvivesRestart(t *testing.T) {
	h, _, saved := flags(t)
	id, _ := h.CreateFlag("Holiday")
	h.Command(TargetFlag(id), Request{Values: map[string]any{"on": true}})
	before, _ := flagValue(h, id)

	restarted := newHome(&fakeBridge{}, nil, nil, *saved, nil, nil, nil, nil)
	s := snapshot(restarted)
	if len(s.Flags) != 1 || s.Flags[0].ID != id || s.Flags[0].Name != "Holiday" || s.Availability[TargetFlag(id)] != Online {
		t.Fatalf("restored %+v, availability %+v", s.Flags, s.Availability)
	}
	if v, ok := flagValue(restarted, id); !ok || v.Data != true || !v.At.Equal(before.At) {
		t.Errorf("restored value %+v, want %+v", v, before)
	}
}

func TestRenameAndDeleteFlag(t *testing.T) {
	h, updates, saved := flags(t)
	id, _ := h.CreateFlag("Night")
	drain(updates)

	if err := h.RenameFlag(id, " Night mode "); err != nil {
		t.Fatal(err)
	}
	if us := drain(updates); len(us) != 1 || us[0].Kind != FlagsChanged || us[0].Flags[0].Name != "Night mode" || (*saved)[0].Name != "Night mode" {
		t.Fatalf("rename updates %+v, saved %+v", us, *saved)
	}
	if err := h.RenameFlag(id, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("blank rename: err = %v", err)
	}
	if err := h.RenameFlag("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown rename: err = %v", err)
	}

	if err := h.DeleteFlag(id); err != nil {
		t.Fatal(err)
	}
	if us := drain(updates); len(us) != 2 || us[0].Kind != TargetDeleted || us[0].Target != TargetFlag(id) ||
		us[1].Kind != FlagsChanged || len(us[1].Flags) != 0 || len(*saved) != 0 {
		t.Fatalf("delete updates %+v, saved %+v", us, *saved)
	}
	if _, ok := flagValue(h, id); ok || len(snapshot(h).Flags) != 0 {
		t.Error("the deleted Flag is still in the Snapshot")
	}
	if err := h.DeleteFlag(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: err = %v", err)
	}
}

func TestAggregateOfFlags(t *testing.T) {
	h, updates, saved := flags(t)
	night, _ := h.CreateFlag("Night")
	holiday, _ := h.CreateFlag("Holiday")
	id, err := h.CreateAggregate("Sleeping house", []Target{TargetFlag(night), TargetFlag(holiday), TargetFlag(night)}, Any, "")
	if err != nil {
		t.Fatal(err)
	}
	a := snapshot(h).Aggregates[0]
	if a.Kind != "flag" || !slices.Equal(a.Members, []Target{TargetFlag(night), TargetFlag(holiday)}) {
		t.Fatalf("aggregate %+v", a)
	}
	if got := about(drain(updates), id); !slices.Equal(got, []string{"value on=false", "availability online"}) {
		t.Errorf("creation updates = %v", got)
	}
	if _, err := h.CreateAggregate("Mixed", []Target{TargetFlag("nope")}, Any, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown flag member: err = %v", err)
	}

	h.Command(TargetFlag(holiday), Request{Values: map[string]any{"on": true}})
	h.Command(TargetFlag(night), Request{Values: map[string]any{"on": true}})
	h.Command(TargetFlag(holiday), Request{Values: map[string]any{"on": false}})
	want := []string{"value on=true", "refresh on=true", "refresh on=true"}
	if got := about(drain(updates), id); !slices.Equal(got, want) {
		t.Errorf("updates = %v, want %v", got, want)
	}

	// Relayed while the Bridge is offline: "any" on turns every member off.
	cmd, err := h.Command(TargetAggregate(id), Request{Values: map[string]any{"on": "toggle"}})
	if err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if ss := aggregateCommands(us, id); !slices.Equal(ss, []CommandStatus{Pending, Confirmed}) || us[0].Command.ID != cmd {
		t.Errorf("aggregate command statuses = %v", ss)
	}
	if v, _ := flagValue(h, night); v.Data != false {
		t.Errorf("Night = %v after toggling the aggregate off", v.Data)
	}

	if err := h.DeleteFlag(night); err != nil {
		t.Fatal(err)
	}
	if a := snapshot(h).Aggregates[0]; !slices.Equal(a.Members, []Target{TargetFlag(holiday)}) {
		t.Errorf("members after deleting Night = %+v", a.Members)
	}

	h.Command(TargetFlag(holiday), Request{Values: map[string]any{"on": true}})
	restarted := newHome(&fakeBridge{}, nil, snapshot(h).Aggregates, *saved, nil, nil, nil, nil)
	s := snapshot(restarted)
	if !slices.ContainsFunc(s.Values, func(rv RefValue) bool { return rv.Ref == TargetAggregate(id).Ref("on") && rv.Value.Data == true }) || s.Availability[TargetAggregate(id)] != Online {
		t.Errorf("after a restart: values %+v, availability %q; want on and online", s.Values, s.Availability[TargetAggregate(id)])
	}
}
