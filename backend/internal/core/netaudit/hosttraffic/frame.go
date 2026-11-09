package hosttraffic

import (
	"encoding/binary"
	"net/netip"
)

const (
	etherTypeIPv4 = 0x0800
	etherTypeIPv6 = 0x86dd
	ethHeader     = 14
	ipv4Min       = 20
	ipv6Header    = 40
	udpHeader     = 8
	tcpMin        = 20

	protoICMP = 1
	protoTCP  = 6
	protoUDP  = 17
	// fragmentBits is the fragment offset: a later fragment has no L4 header to read.
	fragmentBits = 0x1fff
)

// frame is the part of a kept frame that is read: who, which protocol, which ports and how many bytes the IP packet had. Nothing else of the
// frame is kept, and its payload is looked at only for a DNS message.
type frame struct {
	src, dst     netip.Addr
	proto        byte
	sport, dport uint16
	// length is the IP packet's own length, which says how big the packet was even though only its first bytes were kept.
	length uint64
	// payload is the kept bytes after the TCP or UDP header (empty when none were kept).
	payload []byte
}

// parseFrame reads an Ethernet frame carrying IPv4 or IPv6 and TCP, UDP or ICMP. A frame that is too short or of any other shape is not read
// (ok is false): it is counted as unreadable, never guessed.
func parseFrame(b []byte) (frame, bool) {
	if len(b) < ethHeader {
		return frame{}, false
	}
	switch binary.BigEndian.Uint16(b[12:14]) {
	case etherTypeIPv4:
		return parseIPv4(b)
	case etherTypeIPv6:
		return parseIPv6(b)
	}
	return frame{}, false
}

func parseIPv4(b []byte) (frame, bool) {
	ip := b[ethHeader:]
	if len(ip) < ipv4Min || ip[0]>>4 != 4 {
		return frame{}, false
	}
	ihl := int(ip[0]&0xf) * 4
	if ihl < ipv4Min || len(ip) < ihl {
		return frame{}, false
	}
	f := frame{proto: ip[9], length: uint64(binary.BigEndian.Uint16(ip[2:4]))}
	if f.length == 0 { // the length of a large offloaded packet may be left at zero: the frame itself is the best size left
		f.length = uint64(len(ip))
	}
	f.src, f.dst = netip.AddrFrom4([4]byte(ip[12:16])), netip.AddrFrom4([4]byte(ip[16:20]))
	if binary.BigEndian.Uint16(ip[6:8])&fragmentBits != 0 {
		return withPorts(f, nil)
	}
	return withPorts(f, ip[ihl:])
}

func parseIPv6(b []byte) (frame, bool) {
	ip := b[ethHeader:]
	if len(ip) < ipv6Header || ip[0]>>4 != 6 {
		return frame{}, false
	}
	f := frame{proto: ip[6], length: ipv6Header + uint64(binary.BigEndian.Uint16(ip[4:6]))}
	f.src, f.dst = netip.AddrFrom16([16]byte(ip[8:24])), netip.AddrFrom16([16]byte(ip[24:40]))
	return withPorts(f, ip[ipv6Header:])
}

// portsLen is how many bytes of the L4 header are read for the ports: the helper keeps no more than this of an ordinary frame.
const portsLen = 4

// withPorts reads the ports out of the first bytes of the L4 header, and the start of the payload when more was kept (a DNS message); a
// protocol that is none of TCP, UDP and ICMP is not read.
func withPorts(f frame, l4 []byte) (frame, bool) {
	switch f.proto {
	case protoICMP, protoICMPv6:
		return f, true
	case protoTCP, protoUDP:
		if l4 == nil { // a later fragment has no L4 header
			return f, true
		}
		if len(l4) < portsLen {
			return frame{}, false
		}
		f.sport, f.dport = binary.BigEndian.Uint16(l4[0:2]), binary.BigEndian.Uint16(l4[2:4])
		f.payload = payloadOf(f.proto, l4)
		return f, true
	}
	return frame{}, false
}

// payloadOf is the kept bytes after the TCP or UDP header; empty when the header itself was not kept whole.
func payloadOf(proto byte, l4 []byte) []byte {
	if proto == protoUDP {
		if len(l4) < udpHeader {
			return nil
		}
		return l4[udpHeader:]
	}
	if len(l4) < tcpMin {
		return nil
	}
	if off := int(l4[12]>>4) * 4; off >= tcpMin && off <= len(l4) {
		return l4[off:]
	}
	return nil
}

// protoICMPv6 is the next-header value of ICMP over IPv6.
const protoICMPv6 = 58
