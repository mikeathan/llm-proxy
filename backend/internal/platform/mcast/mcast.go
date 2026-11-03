// Package mcast is the one place the program sends a multicast datagram and reads the unicast answers (Constitution I.9). It
// opens an unconnected UDP socket on an ephemeral port bound to one interface's own address, sends fixed questions, reads for
// a short window under hard counts, and closes. It never joins a multicast group, so no multicast datagram is ever received,
// and it never uses port 5353 or 1900 as its own.
package mcast

import (
	"errors"
	"net"
	"net/netip"
	"time"
)

// Hard bounds from Constitution I.9.
const (
	MaxInterfaces   = 4
	MaxDatagrams    = 512
	MaxPerSender    = 32
	MaxDatagramSize = 4096
	ReceiveQueue    = 256 << 10
	MaxWindow       = 5 * time.Second
)

var (
	// ErrUnavailable means this system cannot do it.
	ErrUnavailable = errors.New("multicast discovery is not available on this system")
	// ErrRefused means the system would not open or use the socket; the cause is wrapped.
	ErrRefused = errors.New("the system refused the discovery socket")
)

// Interface is one network interface to ask on, by the IPv4 address it holds.
type Interface struct {
	Name string
	Addr netip.Addr
}

// Question is one fixed datagram and where it goes.
type Question struct {
	To      netip.AddrPort
	Payload []byte
}

// Datagram is one reply: its sender and its bytes.
type Datagram struct {
	From netip.AddrPort
	Data []byte
}

// Limits bound one exchange. A zero field takes the I.9 maximum; a larger one is lowered to it.
type Limits struct {
	Datagrams, PerSender int
	Window               time.Duration
}

func (l Limits) clamped() Limits {
	if l.Datagrams <= 0 || l.Datagrams > MaxDatagrams {
		l.Datagrams = MaxDatagrams
	}
	if l.PerSender <= 0 || l.PerSender > MaxPerSender {
		l.PerSender = MaxPerSender
	}
	if l.Window <= 0 || l.Window > MaxWindow {
		l.Window = MaxWindow
	}
	return l
}

// Interfaces lists up to MaxInterfaces interfaces that are up, multicast-capable, not loopback and hold an IPv4 address that
// ok accepts (the declared scan networks).
func Interfaces(ok func(netip.Addr) bool) ([]Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []Interface
	for _, ifi := range all {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			p, isNet := a.(*net.IPNet)
			if !isNet {
				continue
			}
			if addr, good := netip.AddrFromSlice(p.IP.To4()); good && ok(addr) {
				out = append(out, Interface{Name: ifi.Name, Addr: addr})
				break
			}
		}
		if len(out) == MaxInterfaces {
			break
		}
	}
	return out, nil
}

// counter applies the read bounds: how many datagrams in all and from one sender.
type counter struct {
	lim       Limits
	total     int
	perSender map[netip.Addr]int
}

func newCounter(l Limits) *counter { return &counter{lim: l, perSender: map[netip.Addr]int{}} }

// admit says whether one more datagram from this sender is within the bounds, and counts it when it is.
func (c *counter) admit(from netip.Addr) bool {
	if c.total >= c.lim.Datagrams || c.perSender[from] >= c.lim.PerSender {
		return false
	}
	c.total++
	c.perSender[from]++
	return true
}
