package routerlog

import (
	"strings"
	"testing"
)

// The lines are the shapes of the real datagrams captured from the router (RT-AX86U, stock firmware, 2026-10-31) with a
// placeholder router name, hardware addresses and device name.
const (
	ackLine    = "<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[8548]: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee:01 PHONE-1"
	ackNoName  = "<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[8548]: DHCPACK(br0) 192.168.50.127 AA:BB:CC:DD:EE:01"
	joinedLine = "<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: associated"
)

func TestParse_TheTwoShapesGiveOnlyAKindAndAHardwareAddress(t *testing.T) {
	cases := []struct {
		line string
		want Event
	}{
		{ackLine, Event{Kind: Lease, MAC: "aa:bb:cc:dd:ee:01"}},
		{ackNoName, Event{Kind: Lease, MAC: "aa:bb:cc:dd:ee:01"}},
		{joinedLine, Event{Kind: Joined, MAC: "aa:bb:cc:dd:ee:02"}},
	}
	for _, c := range cases {
		got, ok := Parse([]byte(c.line))
		if !ok || got != c.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", c.line, got, ok, c.want)
		}
	}
}

func TestParse_EverythingElseTheRouterSaysIsIgnored(t *testing.T) {
	ignored := []string{
		"<12>Oct 31 20:02:33 ROUTER kernel: DROP IN=eth0 OUT= MAC=REPLACED SRC=203.0.113.9 DST=192.0.2.1 LEN=52 PROTO=TCP SPT=40408 DPT=58376 SYN MARK=0x8000000",
		"<13>Oct 31 20:02:50 ROUTER wlceventd: wlceventd_proc_event(722): eth7: Assoc AA:BB:CC:DD:EE:02, status: Successful (0), rssi:-65",
		"<30>Oct 31 20:02:35 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: disassociated",
		"<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 WPA: pairwise key handshake completed (RSN)",
		"<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 RADIUS: starting accounting session B9B70692DE96194C",
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[8548]: DHCPREQUEST(br0) 192.168.50.127 aa:bb:cc:dd:ee:01",
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[8548]: DHCPDISCOVER(br0) aa:bb:cc:dd:ee:01",
		"<4>Oct 31 18:55:22 ROUTER kernel: wl0: random key value: PLACEHOLDER",
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee:01 PHONE-1", // no process id
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[x]: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee:01",      // process id not a number
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[1]: DHCPACK(br0) not-an-address aa:bb:cc:dd:ee:01",
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[1]: DHCPACK(br0) fe80::1 aa:bb:cc:dd:ee:01",
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee",        // short hardware address
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.127 01:00:5e:00:00:01",     // multicast
		"<30>Oct 31 20:02:50 ROUTER dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.127 00:00:00:00:00:00",     // zero
		"<30>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: associated extra", // not exactly the shape
		"<30>Oct 31 20:02:50 ROUTER evil: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: associated",
		"<30>Oct 31 20:02:50 ROUTER",
		"Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: associated", // no priority
		"<999999>Oct 31 20:02:50 ROUTER hostapd: eth7: STA aa:bb:cc:dd:ee:02 IEEE 802.11: associated",
		"",
		"\x00\xff\xfe<30>garbage",
	}
	for _, line := range ignored {
		if ev, ok := Parse([]byte(line)); ok {
			t.Errorf("Parse(%q) = %+v, want it ignored", line, ev)
		}
	}
}

func TestParse_TheRouterNameAndTheDeviceNameAreNeverRead(t *testing.T) {
	// Whatever the router calls itself, and whatever a device calls itself, is text a device chose: it can neither make an
	// event nor reach one.
	long := strings.Repeat("A", 4000)
	ev, ok := Parse([]byte("<30>Oct 31 20:02:50 " + long + " dnsmasq-dhcp[8548]: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee:01 " + long))
	if ok {
		t.Errorf("a datagram longer than a syslog line can be is ignored: %+v", ev)
	}
	ev, ok = Parse([]byte("<30>Oct 31 20:02:50 ../../etc/passwd dnsmasq-dhcp[8548]: DHCPACK(br0) 192.168.50.127 aa:bb:cc:dd:ee:01 <script>"))
	if !ok || ev != (Event{Kind: Lease, MAC: "aa:bb:cc:dd:ee:01"}) {
		t.Errorf("the event holds a kind and a hardware address only: %+v %v", ev, ok)
	}
}
