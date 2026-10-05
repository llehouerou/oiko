package automation

import (
	"errors"
	"fmt"
	"time"

	"github.com/nathan-osman/go-sunrise"

	"github.com/llehouerou/oiko/internal/home"
)

// dailyTriggerStep is a time trigger or a sun trigger: it starts a Run every
// day at a time of day, or at a sun event plus an offset, on some weekdays.
// With catchUp, a restart fires the latest occurrence missed while Oiko was
// down. It follows its params, not its saved deadline. Params: {at,
// weekdays, catchUp} for a time trigger, {event, offset, weekdays, catchUp}
// for a sun trigger. Handles: → out.
type dailyTriggerStep struct {
	when    daily
	catchUp bool
	last    time.Time // with catchUp: the latest occurrence fired, or when it started from
	due
}

type dailyParams struct {
	At       string   `json:"at"`
	Event    string   `json:"event"`
	Offset   float64  `json:"offset"`
	Weekdays []string `json:"weekdays"`
	CatchUp  bool     `json:"catchUp"`
}

// parseDailyTrigger parses a trigger of kind, a time or sun one. Sun triggers
// need place.
func parseDailyTrigger(kind string) func(*step, dailyParams, *Place) error {
	return func(s *step, p dailyParams, place *Place) error {
		k := &dailyTriggerStep{catchUp: p.CatchUp}
		if err := k.when.parse(kind, p); err != nil {
			return err
		}
		if kind == sunTrigger && place == nil {
			return errors.New("no home location in config.json")
		}
		s.kind = k
		return nil
	}
}

// arm schedules its first occurrence from now, a catch-up one starting from
// now.
func (s *dailyTriggerStep) arm(e *Engine) {
	now := e.now()
	if s.catchUp {
		s.last = now
	}
	s.scheduleNext(e, now)
}

func (s *dailyTriggerStep) expired(e *Engine) (home.Trigger, int, bool) {
	at := s.at
	if s.catchUp {
		s.last = at
	}
	s.scheduleNext(e, e.now()) // not from at: occurrences missed while running are not caught up
	return home.Trigger{Time: at}, 0, true
}

// scheduleNext sets it to come due at its first occurrence after t.
func (s *dailyTriggerStep) scheduleNext(e *Engine, t time.Time) {
	if at := s.when.next(t, e.place); !at.IsZero() {
		e.schedule(&s.due, at)
	}
}

func (s *dailyTriggerStep) save(st *StepState) { st.Last = s.last }

// restore schedules, with catchUp, the latest occurrence missed since the
// last one fired, if it is already past.
func (s *dailyTriggerStep) restore(e *Engine, st StepState) {
	if !s.catchUp || st.Last.IsZero() {
		return
	}
	s.last = st.Last
	now := e.now()
	missed := s.when.next(st.Last.In(now.Location()), e.place) // its days are the host's
	if missed.IsZero() || missed.After(now) {
		return
	}
	for n := s.when.next(missed, e.place); !n.IsZero() && !n.After(now); n = s.when.next(n, e.place) {
		missed = n
	}
	e.schedule(&s.due, missed)
}

func (s *dailyTriggerStep) forget() { s.last = time.Time{} }

// daily is when a time or sun trigger fires: at a time of day, or at a sun
// event plus offset, on some weekdays.
type daily struct {
	at     int    // a time trigger's
	sun    string // dawn, sunrise, sunset or dusk
	offset time.Duration
	days   uint8 // bit d set for time.Weekday d; none means every day
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func (d *daily) parse(kind string, p dailyParams) error {
	for _, name := range p.Weekdays {
		w, ok := weekdays[name]
		if !ok {
			return fmt.Errorf("unknown weekday %q", name)
		}
		d.days |= 1 << w
	}
	if kind == timeTrigger {
		var err error
		d.at, err = parseClock(p.At)
		return err
	}
	switch p.Event {
	case "dawn", "sunrise", "sunset", "dusk":
	default:
		return fmt.Errorf("unknown sun event %q", p.Event)
	}
	d.sun, d.offset = p.Event, seconds(p.Offset)
	return nil
}

// next returns d's first occurrence after t, in t's location, or the zero
// time if there is none within a year (the sun may not set near the poles).
// A sun event's weekday is that of the day it happens, before the offset.
func (d *daily) next(t time.Time, place *Place) time.Time {
	y, m, day := t.Date()
	for i := -1; i <= 366; i++ { // from the day before: an offset may carry an event past midnight
		date := time.Date(y, m, day+i, 0, 0, 0, 0, t.Location())
		if d.days != 0 && d.days&(1<<date.Weekday()) == 0 {
			continue
		}
		at := time.Date(y, m, day+i, 0, 0, d.at, 0, t.Location())
		if d.sun != "" {
			at = d.sunTime(date, place)
			if at.IsZero() {
				continue
			}
			at = at.Add(d.offset).In(t.Location())
		}
		if at.After(t) {
			return at
		}
	}
	return time.Time{}
}

// sunTime returns when d's sun event happens on date, or the zero time. Dawn
// and dusk are civil: the sun 6° below the horizon.
func (d *daily) sunTime(date time.Time, p *Place) time.Time {
	var morning, evening time.Time
	if d.sun == "sunrise" || d.sun == "sunset" {
		morning, evening = sunrise.SunriseSunset(p.Latitude, p.Longitude, date.Year(), date.Month(), date.Day())
	} else {
		morning, evening = sunrise.TimeOfElevation(p.Latitude, p.Longitude, -6, date.Year(), date.Month(), date.Day())
	}
	if d.sun == "dawn" || d.sun == "sunrise" {
		return morning
	}
	return evening
}
