package home

import (
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

// entrance is an Entrance door's contact sensor: true while open.
func entrance(address string) bridge.Device {
	return bridge.Device{NativeAddress: address, Name: address, Functions: []bridge.Function{{Key: "contact", Kind: "contact", Capabilities: []Capability{
		{Key: "contact", Type: Binary, Category: Primary, Access: Access{Observable: true}},
	}}}}
}

func doorOpen(h *Home, address string, open bool) {
	port(h).Report(address, []bridge.Reading{{Function: "contact", Capability: "contact", Data: open}}, time.Now())
}

// boxed is presences(t) with its Bridge online, a front door, a back door and
// a hall motion sensor; the front door and the hall sensor are bound to the
// Home presence, with the default exit grace. Each door has reported closed.
func boxed(t *testing.T) (h *Home, updates <-chan Update, dir string, front, back, hall Target) {
	t.Helper()
	h, updates, dir = presences(t)
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{entrance("0xfront"), entrance("0xback"), motion("0xhall"), thermometer("0xtherm")})
	front, back, hall = TargetDevice(idOf(t, h, "0xfront"), "contact"), TargetDevice(idOf(t, h, "0xback"), "contact"), TargetDevice(idOf(t, h, "0xhall"), "occupancy")
	doorOpen(h, "0xfront", false)
	doorOpen(h, "0xback", false)
	bindHome(t, h, HomeBindings{EntranceDoors: []Target{front}, SignsOfLife: []Target{hall}, ExitGrace: DefaultExitGrace})
	drain(updates)
	return h, updates, dir, front, back, hall
}

func bindHome(t *testing.T, h *Home, b HomeBindings) {
	t.Helper()
	if err := h.SetHomeBindings(b); err != nil {
		t.Fatal(err)
	}
}

// homeIs fails unless the Home presence is home as want says; nil is no Value.
func homeIs(t *testing.T, h *Home, want any) {
	t.Helper()
	v, ok := value(h, homePresenceRef)
	switch {
	case want == nil && ok:
		t.Errorf("the Home presence = %v, want no Value", v.Data)
	case want != nil && v.Data != want:
		t.Errorf("the Home presence = %v, want %v", v.Data, want)
	}
}

// goesThrough has someone go through the front door: it opens, then closes.
func goesThrough(h *Home) {
	doorOpen(h, "0xfront", true)
	time.Sleep(5 * time.Second)
	doorOpen(h, "0xfront", false)
}

func TestTheHomePresence(t *testing.T) {
	h, _, _ := presences(t)
	s := snapshot(h)
	p := s.Presences[0]
	if p.ID != HomePresence || p.Name != "Home" || p.Kind != "presence" || len(p.Capabilities) != 1 || p.Capabilities[0].Key != "home" ||
		p.Capabilities[0].Access.Settable || p.Sources != nil || p.HomeBindings == nil || p.ExitGrace != DefaultExitGrace {
		t.Errorf("the Home presence = %+v", p)
	}
	if s.Availability[TargetPresence(HomePresence)] != Online {
		t.Errorf("availability = %q, want online", s.Availability[TargetPresence(HomePresence)])
	}
	if got, err := ParseTarget("presence:home"); err != nil || got != TargetPresence(HomePresence) {
		t.Errorf("parsed %v, %v", got, err)
	}
	homeIs(t, h, nil) // until something tells
	for _, values := range []map[string]any{{"home": true}, {"home": "toggle"}} {
		if _, err := h.Command(TargetPresence(HomePresence), Request{Values: values}); !errors.Is(err, ErrInvalid) {
			t.Errorf("a Command %v: err = %v, want ErrInvalid", values, err)
		}
	}
	if _, err := h.CreateAggregate("Everyone", []Target{TargetPresence(HomePresence)}, Any, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("an Aggregate of the Home presence: err = %v", err)
	}
}

