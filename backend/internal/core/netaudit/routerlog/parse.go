// Package routerlog receives the router's remote syslog and reduces it to one fact: a device with a hardware address the
// inventory does not know has just been given a lease or joined the Wi-Fi. The fact only wakes the presence check early; the
// inventory still comes from this computer's own neighbour table, so a forged datagram can cause an early look and nothing
// else (Constitution I.8).
package routerlog

import (
	"bytes"
	"net"
	"net/netip"
	"strings"
)

// Kind says which of the two recognised messages an event came from.
type Kind string

const (
	// Lease is the router's DHCP server acknowledging an address for a device.
	Lease Kind = "lease"
	// Joined is a device associating with the router's Wi-Fi.
	Joined Kind = "joined"
)

// Event is everything kept of a datagram: what kind of message it was, the hardware address in it and, for a lease, the
// device's host name when it is a plain one. The router's name, addresses and every other word are dropped while parsing.
type Event struct {
	Kind Kind
	MAC  string // lower case, colon separated
	// Name is the host name the device gave when it asked for an address, or empty. It is the device's own text: it is only
	// ever passed on to be cleaned and offered to the operator as a suggestion, never shown or sent as it is.
	Name string
}

// maxHostName is the longest host name a DHCP client may send.
const maxHostName = 63

// maxDatagram is the longest syslog line read; the router's longest in the capture was 343 bytes.
const maxDatagram = 1024

// Parse recognises exactly two messages of the stock RT-AX86U firmware, as captured on the wire (2026-10-31):
//
//	<PRI>Mon DD HH:MM:SS HOST dnsmasq-dhcp[PID]: DHCPACK(IFACE) IPV4 MAC [NAME]
//	<PRI>Mon DD HH:MM:SS HOST hostapd: IFACE: STA MAC IEEE 802.11: associated
//
// Anything else, including every firewall line, gives false. HOST is never read. NAME is kept only if it is a plain host name.
func Parse(datagram []byte) (Event, bool) {
	if len(datagram) == 0 || len(datagram) > maxDatagram || !validPriority(datagram) {
		return Event{}, false
	}
	text := string(datagram[strings.IndexByte(string(datagram), '>')+1:])
	f := strings.Fields(text)
	if len(f) < 6 { // month, day, time, host, tag and at least one word of message
		return Event{}, false
	}
	tag, msg := f[4], f[5:]
	switch {
	case isDHCPTag(tag):
		return parseLease(msg)
	case tag == "hostapd:":
		return parseJoined(msg)
	}
	return Event{}, false
}

// validPriority checks for "<" digits ">" at the start: the syslog priority, at most three digits.
func validPriority(b []byte) bool {
	if len(b) < 3 || b[0] != '<' {
		return false
	}
	i := 1
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	return i > 1 && i <= 4 && i < len(b) && b[i] == '>'
}

// isDHCPTag matches "dnsmasq-dhcp[digits]:".
func isDHCPTag(tag string) bool {
	rest, ok := strings.CutPrefix(tag, "dnsmasq-dhcp[")
	if !ok || !strings.HasSuffix(rest, "]:") {
		return false
	}
	pid := strings.TrimSuffix(rest, "]:")
	if pid == "" || len(pid) > 10 {
		return false
	}
	for _, c := range pid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func parseLease(msg []string) (Event, bool) {
	if len(msg) < 3 || !strings.HasPrefix(msg[0], "DHCPACK(") || !strings.HasSuffix(msg[0], ")") {
		return Event{}, false
	}
	if ip, err := netip.ParseAddr(msg[1]); err != nil || !ip.Is4() {
		return Event{}, false
	}
	mac, ok := unicastMAC(msg[2])
	if !ok {
		return Event{}, false
	}
	ev := Event{Kind: Lease, MAC: mac}
	if len(msg) > 3 && plainHostName(msg[3]) {
		ev.Name = msg[3]
	}
	return ev, true
}

// plainHostName accepts letters, digits, dot, hyphen and underscore, up to a host name's length. Anything else (a device
// can call itself anything) is not kept: the lease still counts, the name does not.
func plainHostName(s string) bool {
	if s == "" || len(s) > maxHostName {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// Interesting is a cheap test of whether a datagram can be one of the two messages: it looks for the tag near the start and
// reads nothing else. The firewall lines, which are most of the log, fail it at once, so they never use up the allowance.
func Interesting(datagram []byte) bool {
	head := datagram[:min(len(datagram), tagWindow)]
	return bytes.Contains(head, []byte(" dnsmasq-dhcp[")) || bytes.Contains(head, []byte(" hostapd:"))
}

// tagWindow is how far into a line the tag can start: a priority, a time, and a router name of at most a few dozen characters.
const tagWindow = 160

func parseJoined(msg []string) (Event, bool) {
	if len(msg) != 6 || !strings.HasSuffix(msg[0], ":") || msg[1] != "STA" || msg[3] != "IEEE" || msg[4] != "802.11:" || msg[5] != "associated" {
		return Event{}, false
	}
	mac, ok := unicastMAC(msg[2])
	return Event{Kind: Joined, MAC: mac}, ok
}

// unicastMAC returns a six-byte hardware address in lower case, and false for anything else, the zero address and group
// (multicast) addresses.
func unicastMAC(s string) (string, bool) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 || hw[0]&1 == 1 || strings.Trim(hw.String(), "0:") == "" {
		return "", false
	}
	return hw.String(), true
}
