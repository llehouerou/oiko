package home

import (
	"errors"
	"slices"
	"testing"
)

const (
	alice PersonID = "0190a1ce-0000-7000-8000-000000000001"
	bob   PersonID = "0190a1ce-0000-7000-8000-000000000002"
	chloe PersonID = "0190a1ce-0000-7000-8000-000000000003"
)

// presences is a Home given Alice and Bob at start, saved in the dir it
// answers.
func presences(t *testing.T) (*Home, <-chan Update, string) {
	t.Helper()
	dir := t.TempDir()
	h := restart(t, dir)
	if err := h.SetPersons([]Person{{bob, "Bob"}, {alice, "Alice"}}); err != nil {
		t.Fatal(err)
	}
	_, updates, cancel := h.Subscribe()
	t.Cleanup(cancel)
	return h, updates, dir
}

func value(h *Home, ref Ref) (Value, bool) {
	s := snapshot(h)
	i := slices.IndexFunc(s.Values, func(rv RefValue) bool { return rv.Ref == ref })
	if i < 0 {
		return Value{}, false
	}
	return s.Values[i].Value, true
}

func presenceNames(ps []Presence) []string {
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return names
}

func TestAPresenceForEachPerson(t *testing.T) {
	h, updates, _ := presences(t)
	s := snapshot(h)
	if !slices.Equal(presenceNames(s.Presences), []string{"Home", "Alice", "Bob"}) {
		t.Fatalf("presences = %+v, want the Home presence, then Alice then Bob", s.Presences)
	}
	p := s.Presences[1]
	if p.ID != alice || p.Kind != "presence" || len(p.Capabilities) != 2 || p.Capabilities[0].Key != "home" || !p.Capabilities[0].Access.Settable ||
		p.Capabilities[1].Key != "manual" || p.Capabilities[1].Access.Settable {
		t.Errorf("Alice's presence = %+v", p)
	}
	if s.Availability[TargetPresence(alice)] != Online {
		t.Errorf("availability = %q, want online", s.Availability[TargetPresence(alice)])
	}
	if _, ok := value(h, TargetPresence(alice).Ref("home")); ok {
		t.Error("a Presence has a Value before anything told")
	}

	if err := h.SetPersons([]Person{{alice, "Alice"}, {bob, "Bob"}, {chloe, "Chloe"}}); err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if ks := kinds(us); !slices.Equal(ks, []UpdateKind{PresencesChanged, AvailabilityChanged}) ||
		!slices.Equal(presenceNames(us[0].Presences), []string{"Home", "Alice", "Bob", "Chloe"}) || us[1].Target != TargetPresence(chloe) {
		t.Fatalf("creation updates = %+v", us)
	}

	if err := h.SetPersons([]Person{{alice, "Zoe"}, {bob, "Bob"}, {chloe, "Chloe"}}); err != nil {
		t.Fatal(err)
	}
	if us := drain(updates); len(us) != 1 || us[0].Kind != PresencesChanged || !slices.Equal(presenceNames(us[0].Presences), []string{"Home", "Bob", "Chloe", "Zoe"}) {
		t.Fatalf("rename updates = %+v", us)
	}
	h.SetPersons([]Person{{alice, "Zoe"}, {bob, "Bob"}, {chloe, "Chloe"}})
	if us := drain(updates); len(us) != 0 {
		t.Errorf("unchanged Persons announced %+v", us)
	}
}

