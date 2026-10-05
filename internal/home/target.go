package home

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Target is what a Command addresses and what a Value belongs to, by stable
// id: a Device, one of its Functions, an Aggregate or a Flag. Build one with
// TargetDevice, TargetAggregate or TargetFlag, or parse its Key; the zero
// Target is none. Its Key is the same string on the wire, in the web client,
// in Starlark, in the Command history and in saved documents (ADR 0009):
//
//	device:<id>             the Device itself
//	device:<id>/<function>  one of its Functions, e.g. device:0193…/switch/l2
//	aggregate:<id>
//	flag:<id>
//
// Ids never contain "/", so a Function key may.
type Target struct {
	kind     targetKind
	id       string
	function string // of a Device's Function
}

type targetKind uint8

const (
	noTarget targetKind = iota
	deviceKind
	aggregateKind
	flagKind
)

// TargetDevice is Function function of Device d, or d itself when function
// is "".
func TargetDevice(d DeviceID, function string) Target {
	return Target{deviceKind, string(d), function}
}

func TargetAggregate(a AggregateID) Target { return Target{kind: aggregateKind, id: string(a)} }

func TargetFlag(f FlagID) Target { return Target{kind: flagKind, id: string(f)} }

// Device is the Device t is or belongs to, if any.
func (t Target) Device() DeviceID {
	if t.kind != deviceKind {
		return ""
	}
	return DeviceID(t.id)
}

// Function is the key of the Device's Function t is, if any.
func (t Target) Function() string { return t.function }

func (t Target) Aggregate() AggregateID {
	if t.kind != aggregateKind {
		return ""
	}
	return AggregateID(t.id)
}

func (t Target) Flag() FlagID {
	if t.kind != flagKind {
		return ""
	}
	return FlagID(t.id)
}

func (t Target) IsZero() bool { return t.kind == noTarget }

// Key is t's identity in one string; "" for none.
func (t Target) Key() string {
	switch t.kind {
	case deviceKind:
		if t.function != "" {
			return "device:" + t.id + "/" + t.function
		}
		return "device:" + t.id
	case aggregateKind:
		return "aggregate:" + t.id
	case flagKind:
		return "flag:" + t.id
	}
	return ""
}

func (t Target) String() string { return t.Key() }

// ParseTarget reads a Target's Key.
func ParseTarget(key string) (Target, error) {
	kind, rest, _ := strings.Cut(key, ":")
	id, function, hasFunction := strings.Cut(rest, "/")
	switch {
	case id == "":
	case kind == "device" && (!hasFunction || function != ""):
		return TargetDevice(DeviceID(id), function), nil
	case kind == "aggregate" && !hasFunction:
		return TargetAggregate(AggregateID(id)), nil
	case kind == "flag" && !hasFunction:
		return TargetFlag(FlagID(id)), nil
	}
	return Target{}, fmt.Errorf("%w: bad target %q", ErrInvalid, key)
}

// MarshalText writes t's Key: a JSON string, also as a map key.
func (t Target) MarshalText() ([]byte, error) { return []byte(t.Key()), nil }

// UnmarshalText reads a Key; "" is the zero Target.
func (t *Target) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*t = Target{}
		return nil
	}
	p, err := ParseTarget(string(text))
	if err != nil {
		return err
	}
	*t = p
	return nil
}

// Ref is Capability key of t.
func (t Target) Ref(key string) Ref { return Ref{Target: t, Capability: key} }

// Targets is every Target among devices, aggregates and flags: each Device,
// each of its Functions, each Aggregate and each Flag.
func Targets(devices []Device, aggregates []Aggregate, flags []Flag) map[Target]bool {
	all := map[Target]bool{}
	for _, d := range devices {
		all[TargetDevice(d.ID, "")] = true
		for _, f := range d.Functions {
			all[TargetDevice(d.ID, f.Key)] = true
		}
	}
	for _, a := range aggregates {
		all[TargetAggregate(a.ID)] = true
	}
	for _, f := range flags {
		all[TargetFlag(f.ID)] = true
	}
	return all
}

