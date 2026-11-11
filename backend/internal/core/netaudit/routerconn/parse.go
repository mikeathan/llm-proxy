// Package routerconn turns the router's connection table (/proc/net/nf_conntrack, read over SSH on the operator's press; Constitution I.15) into
// which device held connections to which internet places. Of each line only the protocol, the connection's first source and destination
// address and port, and the state are read, as typed values; every other word of the line is dropped, and nothing of it is kept as text.
package routerconn

import (
	"bufio"
	"io"
	"net/netip"
	"strconv"
	"strings"
)

// MaxLines bounds one read: lines past it are not parsed and the result says so.
const MaxLines = 20000

// Conn is one connection from a device to an internet address, in the direction the device made it.
type Conn struct {
	Device netip.Addr
	Remote netip.Addr
	Port   int
	Proto  string
	State  string
}

// protos are the protocols that carry ports and are kept; the router names them in lower case.
var protos = map[string]bool{"tcp": true, "udp": true, "udplite": true, "sctp": true}

// states are the connection states a tcp line can carry, so a word of a line is kept only when it is one of these.
var states = map[string]bool{
	"SYN_SENT": true, "SYN_RECV": true, "ESTABLISHED": true, "FIN_WAIT": true, "CLOSE_WAIT": true, "LAST_ACK": true, "TIME_WAIT": true, "CLOSE": true,
}

// Parse reads at most MaxLines lines and returns the connections from a device on this network to an internet address, how many lines it read
// and whether it stopped at the limit. A line it cannot read, a connection between two devices on this network, one to the router itself and
// one to a multicast or carrier-grade NAT address are skipped.
func Parse(r io.Reader) (conns []Conn, lines int, truncated bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 16*1024)
	for sc.Scan() {
		if lines >= MaxLines {
			return conns, lines, true
		}
		lines++
		if c, ok := parseLine(sc.Text()); ok {
			conns = append(conns, c)
		}
	}
	return conns, lines, false
}

// words are the typed-to-be words of one line: the first of each key, and the first tcp state.
type words struct{ src, dst, sport, dport, state string }

// scan picks the first source, destination and ports of a line (the original direction, which is the device's own address before the router
// rewrote it) and its tcp state; every other word is dropped.
func scan(fields []string) words {
	var w words
	for _, f := range fields {
		key, val, found := strings.Cut(f, "=")
		if !found {
			if states[f] && w.state == "" {
				w.state = f
			}
			continue
		}
		for _, slot := range []struct {
			name string
			to   *string
		}{{"src", &w.src}, {"dst", &w.dst}, {"sport", &w.sport}, {"dport", &w.dport}} {
			if key == slot.name && *slot.to == "" {
				*slot.to = val
			}
		}
	}
	return w
}

// typed turns the words into addresses and a port, or says a word is not one.
func typed(w words) (device, remote netip.Addr, port int, ok bool) {
	var err1, err2, err3, err4 error
	device, err1 = netip.ParseAddr(w.src)
	remote, err2 = netip.ParseAddr(w.dst)
	_, err3 = strconv.Atoi(w.sport)
	port, err4 = strconv.Atoi(w.dport)
	return device, remote, port, err1 == nil && err2 == nil && err3 == nil && err4 == nil && port >= 0 && port <= 65535
}

// parseLine reads one line as a device-to-internet connection, or says it is none.
func parseLine(line string) (Conn, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || (fields[0] != "ipv4" && fields[0] != "ipv6") || !protos[fields[2]] {
		return Conn{}, false
	}
	w := scan(fields[3:])
	device, remote, port, ok := typed(w)
	if !ok {
		return Conn{}, false
	}
	if !onThisNetwork(device) || !onTheInternet(remote) {
		return Conn{}, false
	}
	return Conn{Device: device, Remote: remote, Port: port, Proto: fields[2], State: w.state}, true
}

var sharedRange = netip.MustParsePrefix("100.64.0.0/10")

func onThisNetwork(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsPrivate() && !a.IsLoopback()
}

func onTheInternet(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() && !a.IsMulticast() && !sharedRange.Contains(a) &&
		a != netip.AddrFrom4([4]byte{255, 255, 255, 255})
}
