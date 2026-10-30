//go:build linux

package neighbors

import (
	"fmt"
	"net"
	"syscall"
)

// read6 asks the kernel for the IPv6 neighbour table over netlink: a local request to the kernel, with no network traffic
// and no program started. It lists only hosts this computer has already talked to.
func read6() ([]Entry, error) {
	rib, err := syscall.NetlinkRIB(syscall.RTM_GETNEIGH, syscall.AF_INET6)
	if err != nil {
		return nil, fmt.Errorf("failed to read the IPv6 neighbour table: %w", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil, fmt.Errorf("failed to read the IPv6 neighbour table: %w", err)
	}
	var out []Entry
	for _, m := range msgs {
		if m.Header.Type != syscall.RTM_NEWNEIGH {
			continue
		}
		if e, ok := ParseNeighbor(m.Data, interfaceName); ok {
			out = append(out, e)
			if len(out) >= maxEntries6 {
				break
			}
		}
	}
	return out, nil
}

func interfaceName(index int) string {
	if ifi, err := net.InterfaceByIndex(index); err == nil {
		return ifi.Name
	}
	return ""
}
