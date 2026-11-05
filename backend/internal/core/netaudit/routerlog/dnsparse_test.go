package routerlog

import (
	"net/netip"
	"strings"
	"testing"
)

// The lines are the shape the router sent on 2026-11-05 (captured on udp 514), with the names and addresses of a documentation
// network.
const dnsHeader = "<30>Nov  5 12:54:33 RT-AX86U-5A60-6FE90A5-C "

func dnsLine(body string) []byte { return []byte(dnsHeader + body) }

func TestParseQuery_KeepsTheTypeTheNameAndWhoAsked(t *testing.T) {
	q, ok := ParseQuery(dnsLine("dnsmasq[31037]: query[A] ccp-lh.googleusercontent.com from 192.168.50.188"))
	if !ok || q.Type != "A" || q.Name != "ccp-lh.googleusercontent.com" || q.From != netip.MustParseAddr("192.168.50.188") {
		t.Fatalf("query = %+v ok = %v", q, ok)
	}
	if q, ok := ParseQuery(dnsLine("dnsmasq[31037]: query[AAAA] ccp-lh.googleusercontent.com from 192.168.50.188")); !ok || q.Type != "AAAA" {
		t.Fatalf("AAAA = %+v ok = %v", q, ok)
	}
}

func TestParseQuery_LowerCasesTheNameAndOnlyTrustsTheRoutersShape(t *testing.T) {
	if _, ok := ParseQuery([]byte("<30>Nov  5 12:54:33 ANY HOST NAME dnsmasq[1]: query[A] Example.COM from 192.168.50.9")); ok {
		t.Fatal("a header with extra words is not the router's shape")
	}
	q, ok := ParseQuery(dnsLine("dnsmasq[1]: query[A] Example.COM from 192.168.50.9"))
	if !ok || q.Name != "example.com" {
		t.Fatalf("query = %+v ok = %v; names are compared in lower case", q, ok)
	}
}

func TestParseQuery_DropsWhatIsNotADeviceLookup(t *testing.T) {
	for name, body := range map[string]string{
		"the router's own lookup":         "dnsmasq[1]: query[A] example.com from 127.0.0.1",
		"IPv6 loopback":                   "dnsmasq[1]: query[AAAA] example.com from ::1",
		"a reverse lookup":                "dnsmasq[1]: query[PTR] 243.147.66.172.in-addr.arpa from 192.168.50.188",
		"a reverse lookup, IPv6":          "dnsmasq[1]: query[PTR] 1.0.0.0.ip6.arpa from 192.168.50.188",
		"a reply line":                    "dnsmasq[1]: reply example.com is 93.184.216.34",
		"a forwarded line":                "dnsmasq[1]: forwarded example.com to 1.1.1.1",
		"a cached line":                   "dnsmasq[1]: cached example.com is 93.184.216.34",
		"the DHCP daemon":                 "dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.9 aa:bb:cc:00:00:01 host",
		"no sender":                       "dnsmasq[1]: query[A] example.com from",
		"a sender that is no address":     "dnsmasq[1]: query[A] example.com from not-an-address",
		"a name with a space trick":       "dnsmasq[1]: query[A] exa mple.com from 192.168.50.9",
		"a name with markup":              "dnsmasq[1]: query[A] <script>.com from 192.168.50.9",
		"an empty label":                  "dnsmasq[1]: query[A] a..b from 192.168.50.9",
		"a name with an underscore start": "dnsmasq[1]: query[A] -a.com from 192.168.50.9",
		"a name that is too long":         "dnsmasq[1]: query[A] " + strings.Repeat("a", 64) + ".com from 192.168.50.9",
		"a type that is not a type":       "dnsmasq[1]: query[a;b] example.com from 192.168.50.9",
		"a bad pid":                       "dnsmasq[x]: query[A] example.com from 192.168.50.9",
	} {
		if q, ok := ParseQuery(dnsLine(body)); ok {
			t.Errorf("%s: kept %+v", name, q)
		}
	}
	if _, ok := ParseQuery(nil); ok {
		t.Error("an empty datagram")
	}
	if _, ok := ParseQuery(append(dnsLine("dnsmasq[1]: query[A] example.com from 192.168.50.9"), make([]byte, maxDatagram)...)); ok {
		t.Error("a datagram longer than the longest line read")
	}
	if _, ok := ParseQuery([]byte("dnsmasq[1]: query[A] example.com from 192.168.50.9")); ok {
		t.Error("a line with no syslog priority")
	}
}

func TestParseQuery_AcceptsServiceNamesWithUnderscores(t *testing.T) {
	// mDNS-style and DNS-SD names legitimately start a label with an underscore.
	if q, ok := ParseQuery(dnsLine("dnsmasq[1]: query[SRV] _googlecast._tcp.local from 192.168.50.9")); !ok || q.Name != "_googlecast._tcp.local" {
		t.Fatalf("query = %+v ok = %v", q, ok)
	}
}

func TestIsQuery_IsACheapTestThatTurnsEverythingElseAway(t *testing.T) {
	if !IsQuery(dnsLine("dnsmasq[31037]: query[A] example.com from 192.168.50.9")) {
		t.Fatal("a query line must pass")
	}
	for _, body := range []string{"dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.9 aa:bb:cc:00:00:01", "kernel: DROP IN=eth0 OUT= SRC=203.0.113.9", "hostapd: wl0.1: STA aa:bb:cc:00:00:01 IEEE 802.11: associated", "dnsmasq[1]: reply example.com is 1.2.3.4"} {
		if IsQuery(dnsLine(body)) {
			t.Errorf("%q passed the cheap test", body)
		}
	}
}

func BenchmarkParseQuery(b *testing.B) {
	line := dnsLine("dnsmasq[31037]: query[A] ccp-lh.googleusercontent.com from 192.168.50.188")
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := ParseQuery(line); !ok {
			b.Fatal("not parsed")
		}
	}
}
