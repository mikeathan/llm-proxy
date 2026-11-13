package helper

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

const (
	routeHead = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"
	// A default route and a local one, on the interface named below.
	routeVia = "%s\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
)

// hostFixture builds a directory like a host: a USB Wi-Fi adapter, a built-in PCI Wi-Fi card, and an Ethernet port.
func hostFixture(t *testing.T) RadioHost {
	t.Helper()
	root := t.TempDir()
	mk := func(path string, link string, phy bool) {
		dir := filepath.Join(root, path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if phy {
			if err := os.MkdirAll(filepath.Join(dir, "phy80211"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(root, "sys/class/net"), 0o755); err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(filepath.Join(root, "sys/class/net"), dir)
		if err := os.Symlink(rel, filepath.Join(root, "sys/class/net", link)); err != nil {
			t.Fatal(err)
		}
	}
	mk("sys/devices/pci0000:00/0000:00:14.0/usb1/1-2/1-2:1.0/net/wlxusb", "wlxusb", true)
	mk("sys/devices/pci0000:00/0000:00:1c.2/0000:02:00.0/net/wlp2s0", "wlp2s0", true)
	mk("sys/devices/pci0000:00/0000:00:1f.6/net/eno1", "eno1", false)
	mk("sys/devices/pci0000:00/0000:00:14.0/usb1/1-3/1-3:1.0/net/usb0", "usb0", false) // a USB Ethernet dongle
	if err := os.MkdirAll(filepath.Join(root, "proc/net"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := RadioHost{Root: root, Addrs: func(string) ([]netip.Addr, error) { return nil, nil }}
	h.write(t, "proc/net/route", routeHead)
	h.write(t, "proc/net/ipv6_route", "")
	h.write(t, "proc/net/tcp", "  sl  local_address rem_address   st\n")
	h.write(t, "proc/net/tcp6", "  sl  local_address rem_address   st\n")
	return h
}

func (h RadioHost) write(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.Root, rel), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnIdleUSBAdapterIsAllowed(t *testing.T) {
	if err := hostFixture(t).Check(t.Context(), "wlxusb"); err != nil {
		t.Fatal(err)
	}
}

func TestTheBuiltInCardAndAnythingThatIsNotWirelessAreRefused(t *testing.T) {
	h := hostFixture(t)
	for iface, want := range map[string]error{"wlp2s0": ErrNotUSB, "eno1": ErrNotWireless, "usb0": ErrNotWireless} {
		if err := h.Check(t.Context(), iface); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", iface, err, want)
		}
	}
	if err := h.Check(t.Context(), "nonesuch"); err == nil {
		t.Error("an interface that does not exist is refused")
	}
}

func TestAnInterfaceWithTheDefaultRouteIsRefusedEvenWhenItIsUSB(t *testing.T) {
	h := hostFixture(t)
	h.write(t, "proc/net/route", routeHead+"eno1\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"+"wlxusb\t0000A8C0\t00000000\t0001\t0\t0\t600\t00FFFFFF\t0\t0\t0\n")
	if err := h.Check(t.Context(), "wlxusb"); err != nil {
		t.Errorf("a local route is not a default route: %v", err)
	}
	h.write(t, "proc/net/route", routeHead+"wlxusb\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	if err := h.Check(t.Context(), "wlxusb"); !errors.Is(err, ErrCarries) {
		t.Errorf("the default route's interface: %v", err)
	}
	h.write(t, "proc/net/route", routeHead)
	h.write(t, "proc/net/ipv6_route", "00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000064 00000001 00000000 00000003 wlxusb\n")
	if err := h.Check(t.Context(), "wlxusb"); !errors.Is(err, ErrCarries) {
		t.Errorf("the IPv6 default route's interface: %v", err)
	}
}

func TestAnInterfaceHoldingAnSSHSessionsAddressIsRefused(t *testing.T) {
	h := hostFixture(t)
	h.Addrs = func(iface string) ([]netip.Addr, error) {
		if iface == "wlxusb" {
			return []netip.Addr{netip.MustParseAddr("192.168.50.10")}, nil
		}
		return nil, nil
	}
	// 192.168.50.10 is C0.A8.32.0A, written 0A32A8C0 on a little-endian host; port 22 is 0016.
	est := "   0: 0A32A8C0:0016 6E32A8C0:D2A4 01 00000000:00000000 00:00000000 00000000     0        0 1 1 0 0\n"
	h.write(t, "proc/net/tcp", "  sl  local_address rem_address   st\n"+est)
	if err := h.Check(t.Context(), "wlxusb"); !errors.Is(err, ErrCarries) {
		t.Errorf("an established SSH session on the adapter's address: %v", err)
	}
	listening := "   0: 0A32A8C0:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0 0\n"
	h.write(t, "proc/net/tcp", "  sl  local_address rem_address   st\n"+listening)
	if err := h.Check(t.Context(), "wlxusb"); err != nil {
		t.Errorf("a listening socket is not a session: %v", err)
	}
	other := "   0: 0A32A8C0:0050 6E32A8C0:D2A4 01 00000000:00000000 00:00000000 00000000     0        0 1 1 0 0\n"
	h.write(t, "proc/net/tcp", "  sl  local_address rem_address   st\n"+other)
	if err := h.Check(t.Context(), "wlxusb"); err != nil {
		t.Errorf("another port is not SSH: %v", err)
	}
}

func TestAnIPv6SSHSessionOnTheAdapterIsRefused(t *testing.T) {
	h := hostFixture(t)
	h.Addrs = func(string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("fd00::10")}, nil }
	// fd00::10 as four little-endian words: 000000FD 00000000 00000000 10000000.
	line := "   0: 000000FD000000000000000010000000:0016 000000FD000000000000000020000000:D2A4 01 00000000:00000000 00:00000000 00000000 0 0 1 1 0 0\n"
	h.write(t, "proc/net/tcp6", "  sl  local_address rem_address   st\n"+line)
	if err := h.Check(t.Context(), "wlxusb"); !errors.Is(err, ErrCarries) {
		t.Errorf("%v", err)
	}
}

func TestATableThatCannotBeReadIsARefusal(t *testing.T) {
	h := hostFixture(t)
	if err := os.Remove(filepath.Join(h.Root, "proc/net/route")); err != nil {
		t.Fatal(err)
	}
	if err := h.Check(t.Context(), "wlxusb"); err == nil {
		t.Error("with no route table the helper cannot say the adapter is free")
	}
}

func FuzzSSHLocalAddr(f *testing.F) {
	f.Add("   0: 0A32A8C0:0016 6E32A8C0:D2A4 01 00000000:00000000")
	f.Fuzz(func(_ *testing.T, line string) { sshLocalAddr(line) })
}
