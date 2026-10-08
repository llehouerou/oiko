// Package automation runs Automations (see GLOSSARY.md): graphs of Steps that
// react to the home's Updates and to the time by issuing Commands. One
// goroutine applies a lossless feed of Updates to its own mirror of the home
// and takes due deadlines from one timer heap, strictly in order, and
// executes each Run to the end at once.
package automation

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

const (
	// More than runawayRuns Runs within runawayWindow mark an Automation
	// runaway, until it is re-enabled.
	runawayRuns   = 20
	runawayWindow = time.Second
	// knownWithin is how long the clock waits for the home to be known: a
	// Bridge offline at startup must not hold every Automation back.
	knownWithin = 10 * time.Second
)

// Home is what the engine sees of the home: the same verbs as the API, plus a
// lossless feed, whether the home is known, and a place to report each
// Automation's status and the end of each Run.
type Home interface {
	Follow(deliver func(home.Update)) (home.Snapshot, func())
	Command(t home.Target, req home.Request) (string, error)
	SetAutomationStatus(list []home.AutomationStatus)
	EndRun(r home.RunEnd)
	// Known is closed once every Bridge has replayed; Waiting names those
	// that have not.
	Known() <-chan struct{}
	Waiting() []string
}

// automation is a loaded Document and its runtime state.
type automation struct {
	doc     Document
	order   int // document order, kept across reloads
	steps   []step
	visited []bool             // by input slot, during a Run
	invalid problem            // why doc is broken whatever the home holds
	broken  problem            // why it is broken now, if it is
	indexed bool               // it is live: its triggers are indexed and it keeps state
	runaway time.Time          // when it became runaway, zero unless it is
	runs    [runawayRuns]int64 // start of the latest Runs in Unix ns, a ring
	oldest  int                // index in runs
}

// problem is why an Automation is broken, and the Step at fault if there is
// one. The zero problem means it isn't.
type problem struct{ step, reason string }

func (a *automation) status() home.AutomationStatus {
	s := home.AutomationStatus{ID: a.doc.ID, Name: a.doc.Name, Status: home.AutomationEnabled}
	for _, step := range a.doc.Steps {
		if step.Kind == manualTrigger {
			s.ManualTriggers = append(s.ManualTriggers, home.ManualTrigger{Step: step.ID, Name: step.Name})
		}
	}
	switch {
	case a.broken.reason != "":
		s.Status, s.Reason, s.Step = home.AutomationBroken, a.broken.reason, a.broken.step
	case !a.doc.Enabled:
		s.Status = home.AutomationDisabled
	case !a.runaway.IsZero():
		s.Status, s.Since = home.AutomationRunaway, a.runaway
	}
	return s
}

type trigger struct {
	a    *automation
	step int
}

type eventKey struct {
	ref   home.Ref
	event string
}

// Engine runs the Automations.
type Engine struct {
	home     Home
	place    *Place             // nil if unknown
	dir      string             // where its documents are saved; "" if it saves nothing
	record   func(*Trace)       // nil in tests that don't care
	notify   func(Notification) // nil unless Notifications are configured
	now      func() time.Time   // in the host timezone
	rand     *rand.Rand         // draws presence simulations; guarded by mu
	unfollow func()
	writing  sync.Mutex // serializes writes of the Step state, and guards written
	written  []byte     // the Step state as last written or loaded, in JSON

	qmu   sync.Mutex
	inbox []home.Update // delivered, not applied yet
	spare []home.Update
	wake  chan struct{}

	// mu guards what follows. Unexported methods touching it are called with
	// mu held, or from fromSaved.
	mu        sync.Mutex
	autos     []*automation // in document order
	nextOrder int
	values    map[home.Ref]home.Value // the mirror
	// Availability of each Target that owns one; absent means unknown.
	availability map[home.Target]home.Availability
	// The Devices, Aggregates and Flags as last announced, and the Targets
	// they make.
	devices    []home.Device
	aggregates []home.Aggregate
	flags      []home.Flag
	targets    map[home.Target]bool
	onEvent    map[eventKey][]trigger // live triggers, by what they watch
	onValue    map[home.Ref][]trigger
	onAvail    map[home.Target][]trigger
	pending    deadlines // the timer heap
	published  []home.AutomationStatus
	trace      *Trace      // of the Run under way
	runner     *runner     // runs Code Steps
	started    time.Time   // when the clock started: deadlines before it are caught up
	dirty      bool        // the Step state changed since it was last written
	flush      *time.Timer // Flushes it, once armed by touch
}