// resolved is a Target as Home knows it, whatever its kind: what a Command
// needs of it. Callers hold h.mu.
type resolved interface {
	// accept checks the values a Command asks for, its toggle unresolved.
	accept(values map[string]any) error
	// carry makes an accepted Command pending, resolving a toggle of
	// Capability key if any, and returns its ID.
	carry(req Request, key string) string
}

// direct is a resolved Target that carries out its Commands itself rather
// than relaying them: a Device, one of its Functions, or a Flag. The members
// of an Aggregate resolve to direct ones.
type direct interface {
	resolved
	target() Target
	kind() string // "" for a Device itself
	capabilities() []Capability
	// attached is false for a Detached Device and its Functions: they take
	// no part in Aggregates' Values and Availability.
	attached() bool
	availability() Availability
	// timeout is how long its Commands wait to be confirmed; 0 for never,
	// when transmit confirms them at once.
	timeout() time.Duration
	// transmit delivers req, the Command just made pending on it.
	transmit(req Request)
}

// lookup resolves t; false if there is no such Target. A Detached Device and
// its Functions resolve. Callers hold h.mu.
func (h *Home) lookup(t Target) (resolved, bool) {
	if t.kind == aggregateKind {
		a, ok := h.aggregates[t.Aggregate()]
		if !ok {
			return nil, false
		}
		return aggregateTarget{h, a}, true
	}
	return h.lookupDirect(t)
}

// lookupDirect resolves t, a Device, one of its Functions or a Flag.
// Callers hold h.mu.
func (h *Home) lookupDirect(t Target) (direct, bool) {
	switch t.kind {
	case flagKind:
		if _, ok := h.flags[t.Flag()]; ok {
			return flagTarget{h, t.Flag()}, true
		}
	case deviceKind:
		d, ok := h.devices[t.Device()]
		if !ok {
			return nil, false
		}
		x := deviceTarget{h: h, d: d}
		if t.function != "" {
			if x.fn = d.function(t.function); x.fn == nil {
				return nil, false
			}
		}
		return x, true
	}
	return nil, false
}

// Capability is the Capability r addresses, or the zero Capability when r's
// is "": the Target's Availability. ErrNotFound if there is no such Target
// or Capability.
func (h *Home) Capability(r Ref) (Capability, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var caps []Capability
	if a, ok := h.aggregates[r.Target.Aggregate()]; ok {
		caps = a.Capabilities
	} else if d, ok := h.lookupDirect(r.Target); ok {
		caps = d.capabilities()
	} else {
		return Capability{}, ErrNotFound
	}
	if r.Capability == "" {
		return Capability{}, nil
	}
	i := slices.IndexFunc(caps, func(c Capability) bool { return c.Key == r.Capability })
	if i < 0 {
		return Capability{}, ErrNotFound
	}
	return caps[i], nil
}

// deviceTarget is Function fn of Device d, or d itself when fn is nil. Its
// Commands go to d's Bridge.
type deviceTarget struct {
	h  *Home
	d  *Device
	fn *Function
}

func (x deviceTarget) target() Target {
	if x.fn == nil {
		return TargetDevice(x.d.ID, "")
	}
	return TargetDevice(x.d.ID, x.fn.Key)
}

func (x deviceTarget) kind() string {
	if x.fn == nil {
		return ""
	}
	return x.fn.Kind
}

func (x deviceTarget) capabilities() []Capability {
	if x.fn == nil {
		return x.d.Capabilities
	}
	return x.fn.Capabilities
}

func (x deviceTarget) attached() bool { return !x.d.Detached }

func (x deviceTarget) availability() Availability { return x.h.effectiveAvailability(x.d.ID) }

func (x deviceTarget) accept(values map[string]any) error {
	switch {
	case x.d.Detached:
		return ErrNotFound
	case !x.h.online(x.d.Bridge):
		return ErrBridgeOffline
	}
	return checkValues(x.capabilities(), values)
}

func (x deviceTarget) carry(req Request, key string) string { return x.h.issue(x, req, key) }

func (deviceTarget) timeout() time.Duration { return commandTimeout }

func (x deviceTarget) transmit(req Request) { x.h.send(x.target(), req) }
