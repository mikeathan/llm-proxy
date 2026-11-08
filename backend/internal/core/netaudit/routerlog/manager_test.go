package routerlog

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type rig struct {
	m      *Manager
	wakes  atomic.Int32
	known  map[string]bool
	named  map[string]bool
	offers [][2]string
	gw     atomic.Value // netip.Addr
	mu     sync.Mutex
	errs   []error
	conn   *net.UDPConn
	enable atomic.Bool
	// drops keeps the first maxKeptDrops drops and nDrops counts them all: the soak test sends tens of thousands of firewall
	// lines, and a rig that kept each one would be the heap growth the test looks for.
	drops  []Drop
	nDrops atomic.Int64
	// queries keeps the first lookups; nQueries counts them all.
	queries  []Query
	nQueries atomic.Int64
}

const maxKeptDrops = 100

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{known: map[string]bool{"aa:bb:cc:dd:ee:09": true, "aa:bb:cc:dd:ee:08": true}, named: map[string]bool{"aa:bb:cc:dd:ee:08": true}}
	r.gw.Store(netip.MustParseAddr("127.0.0.1"))
	r.enable.Store(true)
	r.m = New(Deps{
		Enabled: r.enable.Load,
		Gateway: func() netip.Addr { return r.gw.Load().(netip.Addr) },
		Device: func(mac string) (bool, bool, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.known[mac], r.named[mac], nil
		},
		Suggest: func(mac, name string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.offers = append(r.offers, [2]string{mac, name})
			return nil
		},
		Drop: func(d Drop, _ time.Time) {
			r.nDrops.Add(1)
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.drops) < maxKeptDrops {
				r.drops = append(r.drops, d)
			}
		},
		Query: func(q Query, _ time.Time) {
			r.nQueries.Add(1)
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.queries) < maxKeptDrops {
				r.queries = append(r.queries, q)
			}
		},
		Wake:    func() { r.wakes.Add(1) },
		OnError: func(err error) { r.mu.Lock(); r.errs = append(r.errs, err); r.mu.Unlock() },
		Listen:  func(string) (*net.UDPConn, error) { return listenUDP4("127.0.0.1:0") },
	})
	r.m.wakeGap = time.Hour // one wake is all a test needs unless it says otherwise
	return r
}

// start runs the manager until the test ends and returns the address it listens on.
func (r *rig) start(t *testing.T) *net.UDPAddr {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.m.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a := r.m.Addr(); a != nil {
			return a
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the listener did not start")
	return nil
}

func send(t *testing.T, to *net.UDPAddr, line string) {
	t.Helper()
	c, err := net.DialUDP("udp4", nil, to)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
}

// settle waits for the manager to have read what was sent: it has no output to wait on, so it waits for the counter.
func (r *rig) waitHeard(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !r.m.Status().LastHeard.IsZero() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nothing was heard")
}

func (r *rig) quiet() { time.Sleep(60 * time.Millisecond) }

// drained waits until the drop counter has stopped moving for 100 ms: the reader has caught up with what was sent.
func (r *rig) drained(t *testing.T) {
	t.Helper()
	last, since := r.nDrops.Load(), time.Now()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		if n := r.nDrops.Load(); n != last {
			last, since = n, time.Now()
		} else if time.Since(since) >= 100*time.Millisecond && last > 0 {
			return
		}
	}
	t.Fatal("the reader never caught up with the flood")
}

func TestManager_AnUnknownDeviceFromTheRouterWakesTheCheckOnce(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, ackLine)
	r.waitHeard(t)
	r.quiet()
	if got := r.wakes.Load(); got != 1 {
		t.Errorf("wakes = %d, want 1", got)
	}
}

func TestManager_ADeviceTheInventoryKnowsDoesNothing(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, "<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:09 IEEE 802.11: associated")
	r.waitHeard(t)
	r.quiet()
	if got := r.wakes.Load(); got != 0 {
		t.Errorf("a known phone roaming must not cause a sweep, wakes = %d", got)
	}
}

func TestManager_OnlyTheRoutersAddressIsHeard(t *testing.T) {
	r := newRig(t)
	r.gw.Store(netip.MustParseAddr("192.0.2.1")) // not the sender
	addr := r.start(t)
	send(t, addr, ackLine)
	r.quiet()
	if r.wakes.Load() != 0 || !r.m.Status().LastHeard.IsZero() {
		t.Error("a datagram from another address is dropped before it is looked at")
	}
	r.gw.Store(netip.Addr{}) // no gateway known
	send(t, addr, ackLine)
	r.quiet()
	if r.wakes.Load() != 0 {
		t.Error("with no gateway known nothing is trusted")
	}
}

