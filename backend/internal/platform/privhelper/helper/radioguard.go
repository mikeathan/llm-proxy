package helper

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// RadioHost is the host's own records the radio class reads before it touches an adapter (Constitution I.16): an adapter must be a USB
// device and must not carry the host's connection, so a window can never cut the operator off. Root is "/" in production and a directory of
// fixtures in a test; Addrs lists the addresses an interface holds.
type RadioHost struct {
	Root  string
	Addrs func(iface string) ([]netip.Addr, error)
}

// SystemRadioHost reads this host.
func SystemRadioHost() RadioHost { return RadioHost{Root: "/", Addrs: interfaceAddrs} }

func interfaceAddrs(name string) ([]netip.Addr, error) {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().Unmap())
		}
	}
	return out, nil
}

// Typed refusals, for the journal; the peer is told only that the request is out of scope.
var (
	ErrNotWireless = errors.New("not a wireless interface")
	ErrNotUSB      = errors.New("not a USB adapter: the built-in card is never used for capture")
	ErrCarries     = errors.New("carries this computer's connection")
)

var usbBus = regexp.MustCompile(`^usb[0-9]+$`)

// Check is Config.Interface for the radio class.
func (r RadioHost) Check(_ context.Context, iface string) error {
	if err := r.checkUSB(iface); err != nil {
		return err
	}
	if err := r.checkDefaultRoute(iface); err != nil {
		return err
	}
	return r.checkSSH(iface)
}

// checkUSB requires a wireless interface whose device sits under a USB bus in the sysfs tree.
func (r RadioHost) checkUSB(iface string) error {
	link := filepath.Join(r.Root, "sys/class/net", iface)
	real, err := filepath.EvalSymlinks(link)
	if err != nil {
		return fmt.Errorf("%s: %w", iface, err)
	}
	if _, err := os.Stat(filepath.Join(real, "phy80211")); err != nil {
		return fmt.Errorf("%s: %w", iface, ErrNotWireless)
	}
	root, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return fmt.Errorf("%s: %w", iface, ErrNotUSB)
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if usbBus.MatchString(part) {
			return nil
		}
	}
	return fmt.Errorf("%s: %w", iface, ErrNotUSB)
}

// checkDefaultRoute refuses the interface of an IPv4 or IPv6 default route. A table that cannot be read is a refusal: the helper cannot say
// the interface is free.
func (r RadioHost) checkDefaultRoute(iface string) error {
	v4, err := readLines(filepath.Join(r.Root, "proc/net/route"))
	if err != nil {
		return err
	}
	v6, err := readLines(filepath.Join(r.Root, "proc/net/ipv6_route"))
	if err != nil && !errors.Is(err, os.ErrNotExist) { // a host without IPv6 has no table
		return err
	}
	if defaultRouteV4(v4, iface) || defaultRouteV6(v6, iface) {
		return fmt.Errorf("%s: %w (default route)", iface, ErrCarries)
	}
	return nil
}

// defaultRouteV4 reads /proc/net/route: Iface Destination Gateway Flags RefCnt Use Metric Mask ..., the first line a heading.
func defaultRouteV4(lines []string, iface string) bool {
	for _, l := range lines[min(1, len(lines)):] {
		f := strings.Fields(l)
		if len(f) > 7 && f[0] == iface && f[1] == "00000000" && f[7] == "00000000" {
			return true
		}
	}
	return false
}

// defaultRouteV6 reads /proc/net/ipv6_route: dest prefixlen src prefixlen next-hop metric refcnt use flags device.
func defaultRouteV6(lines []string, iface string) bool {
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) >= 10 && f[9] == iface && f[0] == strings.Repeat("0", 32) && f[1] == "00" {
			return true
		}
	}
	return false
}

// checkSSH refuses an interface that holds the local address of an established SSH session (local port 22).
func (r RadioHost) checkSSH(iface string) error {
	held, err := r.Addrs(iface)
	if err != nil {
		return fmt.Errorf("%s: %w", iface, err)
	}
	for _, file := range []string{"proc/net/tcp", "proc/net/tcp6"} {
		lines, err := readLines(filepath.Join(r.Root, file))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, l := range lines {
			if local, ok := sshLocalAddr(l); ok && containsAddr(held, local) {
				return fmt.Errorf("%s: %w (an SSH session)", iface, ErrCarries)
			}
		}
	}
	return nil
}

func containsAddr(list []netip.Addr, a netip.Addr) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

// sshLocalAddr reads one line of /proc/net/tcp or tcp6 and returns the local address when the socket is established on local port 22.
func sshLocalAddr(line string) (netip.Addr, bool) {
	f := strings.Fields(line)
	if len(f) < 4 || f[3] != "01" { // 01 is TCP_ESTABLISHED
		return netip.Addr{}, false
	}
	host, port, ok := strings.Cut(f[1], ":")
	if !ok {
		return netip.Addr{}, false
	}
	if p, err := strconv.ParseUint(port, 16, 16); err != nil || p != 22 {
		return netip.Addr{}, false
	}
	raw, err := hex.DecodeString(host)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.Addr{}, false
	}
	for i := 0; i+4 <= len(raw); i += 4 { // each 32-bit word is written in host (little-endian) byte order
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	a, ok := netip.AddrFromSlice(raw)
	return a.Unmap(), ok
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}
