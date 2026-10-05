package home

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/store"
)

const (
	// sendInterval is the minimum delay between two transmissions to the same
	// target: bursts (a dragged slider) collapse into the latest values
	// instead of flooding the radio network.
	sendInterval = 100 * time.Millisecond
	// commandTimeout exceeds zigbee2mqtt's own 10 s delivery timeout. It holds
	// for any fade: zigbee2mqtt confirms as soon as the device acks.
	commandTimeout = 15 * time.Second
	// maxTransition is the ZCL transtime ceiling: 65535 tenths of a second.
	maxTransition = 6553500 * time.Millisecond
	// toggle, in place of a bool, asks a binary Capability to flip.
	toggle = "toggle"
	// ponytail: an observer that falls this far behind is dropped and must
	// resubscribe (it gets a fresh Snapshot); per-Capability coalescing if
	// that ever happens in practice.
	subscriberBuffer = 1024
)

// Bridge is what Home needs of a bridge.Bridge: transmitting Commands.
type Bridge interface {
	Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error
}

// link is an attached Bridge, whether it is online, and whether it has
// replayed.
type link struct {
	bridge   Bridge
	online   bool
	replayed bool
}

// native identifies a Device's hardware: its Native Address within its Bridge.
type native struct{ bridge, address string }

type UpdateKind string

const (
	DevicesChanged      UpdateKind = "devices"
	ValueChanged        UpdateKind = "value"
	ValueRefreshed      UpdateKind = "refresh"
	EventOccurred       UpdateKind = "event"
	AvailabilityChanged UpdateKind = "availability"
	BridgeChanged       UpdateKind = "bridge"
	CommandChanged      UpdateKind = "command"
	AggregatesChanged   UpdateKind = "aggregates"
	FlagsChanged        UpdateKind = "flags"
	AreasChanged        UpdateKind = "areas"
	AutomationsChanged  UpdateKind = "automations"
	RunEnded            UpdateKind = "run"
	TargetDeleted       UpdateKind = "deleted"  // Target: a Device, an Aggregate or a Flag, gone with its History
	DeviceReplaced      UpdateKind = "replaced" // Target: the kept Device, which took over Device Replaced's hardware
)

// Update is a numbered notification; Seq is strictly increasing.
type Update struct {
	Seq   uint64     `json:"seq"`
	Kind  UpdateKind `json:"kind"`
	Ref   *Ref       `json:"ref,omitempty"`
	Value *Value     `json:"value,omitempty"`
	// With a new Value: it follows unknown, or it is an Aggregate's and only
	// members' first Values moved it. Replayed state, rather than a change.
	Initial bool `json:"initial,omitempty"`
	// With a Value or an Event: the new hardware's, announced again under the
	// kept identity by a Replace. Not a new reading.
	Moved        bool               `json:"moved,omitempty"`
	Target       Target             `json:"target,omitzero"` // with Availability: a Device, an Aggregate or a Flag
	Availability Availability       `json:"availability,omitempty"`
	Replaced     DeviceID           `json:"replaced,omitempty"` // with DeviceReplaced
	Bridge       string             `json:"bridge,omitempty"`   // with BridgeOnline
	BridgeOnline *bool              `json:"bridgeOnline,omitempty"`
	Command      *CommandState      `json:"command,omitempty"`
	Devices      []Device           `json:"devices,omitempty"`
	Aggregates   []Aggregate        `json:"aggregates,omitempty"`
	Flags        []Flag             `json:"flags,omitempty"`
	Areas        []Area             `json:"areas,omitempty"`
	Automations  []AutomationStatus `json:"automations,omitempty"`
	Run          *RunEnd            `json:"run,omitempty"`
}

// RunOutcome sums up what a Run did.
type RunOutcome string

const (
	RunActed   RunOutcome = "acted"   // it issued Commands
	RunNothing RunOutcome = "nothing" // it issued none
	RunError   RunOutcome = "error"   // something it tried was refused
)

// RunEnd is the end of one Run of an Automation.
type RunEnd struct {
	Automation string     `json:"automation"`
	Run        uuid.UUID  `json:"run"`
	Time       time.Time  `json:"time"`
	Outcome    RunOutcome `json:"outcome"`
	Trigger    Trigger    `json:"trigger"`
	Commands   int        `json:"commands"` // how many it issued
}

