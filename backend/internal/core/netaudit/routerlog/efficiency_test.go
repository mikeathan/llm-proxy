package routerlog

import (
	"net"
	"net/netip"
	"runtime"
	"testing"
	"time"
)

const dropLine = "<12>Oct 31 20:02:33 ROUTER kernel: DROP IN=eth0 OUT= MAC=REPLACED SRC=203.0.113.9 DST=192.168.0.137 LEN=52 TOS=0x00 PREC=0x00 TTL=52 ID=51919 DF PROTO=TCP SPT=40408 DPT=58376 SEQ=1280188400 ACK=0 WINDOW=65535 RES=0x00 SYN URGP=0 OPT (020405A00103030801010402) MARK=0x8000000"

func TestManager_TheSameNameIsOfferedOncePerHourAndAForgedStreamIsCapped(t *testing.T) {
	r := newRig(t)
	addr := r.start(t)
	for i := 0; i < 5; i++ {
		send(t, addr, lease("aa:bb:cc:dd:ee:01", "NEW-PHONE"))
	}
	r.waitHeard(t)
	r.quiet()
	if got := len(r.offered()); got != 1 {
		t.Errorf("the same device and name offered %d times, want once", got)
	}
	for i := 0; i < 100; i++ {
		send(t, addr, lease("aa:bb:cc:dd:"+hex2(i)+":"+hex2(i+1), "FORGED"))
	}
	r.quiet()
	if got := len(r.offered()); got > suggestBurst {
		t.Errorf("offered %d names in one window, want at most %d: each offer rewrites the name file", got, suggestBurst)
	}
}

// A soak: a long mixed stream must leave the heap and the goroutines where they were. The socket is read by one goroutine
// into one buffer; nothing is queued, mapped or kept per datagram.
func TestManager_SoakDoesNotGrowTheHeapOrTheGoroutines(t *testing.T) {
	r := newRig(t)
	r.m.wakeGap = 0
	addr := r.start(t)
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	lines := [][]byte{[]byte(dropLine), []byte(ackLine), []byte(joinedLine), []byte("junk")}
	heap := func() uint64 {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.HeapInuse
	}
	pump := func(n int) {
		for i := 0; i < n; i++ {
			c.Write(lines[i%len(lines)])
			if i%200 == 0 {
				time.Sleep(time.Millisecond) // let the reader keep up; the kernel drops the rest
			}
		}
	}
	pump(2000) // warm up
	before, goroutines := heap(), runtime.NumGoroutine()
	pump(60000)
	time.Sleep(100 * time.Millisecond)
	after := heap()
	if growth := int64(after) - int64(before); growth > 512<<10 {
		t.Errorf("the heap grew by %d bytes over 60,000 datagrams", growth)
	}
	if got := runtime.NumGoroutine(); got > goroutines+1 {
		t.Errorf("goroutines went from %d to %d", goroutines, got)
	}
}

func BenchmarkInteresting_FirewallLine(b *testing.B) {
	line := []byte(dropLine)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Interesting(line) {
			b.Fatal("a firewall line is not interesting")
		}
	}
}

func BenchmarkParse_Lease(b *testing.B) {
	line := []byte(ackLine)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := Parse(line); !ok {
			b.Fatal("not parsed")
		}
	}
}

// BenchmarkHandle_FirewallLine is the whole path a firewall line takes after the read: the sender check and the cheap
// search. It is the cost that matters, since these are most of the log.
func BenchmarkHandle_FirewallLine(b *testing.B) {
	gw := netip.MustParseAddr("192.168.50.1")
	m := New(Deps{Gateway: func() netip.Addr { return gw }, Device: func(string) (bool, bool, error) { return true, true, nil }, Wake: func() {}})
	line, from := []byte(dropLine), netip.AddrPortFrom(gw, 36146)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.handle(line, from)
	}
}
