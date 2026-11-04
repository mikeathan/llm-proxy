package routerlog

import (
	"strings"
	"testing"
)

// The line is the one the router sent on the wire on 2026-11-04 (captured on vertex), with the hardware addresses and the
// router's id zeroed. The destination is a private address because the router sits behind another device.
const wireDrop = "<12>Nov  4 18:17:18 RT-AX86U-XXXX-XXXXXXX-C kernel: DROP IN=eth0 OUT= MAC=00:00:00:00:00:00:00:00:00:00:00:00:08:00 SRC=64.89.163.89 DST=192.168.0.137 LEN=44 TOS=0x00 PREC=0x00 TTL=242 ID=30896 DF PROTO=TCP SPT=55994 DPT=3306 SEQ=1681228632 ACK=0 WINDOW=1025 RES=0x00 SYN URGP=0 OPT (020405A0) MARK=0x8000000"

func TestParseDrop_ReadsTheProtocolAndThePortOfARealLineAndNothingElse(t *testing.T) {
	d, ok := ParseDrop([]byte(wireDrop))
	if !ok || d.Proto != "tcp" || d.Port != 3306 {
		t.Fatalf("drop = %+v, ok = %v", d, ok)
	}
	// A Drop has two fields: nothing of the line, least of all an address, can have been kept.
	if got := (Drop{Proto: "tcp", Port: 3306}); d != got {
		t.Errorf("drop = %+v", d)
	}
}

func TestParseDrop_UdpAndAnIcmpLineWithoutAPort(t *testing.T) {
	udp := strings.Replace(strings.Replace(wireDrop, "PROTO=TCP", "PROTO=UDP", 1), "DPT=3306", "DPT=5060", 1)
	if d, ok := ParseDrop([]byte(udp)); !ok || d.Proto != "udp" || d.Port != 5060 {
		t.Errorf("udp = %+v, %v", d, ok)
	}
	icmp := "<12>Nov  4 18:17:18 R kernel: DROP IN=eth0 OUT= MAC=00 SRC=64.89.163.89 DST=192.168.0.137 LEN=28 PROTO=ICMP TYPE=8 CODE=0 MARK=0x8000000"
	if d, ok := ParseDrop([]byte(icmp)); !ok || d.Proto != "icmp" || d.Port != 0 {
		t.Errorf("icmp = %+v, %v", d, ok)
	}
}

func TestParseDrop_OnlyAPacketFromTheInternetCounts(t *testing.T) {
	for name, src := range map[string]string{
		"private":     "192.168.50.20",
		"ten":         "10.1.2.3",
		"link-local":  "169.254.1.1",
		"loopback":    "127.0.0.1",
		"cgnat":       "100.64.1.1",
		"multicast":   "224.0.0.1",
		"unspecified": "0.0.0.0",
		"ipv6":        "2001:db8::1",
		"not an ip":   "banana",
	} {
		line := strings.Replace(wireDrop, "SRC=64.89.163.89", "SRC="+src, 1)
		if d, ok := ParseDrop([]byte(line)); ok {
			t.Errorf("%s: counted %+v", name, d)
		}
	}
}

func TestParseDrop_RefusesWhatIsNotExactlyAFirewallDropLine(t *testing.T) {
	cases := map[string]string{
		"empty":                "",
		"no priority":          strings.TrimPrefix(wireDrop, "<12>"),
		"another tag":          strings.Replace(wireDrop, "kernel:", "sshd:", 1),
		"not a drop":           strings.Replace(wireDrop, "DROP", "ACCEPT", 1),
		"no inbound interface": strings.Replace(wireDrop, "IN=eth0", "IN=", 1),
		"no source":            strings.Replace(wireDrop, "SRC=64.89.163.89 ", "", 1),
		"no protocol":          strings.Replace(wireDrop, "PROTO=TCP ", "", 1),
		"port out of range":    strings.Replace(wireDrop, "DPT=3306", "DPT=70000", 1),
		"port not a number":    strings.Replace(wireDrop, "DPT=3306", "DPT=ssh", 1),
		"tcp without a port":   strings.Replace(wireDrop, "DPT=3306 ", "", 1),
		"too long":             wireDrop + strings.Repeat(" X=1", 400),
	}
	for name, line := range cases {
		if d, ok := ParseDrop([]byte(line)); ok {
			t.Errorf("%s: parsed %+v", name, d)
		}
	}
}

func TestParseDrop_AnUnknownProtocolIsCountedAsOtherAndNeverCarriesItsText(t *testing.T) {
	line := strings.Replace(wireDrop, "PROTO=TCP", "PROTO=GRE", 1)
	line = strings.Replace(line, "DPT=3306 ", "", 1)
	if d, ok := ParseDrop([]byte(line)); !ok || d.Proto != "other" || d.Port != 0 {
		t.Errorf("drop = %+v, %v", d, ok)
	}
}

func TestIsDrop_IsACheapTestThatSendsOtherLinesAway(t *testing.T) {
	if !IsDrop([]byte(wireDrop)) {
		t.Error("the real line is a drop line")
	}
	for _, other := range []string{"<30>Nov  4 18:17:58 R dnsmasq-dhcp[1]: DHCPACK(br0) 192.168.50.7 aa:bb:cc:dd:ee:ff x", "<12>Nov  4 R kernel: ACCEPT IN=eth0"} {
		if IsDrop([]byte(other)) {
			t.Errorf("%q is not a drop line", other)
		}
	}
}

func FuzzParseDrop(f *testing.F) {
	f.Add([]byte(wireDrop))
	f.Add([]byte("<12>Nov  4 R kernel: DROP IN=eth0 SRC=1.2.3.4 PROTO=TCP DPT=1"))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, ok := ParseDrop(b)
		if ok && (d.Port < 0 || d.Port > 65535 || (d.Proto != "tcp" && d.Proto != "udp" && d.Proto != "icmp" && d.Proto != "other")) {
			t.Fatalf("out-of-range result %+v", d)
		}
	})
}
