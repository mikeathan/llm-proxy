package hostwatch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ErrUnavailable means this computer has no socket table to read (anything but Linux). It is an answer in its own right:
// "could not check" is never reported as "nothing is listening".
var ErrUnavailable = errors.New("this computer cannot list its own open ports (only Linux can)")

// Listener is a port something on this computer is waiting on and other computers can reach: a protocol and a port, nothing
// else. The program behind it is deliberately not named: its name is text any account can set.
type Listener struct {
	Proto string
	Port  int
}

// Proc reads the kernel's tables under Root, which is "/proc" on a real computer.
type Proc struct{ Root string }

// defaultEphemeralLow is the kernel's documented start of the dynamic port range, used when the host's setting cannot be read.
const defaultEphemeralLow = 32768

// Listeners returns the ports open to other computers, once per protocol and port, sorted. Left out: addresses only this
// computer can reach (loopback), this program's own sockets (found by their inodes under self/fd, so a port the service
// itself opens, a decoy for instance, is never reported as news), and UDP ports in the dynamic range, which programs such as
// mDNS pick anew at every start and so would be a new listener each time.
func (p Proc) Listeners() ([]Listener, error) {
	own, err := p.ownSockets()
	if err != nil {
		return nil, err
	}
	low := p.ephemeralLow()
	seen := map[Listener]bool{}
	for _, t := range []struct{ file, proto string }{{"tcp", "tcp"}, {"tcp6", "tcp"}, {"udp", "udp"}, {"udp6", "udp"}} {
		rows, err := p.table(t.file, t.proto)
		if err != nil {
			return nil, err
		}
		for _, s := range rows {
			if s.Addr.IsLoopback() || own[s.Inode] || (s.Proto == "udp" && s.Port >= low) {
				continue
			}
			seen[Listener{s.Proto, s.Port}] = true
		}
	}
	out := make([]Listener, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b Listener) int {
		if a.Proto != b.Proto {
			return strings.Compare(a.Proto, b.Proto)
		}
		return a.Port - b.Port
	})
	return out, nil
}

// table reads one socket table. The IPv4 tables must exist; the IPv6 ones are absent on a host with IPv6 switched off.
func (p Proc) table(file, proto string) ([]Socket, error) {
	f, err := os.Open(filepath.Join(p.Root, "net", file))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if strings.HasSuffix(file, "6") {
				return nil, nil
			}
			return nil, ErrUnavailable
		}
		return nil, fmt.Errorf("failed to open the %s socket table: %w", file, err)
	}
	defer f.Close()
	return ParseSockets(f, proto)
}

// ownSockets returns the inodes of the sockets this process holds open.
func (p Proc) ownSockets() (map[string]bool, error) {
	dir := filepath.Join(p.Root, "self", "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrUnavailable
		}
		return nil, fmt.Errorf("failed to list this program's open files: %w", err)
	}
	own := map[string]bool{}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil { // a descriptor closed since the listing
			continue
		}
		if inode, ok := strings.CutPrefix(target, "socket:["); ok {
			own[strings.TrimSuffix(inode, "]")] = true
		}
	}
	return own, nil
}

// ephemeralLow is the first port of the host's dynamic range.
func (p Proc) ephemeralLow() int {
	raw, err := os.ReadFile(filepath.Join(p.Root, "sys", "net", "ipv4", "ip_local_port_range"))
	if err != nil {
		return defaultEphemeralLow
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 1 {
		return defaultEphemeralLow
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n < 1 || n > 65535 {
		return defaultEphemeralLow
	}
	return n
}
