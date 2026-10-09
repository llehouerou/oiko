// Package bridge is the contract between Oiko and its Bridges (ADR 0017, and
// GLOSSARY.md for the vocabulary): what a Bridge implements, the Port through
// which it feeds Oiko, the terms its Devices are described in, and the
// registry through which a type of Bridge is compiled into Oiko. It depends
// on the standard library only.
//
// A type of Bridge is a Go package that registers its Module from init:
//
//	func init() {
//		bridge.Register(bridge.Module{Type: "hue", New: newBridge})
//	}
//
// An Oiko built with it imports that package for its side effect and runs
// oiko.Main; the configuration then creates its Bridges by type.
package bridge

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Bridge is an external system that makes Devices known to Oiko and relays
// their messages.
type Bridge interface {
	// Run follows the external system and feeds Oiko through port until ctx
	// is cancelled. It reconnects by itself and reports trouble through
	// port.SetOnline and its own logs: Oiko never restarts it.
	Run(ctx context.Context, port Port)
	// Send transmits a Command's values, keyed by Capability key, to
	// Function function ("" for the Device itself) of the Device at address,
	// with the transition requested (0 for none). Oiko only sends values the
	// Capabilities accept, a toggle already resolved to true or false, and
	// one Send at a time per Function or Device; Sends for different ones may
	// run concurrently. ctx ends when the Command times out: past it, a
	// transmission is pointless. An error fails the Command; otherwise it
	// waits for a Report confirming it.
	//
	// Oiko refuses Commands while the Bridge is offline, so Send never comes
	// before Run has called port.SetOnline(true). A Command accepted just
	// before SetOnline(false) may still be sent after it.
	Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error
}

// Cameras is implemented by a Bridge whose Devices have cameras: Functions
// of kind "camera", which may have no Capability (ADR 0036). Oiko finds it
// by a type assertion on the Bridge.
type Cameras interface {
	// Picture returns the latest still image of camera Function function of
	// the Device at address, as its Bridge has it: it must never wake the
	// camera, which a Picture is read for every few minutes. Oiko keeps it in
	// memory only, for a minute at most.
	Picture(ctx context.Context, address, function string) (Picture, error)
	// Stream returns the URL of the live video of camera Function function
	// of the Device at address, which Oiko reads at once, for as long as
	// anyone watches it, sharing it among them: rtsp://, rtsps://, or
	// rtspx:// for RTSP over TLS whose certificate is not verified. It may be
	// new on each call, or the same every time. Oiko plays H.264 video and
	// AAC audio, without transcoding them.
	Stream(ctx context.Context, address, function string) (string, error)
}

// Picture is a camera's latest still image.
type Picture struct {
	Data        []byte
	ContentType string    // e.g. "image/jpeg"
	Taken       time.Time // when the camera took it
}

// Recordings is implemented by a Bridge whose cameras' system keeps
// Recordings: the clips it recorded by itself (ADR 0038). Oiko keeps none of
// them; it finds this interface by a type assertion on the Bridge, beside
// Cameras.
//
// The Bridge announces each new Recording, as soon as its system has it,
// with an Event of the camera Function's RecordingEvent Capability
// (Stateless, Enum): its data is the Recording's Trigger, or "other" when it
// has none, reported as of the Recording's Start, by which Oiko finds it
// (ADR 0039).
type Recordings interface {
	// Recordings lists the Recordings of camera Function function of the
	// Device at address that started within [from, to], the newest first.
	Recordings(ctx context.Context, address, function string, from, to time.Time) ([]Recording, error)
	// RecordingMedia fetches part of Recording id of that camera: its video,
	// an MP4 a browser plays, or its thumbnail, a still image. The Bridge
	// sends the request with header, which holds what Oiko passes on from
	// the browser (Range, If-Range), and whatever its system needs on top,
	// and returns the response, which Oiko relays and closes: its status,
	// Content-Type, Content-Length, Content-Range, Accept-Ranges,
	// Last-Modified and ETag. ErrNotFound when the system no longer has id.
	// Oiko logs the errors of both methods: they must not hold a URL that
	// gives the media to whoever reads it.
	RecordingMedia(ctx context.Context, address, function, id string, part RecordingPart, header http.Header) (*http.Response, error)
}

