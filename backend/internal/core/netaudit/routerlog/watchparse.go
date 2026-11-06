package routerlog

import (
	"bytes"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// WatchedKind says which of the two watched messages a line was.
type WatchedKind string

const (
	// SignInFailed is a failed sign-in to the router's own web page: a wrong password or a wrong captcha.
	SignInFailed WatchedKind = "sign_in_failed"
	// Deauth is the router reporting that a device was disconnected from the Wi-Fi.
	Deauth WatchedKind = "deauth"
)

// Watched is everything kept of such a line: its kind and, for a disconnect, the reason code. The router's name, the address
// that tried to sign in, the hardware address, the interface and every other word are dropped while parsing, so a Watched can only
// be counted (Constitution I.8).
type Watched struct {
	Kind   WatchedKind
	Reason int // the 802.11 reason code of a disconnect, 1 to 65535; 0 for a sign-in
}

const (
	httpdTag    = " HTTPD:"
	wlcTag      = " wlceventd:"
	signInShape = "[LOGIN][http][Web]"
	maxReason   = 65535
)

// IsWatched is a cheap test of whether a datagram can be a failed sign-in or a disconnect, so every other line is sent away
// without a parse.
func IsWatched(datagram []byte) bool {
	head := datagram[:min(len(datagram), tagWindow)]
	if bytes.Contains(head, []byte(httpdTag)) {
		return bytes.Contains(datagram, []byte("[LOGIN]")) && (bytes.Contains(datagram, []byte("] failed (")) || bytes.Contains(datagram, []byte("captcha error (")))
	}
	return bytes.Contains(head, []byte(wlcTag)) && bytes.Contains(datagram, []byte("Deauth_ind "))
}

// ParseWatched recognises exactly two messages of the stock RT-AX86U firmware, as captured on the wire (2026-11-06):
//
//	<PRI>Mon DD HH:MM:SS HOST HTTPD: [LOGIN][http][Web] failed|captcha error (IPV4)
//	<PRI>Mon DD HH:MM:SS HOST wlceventd: wlceventd_proc_event(N): IFACE: Deauth_ind MAC, status: N, reason: TEXT (CODE), rssi:N
//
// The address and the hardware address are read to check the shape and are never kept. A good sign-in, an association and every
// other line give false.
func ParseWatched(datagram []byte) (Watched, bool) {
	if len(datagram) == 0 || len(datagram) > maxDatagram || !validPriority(datagram) {
		return Watched{}, false
	}
	f := strings.Fields(string(datagram[strings.IndexByte(string(datagram), '>')+1:]))
	if len(f) < 7 {
		return Watched{}, false
	}
	switch f[4] {
	case "HTTPD:":
		return parseSignInFailure(f[5:])
	case "wlceventd:":
		return parseDeauth(f[5:])
	}
	return Watched{}, false
}

func parseSignInFailure(msg []string) (Watched, bool) {
	if msg[0] != signInShape {
		return Watched{}, false
	}
	rest := msg[1:]
	switch {
	case len(rest) == 2 && rest[0] == "failed":
	case len(rest) == 3 && rest[0] == "captcha" && rest[1] == "error":
	default:
		return Watched{}, false
	}
	addr, ok := strings.CutPrefix(rest[len(rest)-1], "(")
	addr, ok2 := strings.CutSuffix(addr, ")")
	if ip, err := netip.ParseAddr(addr); !ok || !ok2 || err != nil || !ip.Is4() {
		return Watched{}, false
	}
	return Watched{Kind: SignInFailed}, true
}

func parseDeauth(msg []string) (Watched, bool) {
	if len(msg) < 8 || !strings.HasPrefix(msg[0], "wlceventd_proc_event(") || !strings.HasSuffix(msg[0], "):") ||
		!strings.HasSuffix(msg[1], ":") || msg[2] != "Deauth_ind" {
		return Watched{}, false
	}
	if hw, err := net.ParseMAC(strings.TrimSuffix(msg[3], ",")); err != nil || len(hw) != 6 {
		return Watched{}, false
	}
	n, ok := reasonCode(msg)
	return Watched{Kind: Deauth, Reason: n}, ok
}

// reasonCode reads the code that is the last word before "rssi:", in brackets and followed by a comma: "(3),".
func reasonCode(msg []string) (int, bool) {
	rssi := -1
	for i, w := range msg {
		if strings.HasPrefix(w, "rssi:") {
			rssi = i
		}
	}
	if rssi < 5 {
		return 0, false
	}
	code, ok := strings.CutPrefix(msg[rssi-1], "(")
	code, ok2 := strings.CutSuffix(code, "),")
	n, err := strconv.Atoi(code)
	return n, ok && ok2 && err == nil && n >= 1 && n <= maxReason
}
