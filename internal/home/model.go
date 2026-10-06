// Package home holds Oiko's domain model (see CONTEXT.md): the registry of
// Devices, their latest Values and Availability, Commands, and the ordered
// stream of Updates. It knows nothing about any Bridge protocol.
package home

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

type DeviceID string

// The terms a Bridge describes its Devices in are those of the Bridge
// contract: Home holds them as they come.
type (
	Availability = bridge.Availability
	ValueType    = bridge.ValueType
	Category     = bridge.Category
	Access       = bridge.Access
	Capability   = bridge.Capability
)

const (
	Online  = bridge.Online
	Offline = bridge.Offline
	Unknown = bridge.Unknown

	Binary    = bridge.Binary
	Numeric   = bridge.Numeric
	Enum      = bridge.Enum
	Text      = bridge.Text
	Composite = bridge.Composite
	List      = bridge.List

	Primary    = bridge.Primary
	Config     = bridge.Config
	Diagnostic = bridge.Diagnostic
)

// Function is what a Device does for the household. Key is kind + endpoint,
// e.g. "light" or "switch/l2", unique within its Device.
type Function struct {
	Key          string       `json:"key"`
	Kind         string       `json:"kind"`
	Area         AreaID       `json:"area,omitempty"` // "" for its Device's
	Capabilities []Capability `json:"capabilities"`
}

// Device is a physical device known through a Bridge. Home treats a *Device
// as immutable once stored: changes replace the whole struct.
type Device struct {
	ID            DeviceID     `json:"id"`
	Name          string       `json:"name"`
	Icon          string       `json:"icon,omitempty"` // "" for the dashboard's default
	Area          AreaID       `json:"area,omitempty"` // "" for none
	Bridge        string       `json:"bridge"`         // its name
	NativeAddress string       `json:"nativeAddress"`  // within its Bridge
	Model         string       `json:"model,omitempty"`
	Vendor        string       `json:"vendor,omitempty"`
	Detached      bool         `json:"detached,omitempty"`
	Functions     []Function   `json:"functions"`
	Capabilities  []Capability `json:"capabilities"` // device-level
}

func (d *Device) function(key string) *Function {
	for i := range d.Functions {
		if d.Functions[i].Key == key {
			return &d.Functions[i]
		}
	}
	return nil
}

// fromBridge is the Device a Bridge describes, still to be given its identity.
func fromBridge(b bridge.Device) Device {
	d := Device{NativeAddress: b.NativeAddress, Name: b.Name, Model: b.Model, Vendor: b.Vendor,
		Capabilities: b.Capabilities, Functions: make([]Function, len(b.Functions))}
	for i, f := range b.Functions {
		d.Functions[i] = Function{Key: f.Key, Kind: f.Kind, Capabilities: f.Capabilities}
	}
	return d
}

// inherit gives d, freshly described by its Bridge, the identity, Name, Icon
// and Areas of old: its Functions keep theirs by key.
func (d *Device) inherit(old *Device) {
	d.ID, d.Name, d.Icon, d.Area = old.ID, old.Name, old.Icon, old.Area
	d.Functions = slices.Clone(d.Functions) // the Bridge's own
	for i := range d.Functions {
		if f := old.function(d.Functions[i].Key); f != nil {
			d.Functions[i].Area = f.Area
		}
	}
}

// capability finds a Capability of Function fn, or of the Device itself when fn is "".
func (d *Device) capability(fn, key string) *Capability {
	caps := d.Capabilities
	if fn != "" {
		f := d.function(fn)
		if f == nil {
			return nil
		}
		caps = f.Capabilities
	}
	for i := range caps {
		if caps[i].Key == key {
			return &caps[i]
		}
	}
	return nil
}

// checkValues validates values requested for Capabilities caps.
func checkValues(caps []Capability, values map[string]any) error {
	for k, v := range values {
		i := slices.IndexFunc(caps, func(c Capability) bool { return c.Key == k })
		if i < 0 {
			return ErrNotFound
		}
		if err := check(&caps[i], v); err != nil {
			return err
		}
	}
	return nil
}

// Ref addresses a Capability of a Target: of a Function, or of a Device
// itself for device-level Capabilities.
type Ref struct {
	Target     Target `json:"target"`
	Capability string `json:"capability"`
}

// Value is the typed value of a Capability: bool (Binary), float64 (Numeric),
// string (Enum, Text), map[string]any (Composite) or []any (List). At is when
// it was last reported, Since when it took its Data: how long a state has held.
// An Event has no Since.
type Value struct {
	Data  any       `json:"data"`
	At    time.Time `json:"at"`
	Since time.Time `json:"since,omitzero"`
}

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalid       = errors.New("invalid request")
	ErrBridgeOffline = errors.New("bridge offline")
	ErrNotRunning    = errors.New("automation not running")
)

// check validates a value requested for c against its type and bounds.
func check(c *Capability, v any) error {
	if !c.Access.Settable {
		return fmt.Errorf("%w: %s is not settable", ErrInvalid, c.Key)
	}
	if v == toggle { // even where the string would be a valid value
		if c.Type != Binary {
			return fmt.Errorf("%w: only a binary Capability toggles", ErrInvalid)
		}
		return nil
	}
	ok := false
	switch c.Type {
	case Binary:
		_, ok = v.(bool)
	case Numeric:
		var f float64
		if f, ok = v.(float64); ok && (c.Min != nil && f < *c.Min || c.Max != nil && f > *c.Max) {
			return fmt.Errorf("%w: %s out of range", ErrInvalid, c.Key)
		}
	case Enum:
		s, isString := v.(string)
		ok = isString && slices.Contains(c.Options, s)
	case Text:
		_, ok = v.(string)
	case Composite:
		var m map[string]any
		m, ok = v.(map[string]any)
		for k := range m {
			ok = ok && slices.ContainsFunc(c.Fields, func(f Capability) bool { return f.Key == k })
		}
	case List:
		_, ok = v.([]any)
	}
	if !ok {
		return fmt.Errorf("%w: bad value for %s", ErrInvalid, c.Key)
	}
	return nil
}

// Equal reports whether two Values' data are the same.
func Equal(a, b any) bool {
	switch a.(type) {
	case nil, bool, float64, string:
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

// unreported reports whether no report ever confirms setting c to v: c is not
// Observable, or v is one of its Triggers.
func unreported(c *Capability, v any) bool {
	s, _ := v.(string)
	return !c.Access.Observable || slices.Contains(c.Triggers, s)
}

// matches reports whether a reported value satisfies a requested one; for a
// Composite, only the requested fields are compared. A Numeric one matches
// within half its step, if any: devices round to their own resolution (a Hue
// effect speed of 0.5 comes back as 0.498).
func matches(requested, reported any, step *float64) bool {
	if a, ok := requested.(float64); ok && step != nil {
		b, ok := reported.(float64)
		return ok && math.Abs(a-b) <= *step/2
	}
	want, ok := requested.(map[string]any)
	if !ok {
		return Equal(requested, reported)
	}
	got, ok := reported.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range want {
		if !Equal(v, got[k]) {
			return false
		}
	}
	return true
}