// Trigger is what started a Run.
type Trigger struct {
	Step       string    `json:"step"` // the trigger, or the timing Step whose deadline came due
	Kind       string    `json:"kind"`
	Target     Target    `json:"target"`
	Capability string    `json:"capability"`
	Value      any       `json:"value"`             // the Value, or the Event for an event trigger
	Time       time.Time `json:"time"`              // when it happened, or was scheduled
	CatchUp    bool      `json:"catchUp,omitempty"` // it came due while Oiko was down, and the Run's time is when Oiko was back
	Skipped    int       `json:"skipped,omitempty"` // a presence simulation's earlier switches due at once, skipped for this latest one
}

// AutomationState is what an Automation is doing, as its engine reports it.
type AutomationState string

const (
	AutomationEnabled  AutomationState = "enabled"
	AutomationDisabled AutomationState = "disabled"
	AutomationBroken   AutomationState = "broken"  // with a Reason
	AutomationRunaway  AutomationState = "runaway" // until re-enabled
)

// AutomationStatus is the status of one Automation, by its id.
type AutomationStatus struct {
	ID     string          `json:"id"`
	Status AutomationState `json:"status"`
	Reason string          `json:"reason,omitempty"`
	Step   string          `json:"step,omitempty"` // the Step it is broken at, if any
	Since  time.Time       `json:"since,omitzero"` // when it became runaway
}

type CommandStatus string

const (
	Pending    CommandStatus = "pending"
	Confirmed  CommandStatus = "confirmed"
	Failed     CommandStatus = "failed"
	TimedOut   CommandStatus = "timed_out"
	Superseded CommandStatus = "superseded"
)

// CommandState is the status of a Command on its Target.
type CommandState struct {
	ID     string        `json:"id"`
	Target Target        `json:"target"`
	Status CommandStatus `json:"status"`
	Origin Origin        `json:"origin"`
	// The Aggregate Command relaying this one, if any.
	AggregateCommand string `json:"aggregateCommand,omitempty"`
}

// Origin is what issued a Command: a Step of an Automation, in one of its
// Runs, or the API (the zero Origin), which the UI and external programs
// share. It reads "api" in JSON.
type Origin struct {
	Automation string    `json:"automation"`
	Step       string    `json:"step"`
	Run        uuid.UUID `json:"run"`
}

// plainOrigin is Origin without its JSON methods.
type plainOrigin Origin

func (o Origin) MarshalJSON() ([]byte, error) {
	if o == (Origin{}) {
		return []byte(`"api"`), nil
	}
	return json.Marshal(plainOrigin(o))
}

func (o *Origin) UnmarshalJSON(data []byte) error {
	if string(data) == `"api"` {
		*o = Origin{}
		return nil
	}
	return json.Unmarshal(data, (*plainOrigin)(o))
}

// Request is what a Command asks of its target: Values keyed by Capability
// key, reached over Transition (zero: at once), on behalf of Origin.
type Request struct {
	Values     map[string]any
	Transition time.Duration
	Origin     Origin
}

// CommandRecord is a Command as the Command history keeps it: its state and,
// from when it is pending, what it asks for and when.
type CommandRecord struct {
	CommandState
	Values     map[string]any `json:"values,omitempty"`     // never modified
	Transition float64        `json:"transition,omitempty"` // in seconds, as the API takes it
	Time       time.Time      `json:"time"`
}

// Snapshot is the whole observable state at Seq.
type Snapshot struct {
	Seq        uint64          `json:"seq"`
	Bridges    map[string]bool `json:"bridges"` // whether each is online
	Devices    []Device        `json:"devices"`
	Aggregates []Aggregate     `json:"aggregates"`
	Flags      []Flag          `json:"flags"`
	Areas      []Area          `json:"areas"` // in the occupant's order
	// Availability is that of every Device, Aggregate and Flag; a Function's
	// is its Device's.
	Availability map[Target]Availability `json:"availability"`
	Values       []RefValue              `json:"values"`
	Events       []RefValue              `json:"events"` // last occurrence of each Event
	Automations  []AutomationStatus      `json:"automations"`
}

