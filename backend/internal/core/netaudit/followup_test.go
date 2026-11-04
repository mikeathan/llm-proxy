package netaudit

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

type followUpRig struct {
	mu      sync.Mutex
	scans   []string
	sent    []string
	busy    bool
	enabled bool
	clock   *monitorClock
}

func (r *followUpRig) scan(_ context.Context, addr string) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy {
		return Result{}, ErrScanBusy
	}
	r.scans = append(r.scans, addr)
	a := netip.MustParseAddr(addr)
	return Result{Hosts: []Host{{Addr: a, Up: true, Ports: []Port{{Number: 23, Proto: "tcp", State: "open", Service: "telnet"}, {Number: 80, Proto: "tcp", State: "open"}}}}}, nil
}

func (r *followUpRig) notify(_ context.Context, m string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, m)
	return nil
}

func newFollowUpRig() (*FollowUp, *followUpRig) {
	rig := &followUpRig{enabled: true, clock: &monitorClock{at: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}}
	f := NewFollowUp(FollowUpDeps{
		Enabled: func() bool { rig.mu.Lock(); defer rig.mu.Unlock(); return rig.enabled },
		Scan:    rig.scan, Notify: rig.notify, Now: rig.clock.now,
	})
	return f, rig
}

func snapWith(now time.Time, devices ...models.NetworkDevice) Snapshot {
	for i := range devices {
		if devices[i].FirstSeen.IsZero() {
			devices[i].FirstSeen = now.Add(-time.Minute)
		}
		devices[i].LastSeen = now
	}
	return Snapshot{Inventory: models.DeviceInventory{Devices: devices}}
}

func TestFollowUp_ANewDeviceGetsOneScanAndOneAlertWithItsCounts(t *testing.T) {
	f, rig := newFollowUpRig()
	snap := snapWith(rig.clock.now(), models.NetworkDevice{MAC: "aa:bb:cc:00:00:01", IP: "10.0.0.5"})
	f.Offer(snap)
	f.Step(t.Context())
	f.Offer(snap) // the same device on the next look, within the day
	f.Step(t.Context())
	if len(rig.scans) != 1 || rig.scans[0] != "10.0.0.5" {
		t.Fatalf("scans = %v, want one of 10.0.0.5", rig.scans)
	}
	if len(rig.sent) != 1 || !strings.Contains(rig.sent[0], "10.0.0.5") || !strings.Contains(rig.sent[0], "2 open ports") {
		t.Fatalf("sent = %q", rig.sent)
	}
}

func TestFollowUp_LeavesOutWhatIsNotANewDeviceOfItsOwn(t *testing.T) {
	f, rig := newFollowUpRig()
	now := rig.clock.now()
	f.Offer(snapWith(now,
		models.NetworkDevice{MAC: "aa:bb:cc:00:00:02", IP: "10.0.0.6", Approved: true},
		models.NetworkDevice{MAC: "aa:bb:cc:00:00:03", IP: "10.0.0.7", Name: "printer"},
		models.NetworkDevice{MAC: "da:bb:cc:00:00:04", IP: "10.0.0.8", Randomized: true},
		models.NetworkDevice{MAC: "aa:bb:cc:00:00:05", IP: "10.0.0.9", FirstSeen: now.Add(-48 * time.Hour)},
	))
	f.Step(t.Context())
	if len(rig.scans) != 0 {
		t.Fatalf("scans = %v: approved, named, private-address and old devices are not new arrivals", rig.scans)
	}
}

func TestFollowUp_ARunningScanDefersItAndOffMeansNothing(t *testing.T) {
	f, rig := newFollowUpRig()
	snap := snapWith(rig.clock.now(), models.NetworkDevice{MAC: "aa:bb:cc:00:00:01", IP: "10.0.0.5"})
	rig.busy = true
	f.Offer(snap)
	f.Step(t.Context())
	rig.busy = false
	f.Step(t.Context())
	if len(rig.scans) != 1 {
		t.Fatalf("scans = %v, want the deferred scan to run once the other ended", rig.scans)
	}
	g, rig2 := newFollowUpRig()
	rig2.enabled = false
	g.Offer(snap)
	g.Step(t.Context())
	if len(rig2.scans) != 0 {
		t.Fatal("a switched-off follow-up must scan nothing")
	}
}

func TestFollowUp_AtMostAFewScansAnHour(t *testing.T) {
	f, rig := newFollowUpRig()
	var devices []models.NetworkDevice
	for i := range 10 {
		devices = append(devices, models.NetworkDevice{MAC: "aa:bb:cc:00:01:0" + string(rune('0'+i)), IP: "10.0.1." + string(rune('1'+i))})
	}
	f.Offer(snapWith(rig.clock.now(), devices...))
	for range 10 {
		f.Step(t.Context())
	}
	if len(rig.scans) != followUpPerHour {
		t.Fatalf("scans = %d, want %d in one hour", len(rig.scans), followUpPerHour)
	}
}

func TestFollowUp_RunStopsWithItsContext(t *testing.T) {
	f, _ := newFollowUpRig()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { f.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}
