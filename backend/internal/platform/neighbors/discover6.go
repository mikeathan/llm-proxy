package neighbors

import (
	"context"
	"errors"
	"regexp"
	"time"
)

// Active IPv6 discovery makes the neighbour table list the devices on a link instead of only the ones this computer has
// already talked to. It sends one ICMPv6 echo request to the all-nodes link-local group (ff02::1) on an interface; every
// device that answers has to find this computer's hardware address first, which fills the kernel's neighbour table, and the
// table is then read as before (read6). Nothing is parsed from the replies: they are never read.
//
// Constitution I.7 is the rule this follows: one fixed packet, link-local only, a bounded number of interfaces, and the
// host's own permission (net.ipv4.ping_group_range) decides whether it may be sent; the program asks for no privilege.

// ErrNotPermitted means the host does not let this program open an unprivileged ICMP socket. It is a setting of the host
// (net.ipv4.ping_group_range must include the program's group), so the operator is told what to change; it is never
// worked around.
var ErrNotPermitted = errors.New("this computer does not allow this program to send the IPv6 echo (net.ipv4.ping_group_range)")

const (
	// maxDiscover6Interfaces bounds how many links one discovery touches.
	maxDiscover6Interfaces = 8
	// discover6Settle is how long the sockets stay open after the echo so devices can answer and fill the table.
	discover6Settle = 1500 * time.Millisecond
)

// interfaceName6 is what a Linux interface name may look like (at most 15 characters).
var interfaceName6 = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

// discoverable keeps the plain interface names once each, in order, at most maxDiscover6Interfaces of them.
func discoverable(names []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		if !interfaceName6.MatchString(n) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		if len(out) == maxDiscover6Interfaces {
			break
		}
	}
	return out
}

// Discover6 sends one echo request to ff02::1 on each named interface (at most maxDiscover6Interfaces) and waits briefly
// for the neighbour table to fill. An interface that is down, has no multicast or no longer exists is skipped. It returns
// ErrNotPermitted when the host refuses the socket, and ErrUnavailable off Linux.
func Discover6(ctx context.Context, interfaces []string) error {
	names := discoverable(interfaces)
	if len(names) == 0 {
		return nil
	}
	return discover6(ctx, names)
}