func TestRemovingAPersonDeletesTheirPresence(t *testing.T) {
	h, updates, dir := presences(t)
	h.Command(TargetPresence(bob), Request{Values: map[string]any{"home": true}})
	drain(updates)

	if err := h.SetPersons([]Person{{alice, "Alice"}}); err != nil {
		t.Fatal(err)
	}
	us := drain(updates)
	if ks := kinds(us); !slices.Equal(ks, []UpdateKind{TargetDeleted, PresencesChanged}) || us[0].Target != TargetPresence(bob) ||
		!slices.Equal(presenceNames(us[1].Presences), []string{"Home", "Alice"}) {
		t.Fatalf("removal updates = %+v", us)
	}
	if _, ok := value(h, TargetPresence(bob).Ref("home")); ok {
		t.Error("the removed Person's Presence keeps its Value")
	}
	if saved := onDisk(t, dir).presences; len(saved) != 2 || saved[1].ID != alice {
		t.Errorf("saved %+v, want the Home presence's and Alice's only", saved)
	}
	if _, err := h.Command(TargetPresence(bob), Request{Values: map[string]any{"home": true}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a Command on the removed Presence: err = %v", err)
	}
}

func TestAggregateOfPresences(t *testing.T) {
	h, updates, dir := presences(t)
	children := []Target{TargetPresence(alice), TargetPresence(bob), TargetPresence(alice)}
	anyHome, err := h.CreateAggregate("Children", children, Any, "")
	if err != nil {
		t.Fatal(err)
	}
	allHome, _ := h.CreateAggregate("All children", children, All, "")
	if a := snapshot(h).Aggregates[1]; a.ID != anyHome || a.Kind != "presence" || !slices.Equal(a.Members, []Target{TargetPresence(alice), TargetPresence(bob)}) {
		t.Fatalf("aggregate %+v", a)
	}
	for name, ms := range map[string][]Target{
		"the Home presence": {TargetPresence(alice), TargetPresence(HomePresence)},
		"a Flag besides":    {TargetPresence(alice), TargetFlag(mustFlag(t, h, "Night"))},
	} {
		if _, err := h.CreateAggregate("Bad", ms, Any, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s as a member: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := h.CreateAggregate("Bad", []Target{TargetPresence(chloe)}, Any, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown Presence as a member: err = %v, want ErrNotFound", err)
	}
	drain(updates)

	// Bob has no Value: he is ignored.
	h.Command(TargetPresence(alice), Request{Values: map[string]any{"home": true}})
	if got := about(drain(updates), allHome); !slices.Equal(got, []string{"value home=true", "value manual=true"}) {
		t.Errorf("all, Alice home = %v", got)
	}
	h.Command(TargetPresence(bob), Request{Values: map[string]any{"home": false}})
	us := drain(updates)
	if got := about(us, allHome); !slices.Equal(got, []string{"value home=false", "refresh manual=true"}) {
		t.Errorf("all, Bob away = %v", got)
	}
	if got := about(us, anyHome); !slices.Equal(got, []string{"refresh home=true", "refresh manual=true"}) {
		t.Errorf("any, Bob away = %v", got)
	}

	// "All children home" reaches each of them.
	cmd, err := h.Command(TargetAggregate(allHome), Request{Values: map[string]any{"home": true}})
	if err != nil {
		t.Fatal(err)
	}
	us = drain(updates)
	if ss := aggregateCommands(us, allHome); !slices.Equal(ss, []CommandStatus{Pending, Confirmed}) || us[0].Command.ID != cmd {
		t.Errorf("aggregate command statuses = %v", ss)
	}
	for _, p := range []PersonID{alice, bob} {
		if v, _ := value(h, TargetPresence(p).Ref("home")); v.Data != true {
			t.Errorf("%s home = %v after the Aggregate's Command", p, v.Data)
		}
	}

	// A restart recalls the members' Values, so the Aggregates'; a Person gone
	// is no member any more.
	restarted := restart(t, dir)
	if err := restarted.SetPersons([]Person{{alice, "Alice"}}); err != nil {
		t.Fatal(err)
	}
	s := snapshot(restarted)
	if a := s.Aggregates[1]; a.Kind != "presence" || !slices.Equal(a.Members, []Target{TargetPresence(alice)}) {
		t.Errorf("after a restart without Bob: %+v", a)
	}
	if v, ok := value(restarted, TargetAggregate(anyHome).Ref("home")); !ok || v.Data != true {
		t.Errorf("after a restart: home = %+v, %v; want true", v, ok)
	}
	if saved := onDisk(t, dir).aggregates; !slices.Equal(saved[0].Members, []Target{TargetPresence(alice)}) {
		t.Errorf("saved %+v, want Bob dropped", saved)
	}
}

func mustFlag(t *testing.T, h *Home, name string) FlagID {
	t.Helper()
	id, err := h.CreateFlag(name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAPresenceSetByHand(t *testing.T) {
	h, updates, dir := presences(t)
	home, manual := TargetPresence(alice).Ref("home"), TargetPresence(alice).Ref("manual")

	for _, step := range []struct {
		set  any
		want bool
	}{{true, true}, {"toggle", false}, {"toggle", true}} {
		if _, err := h.Command(TargetPresence(alice), Request{Values: map[string]any{"home": step.set}}); err != nil {
			t.Fatal(err)
		}
		us := drain(updates)
		if ss := commandStatuses(us); !slices.Equal(ss, []CommandStatus{Pending, Confirmed}) || us[0].Command.Target != TargetPresence(alice) {
			t.Errorf("command updates = %+v", us)
		}
		if v, _ := value(h, home); v.Data != step.want {
			t.Errorf("home = %v, want %v", v.Data, step.want)
		}
		if v, _ := value(h, manual); v.Data != true {
			t.Errorf("manual = %v, want true", v.Data)
		}
		if saved := onDisk(t, dir).presences; saved[1].ID != alice || saved[1].Value == nil || saved[1].Value.Data != step.want || saved[1].Manual == nil || saved[1].Manual.Data != true {
			t.Errorf("saved %+v", saved)
		}
	}
	for name, values := range map[string]map[string]any{"manual": {"manual": false}, "not a bool": {"home": 1.0}, "unknown": {"on": true}} {
		if _, err := h.Command(TargetPresence(alice), Request{Values: values}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := h.Command(TargetPresence(chloe), Request{Values: map[string]any{"home": true}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such Person: err = %v", err)
	}
}

func TestAPresenceSurvivesRestart(t *testing.T) {
	h, _, dir := presences(t)
	h.Command(TargetPresence(alice), Request{Values: map[string]any{"home": false}})
	h.Command(TargetPresence(bob), Request{Values: map[string]any{"home": true}})
	before, _ := value(h, TargetPresence(alice).Ref("home"))

	restarted := restart(t, dir)
	var heard []Update
	restarted.Follow(func(u Update) { heard = append(heard, u) })
	if err := restarted.SetPersons([]Person{{alice, "Alice"}}); err != nil { // Bob was removed, and Oiko stopped before writing it
		t.Fatal(err)
	}
	if v, ok := value(restarted, TargetPresence(alice).Ref("home")); !ok || v.Data != false || !v.At.Equal(before.At) || !v.Since.Equal(before.Since) {
		t.Errorf("recalled %+v, want %+v", v, before)
	}
	if v, _ := value(restarted, TargetPresence(alice).Ref("manual")); v.Data != true {
		t.Errorf("manual = %v, want the override still holding", v.Data)
	}
	for _, u := range heard {
		if u.Kind == ValueChanged && !u.Initial {
			t.Errorf("a recalled Value is announced as a change: %+v", u)
		}
	}
	if s := snapshot(restarted); !slices.Equal(presenceNames(s.Presences), []string{"Home", "Alice"}) {
		t.Errorf("presences = %+v, want the Home presence's and Alice's only", s.Presences)
	}
	if saved := onDisk(t, dir).presences; len(saved) != 2 || saved[1].ID != alice {
		t.Errorf("saved %+v, want the orphan dropped", saved)
	}
}

func TestPresenceKeys(t *testing.T) {
	got, err := ParseTarget("presence:" + string(alice))
	if err != nil || got != TargetPresence(alice) || got.Presence() != alice || got.Key() != "presence:"+string(alice) {
		t.Errorf("parsed %+v, %v", got, err)
	}
	for _, bad := range []string{"presence:", "presence:x/y"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
