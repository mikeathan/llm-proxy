package portprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"llm-proxy/internal/core/netaudit"
)

// fakeDialer answers each address:port from a table: open, refused, or (absent) a timeout-like failure.
type fakeDialer struct {
	open    map[string]bool
	refused map[string]bool
	mu      sync.Mutex
	dialed  []string
	active  atomic.Int32
	peak    atomic.Int32
}

func (f *fakeDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	f.mu.Lock()
	f.dialed = append(f.dialed, address)
	f.mu.Unlock()
	switch {
	case f.open[address]:
		c1, c2 := net.Pipe()
		c2.Close()
		return c1, nil
	case f.refused[address]:
		return nil, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	}
	return nil, errors.New("i/o timeout")
}

func allow(netip.Addr) error { return nil }

func engine(d Dialer, guard Guard) *Engine { return &Engine{guard: guard, dialer: d} }

func addrs(ss ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

func TestScan_PortsFindsOpenPortsAndLabelsTheLimit(t *testing.T) {
	d := &fakeDialer{open: map[string]bool{"192.168.1.5:80": true, "192.168.1.5:22": true}, refused: map[string]bool{"192.168.1.6:80": true}}
	res, err := engine(d, allow).Scan(context.Background(), netaudit.ScanRequest{
		Targets: addrs("192.168.1.5", "192.168.1.6", "192.168.1.7"), Mode: netaudit.ModePorts, Ports: "22,80",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 3 {
		t.Fatalf("hosts = %+v", res.Hosts)
	}
	h5, h6, h7 := res.Hosts[0], res.Hosts[1], res.Hosts[2]
	if !h5.Up || len(h5.Ports) != 2 || h5.Ports[0].Number != 22 || h5.Ports[1].Number != 80 {
		t.Errorf("host .5 = %+v (ports must be sorted)", h5)
	}
	if !h6.Up || len(h6.Ports) != 0 {
		t.Errorf("a refusal proves the host is alive: %+v", h6)
	}
	if h7.Up {
		t.Errorf("a host that never answers is down: %+v", h7)
	}
	if len(res.Warnings) == 0 {
		t.Error("the limited engine must say what it cannot do")
	}
}

func TestScan_DiscoverListsNoPorts(t *testing.T) {
	d := &fakeDialer{open: map[string]bool{"10.0.0.2:443": true}}
	res, err := engine(d, allow).Scan(context.Background(), netaudit.ScanRequest{Targets: addrs("10.0.0.2"), Mode: netaudit.ModeDiscover})
	if err != nil || !res.Hosts[0].Up || len(res.Hosts[0].Ports) != 0 {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestScan_TheGuardIsTheSecondLineOfDefence(t *testing.T) {
	d := &fakeDialer{open: map[string]bool{"192.168.1.5:80": true, "8.8.8.8:80": true}}
	guard := func(a netip.Addr) error {
		if a.String() == "8.8.8.8" {
			return errors.New("public")
		}
		return nil
	}
	res, err := engine(d, guard).Scan(context.Background(), netaudit.ScanRequest{Targets: addrs("192.168.1.5", "8.8.8.8"), Mode: netaudit.ModePorts, Ports: "80"})
	if err != nil || len(res.Hosts) != 1 {
		t.Fatalf("a guarded address must be left out: %+v, %v", res, err)
	}
	for _, a := range d.dialed {
		if a == "8.8.8.8:80" {
			t.Error("a refused address must never be dialled")
		}
	}

	d2 := &fakeDialer{}
	if _, err := New(nil).dialerSwap(d2).Scan(context.Background(), netaudit.ScanRequest{Targets: addrs("192.168.1.5"), Mode: netaudit.ModePorts, Ports: "80"}); err != nil {
		t.Fatal(err)
	}
	if len(d2.dialed) != 0 {
		t.Error("an engine built without a guard must scan nothing")
	}
}

func (e *Engine) dialerSwap(d Dialer) *Engine { e.dialer = d; return e }

func TestScan_BoundsConcurrencyAndTotalProbes(t *testing.T) {
	d := &fakeDialer{}
	_, err := engine(d, allow).Scan(context.Background(), netaudit.ScanRequest{Targets: addrs("10.0.0.1", "10.0.0.2", "10.0.0.3"), Mode: netaudit.ModePorts, Ports: "1-1000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.peak.Load() > workers {
		t.Errorf("peak concurrency %d exceeds %d", d.peak.Load(), workers)
	}

	var many []netip.Addr
	for i := 0; i < 60; i++ {
		many = append(many, netip.MustParseAddr(fmt.Sprintf("10.0.0.%d", i+1)))
	}
	if _, err := engine(d, allow).Scan(context.Background(), netaudit.ScanRequest{Targets: many, Mode: netaudit.ModePorts, Ports: "1-1000"}); err == nil {
		t.Error("addresses × ports past the cap must be refused")
	}
}

func TestScan_StopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &fakeDialer{}
	_, err := engine(d, allow).Scan(ctx, netaudit.ScanRequest{Targets: addrs("10.0.0.1"), Mode: netaudit.ModePorts, Ports: "1-1000"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestEngine_IdentityAndSupport(t *testing.T) {
	e := New(allow)
	if e.Name() != "portprobe" || e.Available(context.Background()) != nil {
		t.Error("name/available")
	}
	if !e.Supports(netaudit.ModePorts) || !e.Supports(netaudit.ModeDiscover) || e.Supports(netaudit.ModeServices) {
		t.Error("only discover and ports are supported")
	}
}

func TestScan_RealLoopbackConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback")
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	res, err := New(allow).Scan(context.Background(), netaudit.ScanRequest{Targets: addrs("127.0.0.1"), Mode: netaudit.ModePorts, Ports: fmt.Sprint(port)})
	if err != nil || len(res.Hosts[0].Ports) != 1 || res.Hosts[0].Ports[0].Number != port {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}
