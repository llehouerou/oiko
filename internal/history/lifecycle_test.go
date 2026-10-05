package history

import (
	"testing"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/home"
)

// targets lists the targets having a series.
func targets(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	rows, err := s.db.Query(`SELECT DISTINCT target FROM series`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	all := map[string]bool{}
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			t.Fatal(err)
		}
		all[target] = true
	}
	return all
}

// deleted and replaced are the Updates Home announces as it deletes a Target
// or replaces a Device.
func deleted(t home.Target) home.Update { return home.Update{Kind: home.TargetDeleted, Target: t} }

func replaced(kept, fresh home.DeviceID) home.Update {
	return home.Update{Kind: home.DeviceReplaced, Target: home.TargetDevice(kept, ""), Replaced: fresh}
}

func values(ps []stored) []any {
	vs := make([]any, len(ps))
	for i, p := range ps {
		vs[i] = p.value
	}
	return vs
}

func TestAReplaceCarriesTheNewHardwaresHistoryOver(t *testing.T) {
	s := open(t)
	at := time.Now()
	old, fresh := home.TargetDevice("old", "light"), home.TargetDevice("new", "light")
	s.Follow(value(home.ValueChanged, old.Ref("brightness"), 10.0, at))
	s.Follow(value(home.ValueChanged, old.Ref("brightness"), 20.0, at.Add(time.Second)))
	written(s)
	s.Follow(value(home.ValueChanged, fresh.Ref("brightness"), 30.0, at.Add(2*time.Second)))
	s.Follow(value(home.ValueChanged, fresh.Ref("color_temp"), 300.0, at.Add(2*time.Second)))
	s.Follow(value(home.ValueChanged, home.TargetDevice("new", "").Ref("linkquality"), 5.0, at.Add(2*time.Second)))
	written(s)
	s.Follow(value(home.ValueChanged, fresh.Ref("brightness"), 40.0, at.Add(3*time.Second))) // just before the Replace
	s.Follow(replaced("old", "new"))
	written(s)

	if got := values(points(t, s, old, "brightness")); len(got) != 4 || got[0] != 10.0 || got[1] != 20.0 || got[2] != 30.0 || got[3] != 40.0 {
		t.Errorf("brightness = %v, want both pasts in time order", got)
	}
	if got := values(points(t, s, old, "color_temp")); len(got) != 1 || got[0] != 300.0 {
		t.Errorf("color_temp = %v, want the new hardware's under the kept identity", got)
	}
	if got := values(points(t, s, home.TargetDevice("old", ""), "linkquality")); len(got) != 1 {
		t.Errorf("linkquality = %v, want the new hardware's under the kept identity", got)
	}
	for target := range targets(t, s) {
		if target == fresh.Key() || target == "device:new" {
			t.Errorf("series left for %s", target)
		}
	}

	// The writer's last points follow the merge.
	s.Follow(value(home.ValueRefreshed, old.Ref("brightness"), 40.0, at.Add(4*time.Second)))
	s.Follow(value(home.ValueRefreshed, old.Ref("color_temp"), 300.0, at.Add(4*time.Second)))
	written(s)
	if n, m := len(points(t, s, old, "brightness")), len(points(t, s, old, "color_temp")); n != 4 || m != 1 {
		t.Errorf("%d brightness and %d color_temp points, want the refreshes not recorded", n, m)
	}
}

func TestAReplaceThroughHomeRecordsNoPointTwice(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.SyncDevices([]bridge.Device{plug("0xold")})
	z.Report("0xold", plugReport(1), time.Now())
	z.SyncDevices([]bridge.Device{plug("0xnew")}) // the old plug detached, the new one paired
	z.Report("0xnew", plugReport(2), time.Now())
	z.Report("0xnew", plugReport(2), time.Now()) // a refresh moves the Values' time on
	snap, cancel := h.Follow(func(home.Update) {})
	cancel()
	ids := map[string]home.DeviceID{}
	for _, d := range snap.Devices {
		ids[d.NativeAddress] = d.ID
	}
	if err := h.Replace(ids["0xold"], ids["0xnew"]); err != nil {
		t.Fatal(err)
	}
	stop()
	if got := values(points(t, s, home.TargetDevice(ids["0xold"], "switch"), "power")); len(got) != 2 || got[0] != 1.0 || got[1] != 2.0 {
		t.Errorf("power = %v, want 1 then 2", got)
	}
}

