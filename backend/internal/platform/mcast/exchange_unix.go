//go:build unix

package mcast

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/unix"
)

// Exchange sends the questions out of one interface and returns the replies that accept admits, read for at most the window.
// accept is called with the sender's address before anything is kept; it is where the scan scope, this computer's own
// addresses and the blocked list are applied. A look that ends early because ctx ended returns what it had.
func Exchange(ctx context.Context, ifc Interface, qs []Question, lim Limits, accept func(netip.Addr) bool) ([]Datagram, error) {
	lim = lim.clamped()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ifc.Addr.AsSlice()})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := configure(conn, ifc.Addr); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	deadline := time.Now().Add(lim.Window)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	for _, q := range qs {
		if _, err := conn.WriteToUDPAddrPort(q.Payload, q.To); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %w", ErrRefused, err)
		}
	}
	return readReplies(conn, lim, accept), nil
}

func readReplies(conn *net.UDPConn, lim Limits, accept func(netip.Addr) bool) []Datagram {
	var out []Datagram
	count := newCounter(lim)
	buf := make([]byte, MaxDatagramSize+1)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil { // the window ended, the look was cancelled, or the socket failed: what was read stands
			return out
		}
		addr := from.Addr().Unmap()
		if n > MaxDatagramSize || !addr.Is4() || !accept(addr) || !count.admit(addr) {
			continue
		}
		out = append(out, Datagram{From: netip.AddrPortFrom(addr, from.Port()), Data: append([]byte(nil), buf[:n]...)})
	}
}

// configure sets the outgoing interface, a hop limit of one and no loopback, and bounds the receive queue.
func configure(conn *net.UDPConn, addr netip.Addr) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := raw.Control(func(fd uintptr) {
		f := int(fd)
		opErr = errors.Join(
			unix.SetsockoptInet4Addr(f, unix.IPPROTO_IP, unix.IP_MULTICAST_IF, addr.As4()),
			unix.SetsockoptByte(f, unix.IPPROTO_IP, unix.IP_MULTICAST_TTL, 1),
			unix.SetsockoptByte(f, unix.IPPROTO_IP, unix.IP_MULTICAST_LOOP, 0),
			unix.SetsockoptInt(f, unix.SOL_SOCKET, unix.SO_RCVBUF, ReceiveQueue),
		)
	}); err != nil {
		return err
	}
	return opErr
}
