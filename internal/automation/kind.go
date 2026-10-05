package automation

import (
	"encoding/json"
	"time"

	"github.com/llehouerou/oiko/internal/home"
)

// Step kinds, as Documents name them. Each is one type in its own file, which
// says what its params are. Durations and offsets are in seconds, times of
// day "15:04" or "15:04:05" in the host timezone, and weekdays "mon" to "sun"
// (none means every day).
const (
	eventTrigger        = "eventTrigger"
	valueTrigger        = "valueTrigger"
	availabilityTrigger = "availabilityTrigger"
	timeTrigger         = "timeTrigger"
	sunTrigger          = "sunTrigger"
	manualTrigger       = "manualTrigger"
	valueCondition      = "valueCondition"
	timeWindow          = "timeWindow"
	timer               = "timer"
	cooldown            = "cooldown"
	presenceSimulation  = "presenceSimulation"
	command             = "command"
	notify              = "notify"
	code                = "code"
)

// kindSpec is a Step kind: its handles, by name, and how its params compile.
// A kind without inputs is a trigger: it starts Runs. A Code Step declares
// its outputs in its params.
type kindSpec struct {
	inputs, outputs []string
	parse           func(s *step, raw json.RawMessage, place *Place) error
}

var kinds = map[string]kindSpec{
	eventTrigger:        {outputs: []string{"out"}, parse: with(parseEventTrigger)},
	valueTrigger:        {outputs: []string{"out"}, parse: with(parseValueTrigger)},
	availabilityTrigger: {outputs: []string{"out"}, parse: with(parseAvailabilityTrigger)},
	timeTrigger:         {outputs: []string{"out"}, parse: with(parseDailyTrigger(timeTrigger))},
	sunTrigger:          {outputs: []string{"out"}, parse: with(parseDailyTrigger(sunTrigger))},
	manualTrigger:       {outputs: []string{"out"}, parse: with(parseManualTrigger)},
	valueCondition:      {inputs: []string{"in"}, outputs: []string{"true", "false"}, parse: with(parseValueCondition)},
	timeWindow:          {inputs: []string{"in"}, outputs: []string{"true", "false"}, parse: with(parseTimeWindow)},
	timer:               {inputs: []string{"start", "cancel"}, outputs: []string{"fired"}, parse: with(parseTimer)},
	cooldown:            {inputs: []string{"in"}, outputs: []string{"out"}, parse: with(parseCooldown)},
	presenceSimulation:  {inputs: []string{"enable", "disable"}, outputs: []string{"on", "off"}, parse: with(parsePresenceSimulation)},
	command:             {inputs: []string{"in"}, outputs: []string{"then"}, parse: with(parseCommand)},
	notify:              {inputs: []string{"in"}, outputs: []string{"then"}, parse: with(parseNotify)},
	code:                {inputs: []string{"run"}, parse: with(parseCode)},
}

// with decodes the params P that parse compiles into s: its kind, and the
// targets it refers to.
func with[P any](parse func(s *step, p P, place *Place) error) func(*step, json.RawMessage, *Place) error {
	return func(s *step, raw json.RawMessage, place *Place) error {
		var p P
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		return parse(s, p, place)
	}
}

// stepKind is a compiled Step kind: its params and, while its Automation is
// live, its state. What it does is what it implements of reacher, scheduled
// and remembering. The engine indexes event and value triggers itself.
type stepKind any

// reacher is a kind with inputs.
type reacher interface {
	// reached acts on input in, recording its evidence in the Trace and
	// firing its outputs through x.
	reached(x visit, in int)
}

// scheduled is a kind with a deadline in the timer heap: it embeds a due.
type scheduled interface {
	deadline() *due
	// arm follows its Automation going live.
	arm(e *Engine)
	// expired says how the Run its deadline stands for starts, if it does,
	// and the output it fires from.
	expired(e *Engine) (tg home.Trigger, out int, ok bool)
}

// remembering is a kind that keeps state between Runs and across restarts.
// Its deadline, if it is scheduled, is saved and cancelled for it.
type remembering interface {
	save(s *StepState)
	// restore takes over s, saved by a Step of the same id and kind whatever
	// its params. Its Automation is live.
	restore(e *Engine, s StepState)
	// forget clears its state: its Automation is no longer live.
	forget()
}

// visit is Step i of a, reached at k in the Trace of the Run under way. It
// is passed by value, so crossing the seam allocates nothing.
type visit struct {
	e    *Engine
	a    *automation
	i, k int
}

// record is the Step's entry in the Trace, stale once fire reaches more
// Steps.
func (x visit) record() *Reached { return &x.e.trace.Steps[x.k] }

func (x visit) now() time.Time { return x.e.trace.Time }

func (x visit) fire(out int) { x.e.fire(x.a, x.i, x.k, out) }

func (x visit) issue(t home.Target, req home.Request) { x.e.issue(x.record(), x.a, x.i, t, req) }
