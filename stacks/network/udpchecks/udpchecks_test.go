package udpchecks

import (
	"bytes"
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// tlv encodes one BER element (short or long form length). The fixtures below are built from the RFC 3412 / RFC 3414 shape, not captured
// from a device: the plan marks them as such until the owner supplies a real reply.
func tlv(tag byte, content ...[]byte) []byte {
	body := bytes.Join(content, nil)
	if len(body) < 128 {
		return append([]byte{tag, byte(len(body))}, body...)
	}
	return append([]byte{tag, 0x81, byte(len(body))}, body...)
}

func snmpReport(version byte, pduTag byte) []byte {
	global := tlv(0x30, tlv(0x02, []byte{0x4a, 0x69}), tlv(0x02, []byte{0x00, 0xff, 0xe3}), tlv(0x04, []byte{0x00}), tlv(0x02, []byte{0x03}))
	sec := tlv(0x04, tlv(0x30, tlv(0x04, []byte("engine-id-0001")), tlv(0x02, []byte{0x01}), tlv(0x02, []byte{0x2a}), tlv(0x04), tlv(0x04), tlv(0x04)))
	pdu := tlv(pduTag, tlv(0x02, []byte{0x37, 0xf0}), tlv(0x02, []byte{0x00}), tlv(0x02, []byte{0x00}), tlv(0x30))
	scoped := tlv(0x30, tlv(0x04, []byte("engine-id-0001")), tlv(0x04), pdu)
	return tlv(0x30, tlv(0x02, []byte{version}), global, sec, scoped)
}

func TestSNMPDiscoveryIsTheFixedMessageWithNoCommunity(t *testing.T) {
	want := []byte{0x30, 0x3a, 0x02, 0x01, 0x03, 0x30, 0x0f, 0x02, 0x02, 0x4a, 0x69, 0x02, 0x03, 0x00, 0xff, 0xe3, 0x04, 0x01, 0x04, 0x02, 0x01, 0x03,
		0x04, 0x10, 0x30, 0x0e, 0x04, 0x00, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x04, 0x00, 0x04, 0x00, 0x04, 0x00,
		0x30, 0x12, 0x04, 0x00, 0x04, 0x00, 0xa0, 0x0c, 0x02, 0x02, 0x37, 0xf0, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x30, 0x00}
	if !bytes.Equal(snmpDiscovery(netip.Addr{}), want) {
		t.Fatalf("the SNMPv3 discovery message changed: %x", snmpDiscovery(netip.Addr{}))
	}
	if len(want)-2 != int(want[1]) {
		t.Fatalf("the declared length %d does not match the message", want[1])
	}
}

func TestSNMPReplyMustBeAWellFormedV3Report(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"a v3 report", snmpReport(3, 0xa8), true},
		{"a v3 response PDU is not a report", snmpReport(3, 0xa2), false},
		{"a v2c message", snmpReport(1, 0xa8), false},
		{"empty", nil, false},
		{"not BER", []byte("HTTP/1.1 200 OK\r\n"), false},
		{"truncated", snmpReport(3, 0xa8)[:20], false},
		{"length past the end", []byte{0x30, 0x7f, 0x02, 0x01, 0x03}, false},
	}
	for _, c := range cases {
		if got := validSNMPv3Report(c.data); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSSDPUnicastNamesTheDeviceNotTheGroup(t *testing.T) {
	m := string(ssdpSearch(netip.MustParseAddr("192.168.1.20")))
	if want := "M-SEARCH * HTTP/1.1\r\nHOST: 192.168.1.20:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: upnp:rootdevice\r\n\r\n"; m != want {
		t.Fatalf("got %q", m)
	}
}

func TestSSDPReplyNeedsAnHTTP200FirstLine(t *testing.T) {
	for in, want := range map[string]bool{
		"HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\n\r\n": true,
		"HTTP/1.0 200 OK\r\n\r\n":                        false,
		"HTTP/1.1 500 Error\r\n\r\n":                     false,
		"NOTIFY * HTTP/1.1\r\n\r\n":                      false,
		"":                                               false,
	} {
		if got := validSSDPReply([]byte(in)); got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

// answerer listens on a loopback UDP port and answers each datagram with reply(request); a nil reply stays silent.
func answerer(t *testing.T, reply func([]byte) []byte) (port int, requests *[][]byte) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	var got [][]byte
	var gmu sync.Mutex
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			gmu.Lock()
			got = append(got, append([]byte(nil), buf[:n]...))
			gmu.Unlock()
			if r := reply(buf[:n]); r != nil {
				pc.WriteTo(r, from)
			}
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port, &got
}

// dialTo sends every dial to one local port whatever address was asked, and records the address and network asked.
func dialTo(port int, asked *[]string) Dial {
	var mu sync.Mutex
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		mu.Lock()
		*asked = append(*asked, network+" "+address)
		mu.Unlock()
		var d net.Dialer
		return d.DialContext(ctx, "udp", net.JoinHostPort("127.0.0.1", itoa(port)))
	}
}

func TestAskReportsAnAnswerInTheExpectedFormOnly(t *testing.T) {
	good, _ := answerer(t, func([]byte) []byte { return snmpReport(3, 0xa8) })
	junk, _ := answerer(t, func([]byte) []byte { return []byte("hello") })
	silent, _ := answerer(t, func([]byte) []byte { return nil })
	var asked []string
	snmp := Probes[0]
	if snmp.Port != 161 {
		t.Fatalf("probe 0 is %d, want SNMP on 161", snmp.Port)
	}
	for name, c := range map[string]struct {
		port int
		want bool
	}{"good": {good, true}, "junk": {junk, false}, "silent": {silent, false}} {
		p := Prober{Dial: dialTo(c.port, &asked), Wait: 300 * time.Millisecond}
		if got := p.Ask(context.Background(), netip.MustParseAddr("192.168.1.9"), snmp); got != c.want {
			t.Errorf("%s: got %v", name, got)
		}
	}
	if asked[0] != "udp 192.168.1.9:161" {
		t.Errorf("dialed %q: the probe must go to the device's own address and port", asked[0])
	}
}

func TestAskRefusesNonIPv4AndDialFailure(t *testing.T) {
	var asked []string
	p := Prober{Dial: dialTo(1, &asked), Wait: 50 * time.Millisecond}
	if p.Ask(context.Background(), netip.MustParseAddr("fe80::1"), Probes[0]) || len(asked) != 0 {
		t.Fatal("an IPv6 address must not be probed")
	}
	p = Prober{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, net.ErrClosed }}
	if p.Ask(context.Background(), netip.MustParseAddr("192.168.1.9"), Probes[0]) {
		t.Fatal("a refused dial is not an answer")
	}
}

func TestLookIsBoundedAndReportsOnlyAnswers(t *testing.T) {
	snmpPort, _ := answerer(t, func([]byte) []byte { return snmpReport(3, 0xa8) })
	var addrs []netip.Addr
	for i := 0; i < MaxDevices+10; i++ {
		addrs = append(addrs, netip.AddrFrom4([4]byte{192, 168, 1, byte(i + 1)}))
	}
	var asked []string
	// every dial reaches the SNMP answerer: SNMP answers, SSDP gets a reply that is not an HTTP 200 and so does not count.
	p := Prober{Dial: dialTo(snmpPort, &asked), Wait: 300 * time.Millisecond}
	got := p.Look(context.Background(), addrs)
	if len(asked) != MaxDevices*len(Probes) {
		t.Fatalf("sent %d datagrams, want %d (64 devices x 2)", len(asked), MaxDevices*len(Probes))
	}
	if len(got) != MaxDevices {
		t.Fatalf("got %d answers, want %d", len(got), MaxDevices)
	}
	for _, a := range got {
		if a.Port != 161 {
			t.Fatalf("an answer on %d: only SNMP should have counted", a.Port)
		}
	}
}
