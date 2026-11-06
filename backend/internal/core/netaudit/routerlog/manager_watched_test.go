package routerlog

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

type watchedLog struct {
	mu  sync.Mutex
	got []Watched
}

func (w *watchedLog) add(x Watched, _ time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.got = append(w.got, x)
}

func (w *watchedLog) wait(t *testing.T, n int) []Watched {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		if len(w.got) >= n {
			defer w.mu.Unlock()
			return append([]Watched(nil), w.got...)
		}
		w.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("only %d of %d lines were counted", len(w.got), n)
	return nil
}

func (w *watchedLog) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.got)
}

func TestManager_AFailedSignInAndADisconnectFromTheRouterAreHandedOnAsKindAndNumberOnly(t *testing.T) {
	r := newRig(t)
	w := &watchedLog{}
	r.m.deps.Watch = w.add
	addr := r.start(t)
	send(t, addr, wireFailed)
	send(t, addr, wireDeauth)
	got := w.wait(t, 2)
	if got[0] != (Watched{Kind: SignInFailed}) || got[1] != (Watched{Kind: Deauth, Reason: 3}) {
		t.Errorf("got %+v", got)
	}
	if r.wakes.Load() != 0 || len(r.offers) != 0 {
		t.Error("a watched line never wakes the check or offers a name")
	}
}

func TestManager_AWatchedLineFromAnyOtherAddressIsIgnored(t *testing.T) {
	r := newRig(t)
	r.gw.Store(netip.MustParseAddr("192.0.2.1"))
	w := &watchedLog{}
	r.m.deps.Watch = w.add
	addr := r.start(t)
	send(t, addr, wireFailed)
	r.quiet()
	if w.count() != 0 {
		t.Error("a forged line from another address was counted")
	}
}

func TestManager_WatchedLinesHaveTheirOwnLimitAndDoNotUseTheDeviceAllowance(t *testing.T) {
	r := newRig(t)
	w := &watchedLog{}
	r.m.deps.Watch = w.add
	addr := r.start(t)
	for i := 0; i < watchedPerSecond*3; i++ {
		send(t, addr, wireDeauth)
	}
	r.quiet()
	if n := w.count(); n > watchedPerSecond*2 { // the flood may span two seconds, never more
		t.Errorf("%d lines read in one burst", n)
	}
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.wakes.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.wakes.Load() != 1 {
		t.Errorf("wakes = %d: a lease after a flood of disconnects must still wake the check", r.wakes.Load())
	}
}

func TestManager_WithoutAWatchedLinesAreIgnored(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, wireFailed)
	r.waitHeard(t)
}