// The documents an Engine saves in its directory, and their formats (ADR
// 0019): the Documents, and their runtime state by id.
const (
	documentsFile = "automations.json"
	stateFile     = "automation-state.json"
)

var documentsFormat, stateFormat store.Format

// New follows h from now on and loads docs, saving nothing; place is the
// home's location, if known. record keeps the Trace of every Run: it takes
// the Trace over, must return at once, and Releases it once done with it.
// Updates are applied, and deadlines kept, once Run is called.
func New(h Home, docs []Document, place *Place, record func(*Trace)) *Engine {
	return fromSaved("", h, docs, nil, place, record)
}

// Open is as New, with the Documents saved in dir and the runtime state
// saved for them: it loads them, migrating them if need be, and saves the
// Documents whenever they change, and their runtime state a little after it
// changes.
func Open(dir string, h Home, place *Place, record func(*Trace)) (*Engine, error) {
	var docs []Document
	var state map[string]State
	for _, doc := range []struct {
		file   string
		format store.Format
		v      any
	}{
		{documentsFile, documentsFormat, &docs},
		{stateFile, stateFormat, &state},
	} {
		path := filepath.Join(dir, doc.file)
		if err := store.Load(path, doc.format, doc.v); err != nil {
			return nil, fmt.Errorf("loading %s: %w", path, err)
		}
	}
	return fromSaved(dir, h, docs, state, place, record), nil
}

func fromSaved(dir string, h Home, docs []Document, state map[string]State, place *Place, record func(*Trace)) *Engine {
	e := &Engine{
		home:    h,
		place:   place,
		dir:     dir,
		record:  record,
		now:     time.Now,
		rand:    rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		wake:    make(chan struct{}, 1),
		values:  map[home.Ref]home.Value{},
		onEvent: map[eventKey][]trigger{},
		onValue: map[home.Ref][]trigger{},
		onAvail: map[home.Target][]trigger{},
	}
	e.runner = newRunner(e.values)
	snap, unfollow := h.Follow(e.deliver)
	e.unfollow = unfollow
	for _, rv := range snap.Values {
		e.values[rv.Ref] = rv.Value
	}
	e.availability = map[home.Target]home.Availability{}
	for t, a := range snap.Availability {
		e.availability[t] = a
	}
	e.devices, e.aggregates, e.flags = snap.Devices, snap.Aggregates, snap.Flags
	e.targets = home.Targets(e.devices, e.aggregates, e.flags)
	if state == nil {
		state = map[string]State{}
	}
	e.written, _ = json.Marshal(state)
	e.loadAll(docs, state)
	return e
}

// loadAll loads docs after the Automations already loaded, each with the
// runtime state saved for it: a restart reloads every Automation unchanged.
// Callers hold e.mu, or are New.
func (e *Engine) loadAll(docs []Document, state map[string]State) {
	for _, d := range docs {
		a := e.load(d, e.nextOrder)
		e.nextOrder++
		saved := state[d.ID]
		e.restore(a, saved.Steps)
		a.runaway = saved.Runaway
		e.autos = append(e.autos, a)
	}
	e.publish()
}

// Run waits for the home to be known, at most knownWithin, then applies
// Updates as they arrive and runs deadlines as they come due, until ctx is
// done. It wakes for nothing else. Deadlines that came due while Oiko was
// down run at once, against the home as it is once known.
func (e *Engine) Run(ctx context.Context) {
	defer e.unfollow()
	select {
	case <-e.home.Known():
	case <-time.After(knownWithin):
		log.Printf("automation: %v not replayed after %v; starting anyway", e.home.Waiting(), knownWithin)
	case <-ctx.Done():
		return
	}
	e.start()
	alarm := time.NewTimer(0)
	for {
		e.mu.Lock()
		next := e.tick()
		e.mu.Unlock()
		if e.drain() {
			continue // their Runs may have set deadlines
		}
		var ring <-chan time.Time
		if !next.IsZero() {
			alarm.Reset(next.Sub(e.now()))
			ring = alarm.C
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-ring:
		}
	}
}

