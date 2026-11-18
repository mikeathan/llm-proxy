package netaudit

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/models"
)

type joinedRig struct {
	now    time.Time
	on     bool
	list   *models.MeshClients
	err    error
	reads  int
	marked [][]string
	at     []time.Time
	errs   []error
}

func (r *joinedRig) joined() RouterJoined {
	return NewRouterJoined(RouterJoined{
		Enabled: func() bool { return r.on },
		Read: func(context.Context) (*models.MeshClients, error) {
			r.reads++
			return r.list, r.err
		},
		Mark: func(macs []string, at time.Time) error {
			r.marked = append(r.marked, macs)
			r.at = append(r.at, at)
			return nil
		},
		OnError: func(err error) { r.errs = append(r.errs, err) },
		Now:     func() time.Time { return r.now },
	})
}

// step moves the rig's clock past the gap between reads.
func (r *joinedRig) step() { r.now = r.now.Add(routerJoinedGap) }

// The router's list marks each device it lists as joined at the time of the read, guests left out: a guest is a visitor, never one of the
// owner's devices (ADR-0059), so it never makes an inventory device look at home.
func TestRouterJoined_MarksTheListedDevicesAtTheReadsTime(t *testing.T) {
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	r := &joinedRig{on: true, list: &models.MeshClients{At: at, Clients: []models.MeshClient{
		{MAC: "aa:bb:cc:00:00:01", Band: "5G"}, {MAC: "aa:bb:cc:00:00:02", Band: "2G", Guest: true}, {MAC: "aa:bb:cc:00:00:03", Band: "2G"},
	}}}
	r.joined().Look(context.Background())
	if len(r.marked) != 1 || len(r.marked[0]) != 2 || r.marked[0][0] != "aa:bb:cc:00:00:01" || r.marked[0][1] != "aa:bb:cc:00:00:03" {
		t.Fatalf("marked = %v, want the two non-guest devices", r.marked)
	}
	if !r.at[0].Equal(at) {
		t.Errorf("marked at %v, want the read's time %v", r.at[0], at)
	}
}

// A look woken early does not read again within the gap, so presence signs in to the router at most once per gap however often it looks; a
// read that failed counts, so a failing router is not asked more often.
func TestRouterJoined_ReadsAtMostOncePerGap(t *testing.T) {
	r := &joinedRig{now: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC), on: true, err: errors.New("down")}
	j := r.joined()
	j.Look(context.Background())
	r.now = r.now.Add(time.Minute)
	j.Look(context.Background())
	if r.reads != 1 {
		t.Fatalf("reads = %d within the gap, want 1", r.reads)
	}
	r.now = r.now.Add(routerJoinedGap)
	j.Look(context.Background())
	if r.reads != 2 {
		t.Errorf("reads = %d after the gap, want 2", r.reads)
	}
}

// Switched off, the router is not read at all.
func TestRouterJoined_OffReadsNothing(t *testing.T) {
	r := &joinedRig{on: false}
	r.joined().Look(context.Background())
	if r.reads != 0 || len(r.marked) != 0 {
		t.Errorf("reads = %d, marked = %v; want nothing", r.reads, r.marked)
	}
}

// A router that is not set up, or was just read with nothing to answer, is not trouble: nothing is marked and nothing is reported.
func TestRouterJoined_ASetupNotFinishedOrAReadTooSoonIsQuiet(t *testing.T) {
	for _, err := range []error{routerconn.ErrNotSet, routerconn.ErrMeshWaiting} {
		r := &joinedRig{on: true, err: err}
		r.joined().Look(context.Background())
		if len(r.marked) != 0 || len(r.errs) != 0 {
			t.Errorf("%v: marked = %v, errs = %v; want neither", err, r.marked, r.errs)
		}
	}
}

