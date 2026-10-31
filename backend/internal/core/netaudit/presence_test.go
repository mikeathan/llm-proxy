package netaudit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeInventory struct {
	refreshErr error
	pending    Pending
	marked     []Pending
	refreshes  int
}

func (f *fakeInventory) Refresh(context.Context) (Snapshot, error) {
	f.refreshes++
	return Snapshot{}, f.refreshErr
}
func (f *fakeInventory) Pending() (Pending, error) { return f.pending, nil }
func (f *fakeInventory) MarkNotified(p Pending) error {
	f.marked = append(f.marked, p)
	return nil
}

type presenceRig struct {
	inv      *fakeInventory
	sweeps   [][]string
	sweepErr error
	sent     []string
	sendErr  error
	errs     []error
	settings PresenceSettings
	onLook   func(context.Context, Snapshot)
}

func (r *presenceRig) presence() *Presence {
	return NewPresence(PresenceDeps{
		Settings: func() PresenceSettings { return r.settings },
		Sweep: func(_ context.Context, targets []string) error {
			r.sweeps = append(r.sweeps, targets)
			return r.sweepErr
		},
		Inventory: r.inv,
		Notify:    func(_ context.Context, msg string) error { r.sent = append(r.sent, msg); return r.sendErr },
		OnError:   func(err error) { r.errs = append(r.errs, err) },
		OnLook:    r.onLook,
	})
}

func rig() *presenceRig {
	return &presenceRig{inv: &fakeInventory{}, settings: PresenceSettings{Enabled: true, Every: 10 * time.Minute, Targets: []string{"192.168.1.0/24"}}}
}

func TestPresence_OffByDefaultDoesNothing(t *testing.T) {
	r := rig()
	r.settings.Enabled = false
	r.presence().Tick(context.Background())
	if len(r.sweeps) != 0 || r.inv.refreshes != 0 {
		t.Errorf("a disabled check must not touch the network: %v", r.sweeps)
	}
}

func TestPresence_SweepsTheDeclaredScopeThenLooksAtTheTable(t *testing.T) {
	r := rig()
	r.presence().Tick(context.Background())
	if len(r.sweeps) != 1 || r.sweeps[0][0] != "192.168.1.0/24" || r.inv.refreshes != 1 {
		t.Errorf("sweeps=%v refreshes=%d", r.sweeps, r.inv.refreshes)
	}
	if len(r.sent) != 0 {
		t.Errorf("nothing new, nothing sent: %v", r.sent)
	}
}

func TestPresence_ANewDeviceIsToldOnceInCountsAndFixedWords(t *testing.T) {
	r := rig()
	r.inv.pending = Pending{NewDevices: 2, Changes: 1, Gateway: true, macs: []string{"aa:bb:cc:00:00:01"}}
	p := r.presence()
	p.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent %v", r.sent)
	}
	for _, want := range []string{"2 new devices", "1 address", "router"} {
		if !strings.Contains(r.sent[0], want) {
			t.Errorf("message %q lacks %q", r.sent[0], want)
		}
	}
	if strings.Contains(r.sent[0], "aa:bb") {
		t.Errorf("a message carries counts, not what devices are called: %q", r.sent[0])
	}
	if len(r.inv.marked) != 1 || r.inv.marked[0].NewDevices != 2 {
		t.Errorf("marked = %+v", r.inv.marked)
	}
}

func TestPresence_AFailedSendIsNotMarkedSoItIsTriedAgain(t *testing.T) {
	r := rig()
	r.inv.pending = Pending{NewDevices: 1}
	r.sendErr = errors.New("telegram down")
	r.presence().Tick(context.Background())
	if len(r.inv.marked) != 0 {
		t.Error("an alert that was not delivered must stay pending")
	}
	if len(r.errs) != 1 {
		t.Errorf("the trouble is reported: %v", r.errs)
	}
}

func TestPresence_ABusyScannerSkipsTheCycleQuietly(t *testing.T) {
	r := rig()
	r.sweepErr = ErrScanBusy
	r.presence().Tick(context.Background())
	if r.inv.refreshes != 0 || len(r.errs) != 0 {
		t.Errorf("a scan is running: skip, no error. refreshes=%d errs=%v", r.inv.refreshes, r.errs)
	}
}

func TestPresence_TheSameTroubleIsReportedOnceUntilItChanges(t *testing.T) {
	r := rig()
	r.inv.refreshErr = errors.New("no table")
	p := r.presence()
	p.Tick(context.Background())
	p.Tick(context.Background())
	if len(r.errs) != 1 {
		t.Errorf("a failure every 10 minutes must not fill the log: %v", r.errs)
	}
	r.inv.refreshErr = nil
	p.Tick(context.Background())
	r.inv.refreshErr = errors.New("no table")
	p.Tick(context.Background())
	if len(r.errs) != 2 {
		t.Errorf("a failure after a success is news again: %v", r.errs)
	}
}

