package home

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// HomePresence is the Home presence's id, as in its key presence:home (ADR
// 0051): Person ids are UUIDs, so it cannot be one.
const HomePresence PersonID = "home"

// DefaultExitGrace is the exit grace, in seconds, when none is set.
const DefaultExitGrace = 300

// HomeBindings are what an Admin binds to the Home presence (ADR 0051): its
// Entrance doors, true meaning open, its Signs of life, true meaning someone
// is there, each read as a Presence source is, and its exit grace.
type HomeBindings struct {
	EntranceDoors []Target `json:"entranceDoors"`
	SignsOfLife   []Target `json:"signsOfLife"`
	ExitGrace     int      `json:"exitGrace"` // in seconds
}

// box is the Home presence's "Wasp in a Box" (ADR 0051): whether someone has
// been seen inside since an Entrance door last closed.
type box struct {
	HomeBindings                 // never modified: replaced whole
	inside       bool            // never true while no Entrance door is bound: nothing would empty it
	told         bool            // a Sign of life, or the Value recalled, told whether someone is inside
	open         map[Target]bool // each Entrance door's last Value heard, while it is heard
	closed       time.Time       // when an Entrance door last closed
	timer        *time.Timer     // until the exit grace since closed runs out; nil while none runs
}

// homePresenceFunction is what the Home presence does: home is a state, set
// by no Command. Callers never modify it.
var homePresenceFunction = Function{Key: "presence", Kind: "presence", Capabilities: []Capability{
	{Key: "home", Label: "Home", Type: Binary, Access: Access{Observable: true}, Category: Primary},
}}

var homePresenceRef = TargetPresence(HomePresence).Ref("home")

// recallHome takes what presences.json kept of the Home presence: its
// bindings, and its last Value, as replayed state. The box is recalled as
// that Value. Callers hold h.mu or own h.
func (h *Home) recallHome(s SavedPresence) {
	if s.HomeBindings != nil {
		h.box.HomeBindings = *s.HomeBindings
	}
	if s.Value != nil {
		h.values[homePresenceRef] = *s.Value
		h.box.told = true
		h.box.inside = s.Value.Data == true && len(h.box.EntranceDoors) > 0
	}
}

// SetHomeBindings binds Entrance doors, Signs of life and an exit grace to
// the Home presence, in place of those it had (ADR 0054). Each is checked as
// a Presence source is, and kept once if listed twice; Signs of life need an
// Entrance door, and the grace cannot be negative.
func (h *Home) SetHomeBindings(b HomeBindings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case b.ExitGrace < 0:
		return fmt.Errorf("%w: an exit grace cannot be negative", ErrInvalid)
	case len(b.SignsOfLife) > 0 && len(b.EntranceDoors) == 0:
		return fmt.Errorf("%w: signs of life need an entrance door", ErrInvalid)
	}
	for _, t := range slices.Concat(b.EntranceDoors, b.SignsOfLife) {
		if err := h.checkBinding(t); err != nil {
			return err
		}
	}
	h.bindBox(HomeBindings{EntranceDoors: unique(b.EntranceDoors), SignsOfLife: unique(b.SignsOfLife), ExitGrace: b.ExitGrace})
	err := h.presencesChanged()
	h.followAll(false)
	return err
}

// unique is ts, each Target once, as first listed; never nil.
func unique(ts []Target) []Target {
	kept := []Target{}
	for _, t := range ts {
		if !slices.Contains(kept, t) {
			kept = append(kept, t)
		}
	}
	return kept
}

// bindBox gives the box b: with no Entrance door left, it empties, since
// nothing would. A grace running counts the new one. Callers hold h.mu.
func (h *Home) bindBox(b HomeBindings) {
	h.box.HomeBindings = b
	maps.DeleteFunc(h.box.open, func(t Target, _ bool) bool { return !slices.Contains(b.EntranceDoors, t) })
	if len(b.EntranceDoors) == 0 {
		h.box.inside = false
		h.stopGrace()
	} else if h.box.timer != nil {
		h.startGrace()
	}
}

// dropFromBox unbinds the Entrance doors and Signs of life drop selects,
// reporting whether any was. Callers hold h.mu.
func (h *Home) dropFromBox(drop func(Target) bool) bool {
	b := h.box.HomeBindings
	doors := slices.DeleteFunc(slices.Clone(b.EntranceDoors), drop)
	signs := slices.DeleteFunc(slices.Clone(b.SignsOfLife), drop)
	if len(doors) == len(b.EntranceDoors) && len(signs) == len(b.SignsOfLife) {
		return false
	}
	h.bindBox(HomeBindings{EntranceDoors: doors, SignsOfLife: signs, ExitGrace: b.ExitGrace})
	return true
}

