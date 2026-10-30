// Package neighbors reads the host's neighbour (ARP) table: which link-layer addresses sit behind which IPv4 addresses on
// the networks this machine is attached to. It reads a file the kernel keeps and starts no program and opens no socket,
// so it can see only what the host itself has talked to; the audit's sweep is what makes the host talk to every address.
package neighbors

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"runtime"
	"strings"
)

// ErrUnavailable means this host cannot read a neighbour table. It is an answer in its own right: "could not check" is
// never reported as "nobody is there".
var ErrUnavailable = errors.New("this computer cannot list the devices on its network (only Linux can)")

// maxEntries bounds what one read returns; a /16 full of answers would be 65,534.
const maxEntries = 4096

// flagComplete is the kernel's ATF_COM: the entry has a resolved hardware address. An entry without it is an address the
// host asked about and nobody answered.
const flagComplete = 0x2

// Entry is one neighbour: an address and the hardware address that answered for it.
type Entry struct {
	Addr netip.Addr
	// MAC is lower case, colon separated.
	MAC       string
	Interface string
}

// Table is a read of the neighbour table.
type Table struct {
	Entries []Entry
	// Entries6 are the IPv6 neighbours. They are read on a best effort: IPv6 may be switched off or unreadable without the
	// IPv4 table being any worse, so a failure is in Note6 and does not fail the read.
	Entries6 []Entry
	Note6    string
	// Gateway is the default route's next hop, or the zero Addr when the host has none.
	Gateway netip.Addr
}

// ParseARP reads the layout of /proc/net/arp and keeps the complete, unicast entries. A row it cannot read is skipped, not
// an error: the file is the kernel's, but the host's own tools may add to it.
func ParseARP(r io.Reader) ([]Entry, error) {
	var out []Entry
	sc := bufio.NewScanner(r)
	sc.Scan() // header
	for sc.Scan() && len(out) < maxEntries {
		f := strings.Fields(sc.Text())
		if len(f) < 6 {
			continue
		}
		addr, err := netip.ParseAddr(f[0])
		if err != nil || !addr.Is4() {
			continue
		}
		var flags int
		if _, err := fmt.Sscanf(f[2], "0x%x", &flags); err != nil || flags&flagComplete == 0 {
			continue
		}
		hw, err := net.ParseMAC(f[3])
		if err != nil || len(hw) != 6 || hw[0]&1 == 1 || isZero(hw) {
			continue
		}
		out = append(out, Entry{Addr: addr, MAC: strings.ToLower(hw.String()), Interface: f[5]})
	}
	return out, sc.Err()
}

func isZero(hw net.HardwareAddr) bool {
	for _, b := range hw {
		if b != 0 {
			return false
		}
	}
	return true
}

// ParseDefaultGateway reads /proc/net/route and returns the next hop of the default route. Addresses there are
// little-endian hexadecimal.
func ParseDefaultGateway(r io.Reader) (netip.Addr, error) {
	sc := bufio.NewScanner(r)
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		var raw uint32
		if _, err := fmt.Sscanf(f[2], "%x", &raw); err != nil || raw == 0 {
			continue
		}
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], raw)
		return netip.AddrFrom4(b), nil
	}
	if err := sc.Err(); err != nil {
		return netip.Addr{}, err
	}
	return netip.Addr{}, errors.New("no default route")
}

// LocallyAdministered reports whether a MAC address has the "locally administered" bit set, which means software chose it
// and the manufacturer did not: how a phone or laptop hides its hardware address, but also how a virtual machine or a
// container gets one.
func LocallyAdministered(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) > 0 && hw[0]&2 == 2
}

// Reader reads the live tables. The paths are fields so a test can point it at files.
type Reader struct {
	ARPPath   string
	RoutePath string
	goos      string
}

// NewReader reads the Linux paths.
func NewReader() Reader {
	return Reader{ARPPath: "/proc/net/arp", RoutePath: "/proc/net/route", goos: runtime.GOOS}
}

// Read returns the neighbour table, or ErrUnavailable off Linux. A missing default route is not an error: the table is
// still good, it just has no gateway.
func (rd Reader) Read() (Table, error) {
	if rd.goos != "linux" {
		return Table{}, ErrUnavailable
	}
	f, err := os.Open(rd.ARPPath)
	if err != nil {
		return Table{}, fmt.Errorf("failed to read the neighbour table: %w", err)
	}
	defer f.Close()
	entries, err := ParseARP(f)
	if err != nil {
		return Table{}, fmt.Errorf("failed to read the neighbour table: %w", err)
	}
	table := Table{Entries: entries}
	if e6, err := read6(); err != nil {
		table.Note6 = err.Error()
	} else {
		table.Entries6 = e6
	}
	if rf, err := os.Open(rd.RoutePath); err == nil {
		defer rf.Close()
		table.Gateway, _ = ParseDefaultGateway(rf)
	}
	return table, nil
}
