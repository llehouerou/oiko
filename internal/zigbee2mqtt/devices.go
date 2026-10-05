package zigbee2mqtt

import (
	"fmt"
	"maps"
	"strings"

	"github.com/llehouerou/oiko/bridge"
)

// device is an entry of the retained zigbee2mqtt/bridge/devices message.
type device struct {
	IEEEAddress  string `json:"ieee_address"`
	Type         string `json:"type"`
	FriendlyName string `json:"friendly_name"`
	Definition   *struct {
		Model   string   `json:"model"`
		Vendor  string   `json:"vendor"`
		Exposes []expose `json:"exposes"`
	} `json:"definition"`
}

// expose describes a capability of a zigbee2mqtt device.
// See https://www.zigbee2mqtt.io/guide/usage/exposes.html
type expose struct {
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Property  string   `json:"property"`
	Endpoint  string   `json:"endpoint"`
	Access    int      `json:"access"`
	Category  string   `json:"category"`
	Unit      string   `json:"unit"`
	ValueMin  *float64 `json:"value_min"`
	ValueMax  *float64 `json:"value_max"`
	ValueStep *float64 `json:"value_step"`
	ValueOn   any      `json:"value_on"`
	ValueOff  any      `json:"value_off"`
	Values    []any    `json:"values"`
	Features  []expose `json:"features"`
}

var genericTypes = map[string]bool{"binary": true, "numeric": true, "enum": true, "text": true, "composite": true, "list": true}

// identifyEffects are the effect values zigbee2mqtt plays as a one-off
// (genIdentify triggerEffect, or stopping one): the device keeps reporting its
// ongoing effect, if any.
var identifyEffects = map[string]bool{"blink": true, "breathe": true, "okay": true, "channel_change": true, "finish_effect": true, "stop_effect": true, "stop_hue_effect": true}

// counters are the numeric exposes that are running totals, in kWh.
var counters = map[string]bool{"energy": true, "produced_energy": true}

// binding links a Capability to the zigbee2mqtt property carrying it.
type binding struct {
	function string
	exp      expose
}

// deviceBinding translates between a zigbee2mqtt device's payloads and Oiko Capabilities.
type deviceBinding struct {
	address    string
	topic      string // friendly_name
	byProperty map[string][]binding
	byKey      map[[2]string]binding // {function, capability}
}

func (b *deviceBinding) bind(function string, e expose) {
	bd := binding{function: function, exp: e}
	b.byProperty[e.Property] = append(b.byProperty[e.Property], bd)
	b.byKey[[2]string{function, e.Name}] = bd
}

func functionKey(kind, endpoint string) string {
	if endpoint == "" {
		return kind
	}
	return kind + "/" + endpoint
}

// derive builds the Oiko view of a zigbee2mqtt device:
//   - each specific expose (light, switch, cover…) is a Function, one per endpoint;
//   - "action" is a button Function emitting Events;
//   - a top-level generic expose goes to the specific Function of its endpoint,
//     else to the Device if it is configuration or diagnostic, else to the only
//     specific Function if there is exactly one, else to a Function of its own
//     whose kind is the measured quantity (occupancy, temperature…).
func derive(d device) (bridge.Device, *deviceBinding) {
	dev := bridge.Device{NativeAddress: d.IEEEAddress, Name: d.FriendlyName}
	b := &deviceBinding{address: d.IEEEAddress, topic: d.FriendlyName, byProperty: map[string][]binding{}, byKey: map[[2]string]binding{}}
	if d.Definition == nil {
		return dev, b
	}
	dev.Model, dev.Vendor = d.Definition.Model, d.Definition.Vendor

	specificByEndpoint := map[string]string{}
	for _, e := range d.Definition.Exposes {
		if genericTypes[e.Type] {
			continue
		}
		fn := bridge.Function{Key: functionKey(e.Type, e.Endpoint), Kind: e.Type}
		var colorTemp string // its property
		color := false
		for _, f := range e.Features {
			fn.Capabilities = append(fn.Capabilities, capability(f))
			b.bind(fn.Key, f)
			switch f.Name {
			case "color_temp":
				colorTemp = f.Property
			case "color_xy", "color_hs":
				color = true
			}
		}
		// A light with both a colour and a white temperature shows one at a time;
		// zigbee2mqtt reports which in a color_mode it does not expose.
		if colorTemp != "" && color {
			mode := expose{Type: "enum", Name: "color_mode", Label: "Color mode", Access: 1, Category: "diagnostic",
				Property: strings.Replace(colorTemp, "color_temp", "color_mode", 1), Values: []any{"hs", "xy", "color_temp"}}
			fn.Capabilities = append(fn.Capabilities, capability(mode))
			b.bind(fn.Key, mode)
		}
		dev.Functions = append(dev.Functions, fn)
		specificByEndpoint[e.Endpoint] = fn.Key
	}
	specifics := len(dev.Functions)

	for _, e := range d.Definition.Exposes {
		if !genericTypes[e.Type] {
			continue
		}
		key, kind := "", ""
		if e.Name == "action" {
			key, kind = functionKey("button", e.Endpoint), "button"
		} else if k, ok := specificByEndpoint[e.Endpoint]; ok && e.Endpoint != "" {
			key = k
		} else if e.Category == "config" || e.Category == "diagnostic" {
			key = ""
		} else if specifics == 1 && e.Endpoint == "" {
			key = dev.Functions[0].Key
		} else {
			key, kind = functionKey(e.Name, e.Endpoint), e.Name
		}
		b.bind(key, e)
		if key == "" {
			dev.Capabilities = append(dev.Capabilities, capability(e))
			continue
		}
		fn := function(&dev, key, kind)
		fn.Capabilities = append(fn.Capabilities, capability(e))
	}
	return dev, b
}