// start applies the Updates delivered so far, then starts the clock.
func (e *Engine) start() {
	for e.drain() {
	}
	e.mu.Lock()
	e.started = e.now()
	e.mu.Unlock()
}

// poke wakes the engine goroutine, if it isn't already due to wake.
func (e *Engine) poke() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// deliver queues an Update without ever blocking or dropping it. It runs
// under Home's lock.
func (e *Engine) deliver(u home.Update) {
	e.qmu.Lock()
	e.inbox = append(e.inbox, u)
	e.qmu.Unlock()
	e.poke()
}

// drain applies the Updates delivered so far, reporting whether there were
// any.
func (e *Engine) drain() bool {
	e.qmu.Lock()
	batch := e.inbox
	e.inbox = e.spare[:0]
	e.qmu.Unlock()
	for _, u := range batch {
		e.mu.Lock()
		e.apply(u)
		e.mu.Unlock()
	}
	clear(batch)
	e.spare = batch
	return len(batch) > 0
}

// apply updates the mirror with u, then runs what u triggers. Callers hold
// e.mu.
func (e *Engine) apply(u home.Update) {
	switch u.Kind {
	case home.ValueChanged:
		ref := *u.Ref
		old, had := e.values[ref]
		if u.Value == nil {
			delete(e.values, ref)
		} else {
			e.values[ref] = *u.Value
		}
		first := !had || u.Initial // such as replayed retained state: it fires nothing
		for _, t := range e.onValue[ref] {
			t.a.steps[t.step].kind.(*valueTriggerStep).changed(e, t, old, first, u.Value)
		}
	case home.ValueRefreshed:
		e.values[*u.Ref] = *u.Value
	case home.AvailabilityChanged:
		old := e.availabilityOf(u.Target)
		e.availability[u.Target] = u.Availability
		for _, t := range e.onAvail[u.Target] {
			t.a.steps[t.step].kind.(*availabilityTriggerStep).changed(e, t, old, u.Availability)
		}
	case home.EventOccurred:
		if ev, ok := u.Value.Data.(string); ok {
			for _, t := range e.onEvent[eventKey{*u.Ref, ev}] {
				e.run(t.a, t.step, 0, observed(*u.Ref, *u.Value))
			}
		}
	case home.DevicesChanged:
		e.devices = u.Devices
		e.recheck()
	case home.AggregatesChanged:
		e.aggregates = u.Aggregates
		e.recheck()
	case home.FlagsChanged:
		e.flags = u.Flags
		e.recheck()
	}
}

// observed is a trigger by what a Step watches: v, the Value or Event of ref.
func observed(ref home.Ref, v home.Value) home.Trigger {
	return home.Trigger{Target: ref.Target, Capability: ref.Capability, Value: v.Data, Time: v.At}
}

// availabilityOf is the Availability of t, a Device, an Aggregate or a Flag.
func (e *Engine) availabilityOf(t home.Target) home.Availability {
	return cmp.Or(e.availability[t], home.Unknown)
}

// run starts a Run of a from output out of Step i, which tg fired, unless a
// is runaway or this Run makes it so. The Run ends with its Trace; run
// returns its end, and whether it ran.
func (e *Engine) run(a *automation, i, out int, tg home.Trigger) (home.RunEnd, bool) {
	if !a.runaway.IsZero() {
		return home.RunEnd{}, false
	}
	now := e.now()
	if oldest := a.runs[a.oldest]; oldest != 0 && now.UnixNano()-oldest < int64(runawayWindow) {
		a.runaway = now
		e.touch()
		log.Printf("automation %q: runaway, more than %d runs within %v", a.doc.Name, runawayRuns, runawayWindow)
		e.publish()
		return home.RunEnd{}, false
	}
	a.runs[a.oldest] = now.UnixNano()
	a.oldest = (a.oldest + 1) % runawayRuns

	tr := newTrace()
	tg.Step, tg.Kind = a.doc.Steps[i].ID, a.doc.Steps[i].Kind
	tr.Run, tr.Automation, tr.Time, tr.Trigger = newRunID(now), a.doc.ID, now, tg
	e.trace = tr
	clear(a.visited)
	e.fire(a, i, tr.reach(a.doc.Steps[i].ID), out)
	e.trace = nil

	end := tr.End()
	tr.Outcome = end.Outcome
	if e.record == nil {
		tr.Release()
	} else {
		e.record(tr) // queued before the end is announced
	}
	e.home.EndRun(end)
	return end, true
}

