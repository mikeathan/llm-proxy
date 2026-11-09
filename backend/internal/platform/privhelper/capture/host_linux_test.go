//go:build linux

package capture

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
)

// A real window on the loopback interface, where this computer's own traffic is all there is to see. It needs the packet-socket capability
// (the helper's unit holds it), so it skips where the test does not have it; where it runs it proves the filter, the socket and the program work
// together, not only each on its own.
func TestALiveWindowOnLoopbackKeepsDNSLongAndOtherTrafficShort(t *testing.T) {
	src, stop, err := OpenHost("lo")
	if err != nil {
		t.Skipf("no packet socket on lo (stop %d): the test needs CAP_NET_RAW", stop)
	}
	defer src.Close()

	dnsPayload := bytes.Repeat([]byte{0x5a}, 300)
	dataPayload := bytes.Repeat([]byte{0x6b}, 600)
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(300 * time.Millisecond)
		udp, derr := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53})
		if derr != nil {
			return
		}
		defer udp.Close()
		_, _ = udp.Write(dnsPayload)
		other, derr := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40123})
		if derr != nil {
			return
		}
		defer other.Close()
		_, _ = other.Write(dataPayload)
	}()

	var out bytes.Buffer
	cfg := HostConfig{Seconds: 2, MaxFrames: wire.HostMaxFrames, MaxOutput: wire.HostMaxOutput, BudgetPerSecond: hostBudgetPerSecond}
	if err := RunHost(src, &out, cfg); err != nil {
		t.Fatal(err)
	}
	<-done
	recs, tr, err := wire.DecodeHostCapture(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if tr.Stop != wire.StopWindow || tr.Sample != 1 {
		t.Fatalf("stop=%d sample=%d", tr.Stop, tr.Sample)
	}
	var dns, data int
	for _, r := range recs {
		f := r.Frame
		// Ethernet header on lo is 14 bytes of zeros and the type; IPv4 follows.
		if len(f) < 14+20+4 || binary.BigEndian.Uint16(f[12:14]) != etherTypeIPv4 || f[14+9] != protoUDP {
			continue
		}
		dport := binary.BigEndian.Uint16(f[14+20+2:])
		switch dport {
		case 53:
			dns = len(f)
		case 40123:
			data = len(f)
		}
	}
	if dns != 14+20+8+len(dnsPayload) {
		t.Errorf("the DNS message was kept at %d bytes, want it whole (%d)", dns, 14+20+8+len(dnsPayload))
	}
	if data != snapV4 {
		t.Errorf("the other datagram was kept at %d bytes, want the headers and the ports only (%d)", data, snapV4)
	}
}

// The sampling load is the one part of the filter an interpreter cannot vouch for: it is a number only the kernel draws. This sends many
// datagrams at the coarsest rate and sees the kernel keep a few, then at rate 1 and sees it keep them all.
func TestALiveSamplingRateThinsOrdinaryTrafficAndRateOneKeepsAll(t *testing.T) {
	src, stop, err := OpenHost("lo")
	if err != nil {
		t.Skipf("no packet socket on lo (stop %d): the test needs CAP_NET_RAW", stop)
	}
	defer src.Close()
	const sent = 400
	countKept := func(rate uint16, port int) int {
		if err := src.SetSample(rate); err != nil {
			t.Fatal(err)
		}
		conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		// Sent in small bursts while the reader runs, as traffic really arrives: a burst larger than the socket's queue before anything is read
		// would be dropped by the kernel and counted, which is not what this test is about.
		go func() {
			for i := 0; i < sent; i++ {
				_, _ = conn.Write([]byte("x"))
				if i%10 == 9 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
		kept := 0
		for deadline := time.Now().Add(1500 * time.Millisecond); time.Now().Before(deadline); {
			f, err := src.Next()
			if err != nil {
				t.Fatal(err)
			}
			if len(f) >= 14+20+4 && binary.BigEndian.Uint16(f[12:14]) == etherTypeIPv4 && f[14+9] == protoUDP && int(binary.BigEndian.Uint16(f[14+20+2:])) == port {
				kept++
			}
		}
		return kept
	}
	if got := countKept(hostMaxSample, 40125); got > sent/20 {
		t.Errorf("at 1 in %d the kernel kept %d of %d datagrams", hostMaxSample, got, sent)
	}
	if got := countKept(1, 40126); got != sent {
		t.Errorf("at rate 1 the kernel kept %d of %d datagrams", got, sent)
	}
}
