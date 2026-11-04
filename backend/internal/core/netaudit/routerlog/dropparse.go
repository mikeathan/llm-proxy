package routerlog

import (
	"bytes"
	"net/netip"
	"strconv"
	"strings"
)

// Drop is everything kept of a firewall line: the protocol and the destination port. The sender's address, the router's name
// and every other word are dropped while parsing, so a Drop can only be counted (Constitution I.8).
type Drop struct {
	// Proto is "tcp", "udp", "icmp" or "other"; the line's own text is never kept.
	Proto string
	// Port is the destination port of a tcp or udp packet, and 0 for anything else.
	Port int
}

// cgnat is the shared address space of carrier-grade NAT, which is neither private nor the internet's.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// IsDrop is a cheap test of whether a datagram can be a firewall drop line, so everything else is sent away without a parse.
func IsDrop(datagram []byte) bool {
	return bytes.Contains(datagram[:min(len(datagram), tagWindow)], []byte(" kernel: DROP "))
}

// ParseDrop recognises one message of the stock RT-AX86U firmware, as captured on the wire (2026-11-04):
//
//	<PRI>Mon DD HH:MM:SS HOST kernel: DROP IN=IFACE OUT= MAC=... SRC=IPV4 DST=IPV4 ... PROTO=TCP ... DPT=PORT ...
//
// It counts only a packet that came from the internet: the inbound interface is named and the source is a public IPv4 address
// (not private, loopback, link-local, multicast or carrier-grade NAT). The source is read to decide that and is never kept.
// Anything else, including a line that is not exactly this shape, gives false.
func ParseDrop(datagram []byte) (Drop, bool) {
	if len(datagram) == 0 || len(datagram) > maxDatagram || !validPriority(datagram) {
		return Drop{}, false
	}
	f := strings.Fields(string(datagram[strings.IndexByte(string(datagram), '>')+1:]))
	if len(f) < 8 || f[4] != "kernel:" || f[5] != "DROP" {
		return Drop{}, false
	}
	in, src, proto, dpt := dropFields(f[6:])
	if in == "" || !fromTheInternet(src) {
		return Drop{}, false
	}
	return dropOf(proto, dpt)
}

// dropFields picks the four words ParseDrop needs out of a line's KEY=VALUE words; any other word is ignored.
func dropFields(words []string) (in, src, proto, dpt string) {
	for _, w := range words {
		key, value, ok := strings.Cut(w, "=")
		if !ok {
			continue
		}
		switch key {
		case "IN":
			in = value
		case "SRC":
			src = value
		case "PROTO":
			proto = value
		case "DPT":
			dpt = value
		}
	}
	return in, src, proto, dpt
}

func fromTheInternet(src string) bool {
	ip, err := netip.ParseAddr(src)
	return err == nil && ip.Is4() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !cgnat.Contains(ip)
}

// dropOf names the protocol from a fixed list and reads the port of a tcp or udp packet; a tcp or udp line without a usable
// port is not counted, and an unknown protocol is "other" with no port.
func dropOf(proto, dpt string) (Drop, bool) {
	switch proto {
	case "TCP", "UDP":
		port, err := strconv.Atoi(dpt)
		if err != nil || port < 1 || port > 65535 {
			return Drop{}, false
		}
		return Drop{Proto: strings.ToLower(proto), Port: port}, true
	case "ICMP":
		return Drop{Proto: "icmp"}, true
	case "":
		return Drop{}, false
	}
	return Drop{Proto: "other"}, true
}
