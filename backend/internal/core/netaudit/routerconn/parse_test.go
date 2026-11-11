package routerconn

import (
	"strings"
	"testing"
)

const sample = `ipv4     2 tcp      6 2310 ESTABLISHED src=192.168.50.220 dst=52.48.210.116 sport=55936 dport=8883 src=52.48.210.116 dst=192.168.0.137 sport=8883 dport=55936 [ASSURED] mark=3504 iqprio=0 swaccel=4 hwaccel=0 use=2
ipv4     2 tcp      6 3 TIME_WAIT src=192.168.50.125 dst=192.168.50.1 sport=54492 dport=7788 src=192.168.50.1 dst=192.168.50.125 sport=7788 dport=54492 [ASSURED] mark=41943047 use=2
ipv4     2 udp      17 29 src=192.168.50.31 dst=8.8.8.8 sport=40000 dport=53 src=8.8.8.8 dst=192.168.0.137 sport=53 dport=40000 mark=0 use=2
ipv4     2 icmp     1 29 src=192.168.50.31 dst=1.1.1.1 type=8 code=0 id=1 src=1.1.1.1 dst=192.168.0.137 type=0 code=0 id=1 use=1
ipv4     2 udp      17 29 src=192.168.50.31 dst=224.0.0.251 sport=5353 dport=5353 [UNREPLIED] src=224.0.0.251 dst=192.168.50.31 sport=5353 dport=5353 use=1
ipv4     2 tcp      6 30 SYN_SENT src=192.168.50.32 dst=100.64.1.1 sport=1 dport=2 [UNREPLIED] src=100.64.1.1 dst=192.168.50.32 sport=2 dport=1 use=1
ipv6     10 tcp     6 100 ESTABLISHED src=fd00::5 dst=2606:4700::1111 sport=5000 dport=443 src=2606:4700::1111 dst=fd00::5 sport=443 dport=5000 use=2
this is not a line
ipv4 2 tcp 6 1 ESTABLISHED src=nonsense dst=1.2.3.4 sport=1 dport=2
`

func TestParseKeepsOnlyDeviceToInternetConnectionsInTheDevicesDirection(t *testing.T) {
	conns, lines, truncated := Parse(strings.NewReader(sample))
	if lines != 9 || truncated || len(conns) != 3 {
		t.Fatalf("lines=%d truncated=%v conns=%+v", lines, truncated, conns)
	}
	first := conns[0]
	if first.Device.String() != "192.168.50.220" || first.Remote.String() != "52.48.210.116" || first.Port != 8883 || first.Proto != "tcp" || first.State != "ESTABLISHED" {
		t.Errorf("first: %+v", first)
	}
	if conns[1].Proto != "udp" || conns[1].Port != 53 || conns[1].State != "" {
		t.Errorf("udp: %+v", conns[1])
	}
	if conns[2].Device.String() != "fd00::5" || conns[2].Remote.String() != "2606:4700::1111" {
		t.Errorf("ipv6: %+v", conns[2])
	}
}

func TestParseStopsAtTheLineLimitAndSaysSo(t *testing.T) {
	line := "ipv4 2 tcp 6 1 ESTABLISHED src=192.168.1.2 dst=8.8.4.4 sport=1 dport=443 src=8.8.4.4 dst=1.1.1.1 sport=443 dport=1\n"
	conns, lines, truncated := Parse(strings.NewReader(strings.Repeat(line, MaxLines+5)))
	if lines != MaxLines || !truncated || len(conns) != MaxLines {
		t.Errorf("lines=%d truncated=%v conns=%d", lines, truncated, len(conns))
	}
}

func TestAnUnusualWordInALineIsNeverKept(t *testing.T) {
	conns, _, _ := Parse(strings.NewReader("ipv4 2 tcp 6 1 <script>alert(1)</script> src=192.168.1.2 dst=8.8.4.4 sport=1 dport=443 use=2\n"))
	if len(conns) != 1 || conns[0].State != "" {
		t.Errorf("%+v", conns)
	}
}
