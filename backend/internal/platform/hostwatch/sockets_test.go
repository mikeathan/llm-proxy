package hostwatch

import (
	"net/netip"
	"os"
	"slices"
	"testing"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// The four tables below are a real capture from the Ubuntu server this runs on (cat /proc/net/tcp tcp6 udp udp6).
func TestParseSockets_RealCapture(t *testing.T) {
	cases := []struct {
		file, proto string
		want        []string // "addr:port" of every listening or bound row, in file order
	}{
		{"tcp", "tcp", []string{"127.0.0.1:631", "127.0.0.1:11434", "127.0.0.53:53", "127.0.0.1:61209", "0.0.0.0:2222", "0.0.0.0:3552", "0.0.0.0:22", "127.0.0.54:53", "0.0.0.0:8023", "0.0.0.0:4445"}},
		{"tcp6", "tcp", []string{"[::1]:631", "[::]:4001", "[::]:3552", "[::]:22"}},
		{"udp", "udp", []string{"0.0.0.0:5353", "0.0.0.0:41849", "127.0.0.54:53", "127.0.0.53:53"}},
		{"udp6", "udp", []string{"[::]:5353", "[::]:52039"}},
	}
	for _, c := range cases {
		got, err := ParseSockets(open(t, c.file), c.proto)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, s := range got {
			names = append(names, netip.AddrPortFrom(s.Addr, uint16(s.Port)).String())
		}
		if !slices.Equal(names, c.want) {
			t.Errorf("%s: got %v\nwant %v", c.file, names, c.want)
		}
	}
}

func TestParseSockets_SkipsConnectedRowsAndKeepsInodes(t *testing.T) {
	tcp, _ := ParseSockets(open(t, "tcp"), "tcp")
	for _, s := range tcp {
		if s.Port == 0xD1F2 || s.Addr == netip.MustParseAddr("192.168.50.60") {
			t.Errorf("the established SSH connection must not be a listener: %+v", s)
		}
	}
	if tcp[4].Inode != "2127412" {
		t.Errorf("inode of 0.0.0.0:2222 = %q, want 2127412", tcp[4].Inode)
	}
	udp, _ := ParseSockets(open(t, "udp"), "udp")
	for _, s := range udp {
		if s.Port == 68 {
			t.Errorf("the connected DHCP client must not be a listener: %+v", s)
		}
	}
}
