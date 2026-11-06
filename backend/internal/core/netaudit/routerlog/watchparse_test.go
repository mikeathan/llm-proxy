package routerlog

import (
	"strings"
	"testing"
)

// These lines are the ones the router sent on the wire on 2026-11-06 (captured on vertex), with the router's id and the
// hardware addresses replaced by invented ones.
const (
	wireFailed  = "<14>Nov  6 12:04:00 RT-AX86U-XXXX-XXXXXXX-C HTTPD: [LOGIN][http][Web] failed (192.168.50.251)"
	wireCaptcha = "<14>Nov  6 12:04:11 RT-AX86U-XXXX-XXXXXXX-C HTTPD: [LOGIN][http][Web] captcha error (192.168.50.251)"
	wireSuccess = "<14>Nov  6 12:04:50 RT-AX86U-XXXX-XXXXXXX-C HTTPD: [LOGIN][http][Web] successed (192.168.50.251)"
	wireDeauth  = "<13>Nov  6 12:05:25 RT-AX86U-XXXX-XXXXXXX-C wlceventd: wlceventd_proc_event(645): eth7: Deauth_ind 02:aa:bb:00:00:01, status: 0, reason: Deauthenticated because sending station is leaving (or has left) IBSS or ESS (3), rssi:0"
	wireAuth    = "<13>Nov  6 12:05:15 RT-AX86U-XXXX-XXXXXXX-C wlceventd: wlceventd_proc_event(685): eth7: Auth 02:aa:bb:00:00:01, status: Successful (0), rssi:0"
)

func TestParseWatched_AFailedSignInAndACaptchaErrorBothCountAsAFailure(t *testing.T) {
	for _, line := range []string{wireFailed, wireCaptcha} {
		got, ok := ParseWatched([]byte(line))
		if !ok || got != (Watched{Kind: SignInFailed}) {
			t.Errorf("%q = %+v, %v", line, got, ok)
		}
	}
}

func TestParseWatched_ADisconnectKeepsOnlyItsReasonCode(t *testing.T) {
	got, ok := ParseWatched([]byte(wireDeauth))
	if !ok || got != (Watched{Kind: Deauth, Reason: 3}) {
		t.Fatalf("got %+v, %v", got, ok)
	}
	inactive := strings.Replace(wireDeauth, "Deauthenticated because sending station is leaving (or has left) IBSS or ESS (3)", "Disassociated due to inactivity (4)", 1)
	if got, ok := ParseWatched([]byte(inactive)); !ok || got.Reason != 4 {
		t.Errorf("inactivity = %+v, %v", got, ok)
	}
}

func TestParseWatched_RefusesWhatIsNotExactlyOneOfThem(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"a good sign-in":      wireSuccess,
		"an association":      wireAuth,
		"no priority":         strings.TrimPrefix(wireFailed, "<14>"),
		"another tag":         strings.Replace(wireFailed, "HTTPD:", "sshd:", 1),
		"another outcome":     strings.Replace(wireFailed, "failed", "locked", 1),
		"no address":          strings.Replace(wireFailed, "(192.168.50.251)", "()", 1),
		"a name for address":  strings.Replace(wireFailed, "192.168.50.251", "banana", 1),
		"no reason code":      strings.Replace(wireDeauth, "(3), ", "", 1),
		"a huge reason code":  strings.Replace(wireDeauth, "(3)", "(99999)", 1),
		"a bad hardware addr": strings.Replace(wireDeauth, "02:aa:bb:00:00:01", "zz", 1),
		"too long":            wireFailed + strings.Repeat(" x", maxDatagram),
	}
	for name, line := range cases {
		if got, ok := ParseWatched([]byte(line)); ok {
			t.Errorf("%s: recognised %+v", name, got)
		}
	}
}

func TestIsWatched_IsACheapTestThatTurnsEveryOtherLineAway(t *testing.T) {
	for _, line := range []string{wireFailed, wireCaptcha, wireDeauth} {
		if !IsWatched([]byte(line)) {
			t.Errorf("%q not watched", line)
		}
	}
	for _, line := range []string{wireDrop, wireSuccess, wireAuth, "<14>Nov  6 12:04:00 R dnsmasq[1]: query[A] x from 1.2.3.4"} {
		if IsWatched([]byte(line)) {
			t.Errorf("%q watched", line)
		}
	}
}

func TestWatched_HoldsNoAddressAndNoHardwareAddress(t *testing.T) {
	// A Watched has a kind and a number: nothing of the line, least of all an address, can have been kept.
	if got, _ := ParseWatched([]byte(wireDeauth)); got != (Watched{Kind: Deauth, Reason: 3}) {
		t.Errorf("got %+v", got)
	}
}
