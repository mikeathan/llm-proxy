package neighbors

import (
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"
)

// The /proc/net/arp layout of Linux's net/ipv4/arp.c: a header, then one row per neighbour. The rows are what a router
// (192.168.1.1), a phone and a dead address look like; the dead one is the kernel's incomplete entry (flags 0x0, zero MAC).
const arpSample = `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         aa:bb:cc:00:00:01     *        eth0
192.168.1.20     0x1         0x2         DA:A1:19:2C:00:09     *        eth0
192.168.1.99     0x1         0x0         00:00:00:00:00:00     *        eth0
172.17.0.2       0x1         0x2         02:42:ac:11:00:02     *        docker0
`

func TestParseARP_KeepsCompleteEntriesOnly(t *testing.T) {
	got, err := ParseARP(strings.NewReader(arpSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 complete entries (the incomplete one is a dead address), got %+v", got)
	}
	if got[0].Addr != netip.MustParseAddr("192.168.1.1") || got[0].MAC != "aa:bb:cc:00:00:01" || got[0].Interface != "eth0" {
		t.Errorf("first entry = %+v", got[0])
	}
	if got[1].MAC != "da:a1:19:2c:00:09" {
		t.Errorf("a MAC is stored lower case: %q", got[1].MAC)
	}
}

func TestParseARP_IgnoresRowsItCannotRead(t *testing.T) {
	junk := "IP address HW type Flags HW address Mask Device\n" +
		"not-an-ip 0x1 0x2 aa:bb:cc:00:00:01 * eth0\n" +
		"192.168.1.5 0x1 0x2 not-a-mac * eth0\n" +
		"192.168.1.6 0x1 0x2 aa:bb:cc:00:00:06 * eth0\n" +
		"short row\n" +
		"192.168.1.7 0x1 0x2 ff:ff:ff:ff:ff:ff * eth0\n"
	got, err := ParseARP(strings.NewReader(junk))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Addr != netip.MustParseAddr("192.168.1.6") {
		t.Errorf("only the readable, unicast row survives: %+v", got)
	}
}

func TestParseARP_IsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("IP address HW type Flags HW address Mask Device\n")
	for i := 0; i < maxEntries+50; i++ {
		b.WriteString("10.0.")
		b.WriteString(string(rune('0' + i%10)))
		b.WriteString(".1 0x1 0x2 aa:bb:cc:00:00:01 * eth0\n")
	}
	got, _ := ParseARP(strings.NewReader(b.String()))
	if len(got) > maxEntries {
		t.Errorf("read %d entries, the cap is %d", len(got), maxEntries)
	}
}

// /proc/net/route prints addresses as little-endian hex; the default route has destination and mask 00000000.
const routeSample = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

func TestParseDefaultGateway(t *testing.T) {
	got, err := ParseDefaultGateway(strings.NewReader(routeSample))
	if err != nil || got != netip.MustParseAddr("192.168.1.1") {
		t.Errorf("gateway = %v, %v; want 192.168.1.1", got, err)
	}
	if _, err := ParseDefaultGateway(strings.NewReader("Iface\tDestination\tGateway\n")); err == nil {
		t.Error("no default route is an error, not an empty answer")
	}
}

func TestLocallyAdministered(t *testing.T) {
	for mac, want := range map[string]bool{"da:a1:19:2c:00:09": true, "02:42:ac:11:00:02": true, "aa:bb:cc:00:00:01": true, "00:11:22:33:44:55": false, "bc:ee:7b:00:00:01": false} {
		if got := LocallyAdministered(mac); got != want {
			t.Errorf("LocallyAdministered(%s) = %v, want %v", mac, got, want)
		}
	}
}

func TestReader_ReadsFilesAndReportsUnavailableOffLinux(t *testing.T) {
	dir := t.TempDir()
	arp, route := dir+"/arp", dir+"/route"
	if err := os.WriteFile(arp, []byte(arpSample), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(route, []byte(routeSample), 0o600); err != nil {
		t.Fatal(err)
	}
	table, err := Reader{ARPPath: arp, RoutePath: route, goos: "linux"}.Read()
	if err != nil || len(table.Entries) != 3 || table.Gateway != netip.MustParseAddr("192.168.1.1") {
		t.Fatalf("table = %+v, %v", table, err)
	}
	if _, err := (Reader{ARPPath: arp, RoutePath: route, goos: "darwin"}).Read(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("off Linux must say unavailable, got %v", err)
	}
	if _, err := (Reader{ARPPath: dir + "/none", RoutePath: route, goos: "linux"}).Read(); err == nil || errors.Is(err, ErrUnavailable) {
		t.Errorf("a missing table is a read failure, not 'unavailable': %v", err)
	}
}