// reportToBox follows the Entrance doors and Signs of life whose Values were
// just recorded. A door heard open, then heard closed, starts the exit grace:
// not its first Value, a refresh, or one it took while unheard. A Sign of
// life true puts someone inside, and ends a grace if it turned true since
// the door closed. A change moved only by first Values is replayed state
// (ADR 0054). Callers hold h.mu.
func (h *Home) reportToBox(rs []recorded) {
	b := &h.box
	for _, t := range b.EntranceDoors {
		if fed, _ := h.fed(t, rs); !fed {
			continue
		}
		v, heard := h.sourceValue(t)
		if !heard {
			delete(b.open, t)
			continue
		}
		if b.open[t] && v.Data == false {
			b.closed = v.Since
			h.startGrace()
		}
		if b.open == nil {
			b.open = map[Target]bool{}
		}
		b.open[t] = v.Data == true
	}
	told, replayed := false, true
	for _, t := range b.SignsOfLife {
		fed, initial := h.fed(t, rs)
		if v, heard := h.sourceValue(t); fed && heard && v.Data == true {
			told, replayed = true, replayed && initial
			if v.Since.After(b.closed) {
				h.stopGrace()
			}
		}
	}
	if told && len(b.EntranceDoors) > 0 {
		b.inside, b.told = true, true
		h.followHome(replayed)
	}
}

// startGrace has the box emptied once the exit grace since the last door
// closed runs out, unless a Sign of life is true then. Callers hold h.mu.
func (h *Home) startGrace() {
	h.stopGrace()
	var t *time.Timer
	t = time.AfterFunc(time.Until(h.box.closed.Add(time.Duration(h.box.ExitGrace)*time.Second)), func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.box.timer != t { // stopped or started again since
			return
		}
		h.box.timer = nil
		for _, s := range h.box.SignsOfLife {
			if v, heard := h.sourceValue(s); heard && v.Data == true {
				return
			}
		}
		h.box.inside = false
		h.followHome(false)
	})
	h.box.timer = t // before t runs: it waits for h.mu
}

// stopGrace cancels the exit grace running, if any. Callers hold h.mu.
func (h *Home) stopGrace() {
	if h.box.timer != nil {
		h.box.timer.Stop()
		h.box.timer = nil
	}
}

// followHome sets the Home presence to what it is now: home while a Person's
// Presence is home or someone is inside, away otherwise; unchanged until a
// Presence or a Sign of life first tells. initial marks a change as replayed
// state. Callers hold h.mu.
func (h *Home) followHome(initial bool) {
	home, ok := h.box.inside, h.box.told
	for id := range h.presences {
		if v, had := h.values[homeRef(id)]; had {
			home, ok = home || v.Data == true, true
		}
	}
	if cur, had := h.values[homePresenceRef]; !ok || had && cur.Data == home {
		return
	}
	h.recordAs(homePresenceRef, home, time.Now(), initial)
	h.savePresences()
}

// homePresence is the Home presence as the presences list shows it.
// Callers hold h.mu.
func (h *Home) homePresence() Presence {
	b := h.box.HomeBindings
	return Presence{ID: HomePresence, Name: "Home", Kind: homePresenceFunction.Kind, Capabilities: homePresenceFunction.Capabilities,
		HomeBindings: &HomeBindings{EntranceDoors: append([]Target{}, b.EntranceDoors...), SignsOfLife: append([]Target{}, b.SignsOfLife...), ExitGrace: b.ExitGrace}}
}

// homePresenceTarget is the Home presence: a Command on it is refused, as
// home is not settable.
type homePresenceTarget struct{ h *Home }

func (homePresenceTarget) target() Target                         { return TargetPresence(HomePresence) }
func (homePresenceTarget) kind() string                           { return homePresenceFunction.Kind }
func (homePresenceTarget) capabilities() []Capability             { return homePresenceFunction.Capabilities }
func (homePresenceTarget) attached() bool                         { return true }
func (homePresenceTarget) availability() Availability             { return Online }
func (homePresenceTarget) timeout() time.Duration                 { return 0 }
func (homePresenceTarget) transmit(Request)                       {} // never reached: accept refuses every Command
func (x homePresenceTarget) carry(req Request, key string) string { return x.h.issue(x, req, key) }
func (homePresenceTarget) accept(values map[string]any) error {
	return checkValues(homePresenceFunction.Capabilities, values)
}