func TestManager_AFloodOfNewDevicesWakesAtMostOncePerGap(t *testing.T) {
	r := newRig(t)
	r.m.wakeGap = 200 * time.Millisecond
	addr := r.start(t)
	for i := 0; i < 200; i++ {
		send(t, addr, "<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:"+hex2(i)+" IEEE 802.11: associated")
	}
	time.Sleep(150 * time.Millisecond)
	if got := r.wakes.Load(); got != 1 {
		t.Errorf("wakes = %d inside one gap, want 1", got)
	}
}

func hex2(i int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[(i>>4)&15], digits[i&15]})
}

func TestManager_Garbage_AndOversizeDatagramsAreIgnored(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, "not syslog at all")
	big := make([]byte, 3000)
	for i := range big {
		big[i] = 'A'
	}
	send(t, addr, string(big))
	r.waitHeard(t)
	r.quiet()
	if r.wakes.Load() != 0 {
		t.Error("only the two recognised messages can wake the check")
	}
}

func TestManager_SwitchingOffClosesTheSocket(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	r.enable.Store(false)
	r.m.Poke()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.m.Addr() != nil {
		time.Sleep(5 * time.Millisecond)
	}
	if r.m.Addr() != nil || r.m.Status().Listening {
		t.Fatal("still listening after the switch went off")
	}
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("x"))
	c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Error("expected a refusal or silence from a closed port")
	}
}

func TestManager_APortThatCannotBeBoundIsAStatedProblemNotARetry(t *testing.T) {
	taken, err := listenUDP4("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	r := newRig(t)
	r.m.deps.Listen = func(string) (*net.UDPConn, error) { return listenUDP4(taken.LocalAddr().String()) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.m.Reconcile(ctx)
	if st := r.m.Status(); st.Listening || st.Problem == "" {
		t.Errorf("status = %+v, want a stated problem", st)
	}
}

func TestPort_IsNotAnotherPartOfTheProgramsPort(t *testing.T) {
	for _, taken := range []int{4001, 2222, 4445, 8023, 22, 514} {
		if Port == taken {
			t.Errorf("Port %d collides with a port used elsewhere", Port)
		}
	}
}

func TestManager_OnlyTenDatagramsASecondAreLookedAt(t *testing.T) {
	r := newRig(t)
	r.m.wakeGap = 0 // so only the allowance limits the wakes
	addr := r.start(t)
	for i := 0; i < 300; i++ {
		send(t, addr, "<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:"+hex2(i)+" IEEE 802.11: associated")
	}
	time.Sleep(200 * time.Millisecond)
	// The run may straddle one change of second, so up to two allowances.
	if got := r.wakes.Load(); got == 0 || got > 2*perSecond {
		t.Errorf("wakes = %d, want between 1 and %d", got, 2*perSecond)
	}
}

func (r *rig) offered() [][2]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][2]string(nil), r.offers...)
}

func lease(mac, name string) string {
	return "<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[8548]: DHCPACK(br0) 192.168.50.127 " + mac + " " + name
}

func TestManager_ANameIsOfferedForANewOrUnnamedDeviceAndNeverForANamedOne(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))    // not in the list
	send(t, addr, lease("aa:bb:cc:dd:ee:09", "KNOWN-TV"))     // in the list, no name
	send(t, addr, lease("aa:bb:cc:dd:ee:08", "NAMED-LAPTOP")) // in the list and named by the operator
	r.waitHeard(t)
	r.quiet()
	got := r.offered()
	want := [][2]string{{"aa:bb:cc:dd:ee:01", "NEW-PHONE"}, {"aa:bb:cc:dd:ee:09", "KNOWN-TV"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("offered %v, want %v", got, want)
	}
}

func TestManager_FirewallLinesNeverUseUpTheAllowanceSoALeaseStillGetsThrough(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	drop := "<12>Oct 31 20:02:33 ROUTER kernel: DROP IN=eth0 OUT= SRC=203.0.113.9 DST=192.168.0.137 PROTO=TCP SPT=40408 DPT=58376 SYN"
	for i := 0; i < 500; i++ {
		send(t, addr, drop)
	}
	// UDP has no delivery promise: on a loaded machine the reader falls behind and the kernel drops what overflows its
	// buffer, which could be the lease itself. Let the reader drain the flood before the lease is sent; what is asserted
	// (the flood does not use up the lease's allowance) is unchanged.
	r.drained(t)
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.wakes.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.wakes.Load() != 1 {
		t.Errorf("wakes = %d: a lease after a flood of drops must still wake the check", r.wakes.Load())
	}
}