func TestAnyPresenceHomeMakesTheHomeHome(t *testing.T) {
	h, updates, _ := presences(t)
	set := func(id PersonID, home bool) {
		t.Helper()
		if _, err := h.Command(TargetPresence(id), Request{Values: map[string]any{"home": home}}); err != nil {
			t.Fatal(err)
		}
	}
	set(alice, false) // Bob has no Value: ignored
	homeIs(t, h, false)
	set(bob, true)
	homeIs(t, h, true)
	set(alice, true)
	set(bob, false)
	drain(updates)
	set(alice, false)
	homeIs(t, h, false)
	if i := slices.IndexFunc(drain(updates), func(u Update) bool { return u.Ref != nil && *u.Ref == homePresenceRef }); i < 0 {
		t.Error("the last one leaving announced no change of the Home presence")
	}
}

func TestASignOfLifeFillsTheBoxAndADoorClosingEmptiesIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, updates, _, _, _, _ := boxed(t)
		homeIs(t, h, nil) // the doors alone tell nothing
		seen(h, "0xhall", true)
		homeIs(t, h, true) // at once
		time.Sleep(time.Minute)
		seen(h, "0xhall", false)
		time.Sleep(time.Hour) // asleep: nothing empties the box but a door
		synctest.Wait()
		homeIs(t, h, true)

		goesThrough(h) // they leave
		closed := time.Now()
		time.Sleep(DefaultExitGrace*time.Second - time.Second)
		synctest.Wait()
		homeIs(t, h, true)
		drain(updates)
		time.Sleep(time.Second)
		synctest.Wait()
		homeIs(t, h, false)
		if v, _ := value(h, homePresenceRef); !v.Since.Equal(closed.Add(DefaultExitGrace * time.Second)) {
			t.Errorf("away since %v, want the exit grace after the door closed", v.Since)
		}
		if us := drain(updates); len(us) != 1 || us[0].Kind != ValueChanged || us[0].Initial {
			t.Errorf("the exit grace running out announced %+v", us)
		}
	})
}

func TestASignOfLifeAfterTheCloseOrStillTrueKeepsTheBox(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, _, _, _, _ := boxed(t)
		seen(h, "0xhall", true)
		seen(h, "0xhall", false)
		time.Sleep(time.Hour)

		goesThrough(h) // fetching the mail
		time.Sleep(20 * time.Second)
		seen(h, "0xhall", true)
		time.Sleep(time.Minute)
		seen(h, "0xhall", false)
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, h, true)

		seen(h, "0xhall", true) // still true when the grace runs out
		goesThrough(h)
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, h, true)

		goesThrough(h) // true before the close, cleared after it: whoever left
		time.Sleep(30 * time.Second)
		seen(h, "0xhall", false)
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, h, false)
	})
}

func TestUnheardDoorsAndSignsOfLifeAreIgnored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, _, front, back, hall := boxed(t)
		bindHome(t, h, HomeBindings{EntranceDoors: []Target{front, back}, SignsOfLife: []Target{hall}, ExitGrace: DefaultExitGrace})
		port(h).SetAvailability("0xhall", Offline)
		seen(h, "0xhall", true)
		homeIs(t, h, nil)
		port(h).SetAvailability("0xhall", Online)
		seen(h, "0xhall", false)
		seen(h, "0xhall", true)
		homeIs(t, h, true)
		seen(h, "0xhall", false)

		port(h).SetAvailability("0xfront", Offline)
		goesThrough(h)
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, h, true) // an offline door closing empties nothing

		seen(h, "0xhall", true)
		doorOpen(h, "0xback", true)
		doorOpen(h, "0xback", false)
		port(h).SetAvailability("0xhall", Offline) // true, but unheard when the grace runs out
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, h, false)
	})
}