func function(d *bridge.Device, key, kind string) *bridge.Function {
	for i := range d.Functions {
		if d.Functions[i].Key == key {
			return &d.Functions[i]
		}
	}
	d.Functions = append(d.Functions, bridge.Function{Key: key, Kind: kind})
	return &d.Functions[len(d.Functions)-1]
}

// capability passes zigbee2mqtt's category through as is: correcting it feature
// by feature is a list that never ends, and dashboards pick their own values.
func capability(e expose) bridge.Capability {
	category := bridge.Primary
	if e.Name != "action" && (e.Category == "config" || e.Category == "diagnostic") { // zigbee2mqtt 2.x flags action diagnostic, but pressing is what a button is for
		category = bridge.Category(e.Category)
	}
	c := bridge.Capability{
		Key:       e.Name,
		Label:     e.Label,
		Type:      bridge.ValueType(e.Type),
		Unit:      e.Unit,
		Min:       e.ValueMin,
		Max:       e.ValueMax,
		Step:      e.ValueStep,
		Access:    bridge.Access{Observable: e.Access&1 != 0, Settable: e.Access&2 != 0, Queryable: e.Access&4 != 0},
		Category:  category,
		Stateless: e.Name == "action",
		Counter:   counters[e.Name] && e.Type == "numeric",
	}
	for _, v := range e.Values {
		o := fmt.Sprint(v)
		c.Options = append(c.Options, o)
		if e.Name == "effect" && identifyEffects[o] {
			c.Triggers = append(c.Triggers, o)
		}
	}
	for _, f := range e.Features {
		c.Fields = append(c.Fields, capability(f))
	}
	return c
}

// readings converts a device state payload into Oiko Readings. A replayed
// (retained) payload carries no new Events: withEvents is false for it.
func (b *deviceBinding) readings(payload map[string]any, withEvents bool) []bridge.Reading {
	var rs []bridge.Reading
	for property, raw := range payload {
		for _, bd := range b.byProperty[property] {
			if bd.exp.Name == "action" && !withEvents {
				continue
			}
			if data, ok := bd.fromZ2M(raw); ok {
				rs = append(rs, bridge.Reading{Function: bd.function, Capability: bd.exp.Name, Data: data})
			}
		}
	}
	return rs
}

// setPayload converts Command values into a zigbee2mqtt /set payload.
func (b *deviceBinding) setPayload(function string, values map[string]any) (map[string]any, error) {
	payload := map[string]any{}
	for k, v := range values {
		bd, ok := b.byKey[[2]string{function, k}]
		if !ok {
			return nil, fmt.Errorf("zigbee2mqtt: no property for %s/%s", function, k)
		}
		x := bd.toZ2M(v)
		if m, ok := x.(map[string]any); ok { // composites sharing a property (color_xy, color_hs)
			if prev, ok := payload[bd.exp.Property].(map[string]any); ok {
				maps.Copy(prev, m)
				continue
			}
		}
		payload[bd.exp.Property] = x
	}
	return payload, nil
}

func (bd binding) fromZ2M(v any) (any, bool) {
	switch bd.exp.Type {
	case "binary":
		switch {
		case v == bd.exp.ValueOn:
			return true, true
		case v == bd.exp.ValueOff:
			return false, true
		}
		return nil, false
	case "composite":
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		out := map[string]any{}
		for _, f := range bd.exp.Features {
			if x, ok := m[f.Property]; ok {
				out[f.Name] = x
			}
		}
		return out, len(out) == len(bd.exp.Features)
	}
	return v, v != nil && v != ""
}

func (bd binding) toZ2M(v any) any {
	switch bd.exp.Type {
	case "binary":
		if v == true {
			return bd.exp.ValueOn
		}
		return bd.exp.ValueOff
	case "composite":
		m, _ := v.(map[string]any)
		out := map[string]any{}
		for _, f := range bd.exp.Features {
			if x, ok := m[f.Name]; ok {
				out[f.Property] = x
			}
		}
		return out
	}
	return v
}
