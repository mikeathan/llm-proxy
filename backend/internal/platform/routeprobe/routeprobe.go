// Package routeprobe sends one probe of a route trace (Constitution I.14): a small UDP datagram to an internet address with a limited time to
// live, and reads which router said the time ran out, or that the address itself answered. It needs no privilege on Linux: the kernel hands the
// ICMP answer to the sending socket's error queue (IP_RECVERR), the way tracepath works. Nothing but the answering router's address, the answer's
// kind and the time it took leaves this package.
package routeprobe

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"time"
)

// Kind is what came back for one probe.
type Kind int

const (
	// Silent means nothing came back before the wait ran out.
	Silent Kind = iota
	// Hop means a router on the way said the time to live ran out.
	Hop
	// Reached means the address itself answered (its port was closed, which is what a probe to a high port expects).
	Reached
	// Unreachable means a router said the address cannot be reached from there; the trace stops.
	Unreachable
)

// Request is one probe: where to, with what time to live, to which port, and how long to wait.
type Request struct {
	Dst  netip.Addr
	TTL  int
	Port uint16
	Wait time.Duration
}

// Reply is what one probe found. From is the router or the address that answered (invalid when Silent).
type Reply struct {
	Kind Kind
	From netip.Addr
	RTT  time.Duration
}

// ErrUnsupported is returned where the probe cannot be sent without privilege (every system but Linux).
var ErrUnsupported = errors.New("a route trace needs Linux")

// PayloadLen is the length of a probe's fixed content: zeros, nothing about this computer.
const PayloadLen = 24

// The fields of the kernel's struct sock_extended_err and the ICMP types read from it.
const (
	extErrLen      = 16
	originICMP     = 2
	originICMP6    = 3
	icmpUnreach    = 3
	icmpPortClosed = 3
	icmpTimeOut    = 11
	icmp6Unreach   = 1
	icmp6PortGone  = 4
	icmp6TimeOut   = 3
	familyInet     = 2
	sockaddrIn     = 16
	sockaddrIn6    = 28
)

// families carries AF_INET6, which differs between systems (10 on Linux), so the parser can be tested anywhere.
type families struct{ inet6 uint16 }

// parseExtendedErr reads one IP_RECVERR or IPV6_RECVERR control message: the kind of ICMP answer and the address of the router that sent it.
// A message of any other origin or shape gives ok false and is ignored.
func parseExtendedErr(data []byte, fam families) (kind Kind, from netip.Addr, ok bool) {
	if len(data) < extErrLen {
		return Silent, netip.Addr{}, false
	}
	origin, typ, code := data[4], data[5], data[6]
	switch origin {
	case originICMP:
		kind = icmpKind(typ, code)
	case originICMP6:
		kind = icmp6Kind(typ, code)
	default:
		return Silent, netip.Addr{}, false
	}
	from, ok = offender(data[extErrLen:], fam)
	return kind, from, ok
}

func icmpKind(typ, code byte) Kind {
	switch {
	case typ == icmpTimeOut:
		return Hop
	case typ == icmpUnreach && code == icmpPortClosed:
		return Reached
	}
	return Unreachable
}

func icmp6Kind(typ, code byte) Kind {
	switch {
	case typ == icmp6TimeOut:
		return Hop
	case typ == icmp6Unreach && code == icmp6PortGone:
		return Reached
	}
	return Unreachable
}

// offender reads the sockaddr the kernel puts after the extended error: the router that answered.
func offender(sa []byte, fam families) (netip.Addr, bool) {
	if len(sa) < 2 {
		return netip.Addr{}, false
	}
	switch binary.NativeEndian.Uint16(sa[:2]) {
	case familyInet:
		if len(sa) < sockaddrIn {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte(sa[4:8])), true
	case fam.inet6:
		if len(sa) < sockaddrIn6 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom16([16]byte(sa[8:24])).Unmap(), true
	}
	return netip.Addr{}, false
}
