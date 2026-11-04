package routerlog

import (
	"net/netip"
	"testing"
	"time"
)

func (r *rig) counted() []Drop {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Drop(nil), r.drops...)
}

func (r *rig) waitCounted(t *testing.T, n int) []Drop {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.counted(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("only %d of %d drops were counted", len(r.counted()), n)
	return nil
}

func TestManager_AFirewallDropFromTheRouterIsCountedByProtocolAndPortAndWakesNothing(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, wireDrop)
	got := r.waitCounted(t, 1)
	if len(got) != 1 || got[0] != (Drop{Proto: "tcp", Port: 3306}) {
		t.Errorf("drops = %+v", got)
	}
	r.quiet()
	if r.wakes.Load() != 0 || len(r.offers) != 0 {
		t.Error("a drop is a number and never wakes the check or offers a name")
	}
}

func TestManager_ADropFromAnyOtherAddressOrWithAPrivateSourceIsNotCounted(t *testing.T) {
	r := newRig(t)
	r.gw.Store(netip.MustParseAddr("192.0.2.1"))
	addr := r.start(t)
	send(t, addr, wireDrop)
	r.quiet()
	if got := r.counted(); len(got) != 0 {
		t.Errorf("a datagram from another address was counted: %+v", got)
	}
	r.gw.Store(netip.MustParseAddr("127.0.0.1"))
	send(t, addr, "<12>Nov  4 18:17:18 R kernel: DROP IN=br0 OUT= MAC=00 SRC=192.168.50.20 DST=192.168.50.1 PROTO=TCP SPT=1 DPT=22 SYN")
	r.quiet()
	if got := r.counted(); len(got) != 0 {
		t.Errorf("a packet from inside the house is not an attempt from the internet: %+v", got)
	}
}

func TestManager_DropsAreNotHeldToTheDeviceAllowanceAndALeaseAfterThemStillGetsThrough(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	for i := 0; i < 40; i++ {
		send(t, addr, wireDrop)
	}
	r.waitCounted(t, 40)
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.wakes.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.wakes.Load() != 1 {
		t.Errorf("wakes = %d", r.wakes.Load())
	}
}

func TestManager_WithoutACounterADropIsIgnoredAndStillShowsTheRouterWasHeard(t *testing.T) {
	r := newRig(t)
	r.m.deps.Drop = nil
	addr := r.start(t)
	send(t, addr, wireDrop)
	r.waitHeard(t)
}
