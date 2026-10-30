//go:build linux

package neighbors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/unix"
)

// echoRequest is an ICMPv6 echo request: type 128, code 0, then the checksum, identifier and sequence number, which the
// kernel fills in for this kind of socket, and eight bytes of padding. The packet never changes.
var echoRequest = []byte{128, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}

var allNodes = netip.MustParseAddr("ff02::1")

func discover6(ctx context.Context, names []string) error {
	var socks []int
	defer func() {
		for _, fd := range socks {
			_ = unix.Close(fd)
		}
	}()
	sent := 0
	for _, name := range names {
		ifi, err := net.InterfaceByName(name)
		if err != nil || ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		fd, err := echo6(ifi.Index)
		if err != nil {
			return err
		}
		socks = append(socks, fd)
		sent++
	}
	if sent == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to wait for IPv6 neighbours: %w", ctx.Err())
	case <-time.After(discover6Settle):
	}
	return nil
}

// echo6 opens an unprivileged ICMPv6 socket, sends the echo to ff02::1 out of one interface and returns the socket, which
// the caller closes after the replies had time to arrive.
func echo6(ifindex int) (int, error) {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.IPPROTO_ICMPV6)
	if err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			return -1, ErrNotPermitted
		}
		return -1, fmt.Errorf("failed to open the IPv6 echo socket: %w", err)
	}
	// A link-local group is never forwarded, but a hop limit of one says so to the kernel too.
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_MULTICAST_HOPS, 1); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("failed to prepare the IPv6 echo: %w", err)
	}
	dst := &unix.SockaddrInet6{ZoneId: uint32(ifindex), Addr: allNodes.As16()}
	if err := unix.Sendto(fd, echoRequest, 0, dst); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("failed to send the IPv6 echo: %w", err)
	}
	return fd, nil
}
