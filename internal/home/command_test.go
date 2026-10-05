package home

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// lifecycle is a commands whose announcements are kept as "<target> <status>".
func lifecycle() (*commands, *sync.Mutex, *[]string) {
	var mu sync.Mutex
	var said []string
	c := newCommands(&mu, func(s *CommandState, _ Request) { said = append(said, s.Target.Key()+" "+string(s.Status)) })
	return c, &mu, &said
}

var (
	lamp1  = TargetDevice("l1", "light")
	lamp2  = TargetDevice("l2", "light")
	living = TargetAggregate("living")
	asksOn = Request{Values: map[string]any{"state": true}}
)

func TestNewerCommandIsAnnouncedBeforeTheOneItSupersedes(t *testing.T) {
	c, _, said := lifecycle()
	c.start(lamp1, asksOn, nil, 0)
	c.start(lamp1, asksOn, nil, 0)
	want := []string{"device:l1/light pending", "device:l1/light pending", "device:l1/light superseded"}
	if !slices.Equal(*said, want) {
		t.Errorf("said %v, want %v", *said, want)
	}
}

func TestRelayingCommandFollowsItsMembers(t *testing.T) {
	relay := func() (*commands, *[]string) {
		c, _, said := lifecycle()
		ac := c.start(living, asksOn, nil, 0)
		ac.waiting = 2
		c.start(lamp1, asksOn, ac, 0)
		c.start(lamp2, asksOn, ac, 0)
		*said = nil
		return c, said
	}
	t.Run("confirmed once all are", func(t *testing.T) {
		c, said := relay()
		c.settle(lamp1, Confirmed)
		c.settle(lamp2, Confirmed)
		if want := []string{"device:l1/light confirmed", "device:l2/light confirmed", "aggregate:living confirmed"}; !slices.Equal(*said, want) {
			t.Errorf("said %v, want %v", *said, want)
		}
	})
	t.Run("ended as soon as one fails", func(t *testing.T) {
		c, said := relay()
		c.settle(lamp1, Failed)
		c.settle(lamp2, Confirmed)
		if want := []string{"device:l1/light failed", "aggregate:living failed", "device:l2/light confirmed"}; !slices.Equal(*said, want) {
			t.Errorf("said %v, want %v", *said, want)
		}
	})
	t.Run("abandoned, its members carry on unrelayed", func(t *testing.T) {
		c, said := relay()
		c.abandon(living)
		c.settle(lamp1, Confirmed)
		c.settle(lamp2, Confirmed)
		if want := []string{"device:l1/light confirmed", "device:l2/light confirmed"}; !slices.Equal(*said, want) {
			t.Errorf("said %v, want %v", *said, want)
		}
	})
}

// The timeout fires just as a Delete holding the lock fails the Command of
// the Device it forgets: Stop comes too late, and the timer runs after.
func TestTimeoutAfterTheTargetIsGoneIsIgnored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, mu, said := lifecycle()
		mu.Lock()
		p := c.start(lamp1, asksOn, nil, commandTimeout)
		c.fail(func(Target) bool { return true })
		mu.Unlock()
		c.expire(p)
		if want := []string{"device:l1/light pending", "device:l1/light failed"}; !slices.Equal(*said, want) {
			t.Errorf("said %v, want %v", *said, want)
		}
	})
}

func TestTimeoutEndsAPendingCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, mu, said := lifecycle()
		mu.Lock()
		c.start(lamp1, asksOn, nil, commandTimeout)
		mu.Unlock()
		time.Sleep(commandTimeout)
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if want := []string{"device:l1/light pending", "device:l1/light timed_out"}; !slices.Equal(*said, want) {
			t.Errorf("said %v, want %v", *said, want)
		}
	})
}

func TestToggleSeesThePendingCommandFirst(t *testing.T) {
	c, _, _ := lifecycle()
	if !c.isOn(lamp1, "state", true) || c.isOn(lamp1, "state", false) {
		t.Error("without a pending Command, a toggle follows the current Value")
	}
	c.start(lamp1, Request{Values: map[string]any{"state": false}}, nil, 0)
	if c.isOn(lamp1, "state", true) {
		t.Error("a pending Command asking off must win over a current Value on")
	}
	if !c.isOn(lamp1, "brightness", true) {
		t.Error("a Capability the pending Command leaves alone follows the current Value")
	}
}
