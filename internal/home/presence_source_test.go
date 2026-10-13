package home

import (
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

// phone is a phone on the router: a network Function, connected or not.
func phone(address string) bridge.Device {
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "network", Kind: "network", Capabilities: []Capability{
		{Key: "connected", Type: Binary, Category: Primary, Access: Access{Observable: true}},
	}}}}
}

// sourced is presences(t) with its Bridge online, a phone, a motion sensor,
// a lamp, a thermometer and a remote, and a Flag: Alice's phone, the hall
// sensor, the lamp and the Flag as Targets.
func sourced(t *testing.T) (h *Home, updates <-chan Update, dir string, mobile, hall, light, flag Target) {
	t.Helper()
	h, updates, dir = presences(t)
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{phone("0xphone"), motion("0xhall"), lamp("0xlamp", 0, 254), thermometer("0xtherm"), remote})
	id, err := h.CreateFlag("Alice home")
	if err != nil {
		t.Fatal(err)
	}
	drain(updates)
	return h, updates, dir, TargetDevice(idOf(t, h, "0xphone"), "network"), TargetDevice(idOf(t, h, "0xhall"), "occupancy"),
		TargetDevice(idOf(t, h, "0xlamp"), "light"), TargetFlag(id)
}

func connected(h *Home, address string, on bool) {
	port(h).Report(address, []bridge.Reading{{Function: "network", Capability: "connected", Data: on}}, time.Now())
}

func seen(h *Home, address string, on bool) {
	port(h).Report(address, []bridge.Reading{{Function: "occupancy", Capability: "occupancy", Data: on}}, time.Now())
}

func setFlag(t *testing.T, h *Home, f Target, on bool) {
	t.Helper()
	if _, err := h.Command(f, Request{Values: map[string]any{"on": on}}); err != nil {
		t.Fatal(err)
	}
}

func bind(t *testing.T, h *Home, id PersonID, sources ...PresenceSource) {
	t.Helper()
	if err := h.SetPresenceSources(id, sources); err != nil {
		t.Fatal(err)
	}
}

// presenceIs fails unless id's Presence is home as want says, manual as
// byHand says; want nil is no Value.
func presenceIs(t *testing.T, h *Home, id PersonID, want any, byHand bool) {
	t.Helper()
	v, ok := value(h, homeRef(id))
	switch {
	case want == nil && ok:
		t.Errorf("home = %v, want no Value", v.Data)
	case want != nil && v.Data != want:
		t.Errorf("home = %v, want %v", v.Data, want)
	}
	if m, _ := value(h, manualRef(id)); want != nil && m.Data != byHand {
		t.Errorf("manual = %v, want %v", m.Data, byHand)
	}
}

func sourcesOf(h *Home, id PersonID) []PresenceSource {
	for _, p := range snapshot(h).Presences {
		if p.ID == id {
			return p.Sources
		}
	}
	return nil
}

// A Presence source is read through the Roles the guide's table lists, as
// web/src/roles.test.ts keeps the web client's.
func TestPresenceSourcesReadTheGuidesRoles(t *testing.T) {
	guide, err := os.ReadFile("../../docs/write-a-bridge.md")
	if err != nil {
		t.Fatal(err)
	}
	row := func(role string) []string {
		var keys []string
		for line := range strings.Lines(string(guide)) {
			if cells := strings.Split(line, "|"); strings.HasPrefix(line, "| "+role+" |") && len(cells) > 2 {
				for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(cells[2], -1) {
					keys = append(keys, m[1])
				}
			}
		}
		return keys
	}
	if got := row("Main control"); !slices.Equal(got, mainControls) {
		t.Errorf("the guide's main controls are %v, Home's %v", got, mainControls)
	}
	if got := row("State"); !slices.Equal(got, stateKeys) {
		t.Errorf("the guide's states are %v, Home's %v", got, stateKeys)
	}
}

func TestAPresenceIsHomeWhileAnySourceCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, updates, _, mobile, _, _, flag := sourced(t)
		bind(t, h, alice, PresenceSource{mobile, 900}, PresenceSource{flag, 0})
		// The Flag is off: away, until the phone tells.
		presenceIs(t, h, alice, false, false)

		connected(h, "0xphone", true)
		presenceIs(t, h, alice, true, false) // an arrival counts at once
		setFlag(t, h, flag, false)
		connected(h, "0xphone", false)
		left := time.Now()
		time.Sleep(10 * time.Minute)
		connected(h, "0xphone", false) // a refresh: the delay still counts from when it went off
		time.Sleep(5*time.Minute - time.Second)
		synctest.Wait()
		presenceIs(t, h, alice, true, false)

		drain(updates)
		time.Sleep(time.Second) // the delay runs out, with no report
		synctest.Wait()
		presenceIs(t, h, alice, false, false)
		if v, _ := value(h, homeRef(alice)); !v.Since.Equal(left.Add(15 * time.Minute)) {
			t.Errorf("away since %v, want 15 min after the phone left", v.Since)
		}
		if us := drain(updates); len(us) != 1 || us[0].Kind != ValueChanged || us[0].Initial || *us[0].Ref != homeRef(alice) {
			t.Errorf("the delay running out announced %+v", us)
		}

		setFlag(t, h, flag, true)
		presenceIs(t, h, alice, true, false)
		setFlag(t, h, flag, false) // no delay
		presenceIs(t, h, alice, false, false)
	})
}

