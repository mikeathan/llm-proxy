package capture

import (
	"errors"

	"llm-proxy/internal/platform/privhelper/wire"
)

// The fixed packet filter of the host traffic look (Constitution I.13): IPv4 and IPv6 frames that carry TCP, UDP or ICMP, and nothing else. It
// is a classic BPF program built here from constants and one number, the sampling rate, which the capture program chooses from a count of the
// frames it kept and never from a caller's text. The kernel runs it on each frame before the program sees the frame, and the length it returns
// is how many bytes are kept. A DNS message (UDP or TCP port 53) keeps hostSnapDNS bytes because a name is in it, and is never sampled away.
// The first segment of a TLS ClientHello (a TCP payload that starts 0x16, 0x03 and has a handshake type of 1 at its sixth byte, on any port)
// keeps hostSnapHello bytes, because the server name an address was asked for is in it; it is never sampled away either. Every other frame keeps exactly the Ethernet header, the IP header and the first four bytes after it, which are the ports (so 38 bytes for
// IPv4 without options and 58 for IPv6): the addresses, the ports and the size, and not one byte of payload. It is kept one time in the sampling
// rate. The program is portable: a test runs it on any host with a small interpreter, and only the Linux file sends it to the kernel.
const (
	hostSnapDNS   = wire.HostSnapDNS
	hostSnapHello = wire.HostSnapHello
	hostMaxSample = wire.HostMaxSample
	protoDNS      = 53
	offIPv6Ports  = 54 // Ethernet (14) + IPv6 header (40): the source port, then the destination port
	// afterPorts is what is added to the IPv4 header length to get the end of the ports: the Ethernet header (14) and four bytes.
	afterPorts = 14 + 4
	// tlsHandshake, tlsMajor and helloType are the first byte, the second byte and the sixth byte of the payload of a ClientHello: a handshake
	// record of version 3.x whose handshake message is type 1.
	tlsHandshake = 0x16
	tlsMajor     = 3
	helloType    = 1
	// helloProbe is how many payload bytes must be in the frame before the filter reads the sixth.
	helloProbe    = 6
	tcpOffsetByte = 12 // the byte of the TCP header that holds the data offset in its high four bits
	// scratchPayload is the scratch word that holds where a frame's TCP payload starts.
	scratchPayload = 0
	// snapV6 is the whole ordinary IPv6 frame: Ethernet (14), the fixed IPv6 header (40) and four bytes.
	snapV6 = 14 + 40 + 4
)

var errBadSample = errors.New("the sampling rate is not a power of two from 1 to the limit")

