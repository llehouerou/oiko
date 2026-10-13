package home

import "slices"

// Role is what a Capability is for on its Function (GLOSSARY.md): told by its
// key, type, access and category, never by the hardware model (ADR 0014).
// Oiko decides it, and announces it in the roles of what holds the
// Capability; every client reads it from there (ADR 0056).
type Role string

const (
	RoleControl    Role = "control"    // the main control, at most one; its on/off when binary
	RoleAdjustment Role = "adjustment" // refining the main control: a light's brightness and colour
	RoleState      Role = "state"      // occupied, open, leaking…: active while on; at most one
	RoleReading    Role = "reading"    // counters included
	RoleEvent      Role = "event"
	RoleSetting    Role = "setting"
	RoleHealth     Role = "health" // shown only when wrong
	RoleDiagnostic Role = "diagnostic"
)

// Roles is the Role of each Capability by key; one with none is absent.
type Roles map[string]Role

// The keys below are listed for authors of a type of Bridge in
// docs/write-a-bridge.md, whose Roles table roles_test.go keeps true to them.
var (
	mainControls     = []string{"state", "on", "alarm", "mode"} // by precedence
	lightAdjustments = []string{"brightness", "color_temp", "color_hs"}
	// The Bridge turns a contact's "closed" into off: on is open.
	stateKeys      = []string{"occupancy", "presence", "contact", "water_leak", "smoke", "connected"}
	healthKeys     = []string{"battery", "battpercentage", "battery_low", "tamper"}
	diagnosticKeys = []string{"current", "voltage"} // a plug's follow its power
)

// roles is the Role of each of caps, those of a Function of kind or of a
// Device itself (kind ""), by key. Each has one Role at most: a light's
// composites beside its colour (color_xy is color_hs in other coordinates)
// none. A light's main control is its state; a Presence's is home, the Home
// presence's home a state, and manual, how home was set, a diagnostic (ADR
// 0052).
func roles(kind string, caps []Capability) Roles {
	light, presence := kind == "light", kind == "presence"
	own := map[string]string{"light": "state", "presence": "home"}[kind]
	rank := func(c Capability) int {
		if own == "" {
			return slices.Index(mainControls, c.Key)
		}
		if c.Key == own {
			return 0
		}
		return -1
	}
	rs := Roles{}
	control := -1
	for i, c := range caps {
		if c.Category == Primary && c.Access.Settable && !c.Stateless && rank(c) >= 0 && (control < 0 || rank(c) < rank(caps[control])) {
			control = i
		}
	}
	state := false
	for i, c := range caps {
		switch {
		case i == control:
			rs[c.Key] = RoleControl
		case slices.Contains(healthKeys, c.Key):
			rs[c.Key] = RoleHealth
		case c.Category == Config:
			rs[c.Key] = RoleSetting
		case c.Category == Diagnostic || slices.Contains(diagnosticKeys, c.Key) || presence && c.Key == "manual":
			rs[c.Key] = RoleDiagnostic
		case c.Stateless:
			rs[c.Key] = RoleEvent
		case c.Access.Settable && light && slices.Contains(lightAdjustments, c.Key):
			rs[c.Key] = RoleAdjustment
		case c.Access.Settable && (!light || c.Type != Composite):
			rs[c.Key] = RoleSetting
		case c.Access.Settable: // a light's other composites: none
		case !state && c.Type == Binary && (slices.Contains(stateKeys, c.Key) || presence && c.Key == "home"):
			rs[c.Key], state = RoleState, true
		default:
			rs[c.Key] = RoleReading
		}
	}
	return rs
}

// readCapability is the Capability a Function of kind with caps is read
// through as a Presence source (ADR 0049): its main control if binary,
// otherwise its state.
func readCapability(kind string, caps []Capability) (string, bool) {
	rs := roles(kind, caps)
	for _, c := range caps {
		if rs[c.Key] == RoleControl && c.Type == Binary {
			return c.Key, true
		}
	}
	for _, c := range caps {
		if rs[c.Key] == RoleState {
			return c.Key, true
		}
	}
	return "", false
}
