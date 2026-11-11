package routeprobe

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

var testFamilies = families{inet6: 10}

// extErr builds a sock_extended_err with the router's sockaddr after it.
func extErr(origin, typ, code byte, from netip.Addr) []byte {
	b := make([]byte, extErrLen)
	b[4], b[5], b[6] = origin, typ, code
	if from.Is4() {
		sa := make([]byte, sockaddrIn)
		binary.NativeEndian.PutUint16(sa, familyInet)
		a := from.As4()
		copy(sa[4:], a[:])
		return append(b, sa...)
	}
	sa := make([]byte, sockaddrIn6)
	binary.NativeEndian.PutUint16(sa, testFamilies.inet6)
	a := from.As16()
	copy(sa[8:], a[:])
	return append(b, sa...)
}

func TestTheKernelsAnswerSaysWhichRouterAnsweredAndHow(t *testing.T) {
	router, router6 := netip.MustParseAddr("62.252.1.1"), netip.MustParseAddr("2001:db8::1")
	cases := []struct {
		name string
		data []byte
		kind Kind
		from netip.Addr
	}{
		{"time ran out on the way", extErr(originICMP, icmpTimeOut, 0, router), Hop, router},
		{"the address answered: its port was closed", extErr(originICMP, icmpUnreach, icmpPortClosed, router), Reached, router},
		{"a router could not reach it", extErr(originICMP, icmpUnreach, 1, router), Unreachable, router},
		{"IPv6 time ran out", extErr(originICMP6, icmp6TimeOut, 0, router6), Hop, router6},
		{"IPv6 port closed", extErr(originICMP6, icmp6Unreach, icmp6PortGone, router6), Reached, router6},
		{"IPv6 no route", extErr(originICMP6, icmp6Unreach, 0, router6), Unreachable, router6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, from, ok := parseExtendedErr(c.data, testFamilies)
			if !ok || kind != c.kind || from != c.from {
				t.Fatalf("%v %v %v", kind, from, ok)
			}
		})
	}
}

func TestAnAnswerOfAnotherOriginOrCutShortIsIgnored(t *testing.T) {
	router := netip.MustParseAddr("62.252.1.1")
	local := extErr(1, 0, 0, router) // SO_EE_ORIGIN_LOCAL: the kernel's own complaint, not a router's
	for name, data := range map[string][]byte{
		"local origin":      local,
		"too short":         extErr(originICMP, icmpTimeOut, 0, router)[:10],
		"no sockaddr":       extErr(originICMP, icmpTimeOut, 0, router)[:extErrLen],
		"cut sockaddr":      extErr(originICMP, icmpTimeOut, 0, router)[:extErrLen+6],
		"unknown family":    append(make([]byte, extErrLen-12), append([]byte{0, 0, 0, 0, originICMP, icmpTimeOut, 0, 0, 0, 0, 0, 0}, make([]byte, sockaddrIn)...)...),
		"cut IPv6 sockaddr": extErr(originICMP6, icmp6TimeOut, 0, netip.MustParseAddr("2001:db8::1"))[:extErrLen+20],
	} {
		if _, _, ok := parseExtendedErr(data, testFamilies); ok {
			t.Errorf("%s was read", name)
		}
	}
}
