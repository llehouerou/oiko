package home

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

func capOf(key string, typ ValueType, more ...func(*Capability)) Capability {
	c := Capability{Key: key, Type: typ, Access: Access{Observable: true}, Category: Primary}
	for _, m := range more {
		m(&c)
	}
	return c
}

func canSet(c *Capability)     { c.Access.Settable = true }
func config(c *Capability)     { c.Category = Config }
func diagnostic(c *Capability) { c.Category = Diagnostic }
func stateless(c *Capability)  { c.Stateless = true }

func TestRoles(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		caps []Capability
		want map[string]Role
	}{
		{"a light: its state, its brightness and colour; extras are settings, color_xy nothing", "light", []Capability{
			capOf("state", Binary, canSet), capOf("brightness", Numeric, canSet), capOf("color_temp", Numeric, canSet),
			capOf("color_hs", Composite, canSet), capOf("color_xy", Composite, canSet), capOf("effect", Enum, canSet),
			capOf("color_mode", Enum, diagnostic),
		}, map[string]Role{"state": RoleControl, "brightness": RoleAdjustment, "color_temp": RoleAdjustment, "color_hs": RoleAdjustment,
			"effect": RoleSetting, "color_mode": RoleDiagnostic}},
		{"a plug: current and voltage are diagnostics, its child lock a setting", "switch", []Capability{
			capOf("state", Binary, canSet), capOf("power", Numeric), capOf("current", Numeric), capOf("voltage", Numeric),
			capOf("energy", Numeric), capOf("child_lock", Binary, canSet), capOf("power_on_behavior", Enum, canSet, config),
		}, map[string]Role{"state": RoleControl, "power": RoleReading, "current": RoleDiagnostic, "voltage": RoleDiagnostic,
			"energy": RoleReading, "child_lock": RoleSetting, "power_on_behavior": RoleSetting}},
		{"a contact: a state; battery and tamper are health", "contact", []Capability{
			capOf("contact", Binary), capOf("battery", Numeric, diagnostic), capOf("tamper", Binary),
		}, map[string]Role{"contact": RoleState, "battery": RoleHealth, "tamper": RoleHealth}},
		{"a motion sensor: one state at most", "occupancy", []Capability{
			capOf("illuminance", Numeric), capOf("occupancy", Binary), capOf("presence", Binary), capOf("occupancy_timeout", Numeric, canSet),
		}, map[string]Role{"illuminance": RoleReading, "occupancy": RoleState, "presence": RoleReading, "occupancy_timeout": RoleSetting}},
		{"a phone's connection is a state", "network", []Capability{capOf("connected", Binary)}, map[string]Role{"connected": RoleState}},
		{"a Presence's home is its main control, manual a diagnostic", "presence", presenceFunction.Capabilities,
			map[string]Role{"home": RoleControl, "manual": RoleDiagnostic}},
		{"the Home presence's home is a state", "presence", homePresenceFunction.Capabilities, map[string]Role{"home": RoleState}},
		{"home is no main control elsewhere", "switch", []Capability{capOf("home", Binary, canSet)}, map[string]Role{"home": RoleSetting}},
		{"a button's are Events", "button", []Capability{capOf("action", Enum, stateless), capOf("battery", Numeric, diagnostic)},
			map[string]Role{"action": RoleEvent, "battery": RoleHealth}},
		{"a Flag's on, a siren's alarm, Arlo's arming mode are main controls", "alarm", []Capability{
			capOf("alarm", Binary, canSet), capOf("melody", Enum, canSet),
		}, map[string]Role{"alarm": RoleControl, "melody": RoleSetting}},
		{"Arlo's arming mode", "arming", []Capability{capOf("mode", Enum, canSet)}, map[string]Role{"mode": RoleControl}},
		{"a Flag", "flag", flagFunction.Capabilities, map[string]Role{"on": RoleControl}},
		{"one main control at most, by precedence", "switch", []Capability{capOf("on", Binary, canSet), capOf("state", Binary, canSet)},
			map[string]Role{"state": RoleControl, "on": RoleSetting}},
		{"one in the Bridge's config is a setting", "switch", []Capability{capOf("state", Binary, canSet, config)}, map[string]Role{"state": RoleSetting}},
		{"a Device's own Capabilities", "", []Capability{
			capOf("battery", Numeric, diagnostic), capOf("linkquality", Numeric, diagnostic), capOf("power_on_behavior", Enum, canSet, config),
		}, map[string]Role{"battery": RoleHealth, "linkquality": RoleDiagnostic, "power_on_behavior": RoleSetting}},
	} {
		if got := roles(tc.kind, tc.caps); !maps.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestReadCapability(t *testing.T) {
	for _, tc := range []struct {
		kind string
		caps []Capability
		want string
	}{
		{"network", []Capability{capOf("connected", Binary)}, "connected"},
		{"flag", flagFunction.Capabilities, "on"},
		{"light", []Capability{capOf("brightness", Numeric, canSet), capOf("state", Binary, canSet)}, "state"},
		{"arming", []Capability{capOf("mode", Enum, canSet), capOf("contact", Binary)}, "contact"},
		{"temperature", []Capability{capOf("temperature", Numeric)}, ""},
	} {
		if got, ok := readCapability(tc.kind, tc.caps); got != tc.want || ok != (tc.want != "") {
			t.Errorf("%s: %q, %v; want %q", tc.kind, got, ok, tc.want)
		}
	}
}

// Every Target carries its Roles to clients; a Device saves none of them.
func TestRolesAreAnnouncedNeverSaved(t *testing.T) {
	dir := t.TempDir()
	h := opened(t, dir)
	port(h).SyncDevices([]bridge.Device{{NativeAddress: "0xplug", Name: "Plug",
		Functions:    []bridge.Function{{Key: "switch", Kind: "switch", Capabilities: []Capability{capOf("state", Binary, canSet), capOf("power", Numeric)}}},
		Capabilities: []Capability{capOf("battery", Numeric)},
	}})
	if _, err := h.CreateFlag("Guests"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.CreateAggregate("Plugs", []Target{TargetDevice(snapshot(h).Devices[0].ID, "switch")}, Any, ""); err != nil {
		t.Fatal(err)
	}
	s := snapshot(h)
	d := s.Devices[0]
	for what, got := range map[string]Roles{"Device": d.Roles, "Function": d.Functions[0].Roles, "Aggregate": s.Aggregates[0].Roles,
		"Flag": s.Flags[0].Roles, "Home presence": s.Presences[0].Roles} {
		if len(got) == 0 {
			t.Errorf("%s announced without Roles", what)
		}
	}
	if want := (Roles{"state": RoleControl, "power": RoleReading}); !maps.Equal(d.Functions[0].Roles, want) {
		t.Errorf("the plug's Roles %v, want %v", d.Functions[0].Roles, want)
	}
	saved := onDisk(t, dir)
	if saved.devices[0].Roles != nil || saved.devices[0].Functions[0].Roles != nil || saved.aggregates[0].Roles != nil {
		t.Error("Roles saved")
	}
}

// Authors of a type of Bridge read these keys in the guide's Roles table; the
// web client's roles.test.ts checks its Reading row.
func TestTheGuideListsTheKeysOfEachRole(t *testing.T) {
	guide, err := os.ReadFile("../../docs/write-a-bridge.md")
	if err != nil {
		t.Fatal(err)
	}
	row := func(role string) []string {
		var keys []string
		for line := range strings.Lines(string(guide)) {
			if cells := strings.Split(line, "|"); strings.HasPrefix(line, "| "+role+" |") && len(cells) > 2 {
				for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(cells[2], -1) {
					keys = append(keys, m[1])
				}
			}
		}
		return keys
	}
	for role, keys := range map[string][]string{"Main control": mainControls, "Adjustment": lightAdjustments, "State": stateKeys,
		"Health": healthKeys, "Diagnostic": diagnosticKeys} {
		if got := row(role); !slices.Equal(got, keys) {
			t.Errorf("the guide's %s keys are %v, Home's %v", role, got, keys)
		}
	}
}
