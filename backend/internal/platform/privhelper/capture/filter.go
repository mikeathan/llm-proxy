package capture

// The fixed packet filter of Constitution I.11: ARP, DHCP (UDP 67 and 68), mDNS (UDP 5353) and IPv6 neighbour discovery (ICMPv6 133 to
// 137), and nothing else. It is a classic BPF program built here from constants, so it can be read and is the same on every run; no
// caller's text reaches it. The kernel runs it on each frame before the program sees the frame, and the accepted length is the 64 bytes the
// capture keeps. The program is portable on purpose: a test runs it on any host with a small interpreter, and only the Linux file sends it to
// the kernel.

// filterInstruction is one classic BPF instruction, in the shape the kernel takes.
type filterInstruction struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// The opcodes used (linux/filter.h).
const (
	opLdbAbs = 0x30 // A = byte at packet[k]
	opLdhAbs = 0x28 // A = 16-bit word at packet[k]
	opLdhInd = 0x48 // A = 16-bit word at packet[X+k]
	opLdxMsh = 0xb1 // X = 4 * (packet[k] & 0xf): the IPv4 header length
	opJeq    = 0x15
	opJgt    = 0x25
	opJge    = 0x35
	opJset   = 0x45
	opRet    = 0x06
)

const (
	etherTypeARP  = 0x0806
	etherTypeIPv4 = 0x0800
	etherTypeIPv6 = 0x86dd
	protoUDP      = 17
	protoICMPv6   = 58
	ndFirst       = 133 // router solicitation
	ndLast        = 137 // redirect
	portDHCPSrv   = 67
	portDHCPCli   = 68
	portMDNS      = 5353
	fragmentBits  = 0x1fff // fragment offset: a non-zero offset is not the first fragment, which holds the UDP header
	offEtherType  = 12
	offIPv4Header = 14
	offIPv4Proto  = 23
	offIPv4Frag   = 20
	offIPv6Next   = 20
	offICMPv6Type = 54
)

// step is one instruction with named jump targets; assemble resolves them to offsets. BPF jumps only forward.
type step struct {
	code   uint16
	k      uint32
	jt, jf string
	label  string
}

func assemble(prog []step) []filterInstruction {
	at := map[string]int{}
	for i, s := range prog {
		if s.label != "" {
			at[s.label] = i
		}
	}
	out := make([]filterInstruction, len(prog))
	for i, s := range prog {
		jump := func(label string) uint8 {
			if label == "" {
				return 0
			}
			to, ok := at[label]
			if !ok || to <= i || to-i-1 > 255 {
				panic("capture filter: bad jump to " + label) // a programming error, caught by the first test run
			}
			return uint8(to - i - 1)
		}
		out[i] = filterInstruction{Code: s.code, K: s.k, Jt: jump(s.jt), Jf: jump(s.jf)}
	}
	return out
}

// captureFilter is the program: accept (keep 64 bytes) or reject (keep nothing).
func captureFilter() []filterInstruction {
	return assemble([]step{
		{code: opLdhAbs, k: offEtherType},
		{code: opJeq, k: etherTypeARP, jt: "accept"},
		{code: opJeq, k: etherTypeIPv6, jt: "v6"},
		{code: opJeq, k: etherTypeIPv4, jt: "v4", jf: "reject"},

		{label: "v6", code: opLdbAbs, k: offIPv6Next},
		{code: opJeq, k: protoICMPv6, jf: "reject"},
		{code: opLdbAbs, k: offICMPv6Type},
		{code: opJge, k: ndFirst, jf: "reject"},
		{code: opJgt, k: ndLast, jt: "reject"},
		{code: opRet, k: snapLen},

		{label: "v4", code: opLdbAbs, k: offIPv4Proto},
		{code: opJeq, k: protoUDP, jf: "reject"},
		{code: opLdhAbs, k: offIPv4Frag},
		{code: opJset, k: fragmentBits, jt: "reject"},
		{code: opLdxMsh, k: offIPv4Header},
		{code: opLdhInd, k: offIPv4Header}, // source port
		{code: opJeq, k: portDHCPSrv, jt: "accept"},
		{code: opJeq, k: portDHCPCli, jt: "accept"},
		{code: opJeq, k: portMDNS, jt: "accept"},
		{code: opLdhInd, k: offIPv4Header + 2}, // destination port
		{code: opJeq, k: portDHCPSrv, jt: "accept"},
		{code: opJeq, k: portDHCPCli, jt: "accept"},
		{code: opJeq, k: portMDNS, jt: "accept"},

		{label: "reject", code: opRet, k: 0},
		{label: "accept", code: opRet, k: snapLen},
	})
}