type RefValue struct {
	Ref   Ref   `json:"ref"`
	Value Value `json:"value"`
}

// Home is the single source of truth at runtime. Every mutation happens under
// one lock and emits its Updates in order.
type Home struct {
	save           func([]Device) error             // persists the registry; nil in tests that don't care
	saveAggregates func([]Aggregate) error          // persists Aggregate definitions; nil in tests that don't care
	saveFlags      func([]SavedFlag) error          // persists Flags; nil in tests that don't care
	saveAreas      func([]Area) error               // persists Areas; nil in tests that don't care
	history        func(CommandRecord)              // keeps Commands; nil in tests that don't care
	recall         func(Ref, any) (time.Time, bool) // see Recall; nil in tests that don't care

	mu           sync.Mutex
	seq          uint64
	links        map[string]*link // by Bridge name
	known        chan struct{}    // closed once every attached Bridge has replayed
	devices      map[DeviceID]*Device
	byAddress    map[native]DeviceID // includes Detached Devices, so re-pairing reattaches
	availability map[DeviceID]Availability
	values       map[Ref]Value // every Target's; an Aggregate's are derived, never saved
	aggregates   map[AggregateID]*Aggregate
	flags        map[FlagID]Flag // with no Kind or Capabilities: they are flagFunction's
	areas        []Area          // in the occupant's order
	// derived, as last announced; absent means unknown
	aggregateAvailability map[AggregateID]Availability
	lastEvents            map[Ref]Value
	commands              *commands
	senders               map[Target]*sender // Devices' and their Functions'
	automations           []AutomationStatus // as last reported
	subscribers           map[chan Update]struct{}
	followers             map[*func(Update)]struct{}
}

// The formats of the saved Devices, Aggregates, Flags and Areas (ADR 0019).
var DevicesFormat, AggregatesFormat, FlagsFormat, AreasFormat store.Format

// New starts from previously saved Devices, Aggregates, Flags and Areas and
// saves each of them again, through save, saveAggregates, saveFlags and
// saveAreas, whenever they change. history is handed every accepted Command,
// then each later status of it, under Home's lock: it must return at once.
// Bridges are attached next.
func New(saved []Device, savedAggregates []Aggregate, savedFlags []SavedFlag, savedAreas []Area,
	save func([]Device) error, saveAggregates func([]Aggregate) error, saveFlags func([]SavedFlag) error,
	saveAreas func([]Area) error,
	history func(CommandRecord),
) *Home {
	h := &Home{
		save:                  save,
		saveAggregates:        saveAggregates,
		history:               history,
		saveFlags:             saveFlags,
		saveAreas:             saveAreas,
		areas:                 slices.Clone(savedAreas), // changed in place
		devices:               map[DeviceID]*Device{},
		links:                 map[string]*link{},
		known:                 make(chan struct{}),
		byAddress:             map[native]DeviceID{},
		availability:          map[DeviceID]Availability{},
		values:                map[Ref]Value{},
		aggregates:            map[AggregateID]*Aggregate{},
		flags:                 map[FlagID]Flag{},
		aggregateAvailability: map[AggregateID]Availability{},
		lastEvents:            map[Ref]Value{},
		senders:               map[Target]*sender{},
		subscribers:           map[chan Update]struct{}{},
		followers:             map[*func(Update)]struct{}{},
	}
	h.commands = newCommands(&h.mu, h.commandChanged)
	for _, d := range saved {
		h.devices[d.ID] = &d
		h.byAddress[native{d.Bridge, d.NativeAddress}] = d.ID
	}
	for _, a := range savedAggregates {
		h.aggregates[a.ID] = &a
	}
	for _, f := range savedFlags {
		h.flags[f.ID] = Flag{ID: f.ID, Name: f.Name, Area: f.Area}
		h.values[flagRef(f.ID)] = f.Value
	}
	h.deriveAll()                 // once all are loaded: nested ones resolve through each other
	h.syncAggregateValues()       // from the Flags' Values
	h.syncAggregateAvailability() // unknown until the Bridge is online, unless made of Flags
	return h
}