func TestUnheardSourcesAreIgnoredAndTheLastValueKept(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, _, mobile, hall, _, _ := sourced(t)
		bind(t, h, alice, PresenceSource{mobile, 0}, PresenceSource{hall, 0})
		presenceIs(t, h, alice, nil, false) // neither has a Value

		connected(h, "0xphone", true)
		presenceIs(t, h, alice, true, false)
		port(h).SetAvailability("0xphone", Offline) // the phone ignored, the sensor silent
		time.Sleep(time.Hour)
		synctest.Wait()
		presenceIs(t, h, alice, true, false)
		seen(h, "0xhall", false)
		presenceIs(t, h, alice, false, false)

		port(h).SetAvailability("0xphone", Online)
		presenceIs(t, h, alice, true, false)
		port(h).SetOnline(false) // the Bridge too: none heard
		presenceIs(t, h, alice, true, false)
		port(h).SetOnline(true)
		connected(h, "0xphone", false)
		presenceIs(t, h, alice, false, false)
	})
}

func TestAnOverrideHoldsUntilTheRuleAgrees(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, dir, _, _, _, flag := sourced(t)
		set := func(id PersonID, home bool) {
			t.Helper()
			if _, err := h.Command(TargetPresence(id), Request{Values: map[string]any{"home": home}}); err != nil {
				t.Fatal(err)
			}
		}
		bind(t, h, alice, PresenceSource{flag, 0})
		setFlag(t, h, flag, true)
		presenceIs(t, h, alice, true, false)

		set(alice, false) // the phone stayed home
		presenceIs(t, h, alice, false, true)
		time.Sleep(time.Hour)
		synctest.Wait()
		setFlag(t, h, flag, true) // a refresh changes nothing
		presenceIs(t, h, alice, false, true)
		setFlag(t, h, flag, false)
		presenceIs(t, h, alice, false, false) // the rule agrees: the override ends

		set(alice, true)
		presenceIs(t, h, alice, true, true)
		set(alice, false) // what the rule computes: cleared at once
		presenceIs(t, h, alice, false, false)

		set(alice, true)
		other, _ := h.CreateFlag("Alice's car")
		setFlag(t, h, TargetFlag(other), true)
		bind(t, h, alice, PresenceSource{TargetFlag(other), 0}) // rebinding: the rule now agrees
		presenceIs(t, h, alice, true, false)

		set(bob, true) // no Presence source: nothing ends it
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		presenceIs(t, h, bob, true, true)
		if saved := onDisk(t, dir).presences; saved[1].ID != bob || saved[1].Manual == nil || saved[1].Manual.Data != true {
			t.Errorf("saved %+v", saved)
		}
	})
}

