package neighbors

import (
	"encoding/binary"
	"net"
	"net/netip"
	"strings"
)

// The IPv6 neighbour table is not in a file; the kernel hands it out over netlink (RTM_GETNEIGH). A reply message is a
// struct ndmsg followed by attributes (struct rtattr: length, type, payload, padded to four bytes). This file parses one
// message and is plain Go, so it is tested everywhere; the Linux-only reader that asks the kernel is in neighbors6_linux.go.
const (
	ndmsgLen   = 12 // family u8, three pad bytes, ifindex i32, state u16, flags u8, type u8
	rtattrHead = 4  // length u16, type u16

	attrDst    = 1 // NDA_DST: the neighbour's address
	attrLLAddr = 2 // NDA_LLADDR: its hardware address

	stateReachable = 0x02
	stateStale     = 0x04
	stateDelay     = 0x08
	stateProbe     = 0x10
	// A usable entry is one that has an answer from the neighbour: not incomplete (0x01), failed (0x20), a no-ARP interface
	// (0x40) or this computer's own permanent entry (0x80).
	stateUsable = stateReachable | stateStale | stateDelay | stateProbe

	maxEntries6 = 4096
)

// ParseNeighbor reads the payload of one RTM_NEWNEIGH message for IPv6 and returns the neighbour, or false for an entry that
// has no answer, no hardware address, is a multicast address, or is malformed. ifName turns the interface index into a
// name; nil leaves the name empty.
func ParseNeighbor(data []byte, ifName func(index int) string) (Entry, bool) {
	if len(data) < ndmsgLen || data[0] != afInet6 {
		return Entry{}, false
	}
	order := binary.NativeEndian
	if order.Uint16(data[8:10])&stateUsable == 0 {
		return Entry{}, false
	}
	addr, mac, ok := neighborAttrs(data[ndmsgLen:])
	if !ok || !usableAddr(addr) || mac == "" {
		return Entry{}, false
	}
	e := Entry{Addr: addr, MAC: mac}
	if ifName != nil {
		e.Interface = ifName(int(int32(order.Uint32(data[4:8]))))
	}
	return e, true
}

func usableAddr(a netip.Addr) bool {
	return a.IsValid() && a.Is6() && !a.IsMulticast() && !a.IsUnspecified()
}

// neighborAttrs walks the attributes after the fixed header and returns the neighbour's address and hardware address. A
// length that does not fit makes the whole message unreadable.
func neighborAttrs(attrs []byte) (addr netip.Addr, mac string, ok bool) {
	order := binary.NativeEndian
	for len(attrs) >= rtattrHead {
		l := int(order.Uint16(attrs[0:2]))
		if l < rtattrHead || l > len(attrs) {
			return netip.Addr{}, "", false
		}
		payload := attrs[rtattrHead:l]
		switch order.Uint16(attrs[2:4]) {
		case attrDst:
			addr, _ = netip.AddrFromSlice(payload)
		case attrLLAddr:
			mac = unicastMAC(payload)
		}
		next := (l + 3) &^ 3
		if next >= len(attrs) {
			break
		}
		attrs = attrs[next:]
	}
	return addr, mac, true
}

// unicastMAC formats a six-byte hardware address, or returns "" for one that is not a usable unicast address.
func unicastMAC(b []byte) string {
	if len(b) != 6 || b[0]&1 == 1 || isZero(b) {
		return ""
	}
	return strings.ToLower(net.HardwareAddr(b).String())
}

// afInet6 is AF_INET6 on Linux, the only system this table is read on.
const afInet6 = 10
