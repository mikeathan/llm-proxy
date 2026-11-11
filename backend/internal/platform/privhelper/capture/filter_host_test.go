package capture

import (
	"encoding/binary"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

// hostV4 builds an Ethernet + IPv4 + (TCP or UDP) frame with the given header length in words, fragment field and ports.
func hostV4(proto byte, ihl int, frag uint16, sport, dport uint16) []byte {
	ip := make([]byte, ihl*4)
	ip[0] = byte(0x40 | ihl)
	binary.BigEndian.PutUint16(ip[6:], frag)
	ip[9] = proto
	l4 := make([]byte, 20)
	binary.BigEndian.PutUint16(l4[0:], sport)
	binary.BigEndian.PutUint16(l4[2:], dport)
	return eth(etherTypeIPv4, append(ip, l4...)...)
}

// hostV6 builds an Ethernet + IPv6 + (TCP or UDP) frame with the given next-header value and ports.
func hostV6(next byte, sport, dport uint16) []byte {
	ip := make([]byte, 40+20)
	ip[6] = next
	binary.BigEndian.PutUint16(ip[40:], sport)
	binary.BigEndian.PutUint16(ip[42:], dport)
	return eth(etherTypeIPv6, ip...)
}

func TestNoOrdinaryFrameKeepsMoreThanItsHeadersAndPorts(t *testing.T) {
	prog := mustHostFilter(t, 1)
	// Whatever follows the first four bytes of the L4 header is payload: a frame loaded with it is still cut at the ports.
	for ihl := 5; ihl <= 15; ihl++ {
		f := append(hostV4(protoTCP, ihl, 0, 51000, 443), make([]byte, 400)...)
		if got, want := runFilter(prog, f), uint32(14+ihl*4+4); got != want {
			t.Errorf("IPv4 header of %d words: kept %d bytes, want %d", ihl, got, want)
		}
	}
	if got := runFilter(prog, append(hostV6(protoTCP, 51000, 443), make([]byte, 400)...)); got != snapV6 {
		t.Errorf("IPv6: kept %d bytes, want %d", got, snapV6)
	}
	if max := 14 + 15*4 + 4; max > wire.HostSnapHeaders {
		t.Errorf("the longest ordinary frame is %d bytes but the wire says %d", max, wire.HostSnapHeaders)
	}
}

func mustHostFilter(t *testing.T, n uint16) []filterInstruction {
	t.Helper()
	prog, err := hostFilter(n)
	if err != nil {
		t.Fatal(err)
	}
	return prog
}

// The ordinary frame keeps the Ethernet and IP headers and the first four bytes after them, which are the ports: 14 + the IP header + 4. It keeps
// no payload byte (Constitution I.13), so a frame that has payload is cut exactly there.
const (
	snapV4  = 14 + 20 + 4
	snapV4o = 14 + 24 + 4 // an IPv4 header of six words
)

func TestTheHostFilterKeepsHeadersOfOrdinaryTrafficAndMoreOfDNS(t *testing.T) {
	prog := mustHostFilter(t, 1)
	cases := []struct {
		name string
		pkt  []byte
		want uint32
	}{
		{"HTTPS over IPv4", hostV4(protoTCP, 5, 0, 51000, 443), snapV4},
		{"QUIC over IPv4", hostV4(protoUDP, 5, 0, 51000, 443), snapV4},
		{"ICMP over IPv4", hostV4(protoICMP, 5, 0, 0, 0), snapV4},
		{"HTTPS over IPv6", hostV6(protoTCP, 51000, 443), snapV6},
		{"ICMPv6", hostV6(protoICMPv6, 0, 0), snapV6},
		{"DNS query over UDP", hostV4(protoUDP, 5, 0, 40000, 53), hostSnapDNS},
		{"DNS answer over UDP", hostV4(protoUDP, 5, 0, 53, 40000), hostSnapDNS},
		{"DNS over TCP", hostV4(protoTCP, 5, 0, 40000, 53), hostSnapDNS},
		{"DNS with IPv4 options (header of 6 words)", hostV4(protoUDP, 6, 0, 40000, 53), hostSnapDNS},
		{"HTTPS with IPv4 options keeps the longer header and still no payload", hostV4(protoTCP, 6, 0, 51000, 443), snapV4o},
		{"DNS over IPv6", hostV6(protoUDP, 53, 40000), hostSnapDNS},
		{"a later fragment is ordinary traffic, whatever bytes sit where the ports would", hostV4(protoUDP, 5, 0x00b9, 53, 53), snapV4},
		{"ARP", eth(etherTypeARP, make([]byte, 28)...), 0},
		{"LLDP", eth(0x88cc, make([]byte, 40)...), 0},
		{"GRE over IPv4", hostV4(47, 5, 0, 0, 0), 0},
		{"an IPv6 extension header", hostV6(0, 0, 0), 0},
		{"too short", []byte{1, 2, 3}, 0},
		{"an IPv4 header cut before the ports", hostV4(protoUDP, 5, 0, 1, 53)[:14+20+1], 0},
	}
	for _, c := range cases {
		if got := runFilter(prog, c.pkt); got != c.want {
			t.Errorf("%s: filter returned %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTheHostFilterSamplesOrdinaryTrafficButNeverDNS(t *testing.T) {
	prog := mustHostFilter(t, 8)
	data := hostV4(protoTCP, 5, 0, 51000, 443)
	dns := hostV4(protoUDP, 5, 0, 40000, 53)
	for draw := uint32(0); draw < 32; draw++ {
		want := uint32(0)
		if draw%8 == 0 {
			want = snapV4
		}
		if got := runFilterWith(prog, data, draw); got != want {
			t.Errorf("ordinary frame, draw %d: filter returned %d, want %d (one in 8 is kept)", draw, got, want)
		}
		if got := runFilterWith(prog, dns, draw); got != hostSnapDNS {
			t.Errorf("DNS frame, draw %d: filter returned %d; a name is never sampled away", draw, got)
		}
	}
}

func TestTheHostFilterTakesOnlyPowersOfTwoUpToTheLimit(t *testing.T) {
	for _, n := range []uint16{0, 3, 6, 1000, 2048} {
		if _, err := hostFilter(n); err == nil {
			t.Errorf("sampling 1 in %d was accepted", n)
		}
	}
	for n := uint16(1); n <= hostMaxSample; n *= 2 {
		prog := mustHostFilter(t, n)
		if len(prog) == 0 || len(prog) > 4096 {
			t.Fatalf("1 in %d: %d instructions", n, len(prog))
		}
		if last := prog[len(prog)-1]; last.Code != opRet {
			t.Fatalf("1 in %d: the last instruction is %#x, not a return", n, last.Code)
		}
		for i, in := range prog {
			if in.Code != opRet && (i+1+int(in.Jt) >= len(prog) || i+1+int(in.Jf) >= len(prog)) {
				t.Errorf("1 in %d: instruction %d jumps past the end", n, i)
			}
		}
	}
}

// tcpPayload builds an Ethernet + IPv4 frame carrying a TCP segment with the given TCP header length in words and payload.
func tcpPayloadV4(ihl, dataWords int, dport uint16, payload []byte) []byte {
	ip := make([]byte, ihl*4)
	ip[0] = byte(0x40 | ihl)
	ip[9] = protoTCP
	tcp := make([]byte, dataWords*4)
	binary.BigEndian.PutUint16(tcp[0:], 51000)
	binary.BigEndian.PutUint16(tcp[2:], dport)
	tcp[12] = byte(dataWords << 4)
	return eth(etherTypeIPv4, append(append(ip, tcp...), payload...)...)
}

func tcpPayloadV6(dataWords int, dport uint16, payload []byte) []byte {
	ip := make([]byte, 40)
	ip[6] = protoTCP
	tcp := make([]byte, dataWords*4)
	binary.BigEndian.PutUint16(tcp[0:], 51000)
	binary.BigEndian.PutUint16(tcp[2:], dport)
	tcp[12] = byte(dataWords << 4)
	return eth(etherTypeIPv6, append(append(ip, tcp...), payload...)...)
}

// hello is the start of a TLS ClientHello: a handshake record (0x16, version 3.x) whose handshake type is 1, then filler.
func hello(extra int) []byte {
	return append([]byte{0x16, 0x03, 0x01, 0x02, 0x00, 0x01, 0x00, 0x01, 0xfc}, make([]byte, extra)...)
}

func TestTheHostFilterKeepsTheStartOfATLSClientHelloAndNothingElseOfAnOrdinaryFrame(t *testing.T) {
	prog := mustHostFilter(t, 1)
	long := hello(900)
	cases := []struct {
		name string
		pkt  []byte
		want uint32
	}{
		{"hello over IPv4", tcpPayloadV4(5, 5, 443, long), hostSnapHello},
		{"hello over IPv4 with IP and TCP options", tcpPayloadV4(7, 8, 443, long), hostSnapHello},
		{"hello over IPv6", tcpPayloadV6(5, 443, long), hostSnapHello},
		{"hello to a port that is not 443", tcpPayloadV4(5, 5, 8443, long), hostSnapHello},
		{"a short hello still carries what it has", tcpPayloadV4(5, 5, 443, hello(0)), hostSnapHello},
		{"an acknowledgement with no payload is ordinary, not dropped", tcpPayloadV4(5, 5, 443, nil), snapV4},
		{"an acknowledgement over IPv6 is ordinary", tcpPayloadV6(5, 443, nil), snapV6},
		{"five bytes of payload is too few to be a hello", tcpPayloadV4(5, 5, 443, hello(0)[:5]), snapV4},
		{"application data is ordinary", tcpPayloadV4(5, 5, 443, append([]byte{0x17, 0x03, 0x03, 0x02, 0x00, 0x01}, make([]byte, 600)...)), snapV4},
		{"a ServerHello is ordinary", tcpPayloadV4(5, 5, 443, append([]byte{0x16, 0x03, 0x03, 0x02, 0x00, 0x02}, make([]byte, 600)...)), snapV4},
		{"a handshake that is not version 3 is ordinary", tcpPayloadV4(5, 5, 443, append([]byte{0x16, 0x02, 0x01, 0x02, 0x00, 0x01}, make([]byte, 600)...)), snapV4},
		{"plain HTTP is ordinary", tcpPayloadV4(5, 5, 80, append([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"), make([]byte, 100)...)), snapV4},
		{"a hello over UDP is not a TLS hello", hostV4(protoUDP, 5, 0, 51000, 443), snapV4},
		{"a DNS message over TCP keeps its own length", tcpPayloadV4(5, 5, 53, long), hostSnapDNS},
	}
	for _, c := range cases {
		if got := runFilter(prog, c.pkt); got != c.want {
			t.Errorf("%s: filter returned %d, want %d", c.name, got, c.want)
		}
	}
	// The ordinary ones keep exactly their headers and ports, whatever they carry.
	if got := runFilter(prog, tcpPayloadV4(7, 8, 443, append([]byte{0x17, 3, 3, 0, 9, 0}, make([]byte, 600)...))); got != 14+7*4+4 {
		t.Errorf("an ordinary segment with options kept %d bytes, want %d", got, 14+7*4+4)
	}
}

func TestAClientHelloIsNeverSampledAway(t *testing.T) {
	prog := mustHostFilter(t, 8)
	for draw := uint32(0); draw < 16; draw++ {
		if got := runFilterWith(prog, tcpPayloadV4(5, 5, 443, hello(900)), draw); got != hostSnapHello {
			t.Errorf("hello over IPv4, draw %d: kept %d, a name is never sampled away", draw, got)
		}
		if got := runFilterWith(prog, tcpPayloadV6(5, 443, hello(900)), draw); got != hostSnapHello {
			t.Errorf("hello over IPv6, draw %d: kept %d", draw, got)
		}
	}
}

func TestNoFrameKeepsMoreThanTheWireAllows(t *testing.T) {
	if hostSnapHello > wire.HostSnapMax || hostSnapDNS > wire.HostSnapMax {
		t.Errorf("a kept frame of %d or %d bytes does not fit a record of %d", hostSnapHello, hostSnapDNS, wire.HostSnapMax)
	}
}