func TestDeletingATargetRemovesItsHistoryOnly(t *testing.T) {
	s := open(t)
	at := time.Now()
	gone := []home.Ref{lamp.Ref("brightness"), home.TargetDevice("lamp", "").Ref("linkquality"), home.TargetDevice("lamp", "").Ref(""),
		home.TargetAggregate("lights").Ref("state"), home.TargetFlag("away").Ref("on")}
	kept := []home.Ref{home.TargetDevice("lamp2", "light").Ref("brightness"), home.TargetDevice("lamp2", "").Ref("linkquality"),
		home.TargetAggregate("lights2").Ref("state"), home.TargetFlag("away2").Ref("on")}
	for _, ref := range append(gone, kept...) {
		s.Follow(value(home.ValueChanged, ref, 1.0, at))
	}
	written(s)
	s.Follow(deleted(home.TargetDevice("lamp", "")))
	s.Follow(deleted(home.TargetAggregate("lights")))
	s.Follow(deleted(home.TargetFlag("away")))
	written(s)

	for _, ref := range gone {
		if ps := points(t, s, ref.Target, ref.Capability); len(ps) != 0 {
			t.Errorf("%s %q: points = %+v, want none", ref.Target, ref.Capability, ps)
		}
	}
	for _, ref := range kept {
		if ps := points(t, s, ref.Target, ref.Capability); len(ps) != 1 {
			t.Errorf("%s %q: points = %+v, want untouched", ref.Target, ref.Capability, ps)
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM series`).Scan(&n); err != nil || n != len(kept) {
		t.Errorf("%d series, %v; want %d", n, err, len(kept))
	}

	// The writer's last points follow: a deleted series starts anew.
	s.Follow(value(home.ValueRefreshed, lamp.Ref("brightness"), 1.0, at))
	written(s)
	if ps := points(t, s, lamp, "brightness"); len(ps) != 1 {
		t.Errorf("points = %+v, want a new series", ps)
	}
}

func TestALifecycleChangeIsNeverDroppedAndComesAfterThePointsBefore(t *testing.T) {
	s := open(t)
	at := time.Now()
	fresh := home.TargetDevice("new", "light")
	s.Follow(value(home.ValueChanged, lamp.Ref("brightness"), 1.0, at))  // queued before
	s.Follow(value(home.ValueChanged, fresh.Ref("brightness"), 2.0, at)) // queued before
	fill(s)
	s.Follow(deleted(home.TargetDevice("lamp", "")))
	s.Follow(replaced("old", "new"))
	if n := s.Lost(); n != 0 {
		t.Errorf("lost = %d, want nothing dropped", n)
	}
	written(s)
	if ps := points(t, s, lamp, "brightness"); len(ps) != 0 {
		t.Errorf("points = %+v, want the Delete applied after the queued point", ps)
	}
	if got := values(points(t, s, home.TargetDevice("old", "light"), "brightness")); len(got) != 1 || got[0] != 2.0 {
		t.Errorf("brightness = %v, want the new hardware's under the kept identity", got)
	}
}

func TestADeleteThroughHomeForgetsTheHistory(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.SyncDevices([]bridge.Device{plug("0xplug")})
	device := firstDevice(h)
	z.Report("0xplug", plugReport(1), time.Now())
	z.SyncDevices(nil) // detached
	if err := h.Delete(device); err != nil {
		t.Fatal(err)
	}
	stop()
	if ps := points(t, s, home.TargetDevice(device, "switch"), "power"); len(ps) != 0 {
		t.Errorf("power = %+v, want forgotten", ps)
	}
}

func TestALifecycleChangeInAFailedBatchIsRetriedAlone(t *testing.T) {
	s := open(t)
	s.Follow(value(home.ValueChanged, lamp.Ref("brightness"), 1.0, time.Now()))
	written(s)
	// Inserting "boom" rolls the whole transaction back, so the batch's
	// commit fails.
	if _, err := s.db.Exec(`CREATE TRIGGER boom BEFORE INSERT ON points WHEN NEW.value = 'boom'
		BEGIN SELECT RAISE(ROLLBACK, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	s.Follow(deleted(home.TargetDevice("lamp", "")))
	s.Follow(value(home.ValueChanged, home.TargetDevice("other", "light").Ref("effect"), "boom", time.Now()))
	written(s)
	if ps := points(t, s, lamp, "brightness"); len(ps) != 0 {
		t.Errorf("points = %+v, want the Delete applied", ps)
	}
	if n := s.Lost(); n != 1 {
		t.Errorf("lost = %d, want only the point", n)
	}
}

func TestDetachingOrRenamingKeepsTheHistory(t *testing.T) {
	s := open(t)
	h, stop := following(t, s)
	z := h.Attach("zigbee2mqtt", nopBridge{})
	z.SetOnline(true)
	z.SyncDevices([]bridge.Device{plug("0xplug")})
	device := firstDevice(h)
	z.Report("0xplug", plugReport(1), time.Now())
	z.SyncDevices(nil) // detached
	if err := h.Rename(device, "Plug"); err != nil {
		t.Fatal(err)
	}
	stop()
	if ps := points(t, s, home.TargetDevice(device, "switch"), "power"); len(ps) != 1 {
		t.Errorf("power = %+v, want kept", ps)
	}
}