func TestPresenceSourcesAreChecked(t *testing.T) {
	h, updates, dir, mobile, hall, light, flag := sourced(t)
	lights, err := h.CreateAggregate("Lights", []Target{light}, Any, "")
	if err != nil {
		t.Fatal(err)
	}
	bind(t, h, alice, PresenceSource{mobile, 900})
	drain(updates)

	for name, c := range map[string]struct {
		id     PersonID
		source PresenceSource
		want   error
	}{
		"no such Person":   {chloe, PresenceSource{mobile, 0}, ErrNotFound},
		"unknown device":   {alice, PresenceSource{TargetDevice("nope", "network"), 0}, ErrNotFound},
		"unknown function": {alice, PresenceSource{TargetDevice(mobile.Device(), "light"), 0}, ErrNotFound},
		"unknown flag":     {alice, PresenceSource{TargetFlag("nope"), 0}, ErrNotFound},
		"a Device itself":  {alice, PresenceSource{TargetDevice(mobile.Device(), ""), 0}, ErrInvalid},
		"a Presence":       {alice, PresenceSource{TargetPresence(bob), 0}, ErrInvalid},
		"no target":        {alice, PresenceSource{}, ErrInvalid},
		"a reading":        {alice, PresenceSource{TargetDevice(idOf(t, h, "0xtherm"), "temperature"), 0}, ErrInvalid},
		"an Event":         {alice, PresenceSource{TargetDevice(idOf(t, h, "0xremote"), "button"), 0}, ErrInvalid},
		"a negative delay": {alice, PresenceSource{hall, -1}, ErrInvalid},
	} {
		if err := h.SetPresenceSources(c.id, []PresenceSource{{flag, 0}, c.source}); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if us := drain(updates); len(us) != 0 || !slices.Equal(sourcesOf(h, alice), []PresenceSource{{mobile, 900}}) {
		t.Fatalf("refused bindings changed %v, announced %+v", sourcesOf(h, alice), us)
	}

	// A light's state, a Flag's on, an Aggregate's main control and a state; the phone listed twice.
	want := []PresenceSource{{mobile, 900}, {light, 0}, {flag, 60}, {TargetAggregate(lights), 0}, {hall, 300}}
	bind(t, h, alice, append(want, PresenceSource{mobile, 0})...)
	bind(t, h, bob, PresenceSource{mobile, 0}) // one Presence source, two Persons
	if got := sourcesOf(h, alice); !slices.Equal(got, want) {
		t.Errorf("sources = %v, want %v", got, want)
	}
	if us := drain(updates); len(us) == 0 || us[0].Kind != PresencesChanged || !slices.Equal(us[0].Presences[0].Sources, want) {
		t.Errorf("binding announced %+v", us)
	}
	restarted := restart(t, dir)
	restarted.SetPersons([]Person{{alice, "Alice"}, {bob, "Bob"}})
	if got := sourcesOf(restarted, alice); !slices.Equal(got, want) {
		t.Errorf("sources after a restart = %v, want %v", got, want)
	}
	if got := sourcesOf(restarted, bob); !slices.Equal(got, []PresenceSource{{mobile, 0}}) {
		t.Errorf("Bob's sources after a restart = %v", got)
	}
}

func TestAPresenceSourceFollowsItsTarget(t *testing.T) {
	h, updates, _, mobile, hall, _, flag := sourced(t)
	cluster, err := h.CreateAggregate("Hall", []Target{hall}, Any, "")
	if err != nil {
		t.Fatal(err)
	}
	all := []PresenceSource{{mobile, 0}, {hall, 0}, {flag, 0}, {TargetAggregate(cluster), 0}}
	bind(t, h, alice, all...)
	drain(updates)
	unbound := func(what string, gone Target) {
		t.Helper()
		all = slices.DeleteFunc(all, func(s PresenceSource) bool { return s.Target == gone })
		if got := sourcesOf(h, alice); !slices.Equal(got, all) {
			t.Errorf("once %s: sources = %v, want %v", what, got, all)
		}
		us := drain(updates)
		if i := slices.IndexFunc(us, func(u Update) bool { return u.Kind == PresencesChanged }); i < 0 || !slices.Equal(us[i].Presences[0].Sources, all) {
			t.Errorf("once %s: announced %+v", what, us)
		}
	}
	if err := h.DeleteFlag(flag.Flag()); err != nil {
		t.Fatal(err)
	}
	unbound("the Flag is deleted", flag)
	if err := h.DeleteAggregate(cluster); err != nil {
		t.Fatal(err)
	}
	unbound("the Aggregate is deleted", TargetAggregate(cluster))

	// The phone vanishes from its description, then is Detached: still bound, ignored.
	connected(h, "0xphone", true)
	presenceIs(t, h, alice, true, false)
	port(h).SyncDevices([]bridge.Device{{NativeAddress: "0xphone", Name: "0xphone"}, motion("0xhall")})
	port(h).SyncDevices([]bridge.Device{motion("0xhall")})
	if got := sourcesOf(h, alice); !slices.Equal(got, all) {
		t.Errorf("the Detached phone is unbound: %v", got)
	}
	seen(h, "0xhall", false)
	presenceIs(t, h, alice, false, false)

	// A new phone replaces it: the binding holds, and the new phone counts.
	port(h).SyncDevices([]bridge.Device{motion("0xhall"), phone("0xphone2")})
	connected(h, "0xphone2", true)
	if err := h.Replace(mobile.Device(), idOf(t, h, "0xphone2")); err != nil {
		t.Fatal(err)
	}
	if got := sourcesOf(h, alice); !slices.Equal(got, all) {
		t.Errorf("a Replace unbound the phone: %v", got)
	}
	presenceIs(t, h, alice, true, false)

	port(h).SyncDevices([]bridge.Device{motion("0xhall")})
	drain(updates)
	if err := h.Delete(mobile.Device()); err != nil {
		t.Fatal(err)
	}
	unbound("the phone is deleted", mobile)
}