// Subscribe returns the current Snapshot and the channel of every later
// Update. The channel is closed if the observer falls too far behind.
func (h *Home) Subscribe() (Snapshot, <-chan Update, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Update, subscriberBuffer)
	h.subscribers[ch] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
	}
	return h.snapshot(), ch, cancel
}

// Follow returns the current Snapshot and hands every later Update to
// deliver, in order, never dropping one: the lossless feed of the automation
// engine. deliver runs under Home's lock, so it must return at once and never
// call back into Home.
func (h *Home) Follow(deliver func(Update)) (Snapshot, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.followers[&deliver] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.followers, &deliver)
	}
	return h.snapshot(), cancel
}

// SetAutomationStatus records the status of every Automation, in the order
// of their document, and announces it.
func (h *Home) SetAutomationStatus(list []AutomationStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.automations = list
	h.emit(Update{Kind: AutomationsChanged, Automations: list})
}

// EndRun announces the end of a Run.
func (h *Home) EndRun(r RunEnd) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emit(Update{Kind: RunEnded, Run: &r})
}

// Port is how one Bridge feeds Home: what it describes and reports concerns
// its own Devices only.
type Port struct {
	h      *Home
	bridge string
}

var _ bridge.Port = (*Port)(nil)

// Attach makes Bridge b known under name, offline until it says otherwise,
// and returns the Port it feeds Home through.
func (h *Home) Attach(name string, b Bridge) *Port {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.links[name] = &link{bridge: b}
	return &Port{h, name}
}

// Replayed tells Home the Bridge has handed it its Replay: its Devices'
// state is as known as it will get. Later calls, on reconnections, change
// nothing.
func (p *Port) Replayed() {
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	h.links[p.bridge].replayed = true
	h.checkKnown()
}

// Known is closed once every attached Bridge has replayed: the home is
// known. Bridges are attached before.
func (h *Home) Known() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkKnown() // with no Bridge, at once
	return h.known
}

