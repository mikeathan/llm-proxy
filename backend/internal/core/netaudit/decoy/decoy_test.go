package decoy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

type memStore struct {
	mu    sync.Mutex
	items []models.DecoyTouch
}

func (m *memStore) Read() ([]models.DecoyTouch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]models.DecoyTouch(nil), m.items...), nil
}
func (m *memStore) Write(v []models.DecoyTouch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append([]models.DecoyTouch(nil), v...)
	return nil
}

type rig struct {
	fmu     sync.RWMutex
	store   *memStore
	enabled bool
	own     func(netip.Addr) bool
	scope   func(netip.Addr) bool
	mu      sync.Mutex
	sent    []string
	sendErr error
	errs    []error
	now     time.Time
	m       *Manager
}

func newRig(specs []Spec) *rig {
	r := &rig{store: &memStore{}, enabled: true, own: func(netip.Addr) bool { return false }, scope: func(netip.Addr) bool { return true }, now: time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)}
	r.m = New(Deps{
		Specs:   specs,
		Enabled: func() bool { return r.enabled },
		InScope: func(a netip.Addr) bool { r.fmu.RLock(); defer r.fmu.RUnlock(); return r.scope(a) },
		IsOwn:   func(a netip.Addr) bool { r.fmu.RLock(); defer r.fmu.RUnlock(); return r.own(a) },
		Store:   r.store,
		Notify: func(_ context.Context, msg string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.sent = append(r.sent, msg)
			return r.sendErr
		},
		OnError: func(err error) { r.mu.Lock(); r.errs = append(r.errs, err); r.mu.Unlock() },
		Now:     func() time.Time { return r.now },
	})
	return r
}

func (r *rig) setOwn(f func(netip.Addr) bool)   { r.fmu.Lock(); r.own = f; r.fmu.Unlock() }
func (r *rig) setScope(f func(netip.Addr) bool) { r.fmu.Lock(); r.scope = f; r.fmu.Unlock() }

func (r *rig) start(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r.m.Reconcile(ctx)
	return ctx, cancel
}

func touch(t *testing.T, addr string, payload string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if payload != "" {
		_, _ = c.Write([]byte(payload))
	}
	buf := make([]byte, 256)
	n, _ := c.Read(buf)
	return string(buf[:n])
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

var specs = []Spec{{Number: 0, Banner: "SSH-2.0-OpenSSH_9.2p1 Debian-2\r\n"}, {Number: 0}}

func TestManager_ATouchIsRecordedAsSourcePortAndTimeOnly(t *testing.T) {
	r := newRig(specs)
	r.start(t)
	addrs := r.m.Addrs()
	if len(addrs) != 2 {
		t.Fatalf("addrs = %v", addrs)
	}
	banner := touch(t, addrs[0], strings.Repeat("A", 4000))
	if !strings.HasPrefix(banner, "SSH-2.0-") {
		t.Errorf("the SSH decoy sends its fixed banner: %q", banner)
	}
	eventually(t, func() bool { items, _ := r.store.Read(); return len(items) == 1 })
	items, _ := r.store.Read()
	if items[0].Source != "127.0.0.1" || items[0].Count != 1 || items[0].First != r.now {
		t.Errorf("touch = %+v", items[0])
	}
	// Two more from the same source on the same decoy are one entry, counted.
	touch(t, addrs[0], "")
	time.Sleep(1100 * time.Millisecond) // the per-source limit allows one connection a second
	touch(t, addrs[0], "")
	eventually(t, func() bool { items, _ := r.store.Read(); return len(items) == 1 && items[0].Count >= 2 })
}

func TestManager_NothingIsKeptOfWhatWasSent(t *testing.T) {
	r := newRig(specs)
	r.start(t)
	touch(t, r.m.Addrs()[1], "secret-looking payload "+strings.Repeat("x", 100))
	eventually(t, func() bool { items, _ := r.store.Read(); return len(items) == 1 })
	items, _ := r.store.Read()
	// The record type has no field that could hold bytes; the only strings are the source address.
	if strings.Contains(items[0].Source, "secret") {
		t.Errorf("payload leaked into the record: %+v", items[0])
	}
}

func TestManager_TheHostItselfAndStrangersOutsideTheScopeAreIgnored(t *testing.T) {
	r := newRig(specs)
	r.setOwn(func(a netip.Addr) bool { return a == netip.MustParseAddr("127.0.0.1") })
	r.start(t)
	touch(t, r.m.Addrs()[1], "")
	time.Sleep(200 * time.Millisecond)
	if items, _ := r.store.Read(); len(items) != 0 {
		t.Errorf("our own audit and sweep must never count as an intruder: %+v", items)
	}
	r.setOwn(func(netip.Addr) bool { return false })
	r.setScope(func(netip.Addr) bool { return false })
	time.Sleep(1100 * time.Millisecond)
	touch(t, r.m.Addrs()[1], "")
	time.Sleep(200 * time.Millisecond)
	if items, _ := r.store.Read(); len(items) != 0 {
		t.Errorf("a source outside the declared networks is ignored: %+v", items)
	}
}

func TestManager_OffByDefaultListensOnNothingAndSwitchingOffStops(t *testing.T) {
	r := newRig(specs)
	r.enabled = false
	ctx, _ := r.start(t)
	if len(r.m.Addrs()) != 0 {
		t.Fatalf("disabled but listening: %v", r.m.Addrs())
	}
	r.enabled = true
	r.m.Reconcile(ctx)
	addrs := r.m.Addrs()
	if len(addrs) != 2 {
		t.Fatalf("addrs = %v", addrs)
	}
	r.enabled = false
	r.m.Reconcile(ctx)
	if len(r.m.Addrs()) != 0 {
		t.Error("switching off closes the listeners")
	}
	if c, err := net.DialTimeout("tcp", addrs[0], 300*time.Millisecond); err == nil {
		c.Close()
		t.Error("the port must be closed after switching off")
	}
}

func TestManager_TheOperatorIsToldOnceInCountsAndFixedWords(t *testing.T) {
	r := newRig(specs)
	r.start(t)
	touch(t, r.m.Addrs()[1], "")
	eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.sent) == 1 })
	r.mu.Lock()
	msg := r.sent[0]
	r.mu.Unlock()
	if !strings.Contains(msg, "1 device") || strings.Contains(msg, "127.0.0.1") {
		t.Errorf("counts, not addresses: %q", msg)
	}
	items, _ := r.store.Read()
	if !items[0].Notified {
		t.Error("marked as told after it was sent")
	}
}

