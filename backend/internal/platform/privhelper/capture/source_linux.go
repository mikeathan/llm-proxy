//go:build linux

package capture

import (
	"errors"
	"net"

	"golang.org/x/sys/unix"

	"llm-proxy/internal/platform/privhelper/wire"
)

// pollInterval is how long one read waits, so the window's end is noticed even when no frame arrives.
var pollInterval = unix.Timeval{Sec: 1}

// packetSource reads frames from an AF_PACKET socket.
type packetSource struct {
	fd  int
	buf [snapLen]byte
}

// Open opens a packet socket on the interface. The order matters: the socket is created with protocol 0, so nothing is queued; the filter
// is attached; and only then is it bound to the interface with every protocol, so no frame reaches the program before the filter has seen
// it (a socket created with ETH_P_ALL starts queuing at once). Promiscuous mode is never requested: there is no PACKET_ADD_MEMBERSHIP.
func Open(name string) (Source, wire.CaptureStop, error) {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return nil, wire.StopNoInterface, errNotOpened
	}
	if ifc.Flags&net.FlagUp == 0 {
		return nil, wire.StopInterfaceDown, errNotOpened
	}
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, wire.StopSocketError, errNotOpened
	}
	src := &packetSource{fd: fd}
	if err := src.attach(); err != nil {
		unix.Close(fd)
		return nil, wire.StopSocketError, errNotOpened
	}
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL), Ifindex: ifc.Index}); err != nil {
		unix.Close(fd)
		return nil, wire.StopSocketError, errNotOpened
	}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &pollInterval); err != nil {
		unix.Close(fd)
		return nil, wire.StopSocketError, errNotOpened
	}
	return src, wire.StopWindow, nil
}

// attach sends the fixed filter to the kernel.
func (s *packetSource) attach() error { return attachFilter(s.fd, captureFilter()) }

func attachFilter(fd int, prog []filterInstruction) error {
	raw := make([]unix.SockFilter, len(prog))
	for i, in := range prog {
		raw[i] = unix.SockFilter{Code: in.Code, Jt: in.Jt, Jf: in.Jf, K: in.K}
	}
	fprog := unix.SockFprog{Len: uint16(len(raw)), Filter: &raw[0]}
	return unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &fprog)
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// Next reads one frame, at most snapLen bytes of it.
func (s *packetSource) Next() ([]byte, error) {
	n, _, err := unix.Recvfrom(s.fd, s.buf[:], 0)
	switch {
	case err == nil:
		return append([]byte(nil), s.buf[:n]...), nil
	case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EINTR):
		return nil, nil
	}
	return nil, err
}

// Stats reads the socket's counters once; reading them resets them in the kernel.
func (s *packetSource) Stats() (uint32, uint32) {
	st, err := unix.GetsockoptTpacketStats(s.fd, unix.SOL_PACKET, unix.PACKET_STATISTICS)
	if err != nil || st == nil {
		return 0, 0
	}
	return st.Packets, st.Drops
}

func (s *packetSource) Close() error { return unix.Close(s.fd) }