// Recording is a clip a camera's system recorded by itself.
type Recording struct {
	ID       string // chosen by the Bridge, stable while the system keeps it
	Start    time.Time
	Duration time.Duration
	Trigger  string // what triggered it, a short lower-case word ("motion", "person"), or ""
}

// RecordingEvent is the key of the Capability of a camera Function whose
// Events announce its new Recordings.
const RecordingEvent = "recording"

// RecordingPart is what RecordingMedia fetches of a Recording.
type RecordingPart string

const (
	Video     RecordingPart = "video"
	Thumbnail RecordingPart = "thumbnail"
)

// ErrNotFound tells Oiko that what it asked a Bridge for, such as a
// Recording, is not there.
var ErrNotFound = errors.New("not found")

// Port is how one Bridge feeds Oiko: what it describes and reports concerns
// its own Devices only. Its methods are safe for concurrent use.
type Port interface {
	// SyncDevices hands Oiko the full list of the Bridge's Devices. Those it
	// no longer lists become Detached; reports for a Device not listed yet
	// are ignored.
	SyncDevices(devices []Device)
	// SetOnline tells whether the external system is reachable; while it is
	// not, the Availability of each of its Devices is unknown, and Commands
	// to them are refused. A Bridge starts offline.
	SetOnline(online bool)
	// SetAvailability records the reachability of the Device at address.
	SetAvailability(address string, a Availability)
	// Report records Values and Events of the Device at address as of at,
	// when the device sent them, and confirms the Commands they satisfy.
	Report(address string, readings []Reading, at time.Time)
	// Replayed tells Oiko the Bridge has handed it its Replay: its Devices'
	// state is as known as it will get. Oiko's automations wait for every
	// Bridge's, at most about 10 s. Later calls change nothing.
	Replayed()
}

// Device is a physical device as its Bridge describes it. Oiko gives it its
// identity, and keeps its Name, Icon and Areas, by NativeAddress.
type Device struct {
	NativeAddress string // its identifier within the Bridge
	Name          string // its label in the Bridge, the Name of a new Device in Oiko
	Model         string
	Vendor        string
	Functions     []Function
	Capabilities  []Capability // device-level: configuration and diagnostics
}

// Function is what a Device does for the household. Key is kind + endpoint,
// e.g. "light" or "switch/l2", unique within its Device; Kind and the
// Capability keys shape its Tile (ADR 0014).
//
// A Bridge may give any Kind; Functions of the same Kind are aggregated and
// grouped together. The kinds Oiko knows are:
//   - "light": its brightness and colour adjust its main control; each Area
//     aggregates its lights (ADR 0013)
//   - "switch" (a plug) and "alarm" (a siren): a bar with its own icon
//   - "camera": see Cameras (ADR 0036)
//   - "occupancy", "contact" (a door or window), "temperature", "humidity"
//     and "co2": each Area aggregates them
//   - "button", "pressure" and "illuminance": grouped in the History, as are
//     the kinds above
//
// Oiko's own Flags are of kind "flag".
type Function struct {
	Key          string
	Kind         string
	Capabilities []Capability
}

// Reading is a Capability's value reported by a Bridge: of Function
// Function, or of the Device itself when Function is "". Data is typed as
// the Capability's Type says; a Stateless Capability's reading is an Event.
type Reading struct {
	Function   string
	Capability string
	Data       any
}

// Availability is the reachability of a Device.
type Availability string

const (
	Online  Availability = "online"
	Offline Availability = "offline"
	Unknown Availability = "unknown"
)

// ValueType is the type of a Capability's values, and of their Go
// representation.
type ValueType string

const (
	Binary    ValueType = "binary"    // bool
	Numeric   ValueType = "numeric"   // float64
	Enum      ValueType = "enum"      // string: a Command sets one of the Options; a report may carry another, shown as is
	Text      ValueType = "text"      // string
	Composite ValueType = "composite" // map[string]any, keyed by the Fields' keys
	List      ValueType = "list"      // []any
)

// Category is what a Capability is for.
type Category string

const (
	Primary    Category = "primary"    // what its Function is for: its control, state, readings and Events
	Config     Category = "config"     // a setting that changes how the Device behaves: only an Admin sets it, never through an Aggregate (ADR 0023)
	Diagnostic Category = "diagnostic" // how the Device itself is doing (link quality, firmware): shown apart from what its Function does
)

