package neighbors

import (
	"encoding/binary"
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

// testdata/arp_vertex.txt is the real /proc/net/arp of the audit host (a laptop on Wi-Fi, 2026-10-30), trimmed, with the
// hardware addresses replaced by documentation addresses that keep the locally-administered bit. It shows what the kernel
// really lists after a sweep: complete entries, many dead addresses (flags 0x0, zero MAC), and one failed entry that still
// remembers an old MAC (flags 0x0 with a non-zero MAC), which must not count as a device.
func TestParseARP_RealTableFromTheAuditHost(t *testing.T) {
	f, err := os.Open("testdata/arp_vertex.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseARP(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 14 {
		t.Fatalf("want the 14 complete entries, got %d: %+v", len(got), got)
	}
	private := 0
	for _, e := range got {
		if e.Addr == netip.MustParseAddr("192.168.50.34") {
			t.Errorf("a failed entry that remembers an old MAC is not a device: %+v", e)
		}
		if e.Interface != "wlp2s0" {
			t.Errorf("interface = %q", e.Interface)
		}
		if LocallyAdministered(e.MAC) {
			private++
		}
	}
	if private != 4 {
		t.Errorf("four phones use a private address, got %d", private)
	}
	if got[0].Addr != netip.MustParseAddr("192.168.50.1") {
		t.Errorf("the router is first: %+v", got[0])
	}
}

// kernelNeighbor builds one RTM_NEWNEIGH payload in the layout of struct ndmsg and struct rtattr. These bytes are built by
// this test, so they only show that the parser follows the documented layout; TestRead6_AgainstARealKernel is the check
// against what a Linux kernel really sends.
func kernelNeighbor(state uint16, addr string, mac []byte) []byte {
	order := binary.NativeEndian
	b := make([]byte, 12)
	b[0] = afInet6
	order.PutUint32(b[4:], 3)
	order.PutUint16(b[8:], state)
	attr := func(kind uint16, payload []byte) {
		h := make([]byte, 4)
		order.PutUint16(h[0:], uint16(4+len(payload)))
		order.PutUint16(h[2:], kind)
		b = append(b, h...)
		b = append(b, payload...)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
	}
	a := netip.MustParseAddr(addr).As16()
	attr(attrDst, a[:])
	if mac != nil {
		attr(attrLLAddr, mac)
	}
	return b
}

func TestParseNeighbor_FollowsTheDocumentedLayout(t *testing.T) {
	mac := []byte{0x02, 0x00, 0x5e, 0x00, 0x53, 0x07}
	e, ok := ParseNeighbor(kernelNeighbor(stateReachable, "fe80::1", mac), func(i int) string { return "wlp2s0" })
	if !ok || e.Addr != netip.MustParseAddr("fe80::1") || e.MAC != "02:00:5e:00:53:07" || e.Interface != "wlp2s0" {
		t.Fatalf("entry = %+v, %v", e, ok)
	}
	if _, ok := ParseNeighbor(kernelNeighbor(stateStale, "2001:db8::5", mac), nil); !ok {
		t.Error("a stale entry still has an answer")
	}
}

func TestParseNeighbor_SkipsWhatIsNotANeighbourThatAnswered(t *testing.T) {
	mac := []byte{0x02, 0x00, 0x5e, 0x00, 0x53, 0x07}
	for name, data := range map[string][]byte{
		"incomplete":   kernelNeighbor(0x01, "fe80::1", nil),
		"failed":       kernelNeighbor(0x20, "fe80::1", mac),
		"permanent":    kernelNeighbor(0x80, "fe80::1", mac),
		"multicast":    kernelNeighbor(stateReachable, "ff02::1", mac),
		"no hardware":  kernelNeighbor(stateReachable, "fe80::1", nil),
		"multicast hw": kernelNeighbor(stateReachable, "fe80::1", []byte{0x33, 0x33, 0, 0, 0, 1}),
		"zero hw":      kernelNeighbor(stateReachable, "fe80::1", make([]byte, 6)),
		"too short":    {10, 0, 0},
		"empty":        nil,
	} {
		if e, ok := ParseNeighbor(data, nil); ok {
			t.Errorf("%s must be skipped, got %+v", name, e)
		}
	}
}

func TestParseNeighbor_HostileAttributeLengthsAreRefused(t *testing.T) {
	good := kernelNeighbor(stateReachable, "fe80::1", []byte{0x02, 0, 0x5e, 0, 0x53, 7})
	for _, bad := range []uint16{0, 3, 200, 0xffff} {
		data := append([]byte(nil), good...)
		binary.NativeEndian.PutUint16(data[ndmsgLen:], bad) // the first attribute's length
		if _, ok := ParseNeighbor(data, nil); ok {
			t.Errorf("an attribute length of %d must not parse", bad)
		}
	}
}
