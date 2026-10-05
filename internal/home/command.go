package home

import (
	"context"
	"sync"
	"time"
	"uuid"
)

// inFlight is a pending Command on its Target. A Command on an Aggregate
// relays one per counted member, each pointing back to it.
type inFlight struct {
	id       string
	target   Target
	req      Request     // with its toggle resolved
	timeout  *time.Timer // nil if it never times out
	deadline time.Time   // when it times out; zero if it never does
	relay    *inFlight   // the Aggregate Command relaying it, if any
	waiting  int         // when relaying: member Commands not confirmed yet
}

func (p *inFlight) state(status CommandStatus) *CommandState {
	c := &CommandState{ID: p.id, Target: p.target, Status: status, Origin: p.req.Origin}
	if p.relay != nil {
		c.AggregateCommand = p.relay.id
	}
	return c
}

// commands is the lifecycle of every Command: pending on its Target until
// confirmed, failed, timed out or superseded by a newer one there, and, when
// relayed for an Aggregate, followed up on the Command relaying it (ADR 0004).
// Each status is handed to announce. Callers hold mu, which its timers take.
type commands struct {
	mu       sync.Locker
	announce func(c *CommandState, req Request) // req: what a pending Command asks; none later
	pending  map[Target]*inFlight
}

func newCommands(mu sync.Locker, announce func(*CommandState, Request)) *commands {
	return &commands{mu: mu, announce: announce, pending: map[Target]*inFlight{}}
}

// start makes req the pending Command on t, announced before the one it
// supersedes, and returns it. relay is the Aggregate Command relaying it, if
// any. Unless over after timeout, it times out; 0 is never.
func (c *commands) start(t Target, req Request, relay *inFlight, timeout time.Duration) *inFlight {
	p := &inFlight{id: uuid.NewV7().String(), target: t, req: req, relay: relay}
	if timeout > 0 {
		p.timeout = time.AfterFunc(timeout, func() { c.expire(p) })
		p.deadline = time.Now().Add(timeout)
	}
	old := c.pending[t]
	c.pending[t] = p
	c.announce(p.state(Pending), req)
	if old != nil {
		c.end(old, Superseded)
	}
	return p
}

// sending is the context of a Send carrying the pending Command on t: it ends
// when that Command times out, at once without one. Device Targets, the only
// ones sent to, always time out.
func (c *commands) sending(t Target) (context.Context, context.CancelFunc) {
	var deadline time.Time
	if p := c.pending[t]; p != nil {
		deadline = p.deadline
	}
	return context.WithDeadline(context.Background(), deadline)
}

// asking is what the pending Command on t asks for; nil without one.
func (c *commands) asking(t Target) map[string]any {
	if p := c.pending[t]; p != nil {
		return p.req.Values
	}
	return nil
}

// isOn is whether binary Capability key of t is on, as a toggle sees it: as
// the pending Command asks or, failing one, as current is.
func (c *commands) isOn(t Target, key string, current any) bool {
	if v, ok := c.asking(t)[key]; ok {
		return v == true
	}
	return current == true
}

// settle ends the pending Command on t with status, if any.
func (c *commands) settle(t Target, status CommandStatus) {
	if p := c.pending[t]; p != nil {
		delete(c.pending, t)
		c.end(p, status)
	}
}

// fail ends as failed the pending Command on every Target gone selects:
// nothing will confirm them, and an Aggregate Command must not wait for them.
func (c *commands) fail(gone func(Target) bool) {
	for t := range c.pending {
		if gone(t) {
			c.settle(t, Failed)
		}
	}
}

// abandon forgets the pending Command on t, unannounced: t is gone, and the
// Commands it relayed carry on unrelayed.
func (c *commands) abandon(t Target) {
	if p := c.pending[t]; p != nil {
		delete(c.pending, t)
		p.stop()
	}
}

// end announces the outcome of p, no longer pending, and follows it up on the
// Command relaying it: confirmed once every member Command is, otherwise at
// once.
func (c *commands) end(p *inFlight, status CommandStatus) {
	p.stop()
	c.announce(p.state(status), Request{})
	r := p.relay
	if r == nil || c.pending[r.target] != r { // not relayed, or its relay is over
		return
	}
	if status == Confirmed {
		if r.waiting--; r.waiting > 0 {
			return
		}
	}
	c.settle(r.target, status)
}

func (p *inFlight) stop() {
	if p.timeout != nil {
		p.timeout.Stop()
	}
}

// expire times out p, unless it is over: its Target may even be gone, when
// its timer fired while the lock was held to forget it.
func (c *commands) expire(p *inFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[p.target] == p {
		c.settle(p.target, TimedOut)
	}
}
