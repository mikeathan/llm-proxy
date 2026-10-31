package names

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// replyWait is how long to wait for a device's answer; a device on the local network answers in milliseconds or not at all.
const replyWait = 2 * time.Second

// maxReply bounds what is read: an mDNS answer to one question is small.
const maxReply = 1500

// Dial is the guarded dial function (NetworkTools.DialContext): it refuses addresses the network policy forbids, so a
// device outside the declared scan scope is never asked.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Prober asks one device its name over mDNS.
type Prober struct{ Dial Dial }

// Ask puts one question to addr on port 5353 and returns the cleaned name in the answer. The socket is connected to the
// device, so an answer from anywhere else is never read.
func (p Prober) Ask(ctx context.Context, addr netip.Addr) (string, error) {
	if !addr.Is4() {
		return "", fmt.Errorf("names: %s is not an IPv4 address", addr)
	}
	conn, err := p.Dial(ctx, "udp", net.JoinHostPort(addr.String(), strconv.Itoa(MDNSPort)))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(replyWait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	id := uint16(rand.Uint32())
	if _, err := conn.Write(Query(addr, id)); err != nil {
		return "", err
	}
	buf := make([]byte, maxReply)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return ParseReply(buf[:n], addr, id)
}
