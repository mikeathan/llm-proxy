package capture

import (
	"errors"

	"llm-proxy/internal/platform/privhelper/wire"
)

// The fixed packet filter of the host traffic look (Constitution I.13): IPv4 and IPv6 frames that carry TCP, UDP or ICMP, and nothing else. It
// is a classic BPF program built here from constants and one number, the sampling rate, which the capture program chooses from a count of the
// frames it kept and never from a caller's text. The kernel runs it on each frame before the program sees the frame, and the length it returns
// is how many bytes are kept. A DNS message (UDP or TCP port 53) keeps hostSnapDNS bytes because a name is in it, and is never sampled away.
// Every other frame keeps exactly the Ethernet header, the IP header and the first four bytes after it, which are the ports (so 38 bytes for
// IPv4 without options and 58 for IPv6): the addresses, the ports and the size, and not one byte of payload. It is kept one time in the sampling
// rate. The program is portable: a test runs it on any host with a small interpreter, and only the Linux file sends it to the kernel.
const (
	hostSnapDNS   = wire.HostSnapMax
	hostMaxSample = wire.HostMaxSample
	protoDNS      = 53
	offIPv6Ports  = 54 // Ethernet (14) + IPv6 header (40): the source port, then the destination port
	// afterPorts is what is added to the IPv4 header length to get the end of the ports: the Ethernet header (14) and four bytes.
	afterPorts = 14 + 4
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
		{code: opJeq, k: protoTCP, jt: "v4ports"},
		{code: opJeq, k: protoUDP, jt: "v4ports"},
		{code: opJeq, k: protoICMP, jt: "data4", jf: "reject"},

		{label: "v4ports", code: opLdhAbs, k: offIPv4Frag},
		{code: opJset, k: fragmentBits, jt: "data4"}, // a later fragment has no ports to read
		{code: opLdhInd, k: offIPv4Header},           // source port
		{code: opJeq, k: protoDNS, jt: "dns"},
		{code: opLdhInd, k: offIPv4Header + 2}, // destination port
		{code: opJeq, k: protoDNS, jt: "dns", jf: "data4"},
	}
	// An ordinary IPv4 frame keeps the header length plus the Ethernet header and four bytes: the headers and the ports, whatever the header length was.
	prog = append(prog, sampledTail("data4", n, []step{{code: opTxa}, {code: opAddK, k: afterPorts}, {code: opRetA}})...)
	prog = append(prog,
		step{label: "v6", code: opLdbAbs, k: offIPv6Next},
		step{code: opJeq, k: protoTCP, jt: "v6ports"},
		step{code: opJeq, k: protoUDP, jt: "v6ports"},
		step{code: opJeq, k: protoICMPv6, jt: "data6", jf: "reject"},

		step{label: "v6ports", code: opLdhAbs, k: offIPv6Ports},
		step{code: opJeq, k: protoDNS, jt: "dns"},
		step{code: opLdhAbs, k: offIPv6Ports + 2},
		step{code: opJeq, k: protoDNS, jt: "dns", jf: "data6"},
	)
	prog = append(prog, sampledTail("data6", n, []step{{code: opRet, k: snapV6}})...)
	prog = append(prog,
		step{label: "dns", code: opRet, k: hostSnapDNS},
		step{label: "reject", code: opRet, k: 0},
	)
	return assemble(prog), nil
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