func TestManager_AnyLineFromTheRouterShowsItWasHeard(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, "<12>Oct 31 20:02:33 ROUTER kernel: DROP IN=eth0 SRC=203.0.113.9 PROTO=TCP")
	r.waitHeard(t)
}

const dnsQueryLine = "<30>Nov  5 12:54:33 ROUTER dnsmasq[31037]: query[A] ccp-lh.googleusercontent.com from 192.168.50.188"

func (r *rig) waitQueries(t *testing.T, n int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.nQueries.Load() < n {
		time.Sleep(5 * time.Millisecond)
	}
	if got := r.nQueries.Load(); got < n {
		t.Fatalf("queries = %d, want %d", got, n)
	}
}

func TestManager_ADNSQueryFromTheRouterIsHandedOnWithItsNameAndSender(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, dnsQueryLine)
	r.waitQueries(t, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if q := r.queries[0]; q.Name != "ccp-lh.googleusercontent.com" || q.From != netip.MustParseAddr("192.168.50.188") {
		t.Fatalf("query = %+v", q)
	}
}

func TestManager_ADNSQueryFromAnyoneButTheRouterIsIgnored(t *testing.T) {
	r := newRig(t)
	r.gw.Store(netip.MustParseAddr("192.0.2.1")) // the datagrams arrive from loopback, which is now not the router
	addr := r.start(t)
	send(t, addr, dnsQueryLine)
	r.quiet()
	if r.nQueries.Load() != 0 {
		t.Fatal("a query line from an address that is not the router must be dropped unread")
	}
}

func TestManager_NoQueryIsReadWhileNothingWantsThem(t *testing.T) {
	r := newRig(t)
	r.m.deps.Query = nil
	addr := r.start(t)
	send(t, addr, dnsQueryLine)
	r.quiet()
	if r.nQueries.Load() != 0 {
		t.Fatal("with no consumer the lines are not parsed")
	}
}

func TestManager_QueryLinesHaveTheirOwnLimitAndNeverUseUpTheAllowanceOfALease(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	for i := 0; i < 3*queriesPerSecond; i++ {
		send(t, addr, dnsQueryLine)
	}
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.wakes.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.wakes.Load() != 1 {
		t.Errorf("wakes = %d: a lease after a flood of query lines must still wake the check", r.wakes.Load())
	}
	r.quiet()
	if got := r.nQueries.Load(); got > int64(queriesPerSecond)+1 {
		t.Errorf("queries = %d, want at most %d in one second", got, queriesPerSecond)
	}
}

func TestManager_StatusCountsTheQueryLinesReadAndTheOnesTheLimitTurnedAway(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	for i := 0; i < queriesPerSecond+30; i++ {
		send(t, addr, dnsQueryLine)
	}
	r.quiet()
	st := r.m.Status()
	if st.QueriesRead == 0 || st.QueriesRead > int64(queriesPerSecond) {
		t.Fatalf("read = %d, want 1 to %d", st.QueriesRead, queriesPerSecond)
	}
	if st.QueriesRead+st.QueriesLimited < int64(queriesPerSecond) {
		t.Fatalf("read %d + limited %d: the lines that arrived must all be accounted for", st.QueriesRead, st.QueriesLimited)
	}
	if st.QueriesPeak < 1 || st.QueriesPeak > queriesPerSecond || st.QueriesSince.IsZero() {
		t.Fatalf("peak = %d since = %v", st.QueriesPeak, st.QueriesSince)
	}
}

func TestManager_QueryCountsAreZeroUntilALineArrivesAndNeverCountOtherLines(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	send(t, addr, "<12>Oct 31 20:02:33 ROUTER kernel: DROP IN=eth0 OUT= SRC=203.0.113.9 DST=192.168.0.137 PROTO=TCP SPT=40408 DPT=58376 SYN")
	r.quiet()
	if st := r.m.Status(); st.QueriesRead != 0 || st.QueriesLimited != 0 || st.QueriesPeak != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestManager_StatusSaysWhenTheLastDNSLineWasRead(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	if !r.m.Status().QueriesLast.IsZero() {
		t.Fatal("no line has been read yet")
	}
	send(t, addr, dnsQueryLine)
	r.waitQueries(t, 1)
	if r.m.Status().QueriesLast.IsZero() {
		t.Fatal("the time of the last DNS line must be kept: the phone-home watch is judged by it")
	}
}
