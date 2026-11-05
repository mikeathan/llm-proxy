package routerlog

import (
	"bytes"
	"net/netip"
	"strings"
)

// Query is everything kept of a DNS query line: the record type, the name that was asked for and which device asked. The router's
// name, the process id and every other word are dropped while parsing. The name is a plain DNS name vetted to letters, digits,
// hyphen, underscore and dots, in lower case; it is shown as text and never sent to a model as it is.
type Query struct {
	Type string
	Name string
	From netip.Addr
}

const (
	maxNameLen  = 253
	maxLabelLen = 63
	maxTypeLen  = 10
)

// IsQuery is a cheap test of whether a datagram can be a DNS query line: one search near the start for the daemon's tag followed
// by "query[". Everything else (the DHCP lines, the firewall lines, the daemon's replies) fails it without a parse.
func IsQuery(datagram []byte) bool {
	head := datagram[:min(len(datagram), tagWindow+len(queryTag)+maxPIDLen+8)]
	i := bytes.Index(head, []byte(dnsTag))
	return i >= 0 && bytes.Contains(head[i:], []byte(queryTag))
}

const (
	dnsTag    = " dnsmasq["
	queryTag  = ": query["
	maxPIDLen = 10
)

// ParseQuery recognises one message of the stock RT-AX86U firmware, as captured on the wire (2026-11-05):
//
//	<PRI>Mon DD HH:MM:SS HOST dnsmasq[PID]: query[TYPE] NAME from ADDRESS
//
// It drops a lookup made by the router itself (loopback), every reverse lookup (*.arpa), a name that is not a plain DNS name and
// any line that is not exactly this shape. Whether the sender is this computer is the caller's to decide: it knows its own addresses.
func ParseQuery(datagram []byte) (Query, bool) {
	if len(datagram) == 0 || len(datagram) > maxDatagram || !validPriority(datagram) {
		return Query{}, false
	}
	f := strings.Fields(string(datagram[strings.IndexByte(string(datagram), '>')+1:]))
	// month, day, time, host, tag, "query[TYPE]", name, "from", address
	if len(f) != 9 || !isDNSTag(f[4]) || f[7] != "from" {
		return Query{}, false
	}
	typ, ok := queryType(f[5])
	if !ok {
		return Query{}, false
	}
	from, err := netip.ParseAddr(f[8])
	if err != nil || from.IsLoopback() || from.IsUnspecified() {
		return Query{}, false
	}
	name, ok := plainDNSName(f[6])
	if !ok {
		return Query{}, false
	}
	return Query{Type: typ, Name: name, From: from.Unmap()}, true
}

// isDNSTag matches "dnsmasq[digits]:".
func isDNSTag(tag string) bool {
	rest, ok := strings.CutPrefix(tag, "dnsmasq[")
	if !ok || !strings.HasSuffix(rest, "]:") {
		return false
	}
	pid := strings.TrimSuffix(rest, "]:")
	if pid == "" || len(pid) > maxPIDLen {
		return false
	}
	for _, c := range pid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// queryType reads "query[TYPE]" and accepts a short run of upper-case letters and digits as the type.
func queryType(word string) (string, bool) {
	rest, ok := strings.CutPrefix(word, "query[")
	if !ok || !strings.HasSuffix(rest, "]") {
		return "", false
	}
	typ := strings.TrimSuffix(rest, "]")
	if typ == "" || len(typ) > maxTypeLen {
		return "", false
	}
	for _, c := range typ {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", false
		}
	}
	return typ, true
}

// plainDNSName vets and lower-cases a name: dot-separated labels of letters, digits, hyphen and underscore, none empty or longer
// than 63, none starting or ending with a hyphen, the whole at most 253. A reverse lookup (under in-addr.arpa or ip6.arpa) is not
// kept. An underscore is allowed because service names start a label with one.
func plainDNSName(raw string) (string, bool) {
	name := strings.ToLower(raw)
	if name == "" || len(name) > maxNameLen || name == "arpa" || strings.HasSuffix(name, ".arpa") {
		return "", false
	}
	for label := range strings.SplitSeq(name, ".") {
		if !plainLabel(label) {
			return "", false
		}
	}
	return name, true
}

func plainLabel(label string) bool {
	if label == "" || len(label) > maxLabelLen || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