func TestHomeBindingsAreChecked(t *testing.T) {
	h, updates, dir, front, back, hall := boxed(t)
	f, err := h.CreateFlag("Guests")
	if err != nil {
		t.Fatal(err)
	}
	guests := TargetFlag(f)
	drain(updates)
	therm := TargetDevice(idOf(t, h, "0xtherm"), "temperature")
	for name, c := range map[string]struct {
		b    HomeBindings
		want error
	}{
		"a negative grace":           {HomeBindings{EntranceDoors: []Target{front}, ExitGrace: -1}, ErrInvalid},
		"signs of life with no door": {HomeBindings{SignsOfLife: []Target{hall}}, ErrInvalid},
		"the Home presence":          {HomeBindings{EntranceDoors: []Target{front}, SignsOfLife: []Target{TargetPresence(HomePresence)}}, ErrInvalid},
		"a Presence":                 {HomeBindings{EntranceDoors: []Target{TargetPresence(alice)}}, ErrInvalid},
		"a Device itself":            {HomeBindings{EntranceDoors: []Target{TargetDevice(front.Device(), "")}}, ErrInvalid},
		"a reading":                  {HomeBindings{EntranceDoors: []Target{front}, SignsOfLife: []Target{therm}}, ErrInvalid},
		"unknown":                    {HomeBindings{EntranceDoors: []Target{TargetFlag("nope")}}, ErrNotFound},
	} {
		if err := h.SetHomeBindings(c.b); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if us := drain(updates); len(us) != 0 {
		t.Fatalf("refused bindings announced %+v", us)
	}

	want := HomeBindings{EntranceDoors: []Target{front, back}, SignsOfLife: []Target{hall, guests}, ExitGrace: 60}
	bindHome(t, h, HomeBindings{EntranceDoors: []Target{front, back, front}, SignsOfLife: []Target{hall, guests, hall}, ExitGrace: 60})
	is := func(when string, got *HomeBindings, want HomeBindings) {
		t.Helper()
		if got == nil || !slices.Equal(got.EntranceDoors, want.EntranceDoors) || !slices.Equal(got.SignsOfLife, want.SignsOfLife) || got.ExitGrace != want.ExitGrace {
			t.Errorf("%s: bindings %+v, want %+v", when, got, want)
		}
	}
	us := drain(updates)
	if len(us) == 0 || us[0].Kind != PresencesChanged {
		t.Fatalf("binding announced %+v", us)
	}
	is("announced", us[0].Presences[0].HomeBindings, want)
	restarted := restart(t, dir)
	is("after a restart", snapshot(restarted).Presences[0].HomeBindings, want)

	if err := h.DeleteFlag(f); err != nil {
		t.Fatal(err)
	}
	want.SignsOfLife = []Target{hall}
	is("once the Flag is deleted", snapshot(h).Presences[0].HomeBindings, want)
	if i := slices.IndexFunc(drain(updates), func(u Update) bool { return u.Kind == PresencesChanged }); i < 0 {
		t.Error("unbinding the deleted Flag announced no presences")
	}
}

func TestTheHomePresenceSurvivesRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, dir, _, _, _ := boxed(t)
		seen(h, "0xhall", true)
		seen(h, "0xhall", false)
		before, _ := value(h, homePresenceRef)

		restarted := restart(t, dir)
		var heard []Update
		restarted.Follow(func(u Update) { heard = append(heard, u) })
		port(restarted).SetOnline(true)
		port(restarted).SyncDevices([]bridge.Device{entrance("0xfront"), entrance("0xback"), motion("0xhall")})
		restarted.SetPersons([]Person{{alice, "Alice"}, {bob, "Bob"}})
		doorOpen(restarted, "0xfront", false) // first Values, replayed
		seen(restarted, "0xhall", false)
		if v, ok := value(restarted, homePresenceRef); !ok || v.Data != true || !v.Since.Equal(before.Since) {
			t.Errorf("recalled %+v, want %+v", v, before)
		}
		for _, u := range heard {
			if u.Kind == ValueChanged && !u.Initial {
				t.Errorf("replayed state announced as a change: %+v", u)
			}
		}

		doorOpen(restarted, "0xfront", false) // a refresh: no closing
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, restarted, true)

		goesThrough(restarted) // the box was recalled: the next door closing empties it
		time.Sleep(time.Hour)
		synctest.Wait()
		homeIs(t, restarted, false)
	})
}

// With no Entrance door, a recalled home is no box: it follows the Presences.
func TestWithNoEntranceDoorTheHomePresenceFollowsThePresences(t *testing.T) {
	h, _, dir := presences(t)
	h.Command(TargetPresence(alice), Request{Values: map[string]any{"home": true}})
	homeIs(t, h, true)

	restarted := restart(t, dir)
	restarted.SetPersons([]Person{{alice, "Alice"}, {bob, "Bob"}})
	homeIs(t, restarted, true)
	restarted.Command(TargetPresence(alice), Request{Values: map[string]any{"home": false}})
	homeIs(t, restarted, false)
}