// fire follows the edges from output handle out of Step i, reached at k in
// the Trace, in edge order; each input acts at most once per Run.
func (e *Engine) fire(a *automation, i, k, out int) {
	r := &e.trace.Steps[k]
	r.Fired = append(r.Fired, a.steps[i].outputs[out])
	for _, l := range a.steps[i].next[out] {
		if a.visited[l.slot] {
			continue
		}
		a.visited[l.slot] = true
		e.act(a, l.step, l.in)
	}
}

// act executes input in of Step i, which records it and its evidence in the
// Trace.
func (e *Engine) act(a *automation, i, in int) {
	k := e.trace.reach(a.doc.Steps[i].ID)
	a.steps[i].kind.(reacher).reached(visit{e, a, i, k}, in)
}

// issue issues req to t on behalf of Step i of a, recording it in r.
func (e *Engine) issue(r *Reached, a *automation, i int, t home.Target, req home.Request) {
	req.Origin = home.Origin{Automation: a.doc.ID, Step: a.doc.Steps[i].ID, Run: e.trace.Run}
	id, err := e.home.Command(t, req)
	c := Issued{Target: t, ID: id, Values: req.Values}
	if err != nil {
		c.Refused = err.Error()
	}
	r.Commands = append(r.Commands, c)
}

// load compiles doc into an automation at order in document order. Callers
// hold e.mu, or are New.
func (e *Engine) load(doc Document, order int) *automation {
	a := &automation{doc: doc, order: order}
	var slots int
	a.steps, slots, a.invalid = compile(doc, e.place)
	a.visited = make([]bool, slots)
	for i, s := range a.steps {
		if t, ok := s.kind.(scheduled); ok {
			*t.deadline() = due{a: a, step: i, index: -1}
		}
	}
	e.refresh(a)
	return a
}

// recheck follows a change of the Devices, Aggregates or Flags, and so of the
// targets. Callers hold e.mu.
func (e *Engine) recheck() {
	e.targets = home.Targets(e.devices, e.aggregates, e.flags)
	for _, a := range e.autos {
		e.refresh(a)
	}
	e.publish()
}

// refresh recomputes whether a is broken, and makes it live exactly when it
// is enabled and not broken. Callers hold e.mu.
func (e *Engine) refresh(a *automation) {
	a.broken = a.invalid
	if a.broken.reason == "" {
		a.broken = e.missing(a)
	}
	if live := a.doc.Enabled && a.broken.reason == ""; live != a.indexed {
		e.index(a, live)
	}
}

// missing names a Step referring to a deleted target, if any.
func (e *Engine) missing(a *automation) problem {
	for i, s := range a.steps {
		for _, t := range s.targets {
			if !e.targets[t] {
				id := a.doc.Steps[i].ID
				return problem{id, fmt.Sprintf("step %q: target %+v is deleted", id, t)}
			}
		}
	}
	return problem{}
}

// index makes a live: its triggers indexed and its scheduled Steps armed.
// Or it makes it dead, which clears its Step state: a timer is cancelled
// without firing.
func (e *Engine) index(a *automation, add bool) {
	a.indexed = add
	for i, s := range a.steps {
		t := trigger{a, i}
		switch k := s.kind.(type) {
		case *eventTriggerStep:
			for _, ev := range k.events {
				reindex(e.onEvent, eventKey{k.ref, ev}, t, add)
			}
		case *valueTriggerStep:
			reindex(e.onValue, k.ref, t, add)
		case *availabilityTriggerStep:
			reindex(e.onAvail, k.target, t, add)
		}
		if k, ok := s.kind.(scheduled); ok && add {
			k.arm(e)
		}
	}
	if add {
		return
	}
	e.touch()
	for _, s := range a.steps {
		if k, ok := s.kind.(scheduled); ok {
			e.unschedule(k.deadline())
		}
		if k, ok := s.kind.(remembering); ok {
			k.forget()
		}
	}
}