// hostFilter is the program for the sampling rate n (1 keeps every ordinary frame; n keeps one in n, n a power of two).
func hostFilter(n uint16) ([]filterInstruction, error) {
	if n < 1 || n > hostMaxSample || n&(n-1) != 0 {
		return nil, errBadSample
	}
	prog := []step{
		{code: opLdhAbs, k: offEtherType},
		{code: opJeq, k: etherTypeIPv4, jt: "v4"},
		{code: opJeq, k: etherTypeIPv6, jt: "v6", jf: "reject"},

		{label: "v4", code: opLdxMsh, k: offIPv4Header}, // X = the IPv4 header length in bytes
		{code: opLdbAbs, k: offIPv4Proto},
		{code: opJeq, k: protoTCP, jt: "v4tcp"},
		{code: opJeq, k: protoUDP, jt: "v4udp"},
		{code: opJeq, k: protoICMP, jt: "data4", jf: "reject"},

		{label: "v4udp", code: opLdhAbs, k: offIPv4Frag},
		{code: opJset, k: fragmentBits, jt: "data4"}, // a later fragment has no ports to read
		{code: opLdhInd, k: offIPv4Header},           // source port
		{code: opJeq, k: protoDNS, jt: "dns"},
		{code: opLdhInd, k: offIPv4Header + 2}, // destination port
		{code: opJeq, k: protoDNS, jt: "dns", jf: "data4"},

		{label: "v4tcp", code: opLdhAbs, k: offIPv4Frag},
		{code: opJset, k: fragmentBits, jt: "data4"},
		{code: opLdhInd, k: offIPv4Header},
		{code: opJeq, k: protoDNS, jt: "dns"},
		{code: opLdhInd, k: offIPv4Header + 2},
		{code: opJeq, k: protoDNS, jt: "dns"},
		// The TCP data offset is at 14 + the IP header length + 12: the payload starts at 14 + IP header + 4 * offset.
		{code: opLdbInd, k: offIPv4Header + tcpOffsetByte},
		{code: opAndK, k: 0xf0},
		{code: opRshK, k: 2},
		{code: opAddX},
		{code: opAddK, k: offIPv4Header},
	}
	// The probe used X for its own arithmetic, so a frame that is not a hello re-reads the IPv4 header length before the ordinary path needs it.
	prog = append(prog, helloProbeSteps("miss4")...)
	prog = append(prog, step{label: "miss4", code: opLdxMsh, k: offIPv4Header})
	// An ordinary IPv4 frame keeps the header length plus the Ethernet header and four bytes: the headers and the ports, whatever the header length was.
	prog = append(prog, sampledTail("data4", n, []step{{code: opTxa}, {code: opAddK, k: afterPorts}, {code: opRetA}})...)
	prog = append(prog,
		step{label: "v6", code: opLdbAbs, k: offIPv6Next},
		step{code: opJeq, k: protoTCP, jt: "v6tcp"},
		step{code: opJeq, k: protoUDP, jt: "v6udp"},
		step{code: opJeq, k: protoICMPv6, jt: "data6", jf: "reject"},

		step{label: "v6udp", code: opLdhAbs, k: offIPv6Ports},
		step{code: opJeq, k: protoDNS, jt: "dns"},
		step{code: opLdhAbs, k: offIPv6Ports + 2},
		step{code: opJeq, k: protoDNS, jt: "dns", jf: "data6"},

		step{label: "v6tcp", code: opLdhAbs, k: offIPv6Ports},
		step{code: opJeq, k: protoDNS, jt: "dns"},
		step{code: opLdhAbs, k: offIPv6Ports + 2},
		step{code: opJeq, k: protoDNS, jt: "dns"},
		// IPv6 with no extension header: the TCP header starts at 54, so its data offset byte is at 54 + 12.
		step{code: opLdbAbs, k: offIPv6Ports + tcpOffsetByte},
		step{code: opAndK, k: 0xf0},
		step{code: opRshK, k: 2},
		step{code: opAddK, k: offIPv6Ports},
	)
	prog = append(prog, helloProbeSteps("data6")...)
	prog = append(prog, sampledTail("data6", n, []step{{code: opRet, k: snapV6}})...)
	prog = append(prog,
		step{label: "hello-keep", code: opRet, k: hostSnapHello},
		step{label: "dns", code: opRet, k: hostSnapDNS},
		step{label: "reject", code: opRet, k: 0},
	)
	return assemble(prog), nil
}

// helloProbeSteps is the end of a TCP frame's path, entered with A the offset of the TCP payload: it keeps hostSnapHello bytes of a frame whose
// payload starts like a TLS ClientHello, and sends every other frame to miss (its ordinary path). A frame with fewer than helloProbe payload bytes
// (an acknowledgement) goes to miss at once, because a load past the end of a frame would end the program with a drop, and a dropped acknowledgement
// would be a hole in the totals.
func helloProbeSteps(miss string) []step {
	return []step{
		{code: opSt, k: scratchPayload},
		{code: opAddK, k: helloProbe},
		{code: opTax},            // X = the offset of the sixth payload byte, plus one
		{code: opLdLen},          // A = the whole frame's length
		{code: opJgeX, jf: miss}, // the sixth byte must be inside the frame
		{code: opLdxMem, k: scratchPayload},
		{code: opLdbInd, k: 0},
		{code: opJeq, k: tlsHandshake, jf: miss},
		{code: opLdbInd, k: 1},
		{code: opJeq, k: tlsMajor, jf: miss},
		{code: opLdbInd, k: helloProbe - 1},
		{code: opJeq, k: helloType, jt: "hello-keep", jf: miss},
	}
}

// sampledTail is the end of an ordinary frame's path. With a rate above 1 it draws a random number and rejects the frame unless the draw is a
// multiple of the rate; the frame that passes (every frame, at rate 1) runs keep, which returns its length. name is where the path arrives.
func sampledTail(name string, n uint16, keep []step) []step {
	if n == 1 {
		keep[0].label = name
		return keep
	}
	passed := name + "-keep"
	keep[0].label = passed
	return append([]step{
		{label: name, code: opLdwAbs, k: ancRandom},
		{code: opAndK, k: uint32(n - 1)},
		{code: opJeq, k: 0, jt: passed, jf: "reject"},
	}, keep...)
}
