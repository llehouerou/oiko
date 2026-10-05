package home

import (
	"maps"
	"time"
)

// sender transmits the Commands on one Device Target to its Bridge: a burst
// collapses into its latest values and transition, at most one Send per
// sendInterval, in order, each outside Home's lock and until the pending
// Command times out. A failed Send fails the pending Command.
type sender struct {
	queued     map[string]any // values not yet transmitted
	transition time.Duration  // of the latest queued Command
	flush      *time.Timer
	sending    bool
	lastSent   time.Time
}

// send queues req's values for Device Target t. Callers hold h.mu.
func (h *Home) send(t Target, req Request) {
	s := h.senders[t]
	if s == nil {
		s = &sender{}
		h.senders[t] = s
	}
	if s.queued == nil {
		s.queued = map[string]any{}
	}
	maps.Copy(s.queued, req.Values)
	s.transition = req.Transition
	if s.flush == nil {
		s.flush = time.AfterFunc(max(0, sendInterval-time.Since(s.lastSent)), func() { h.flush(t) })
	}
}

// flush transmits the queued values of t, outside the lock.
func (h *Home) flush(t Target) {
	h.mu.Lock()
	s := h.senders[t]
	if s == nil { // its Device was forgotten meanwhile
		h.mu.Unlock()
		return
	}
	if s.sending { // previous transmission still in flight: keep order, retry later
		s.flush = time.AfterFunc(sendInterval, func() { h.flush(t) })
		h.mu.Unlock()
		return
	}
	values, transition := s.queued, s.transition
	s.queued, s.flush, s.sending, s.lastSent = nil, nil, true, time.Now()
	d := h.devices[t.Device()]
	bridge := h.links[d.Bridge].bridge // attached: the Command was accepted
	ctx, cancel := h.commands.sending(t)
	h.mu.Unlock()

	err := bridge.Send(ctx, d.NativeAddress, t.Function(), values, transition)
	cancel()

	h.mu.Lock()
	defer h.mu.Unlock()
	s.sending = false
	if err != nil {
		h.commands.settle(t, Failed)
	}
}

// stopSending drops what is queued for Device id. Callers hold h.mu.
func (h *Home) stopSending(id DeviceID) {
	for t, s := range h.senders {
		if t.Device() == id {
			if s.flush != nil {
				s.flush.Stop()
			}
			delete(h.senders, t)
		}
	}
}