// reindex adds t to m[k] in document order, or removes it.
func reindex[K comparable](m map[K][]trigger, k K, t trigger, add bool) {
	ts := m[k]
	switch {
	case !add:
		ts = slices.DeleteFunc(ts, func(o trigger) bool { return o == t })
	case !slices.Contains(ts, t):
		ts = append(ts, t)
		slices.SortFunc(ts, func(x, y trigger) int {
			return cmp.Or(cmp.Compare(x.a.order, y.a.order), cmp.Compare(x.step, y.step))
		})
	}
	if len(ts) == 0 {
		delete(m, k)
		return
	}
	m[k] = ts
}

// publish reports every Automation's status to Home, if any changed.
// Callers hold e.mu.
func (e *Engine) publish() {
	list := make([]home.AutomationStatus, len(e.autos))
	for i, a := range e.autos {
		list[i] = a.status()
	}
	if slices.EqualFunc(list, e.published, func(a, b home.AutomationStatus) bool { return reflect.DeepEqual(a, b) }) {
		return
	}
	e.published = list
	e.home.SetAutomationStatus(list)
}

// Documents returns every Automation's Document, in document order.
func (e *Engine) Documents() []Document {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.documents()
}

func (e *Engine) documents() []Document {
	docs := make([]Document, len(e.autos))
	for i, a := range e.autos {
		docs[i] = a.doc
	}
	return docs
}

// Create adds an Automation, last, and returns its id.
func (e *Engine) Create(doc Document) (string, error) {
	doc, err := validDocument(doc)
	if err != nil {
		return "", err
	}
	doc.ID = uuid.NewV7().String()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.autos = append(e.autos, e.load(doc, e.nextOrder))
	e.nextOrder++
	return doc.ID, e.changed()
}

// Replace swaps Automation id for doc, alone. Its Steps keep their state if
// their id and kind are unchanged; the rest of its runtime state starts
// afresh, so replacing it with enabled true re-enables it.
func (e *Engine) Replace(id string, doc Document) error {
	doc, err := validDocument(doc)
	if err != nil {
		return err
	}
	doc.ID = id
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.find(id)
	if i < 0 {
		return home.ErrNotFound
	}
	old := e.autos[i]
	e.autos[i] = e.load(doc, old.order)
	saved := old.stepStates()
	for j := range saved {
		saved[j].Last = time.Time{} // an edited time trigger follows its params from now: nothing to catch up
	}
	e.restore(e.autos[i], saved)
	e.index(old, false)
	return e.changed()
}

// Delete removes Automation id.
func (e *Engine) Delete(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.find(id)
	if i < 0 {
		return home.ErrNotFound
	}
	e.index(e.autos[i], false)
	e.autos = slices.Delete(e.autos, i, i+1)
	return e.changed()
}

func (e *Engine) find(id string) int {
	return slices.IndexFunc(e.autos, func(a *automation) bool { return a.doc.ID == id })
}

// changed publishes the statuses and saves the Documents. Callers hold e.mu.
func (e *Engine) changed() error {
	e.publish()
	e.touch() // their runtime state follows
	if e.dir == "" {
		return nil
	}
	if err := store.Save(filepath.Join(e.dir, documentsFile), documentsFormat, e.documents()); err != nil {
		log.Printf("automation: saving: %v", err)
		return err
	}
	return nil
}

// validDocument checks what makes a Document unsavable, rather than broken:
// its Name.
func validDocument(doc Document) (Document, error) {
	name, err := home.ValidName(doc.Name)
	if err != nil {
		return doc, err
	}
	doc.Name = name
	if doc.Steps == nil {
		doc.Steps = []Step{}
	}
	if doc.Edges == nil {
		doc.Edges = []Edge{}
	}
	return doc, nil
}