func TestManager_AFailedAlertIsNotMarkedSoItIsTriedAgain(t *testing.T) {
	r := newRig(specs)
	r.sendErr = errors.New("telegram down")
	r.start(t)
	touch(t, r.m.Addrs()[1], "")
	eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.errs) == 1 })
	if items, _ := r.store.Read(); items[0].Notified {
		t.Error("an alert that did not go out must stay pending")
	}
}

func TestManager_AtMostSomeConnectionsAreServedAtOnce(t *testing.T) {
	r := newRig([]Spec{{Number: 0}})
	r.m.maxConns = 2
	r.start(t)
	addr := r.m.Addrs()[0]
	var held []net.Conn
	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	defer func() {
		for _, c := range held {
			c.Close()
		}
	}()
	time.Sleep(100 * time.Millisecond)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("beyond the limit a connection is closed at once")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Error("beyond the limit a connection must be closed, not held")
	}
}

func TestManager_APortInUseIsStatedAndTheRestStillListen(t *testing.T) {
	busy, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	r := newRig([]Spec{{Number: busy.Addr().(*net.TCPAddr).Port}, {Number: 0}})
	r.start(t)
	if len(r.m.Addrs()) != 1 {
		t.Errorf("the free port still listens: %v", r.m.Addrs())
	}
	if st := r.m.Status(); st.Problem == "" || !strings.Contains(st.Problem, "in use") {
		t.Errorf("status = %+v", st)
	}
}

func TestSpecs_ArePortsNothingLegitimateScansAndAreNotPrivileged(t *testing.T) {
	for _, s := range Specs {
		if s.Number < 1024 || s.Number > 65535 {
			t.Errorf("decoy %d must be an unprivileged port", s.Number)
		}
		if len(s.Banner) > 80 {
			t.Errorf("a banner is a fixed short string: %q", s.Banner)
		}
	}
}

// A connection still being served when the decoys are switched off and on again must give back a slot of the old set, not
// take one from the new set, or the new listeners would hang with every slot taken.
func TestManager_AConnectionHeldAcrossASwitchDoesNotBreakTheNextStart(t *testing.T) {
	r := newRig([]Spec{{Number: 0}})
	r.m.maxConns = 1
	ctx, _ := r.start(t)
	old, err := net.Dial("tcp", r.m.Addrs()[0])
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.enabled = false
	r.m.Reconcile(ctx)
	r.enabled = true
	r.m.Reconcile(ctx)
	old.Close() // the old connection ends after the new listeners exist
	time.Sleep(2500 * time.Millisecond)
	if got := touch(t, r.m.Addrs()[0], ""); got != "" {
		t.Logf("answer %q", got)
	}
	eventually(t, func() bool { items, _ := r.store.Read(); return len(items) >= 1 })
}

func TestManager_SwitchingOffClearsTheOldProblem(t *testing.T) {
	busy, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	r := newRig([]Spec{{Number: busy.Addr().(*net.TCPAddr).Port}})
	ctx, _ := r.start(t)
	if r.m.Status().Problem == "" {
		t.Fatal("the busy port is a problem")
	}
	r.enabled = false
	r.m.Reconcile(ctx)
	r.m.stop()
	if r.m.Status().Problem != "" {
		t.Error("a switched-off service has no problem")
	}
}
