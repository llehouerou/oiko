package automation

import (
	"encoding/binary"
	"math/rand/v2"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/internal/home"
)

// Trace is the record of one Run: its trigger, then each Step it reached, in
// execution order, with the evidence for the handles it fired. There is no
// copy of the mirror. Traces are recycled across Runs: see Release.
type Trace struct {
	Run        uuid.UUID       `json:"run"`
	Automation string          `json:"automation"`
	Time       time.Time       `json:"time"` // when the Run started
	Outcome    home.RunOutcome `json:"outcome"`
	Trigger    home.Trigger    `json:"trigger"`
	Steps      []Reached       `json:"steps"`
}

// Reached is a Step a Run went through, with the evidence for the handles it
// fired.
type Reached struct {
	Step     string    `json:"step"`
	Fired    []string  `json:"fired"`             // a time window's verdict too
	Read     any       `json:"read,omitempty"`    // the Value a condition read
	At       time.Time `json:"at,omitzero"`       // when that Value was reported
	Unknown  bool      `json:"unknown,omitempty"` // a condition read no Value
	Action   string    `json:"action,omitempty"`  // a timer's start, restart, keep or cancel; a cooldown's pass or block; a presence simulation's enable or disable
	Until    time.Time `json:"until,omitzero"`    // a timer's deadline, a cooldown's next opening, a presence simulation's next switch
	Commands []Issued  `json:"commands,omitempty"`
	Error    string    `json:"error,omitempty"` // why a Code Step failed (it issued nothing and fired nothing), or a Notification could not be sent
	Print    string    `json:"print,omitempty"` // what a Code Step printed, capped

	Notification *Notification `json:"notification,omitempty"` // what a notify Step sent
}

// Issued is a Command a Step issued, by id with what it asked for, or why it
// was refused.
type Issued struct {
	Target  home.Target        `json:"target"`
	ID      string             `json:"id,omitempty"`
	Values  map[string]any     `json:"values,omitempty"` // never modified
	Refused string             `json:"refused,omitempty"`
	Status  home.CommandStatus `json:"status,omitempty"` // its final status, filled in when the Trace is read back from the history
}

// newRunID returns a UUIDv7 (RFC 9562) for a Run started at now. Unlike
// uuid.NewV7, whose crypto/rand buffer escapes under the race detector, it
// allocates nothing; Run ids need no cryptographic randomness.
func newRunID(now time.Time) uuid.UUID {
	var u uuid.UUID
	binary.BigEndian.PutUint64(u[:8], uint64(now.UnixMilli())<<16|0x7000|rand.Uint64()&0x0fff)
	binary.BigEndian.PutUint64(u[8:], 1<<63|rand.Uint64()>>2)
	return u
}

// spare recycles Traces, so that building one allocates nothing. Unlike a
// sync.Pool, it never drops them (a pool does under the race detector).
var spare = make(chan *Trace, 256)

func newTrace() *Trace {
	select {
	case t := <-spare:
		t.Steps = t.Steps[:0]
		return t
	default:
		return new(Trace)
	}
}

// Release hands t back for a later Run; t must not be used afterwards.
func (t *Trace) Release() {
	select {
	case spare <- t:
	default: // enough spares already
	}
}

// reach records that the Run reached Step id, reusing an earlier Run's
// storage, and returns its index in Steps.
func (t *Trace) reach(id string) int {
	k := len(t.Steps)
	if k == cap(t.Steps) {
		t.Steps = append(t.Steps, Reached{Step: id})
		return k
	}
	t.Steps = t.Steps[:k+1]
	r := &t.Steps[k]
	*r = Reached{Step: id, Fired: r.Fired[:0], Commands: r.Commands[:0]}
	return k
}

// End is how the Run ends: an error if a Command was refused or a Step
// failed, otherwise acted if a Command was issued or a Notification sent.
func (t *Trace) End() home.RunEnd {
	end := home.RunEnd{Automation: t.Automation, Run: t.Run, Time: t.Time, Outcome: home.RunNothing, Trigger: t.Trigger}
	failed, notified := false, false
	for _, r := range t.Steps {
		failed = failed || r.Error != ""
		notified = notified || r.Notification != nil
		for _, c := range r.Commands {
			if c.Refused != "" {
				failed = true
			} else {
				end.Commands++
			}
		}
	}
	switch {
	case failed:
		end.Outcome = home.RunError
	case end.Commands > 0 || notified:
		end.Outcome = home.RunActed
	}
	return end
}