func TestPresence_RunStopsWithItsContext(t *testing.T) {
	r := rig()
	r.settings.Every = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.presence().Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop when its context ended")
	}
}

func TestPresenceMessage_NamesWhereTheNewDevicesAre(t *testing.T) {
	p := Pending{NewDevices: 2, newAddrs: []string{"192.168.50.127 (private address)", "192.168.50.88"}, Changes: 1, movedAddrs: []string{"192.168.50.20"}}
	got := PresenceMessage(p)
	for _, want := range []string{"2 new devices: 192.168.50.127 (private address), 192.168.50.88", "1 address now used by a different device: 192.168.50.20"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q lacks %q", got, want)
		}
	}
}

func TestPresenceMessage_ABurstIsCountedNotListedInFull(t *testing.T) {
	var addrs []string
	for i := 1; i <= 9; i++ {
		addrs = append(addrs, "10.0.0."+strconv.Itoa(i))
	}
	got := PresenceMessage(Pending{NewDevices: 9, newAddrs: addrs})
	if !strings.Contains(got, "9 new devices") || !strings.Contains(got, "and 4 more") || strings.Contains(got, "10.0.0.9") {
		t.Errorf("message = %q", got)
	}
}

func TestPresenceMessage(t *testing.T) {
	if got := PresenceMessage(Pending{NewDevices: 1}); !strings.Contains(got, "1 new device") || strings.Contains(got, "devices") {
		t.Errorf("singular: %q", got)
	}
	if got := PresenceMessage(Pending{Changes: 3}); !strings.Contains(got, "3 addresses") {
		t.Errorf("plural: %q", got)
	}
}

func TestPresence_StatusSaysWhenItLastLookedAndWhatIsWrong(t *testing.T) {
	r := rig()
	p := r.presence()
	if s := p.Status(); !s.LastCheck.IsZero() || s.Problem != "" {
		t.Errorf("before any cycle: %+v", s)
	}
	r.inv.refreshErr = errors.New("this computer cannot list the devices on its network")
	p.Tick(context.Background())
	if s := p.Status(); s.Problem == "" || !s.LastCheck.IsZero() {
		t.Errorf("a failed cycle is not a check: %+v", s)
	}
	r.inv.refreshErr = nil
	p.Tick(context.Background())
	if s := p.Status(); s.Problem != "" || s.LastCheck.IsZero() {
		t.Errorf("a good cycle clears the trouble: %+v", s)
	}
}

func TestPresence_EachLookIsHandedToTheNamingStepAndAFailedLookIsNot(t *testing.T) {
	r := rig()
	var looks int
	r.onLook = func(context.Context, Snapshot) { looks++ }
	r.presence().Tick(context.Background())
	if looks != 1 {
		t.Fatalf("looks = %d, want 1", looks)
	}
	r.inv.refreshErr = errors.New("no table")
	r.presence().Tick(context.Background())
	if looks != 1 {
		t.Errorf("a look that failed must not be handed on: %d", looks)
	}
}

func TestPresence_TheAlertGoesOutBeforeTheNamingStepStarts(t *testing.T) {
	r := rig()
	r.inv.pending = Pending{NewDevices: 1}
	var order []string
	r.onLook = func(context.Context, Snapshot) { order = append(order, "look") }
	p := NewPresence(PresenceDeps{
		Settings:  func() PresenceSettings { return r.settings },
		Sweep:     func(context.Context, []string) error { return nil },
		Inventory: r.inv,
		Notify:    func(context.Context, string) error { order = append(order, "alert"); return nil },
		OnLook:    r.onLook,
	})
	p.Tick(context.Background())
	if len(order) != 2 || order[0] != "alert" || order[1] != "look" {
		t.Errorf("order = %v: a slow naming step must never hold back an alert", order)
	}
}

func TestPresence_TheNamingStepStillRunsWhenTheAlertCannotBeSent(t *testing.T) {
	r := rig()
	r.inv.pending = Pending{NewDevices: 1}
	r.sendErr = errors.New("no connector is set up to send alerts")
	var looks int
	r.onLook = func(context.Context, Snapshot) { looks++ }
	p := r.presence()
	p.Tick(context.Background())
	if looks != 1 {
		t.Errorf("looks = %d: with no connector the naming step is still due", looks)
	}
	if len(r.errs) != 1 || len(r.inv.marked) != 0 {
		t.Errorf("errors = %v, marked = %v: the alert must stay pending", r.errs, r.inv.marked)
	}
}
