package automation

import (
	"encoding/json"
	"testing"
)

// The engine asserts that a Step it reaches through an input is a reacher:
// every kind with inputs must be one, and no trigger.
func TestEveryKindWithInputsIsReachable(t *testing.T) {
	valid := map[string]string{
		eventTrigger:        `{"target": "flag:a", "capability": "on", "events": ["single"]}`,
		valueTrigger:        `{"target": "flag:a", "capability": "on", "op": "eq", "value": true}`,
		timeTrigger:         `{"at": "06:00"}`,
		sunTrigger:          `{"event": "sunset"}`,
		manualTrigger:       `{}`,
		valueCondition:      `{"target": "flag:a", "capability": "on", "op": "eq", "value": true}`,
		timeWindow:          `{"from": "21:00", "to": "06:00"}`,
		timer:               `{"duration": 60, "reentry": "keep"}`,
		cooldown:            `{"duration": 60}`,
		presenceSimulation:  `{"from": "19:00", "to": "23:00", "minBlocks": 1, "maxBlocks": 2, "minDuration": 600, "maxDuration": 1200}`,
		command:             `{"targets": ["flag:a"], "values": {"on": true}}`,
		code:                `{"source": "def run(trigger, state):\n    return None\n"}`,
		availabilityTrigger: `{"target": "device:a", "op": "ne", "availability": "online", "heldFor": 900}`,
		notify:              `{"title": "Camera down", "message": "{name}"}`,
	}
	for name, k := range kinds {
		p, ok := valid[name]
		if !ok {
			t.Errorf("%s: no params to try it with", name)
			continue
		}
		var s step
		if err := k.parse(&s, json.RawMessage(p), &Place{}); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if _, ok := s.kind.(reacher); ok != (len(k.inputs) > 0) {
			t.Errorf("%s: %d inputs, but reacher is %v", name, len(k.inputs), ok)
		}
	}
}
