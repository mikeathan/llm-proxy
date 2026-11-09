package capture

import (
	"encoding/binary"
	"testing"
)

// run is a classic BPF interpreter for the opcodes the filter uses, so the program is tested on every host, not only where the kernel can
// load it. It returns the length the program accepts (0 rejects). A load past the end of the frame rejects, as in the kernel.
func runFilter(prog []filterInstruction, pkt []byte) uint32 {
	return runFilterWith(prog, pkt, 0)
}

// runFilterWith is runFilter with the number the kernel's random ancillary load would answer, so a sampling filter is tested for each draw.
func runFilterWith(prog []filterInstruction, pkt []byte, random uint32) uint32 {
	var a, x uint32
	pc := 0
	u16 := func(off uint32) (uint32, bool) {
		if int(off)+2 > len(pkt) {
			return 0, false
		}
		return uint32(binary.BigEndian.Uint16(pkt[off:])), true
	}
	for steps := 0; steps < 1000 && pc < len(prog); steps++ {
		in := prog[pc]
		pc++
		var ok = true
		switch in.Code {
		case opLdwAbs:
			if in.K != ancRandom {
				panic("this interpreter loads only the random ancillary word")
			}
			a = random
		case opAndK:
			a &= in.K
		case opTxa:
			a = x
		case opAddK:
			a += in.K
		case opRetA:
			return a
		case opLdbAbs:
			if int(in.K) >= len(pkt) {
				return 0
			}
			a = uint32(pkt[in.K])
		case opLdhAbs:
			if a, ok = u16(in.K); !ok {
				return 0
			}
		case opLdhInd:
			if a, ok = u16(x + in.K); !ok {
				return 0
			}
		case opLdxMsh:
			if int(in.K) >= len(pkt) {
				return 0
			}
			x = 4 * uint32(pkt[in.K]&0xf)
		case opJeq, opJgt, opJge, opJset:
			cond := map[uint16]bool{opJeq: a == in.K, opJgt: a > in.K, opJge: a >= in.K, opJset: a&in.K != 0}[in.Code]
			if cond {
				pc += int(in.Jt)
			} else {
				pc += int(in.Jf)
			}
		case opRet:
			return in.K
		default:
			panic("unknown opcode")
		}
	}
	return 0
}

func eth(et uint16, rest ...byte) []byte {
	f := make([]byte, 14)
	binary.BigEndian.PutUint16(f[12:], et)
	return append(f, rest...)
}

// ipv4udp builds an Ethernet + IPv4 + UDP frame with the given header length words and ports.
func ipv4udp(ihl int, frag uint16, sport, dport uint16) []byte {
	ip := make([]byte, ihl*4)
	ip[0] = byte(0x40 | ihl)
	binary.BigEndian.PutUint16(ip[6:], frag)
	ip[9] = 17
	udp := make([]byte, 8)
	binary.BigEndian.PutUint16(udp[0:], sport)
	binary.BigEndian.PutUint16(udp[2:], dport)
	return eth(etherTypeIPv4, append(ip, udp...)...)
}

func ipv6(next byte, icmpType byte) []byte {
	ip := make([]byte, 40+8)
	ip[6] = next
	ip[40] = icmpType
	return eth(etherTypeIPv6, ip...)
}

func TestTheFilterKeepsOnlyARPDHCPMDNSAndNeighbourDiscovery(t *testing.T) {
	prog := captureFilter()
	cases := []struct {
		name string
		pkt  []byte
		want bool
	}{
		{"ARP", eth(etherTypeARP, make([]byte, 28)...), true},
		{"DHCP client to server", ipv4udp(5, 0, 68, 67), true},
		{"DHCP server to client", ipv4udp(5, 0, 67, 68), true},
		{"mDNS", ipv4udp(5, 0, 5353, 5353), true},
		{"mDNS from a high port", ipv4udp(5, 0, 40000, 5353), true},
		{"DHCP with IPv4 options (header of 6 words)", ipv4udp(6, 0, 68, 67), true},
		{"router solicitation", ipv6(58, 133), true},
		{"neighbour advertisement", ipv6(58, 136), true},
		{"redirect", ipv6(58, 137), true},
		{"echo request (ICMPv6 128)", ipv6(58, 128), false},
		{"ICMPv6 type 138", ipv6(58, 138), false},
		{"IPv6 UDP", ipv6(17, 135), false},
		{"DNS", ipv4udp(5, 0, 40000, 53), false},
		{"NTP", ipv4udp(5, 0, 123, 123), false},
		{"a later fragment", ipv4udp(5, 0x00b9, 68, 67), false},
		{"TCP", append(eth(etherTypeIPv4, make([]byte, 40)...)[:14+9], 6), false},
		{"LLDP", eth(0x88cc, make([]byte, 40)...), false},
		{"too short", []byte{1, 2, 3}, false},
		{"ARP header only, cut", eth(etherTypeARP), true},
	}
	for _, c := range cases {
		got := runFilter(prog, c.pkt)
		if c.want && got != snapLen || !c.want && got != 0 {
			t.Errorf("%s: filter returned %d", c.name, got)
		}
	}
}

func TestTheFilterIsAForwardOnlyProgramThatEndsInAReturn(t *testing.T) {
	prog := captureFilter()
	if len(prog) == 0 || len(prog) > 4096 {
		t.Fatalf("%d instructions", len(prog))
	}
	if last := prog[len(prog)-1]; last.Code != opRet {
		t.Fatalf("the last instruction is %#x, not a return", last.Code)
	}
	for i, in := range prog {
		if in.Code != opRet && (i+1+int(in.Jt) >= len(prog) || i+1+int(in.Jf) >= len(prog)) {
			t.Errorf("instruction %d jumps past the end", i)
		}
	}
}