// A real failure is reported once until it changes, and marks nothing: a failed read is never "nobody is joined".
func TestRouterJoined_AFailedReadIsReportedOnceAndMarksNothing(t *testing.T) {
	r := &joinedRig{on: true, err: errors.New("connection refused")}
	j := r.joined()
	j.Look(context.Background())
	r.step()
	j.Look(context.Background())
	if len(r.marked) != 0 || len(r.errs) != 1 {
		t.Errorf("marked = %v, errs = %v; want nothing marked and one report", r.marked, r.errs)
	}
	r.err, r.list = nil, &models.MeshClients{At: time.Now()}
	r.step()
	j.Look(context.Background())
	r.err = errors.New("connection refused")
	r.step()
	j.Look(context.Background())
	if len(r.errs) != 2 {
		t.Errorf("errs = %v; a failure after a good read is reported again", r.errs)
	}
}

// An inventory marks only the devices it already knows: the router's list never adds a device (new devices come from the neighbour table and
// keep their alert), and never changes when a device was last seen at its address, which decides who holds an address.
func TestInventory_MarkJoinedTouchesOnlyKnownDevicesAndNotTheirAddressTime(t *testing.T) {
	seen := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	st := &memInventory{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.20", LastSeen: seen},
		{MAC: "aa:bb:cc:00:00:09", IP: "192.168.1.21", LastSeen: seen},
	}}}
	v := NewInventory(st, nil, nil)
	at := seen.Add(3 * time.Hour)
	if err := v.MarkJoined([]string{"AA:BB:CC:00:00:01", "aa:bb:cc:00:00:77"}, at); err != nil {
		t.Fatal(err)
	}
	got := st.inv.Devices
	if len(got) != 2 || !got[0].RouterSeen.Equal(at) || !got[0].LastSeen.Equal(seen) || !got[1].RouterSeen.IsZero() {
		t.Errorf("devices = %+v", got)
	}
	writes := st.writes
	if err := v.MarkJoined([]string{"aa:bb:cc:00:00:77"}, at); err != nil || st.writes != writes {
		t.Errorf("a list with no known device must not write (writes %d -> %d, err %v)", writes, st.writes, err)
	}
}

// The router's list makes a dozing device online; it never makes one away, and the check's own freshness still decides whether anything is said.
func TestPresenceOf_TheRoutersListMakesADeviceOnlineButNeverAway(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fresh := PresenceStatus{LastLook: now.Add(-5 * time.Minute)}
	dozing := models.NetworkDevice{LastSeen: now.Add(-2 * time.Hour), RouterSeen: now.Add(-time.Minute)}
	if got := PresenceOf(dozing, true, fresh, now); got != models.PresenceOnline {
		t.Errorf("a device the router listed a minute ago: got %q, want online", got)
	}
	answering := models.NetworkDevice{LastSeen: now.Add(-time.Minute), RouterSeen: now.Add(-5 * time.Hour)}
	if got := PresenceOf(answering, true, fresh, now); got != models.PresenceOnline {
		t.Errorf("an old router mark must not make an answering device away: got %q", got)
	}
	if got := PresenceOf(dozing, false, fresh, now); got != models.PresenceUnknown {
		t.Errorf("with the check off the router's list says nothing: got %q", got)
	}
}

// The screen gets the router's mark, so it can say the device is online too.
func TestViewOf_CarriesTheRoutersMark(t *testing.T) {
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	v := ViewOf(models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:01", RouterSeen: at}}}, at, nil)
	if len(v.Devices) != 1 || !v.Devices[0].RouterSeen.Equal(at) {
		t.Errorf("view = %+v", v.Devices)
	}
}

// A named device the router keeps listing is not reported missing, though the sweep has not heard it for days.
func TestInventoryFindings_ADeviceTheRouterListsIsNotMissing(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	d := models.NetworkDevice{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.20", Name: "Phone", Approved: true,
		FirstSeen: now.Add(-60 * 24 * time.Hour), LastSeen: now.Add(-20 * 24 * time.Hour), RouterSeen: now.Add(-time.Hour)}
	for _, f := range InventoryFindings(models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{d}}, now) {
		if f.Check == CheckDeviceMissing {
			t.Errorf("a device the router listed an hour ago is reported missing: %+v", f)
		}
	}
}