// Access is what can be done with a Capability's Value.
type Access struct {
	Observable bool `json:"observable"` // the device reports it: a Command waits for a Report confirming it
	Settable   bool `json:"settable"`   // a Command may set it
	Queryable  bool `json:"queryable"`  // the device answers when asked for it; Oiko never asks
}

// Capability is a typed property of a Function, or of a Device for
// configuration and diagnostic properties. Key is unique within its owner.
type Capability struct {
	Key       string       `json:"key"`
	Label     string       `json:"label"`
	Type      ValueType    `json:"type"`
	Unit      string       `json:"unit,omitempty"`
	Min       *float64     `json:"min,omitempty"`
	Max       *float64     `json:"max,omitempty"`
	Step      *float64     `json:"step,omitempty"`
	Options   []string     `json:"options,omitempty"`  // Enum values a Command may set
	Triggers  []string     `json:"triggers,omitempty"` // Options that trigger a one-off action, never reported as the Value
	Fields    []Capability `json:"fields,omitempty"`   // Composite members
	Access    Access       `json:"access"`
	Category  Category     `json:"category"`
	Stateless bool         `json:"stateless,omitempty"` // emits Events, has no Value
	Counter   bool         `json:"counter,omitempty"`   // a numeric running total that only rises, except on reset
}

// Env is what Oiko hands a Bridge it creates, or one of its Commands.
type Env struct {
	Name    string          // the Bridge's name in the configuration, recorded on its Devices
	Config  json.RawMessage // its section of the configuration, without "type"; decode it with Decode
	DataDir string          // a directory of its own, for what it keeps: tokens, sessions, pairings
	Log     *slog.Logger    // where it logs, its records carrying its name as "bridge"
}

// Decode decodes the Bridge's section of the configuration into v, refusing a
// key v has no field for, in its case: a misspelt or removed key stops Oiko from starting
// instead of being ignored (ADR 0019). No section decodes as an empty one.
func (e Env) Decode(v any) error {
	if len(e.Config) == 0 {
		return nil
	}
	return jsonv2.Unmarshal(e.Config, v, jsonv2.RejectUnknownMembers(true)) // names match in their case only
}

// Module is a type of Bridge compiled into Oiko.
type Module struct {
	Type string // what the configuration names it by
	// New creates a Bridge, which Oiko then runs. It reports a configuration
	// it cannot work with, so that Oiko refuses to start: decoding it with
	// env.Decode refuses unknown keys.
	New func(env Env) (Bridge, error)
	// Commands are run by name from the command line, on a Bridge of this
	// type, while Oiko is not serving: `oiko <bridge> <command> [args]`.
	Commands map[string]func(env Env, args []string) error
}

// registered is a Module and the package that registered it.
type registered struct {
	Module
	pkg string
}

var modules = map[string]registered{}

// Register makes a type of Bridge known to Oiko. It is called from init and
// panics on a type registered twice. The package calling it is recorded as
// the type's: Oiko reports the module and version it comes from.
func Register(m Module) {
	if _, dup := modules[m.Type]; dup || m.Type == "" || m.New == nil {
		panic(fmt.Sprintf("bridge: Register %q: duplicate, or no type or New", m.Type))
	}
	pc := make([]uintptr, 1)
	runtime.Callers(2, pc)
	frame, _ := runtime.CallersFrames(pc).Next()
	modules[m.Type] = registered{m, packageOf(frame.Function)}
}

// packageOf is the package path of fn, a function's qualified name such as
// "example.com/oiko-hue.init.0", where Go escapes the dots of the path's last
// element as %2e.
func packageOf(fn string) string {
	slash := strings.LastIndex(fn, "/") + 1
	dot := strings.Index(fn[slash:], ".")
	if dot < 0 {
		return fn
	}
	pkg, err := url.PathUnescape(fn[:slash+dot])
	if err != nil {
		return fn[:slash+dot]
	}
	return pkg
}

// Lookup finds the Module registered under type t.
func Lookup(t string) (Module, bool) {
	r, ok := modules[t]
	return r.Module, ok
}

// Registered is a type of Bridge compiled into Oiko and the Go package that
// registered it.
type Registered struct {
	Type    string
	Package string
}

// Types lists the types of Bridge compiled into Oiko, by type.
func Types() []Registered {
	var types []Registered
	for _, t := range slices.Sorted(maps.Keys(modules)) {
		types = append(types, Registered{t, modules[t].pkg})
	}
	return types
}