// Waiting names the attached Bridges that have not replayed yet, sorted.
func (h *Home) Waiting() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var names []string
	for name, l := range h.links {
		if !l.replayed {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// checkKnown closes known once no attached Bridge is left to replay. Callers
// hold h.mu.
func (h *Home) checkKnown() {
	for _, l := range h.links {
		if !l.replayed {
			return
		}
	}
	select {
	case <-h.known:
	default:
		close(h.known)
	}
}

// online reports whether Bridge name is attached and online. Callers hold h.mu.
func (h *Home) online(bridge string) bool {
	l := h.links[bridge]
	return l != nil && l.online
}

// SetOnline records the Bridge's state; while it is offline, the Availability
// of each of its Devices is unknown.
func (p *Port) SetOnline(online bool) {
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.links[p.bridge]
	if l.online == online {
		return
	}
	l.online = online
	h.emit(Update{Kind: BridgeChanged, Bridge: p.bridge, BridgeOnline: &online})
	for id, a := range h.availability {
		if d := h.devices[id]; a != Unknown && !d.Detached && d.Bridge == p.bridge {
			h.emit(Update{Kind: AvailabilityChanged, Target: TargetDevice(id, ""), Availability: h.effectiveAvailability(id)})
		}
	}
	h.syncAggregateAvailability()
}

// SyncDevices reconciles the registry with the full list of Devices described
// by the Bridge: its other Devices become Detached. Known Native Addresses
// keep their Device identity, Name, Icon and Areas.
func (p *Port) SyncDevices(described []bridge.Device) {
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[DeviceID]bool{}
	for _, b := range described {
		d := fromBridge(b)
		d.Bridge = p.bridge
		key := native{p.bridge, d.NativeAddress}
		if id, known := h.byAddress[key]; known {
			d.inherit(h.devices[id])
		} else {
			d.ID = DeviceID(uuid.NewV7().String())
			h.byAddress[key] = d.ID
		}
		d.Detached = false
		h.devices[d.ID] = &d
		seen[d.ID] = true
	}
	for id, d := range h.devices {
		if d.Bridge == p.bridge && !seen[id] && !d.Detached {
			detached := *d
			detached.Detached = true
			h.devices[id] = &detached
		}
	}
	for _, m := range []map[Ref]Value{h.values, h.lastEvents} {
		for ref := range m {
			if d := ref.Target.Device(); d != "" && h.devices[d].capability(ref.Target.Function(), ref.Capability) == nil {
				delete(m, ref)
			}
		}
	}
	h.registryChanged()
}

// SetAvailability records the reachability of the Device at address.
func (p *Port) SetAvailability(address string, a Availability) {
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	id, ok := h.byAddress[native{p.bridge, address}]
	if !ok || h.availability[id] == a {
		return
	}
	before := h.effectiveAvailability(id)
	h.availability[id] = a
	if after := h.effectiveAvailability(id); after != before {
		h.emit(Update{Kind: AvailabilityChanged, Target: TargetDevice(id, ""), Availability: after})
		h.syncAggregateAvailability()
	}
}

// Report records Values and Events reported for the Device at address, and
// confirms pending Commands they satisfy.
func (p *Port) Report(address string, readings []bridge.Reading, at time.Time) {
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	id, ok := h.byAddress[native{p.bridge, address}]
	if !ok {
		return
	}
	d := h.devices[id]
	rs := make([]recorded, 0, len(readings))
	for _, r := range readings {
		ref := TargetDevice(id, r.Function).Ref(r.Capability)
		rs = append(rs, recorded{ref: ref})
		c := d.capability(r.Function, r.Capability)
		switch {
		case c == nil:
		case c.Stateless:
			v := Value{Data: r.Data, At: at}
			h.lastEvents[ref] = v
			h.emit(Update{Kind: EventOccurred, Ref: &ref, Value: &v})
		default:
			rs[len(rs)-1].first = h.record(ref, r.Data, at)
		}
	}
	h.reportToAggregates(rs, at)
	for _, r := range readings {
		h.confirm(TargetDevice(id, r.Function))
	}
}

// Command validates and queues a Command on t, superseding any pending one
// on the same target. It returns the Command's ID; its outcome arrives as
// Updates.
func (h *Home) Command(t Target, req Request) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key, err := validCommand(req)
	switch {
	case err != nil:
		return "", err
	case t.IsZero():
		return "", fmt.Errorf("%w: a command targets one Function, Device, Aggregate or Flag", ErrInvalid)
	}
	r, ok := h.lookup(t)
	if !ok {
		return "", ErrNotFound
	}
	if err := r.accept(req.Values); err != nil {
		return "", err
	}
	return r.carry(req, key), nil
}

// record sets the Value of ref, announcing a change or a refresh, and reports
// whether it is ref's first. A refresh keeps its Since; a first Value takes
// the one recalled, if it had the same Data before Oiko started. Callers hold
// h.mu.
func (h *Home) record(ref Ref, data any, at time.Time) (first bool) {
	v := Value{Data: data, At: at, Since: at}
	kind := ValueChanged
	old, had := h.values[ref]
	switch {
	case had && Equal(old.Data, v.Data):
		kind, v.Since = ValueRefreshed, old.Since
	case !had && h.recall != nil:
		if t, ok := h.recall(ref, data); ok {
			v.Since = t
		}
	}
	h.values[ref] = v
	h.emit(Update{Kind: kind, Ref: &ref, Value: &v, Initial: !had})
	return !had
}

// Recall has Home ask recall, for each Ref's first Value since it started,
// when it took that Data before: the History's last point. Call it before the
// Bridges run.
func (h *Home) Recall(recall func(ref Ref, data any) (time.Time, bool)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recall = recall
}

// issue resolves a toggle of Capability key, if any, against the pending
// Command on d or, failing one, the current Value, then delivers the Command.
// Callers hold h.mu.
func (h *Home) issue(d direct, req Request, key string) string {
	if key != "" {
		t := d.target()
		req.Values = toggled(req.Values, key, h.commands.isOn(t, key, h.values[t.Ref(key)].Data))
	}
	return h.deliver(d, req, nil).id
}

// deliver makes a validated Command pending on d, relayed by relay if any,
// and has d transmit it. Callers hold h.mu.
func (h *Home) deliver(d direct, req Request, relay *inFlight) *inFlight {
	p := h.commands.start(d.target(), req, relay, d.timeout())
	d.transmit(req)
	return p
}

// validCommand checks what does not depend on the target, and returns the
// key of the Capability to toggle, if any.
func validCommand(req Request) (string, error) {
	if len(req.Values) == 0 || req.Transition < 0 || req.Transition > maxTransition {
		return "", ErrInvalid
	}
	key := ""
	for k, v := range req.Values {
		if v == toggle {
			if key != "" {
				return "", fmt.Errorf("%w: at most one toggle per command", ErrInvalid)
			}
			key = k
		}
	}
	return key, nil
}

// toggled resolves a toggle of Capability key, currently on or not: on turns
// it off and drops the other values, off turns it on with them.
func toggled(values map[string]any, key string, on bool) map[string]any {
	if on {
		return map[string]any{key: false}
	}
	values = maps.Clone(values)
	values[key] = true
	return values
}

// confirm settles the pending Command on t if its Values now match what it
// asks for. Callers hold h.mu.
func (h *Home) confirm(t Target) {
	asked := h.commands.asking(t)
	if asked == nil {
		return
	}
	var caps []Capability
	if d, ok := h.lookupDirect(t); ok {
		caps = d.capabilities()
	}
	for k, want := range asked {
		var step *float64
		if i := slices.IndexFunc(caps, func(c Capability) bool { return c.Key == k }); i >= 0 {
			if unreported(&caps[i], want) {
				continue
			}
			step = caps[i].Step
		}
		got, ok := h.values[t.Ref(k)]
		if !ok || !matches(want, got.Data, step) {
			return
		}
	}
	h.commands.settle(t, Confirmed)
}

// commandChanged announces the status of Command c and hands it to the
// Command history, with what req asks for: that of a pending Command, none
// for a later status. Callers hold h.mu.
func (h *Home) commandChanged(c *CommandState, req Request) {
	h.emit(Update{Kind: CommandChanged, Command: c})
	if h.history != nil {
		h.history(CommandRecord{CommandState: *c, Values: req.Values, Transition: req.Transition.Seconds(), Time: time.Now()})
	}
}

func (h *Home) effectiveAvailability(id DeviceID) Availability {
	a, ok := h.availability[id]
	if !ok || h.devices[id].Detached || !h.online(h.devices[id].Bridge) {
		return Unknown
	}
	return a
}

// emit numbers an Update and hands it to every observer and follower.
// Callers hold h.mu.
func (h *Home) emit(u Update) {
	h.seq++
	u.Seq = h.seq
	for ch := range h.subscribers {
		select {
		case ch <- u:
		default:
			delete(h.subscribers, ch)
			close(ch)
		}
	}
	for deliver := range h.followers {
		(*deliver)(u)
	}
}

func (h *Home) deviceList() []Device {
	list := make([]Device, 0, len(h.devices))
	for _, d := range h.devices {
		list = append(list, *d)
	}
	slices.SortFunc(list, func(a, b Device) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return list
}

func (h *Home) snapshot() Snapshot {
	s := Snapshot{
		Seq:          h.seq,
		Bridges:      map[string]bool{},
		Devices:      h.deviceList(),
		Aggregates:   h.aggregateList(),
		Flags:        h.flagList(),
		Areas:        append([]Area{}, h.areas...), // [] rather than null in JSON
		Availability: map[Target]Availability{},
		Values:       make([]RefValue, 0, len(h.values)),
		Events:       make([]RefValue, 0, len(h.lastEvents)),
		Automations:  append([]AutomationStatus{}, h.automations...),
	}
	for name, l := range h.links {
		s.Bridges[name] = l.online
	}
	for id := range h.devices {
		s.Availability[TargetDevice(id, "")] = h.effectiveAvailability(id)
	}
	for id, a := range h.aggregateAvailability {
		s.Availability[TargetAggregate(id)] = a
	}
	for id := range h.flags {
		s.Availability[TargetFlag(id)] = Online
	}
	for ref, v := range h.values {
		s.Values = append(s.Values, RefValue{Ref: ref, Value: v})
	}
	for ref, v := range h.lastEvents {
		s.Events = append(s.Events, RefValue{Ref: ref, Value: v})
	}
	return s
}
