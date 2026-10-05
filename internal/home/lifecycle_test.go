package home

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

// Home announces every Delete and Replace it applied, even when saving it
// fails afterwards: the History follows them.
func TestDeletesAndReplacesAreAnnouncedEvenWhenSavingFails(t *testing.T) {
	var full atomic.Bool
	fail := func() error {
		if full.Load() {
			return errors.New("disk full")
		}
		return nil
	}
	h := New(nil, nil, nil, nil,
		func([]Device) error { return fail() }, func([]Aggregate) error { return fail() },
		func([]SavedFlag) error { return fail() }, func([]Area) error { return fail() }, nil)
	h.Attach(zb, &fakeBridge{})
	port(h).SetOnline(true)
	port(h).SyncDevices([]bridge.Device{bulb, remote})
	old, gone := idOf(t, h, "0xbulb"), idOf(t, h, "0xremote")
	fresh := bulb
	fresh.NativeAddress = "0xnew"
	port(h).SyncDevices([]bridge.Device{fresh}) // the bulb and the remote leave; a new bulb joins
	replacing := idOf(t, h, "0xnew")
	lights, err := h.CreateAggregate("Lights", []Target{TargetDevice(old, "light")}, Any, "")
	if err != nil {
		t.Fatal(err)
	}
	away, err := h.CreateFlag("Away")
	if err != nil {
		t.Fatal(err)
	}
	living, err := h.CreateArea("Living room")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	_, cancel := h.Follow(func(u Update) {
		switch u.Kind {
		case TargetDeleted:
			got = append(got, "deleted "+u.Target.Key())
		case DeviceReplaced:
			got = append(got, "replaced "+u.Target.Key()+" with "+string(u.Replaced))
		}
	})
	defer cancel()

	full.Store(true)
	for name, err := range map[string]error{
		"Replace":         h.Replace(old, replacing),
		"Delete":          h.Delete(gone),
		"DeleteAggregate": h.DeleteAggregate(lights),
		"DeleteFlag":      h.DeleteFlag(away),
		"DeleteArea":      h.DeleteArea(living),
	} {
		if err == nil {
			t.Errorf("%s: no error, want the save's", name)
		}
	}
	want := []string{
		"replaced " + TargetDevice(old, "").Key() + " with " + string(replacing),
		"deleted " + TargetDevice(gone, "").Key(),
		"deleted " + TargetAggregate(lights).Key(),
		"deleted " + TargetFlag(away).Key(),
	}
	for _, a := range areaAggregates(living) {
		want = append(want, "deleted "+a.Key())
	}
	if len(got) != len(want) {
		t.Fatalf("announced %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("announced %q, want %q", got, want)
			break
		}
	}
}
