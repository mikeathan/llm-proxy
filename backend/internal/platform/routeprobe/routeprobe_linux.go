//go:build linux

package routeprobe

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// pollSlice bounds one wait, so a stopped trace ends within it.
	pollSlice = 100 * time.Millisecond
	oobLen    = 512
	bufLen    = 512
)

var linuxFamilies = families{inet6: unix.AF_INET6}

// Probe sends one datagram and waits for its answer (see the package comment). Each probe has its own socket, closed before it returns.
func Probe(ctx context.Context, r Request) (Reply, error) {
	if err := ctx.Err(); err != nil {
		return Reply{}, err
	}
	fd, err := openProbeSocket(r)
	if err != nil {
		return Reply{}, err
	}
	defer unix.Close(fd)
	start := time.Now()
	if err := unix.Sendto(fd, make([]byte, PayloadLen), 0, sockaddrOf(r.Dst, r.Port)); err != nil && !isQueuedError(err) {
		return Reply{}, fmt.Errorf("the probe could not be sent: %w", err)
	}
	deadline := start.Add(r.Wait)
	for {
		if err := ctx.Err(); err != nil {
			return Reply{}, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return Reply{Kind: Silent}, nil
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLERR}}
		n, err := unix.Poll(fds, int(min(left, pollSlice).Milliseconds())+1)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return Reply{}, fmt.Errorf("the probe's answer could not be waited for: %w", err)
		}
		if n == 0 {
			continue
		}
		if reply, ok := readAnswer(fd, start); ok {
			return reply, nil
		}
	}
}

// openProbeSocket opens an unprivileged UDP socket that asks the kernel for ICMP answers and sends with the request's time to live.
func openProbeSocket(r Request) (int, error) {
	family, level, ttlOpt, errOpt := unix.AF_INET, unix.IPPROTO_IP, unix.IP_TTL, unix.IP_RECVERR
	if r.Dst.Is6() {
		family, level, ttlOpt, errOpt = unix.AF_INET6, unix.IPPROTO_IPV6, unix.IPV6_UNICAST_HOPS, unix.IPV6_RECVERR
	}
	fd, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.IPPROTO_UDP)
	if err != nil {
		return -1, fmt.Errorf("the probe socket could not be opened: %w", err)
	}
	if err := unix.SetsockoptInt(fd, level, ttlOpt, r.TTL); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("the probe's time to live could not be set: %w", err)
	}
	if err := unix.SetsockoptInt(fd, level, errOpt, 1); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("the probe could not ask for answers: %w", err)
	}
	return fd, nil
}

// isQueuedError is a send that failed because an earlier answer is already queued: the answer is read below.
func isQueuedError(err error) bool {
	return errors.Is(err, unix.EHOSTUNREACH) || errors.Is(err, unix.ENETUNREACH) || errors.Is(err, unix.ECONNREFUSED)
}

func sockaddrOf(a netip.Addr, port uint16) unix.Sockaddr {
	if a.Is4() {
		return &unix.SockaddrInet4{Port: int(port), Addr: a.As4()}
	}
	return &unix.SockaddrInet6{Port: int(port), Addr: a.As16()}
}

// readAnswer reads the error queue; a datagram that came back on the normal path (a server that answered the probe) also means it was reached.
func readAnswer(fd int, start time.Time) (Reply, bool) {
	buf, oob := make([]byte, bufLen), make([]byte, oobLen)
	_, oobn, _, _, err := unix.Recvmsg(fd, buf, oob, unix.MSG_ERRQUEUE)
	if err != nil {
		if _, from, rerr := unix.Recvfrom(fd, buf, unix.MSG_DONTWAIT); rerr == nil {
			if addr, ok := addrOf(from); ok {
				return Reply{Kind: Reached, From: addr, RTT: time.Since(start)}, true
			}
		}
		return Reply{}, false
	}
	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return Reply{}, false
	}
	for _, m := range msgs {
		if !isExtendedErr(m.Header) {
			continue
		}
		if kind, from, ok := parseExtendedErr(m.Data, linuxFamilies); ok {
			return Reply{Kind: kind, From: from, RTT: time.Since(start)}, true
		}
	}
	return Reply{}, false
}

func isExtendedErr(h unix.Cmsghdr) bool {
	return (h.Level == unix.SOL_IP && h.Type == unix.IP_RECVERR) || (h.Level == unix.SOL_IPV6 && h.Type == unix.IPV6_RECVERR)
}

func addrOf(sa unix.Sockaddr) (netip.Addr, bool) {
	switch s := sa.(type) {
	case *unix.SockaddrInet4:
		return netip.AddrFrom4(s.Addr), true
	case *unix.SockaddrInet6:
		return netip.AddrFrom16(s.Addr).Unmap(), true
	}
	return netip.Addr{}, false
}
