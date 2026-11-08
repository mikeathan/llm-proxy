// Package udpchecks asks one device two fixed questions over UDP and reports only whether it answered in the expected form: an SNMPv3
// engine-discovery message (no community string, no credential) to port 161 and a unicast SSDP M-SEARCH to port 1900. The datagrams are built
// here and never shaped by an operator, a model or a device (Constitution item 12, plan network-nightly-udp-probes). The package sends
// through the guarded dial it is given, so an address outside the declared scan scope is never asked, and keeps nothing a device said: a
// reply is judged by its shape and thrown away. Silence is not an answer and never means a port is closed.
package udpchecks

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

const (
	// MaxDevices is how many devices one look asks.
	MaxDevices = 64
	// maxInFlight is how many devices are asked at once.
	maxInFlight = 4
	// replyWait is how long a device has to answer; one on the local network answers in milliseconds or not at all.
	replyWait = 2 * time.Second
	// maxReply bounds what is read from one answer.
	maxReply = 1500
)

// Dial is the guarded dial function (NetworkTools.DialContext).
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Probe is one fixed question: the port, the datagram for a device, and whether an answer has the expected form.
type Probe struct {
	Port     int
	Datagram func(addr netip.Addr) []byte
	Valid    func(reply []byte) bool
}

// Probes are the only two questions asked, in a fixed order. TFTP, DNS and NTP are left out on purpose and SNMP v1 and v2c would need a
// community string, which is a guessed password: see the plan's section 2.
var Probes = []Probe{
	{Port: 161, Datagram: snmpDiscovery, Valid: validSNMPv3Report},
	{Port: 1900, Datagram: ssdpSearch, Valid: validSSDPReply},
}

// Answer is one device that replied to one probe in the expected form.
type Answer struct {
	Addr netip.Addr
	Port int
}

// Prober asks devices through a guarded dial.
type Prober struct {
	Dial Dial
	// Wait overrides the reply wait; zero means two seconds.
	Wait time.Duration
}

// Ask sends one probe to the device's own address and reports whether it answered in the expected form. The socket is connected to the
// device, so an answer from anywhere else is never read. Only IPv4 is asked; a refused dial, an error and silence are all "no answer".
func (p Prober) Ask(ctx context.Context, addr netip.Addr, probe Probe) bool {
	if !addr.Is4() {
		return false
	}
	conn, err := p.Dial(ctx, "udp", net.JoinHostPort(addr.String(), strconv.Itoa(probe.Port)))
	if err != nil {
		return false
	}
	defer conn.Close()
	wait := p.Wait
	if wait <= 0 {
		wait = replyWait
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if conn.SetDeadline(deadline) != nil {
		return false
	}
	if _, err := conn.Write(probe.Datagram(addr)); err != nil {
		return false
	}
	buf := make([]byte, maxReply)
	n, err := conn.Read(buf)
	return err == nil && probe.Valid(buf[:n])
}

// Look asks the first MaxDevices addresses both questions, four devices at a time, and returns the answers. A device that did not answer
// is not in the result; the caller treats that as unknown, not as fixed.
func (p Prober) Look(ctx context.Context, addrs []netip.Addr) []Answer {
	if len(addrs) > MaxDevices {
		addrs = addrs[:MaxDevices]
	}
	var (
		mu  sync.Mutex
		out []Answer
		wg  sync.WaitGroup
	)
	gate := make(chan struct{}, maxInFlight)
	for _, a := range addrs {
		gate <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-gate; wg.Done() }()
			for _, probe := range Probes {
				if ctx.Err() != nil {
					return
				}
				if p.Ask(ctx, a, probe) {
					mu.Lock()
					out = append(out, Answer{Addr: a, Port: probe.Port})
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
