// Package portprobe is the limited scan engine: a plain TCP connect probe written in Go. It exists for hosts where
// nmap is not installed and a person is reading the result. It finds live addresses and open ports and nothing
// more — no service or version detection — and the netaudit service labels its results as limited.
package portprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/core/netaudit"
)

// EngineName labels results produced by this engine.
const EngineName = "portprobe"

const (
	dialTimeout = 1500 * time.Millisecond
	workers     = 64
	// maxProbes bounds addresses × ports for one scan.
	maxProbes = 50000
	stateOpen = "open"
	protoTCP  = "tcp"
)

// defaultPorts are tried when the request names none: the services a home or office network most often exposes.
var defaultPorts = []int{21, 22, 23, 25, 53, 80, 110, 139, 143, 443, 445, 515, 631, 1883, 3306, 3389, 5000, 5353, 5432, 5900, 8000, 8080, 8443, 9100}

// discoverPorts are the few ports tried to see whether an address is alive: any answer, even a refusal, means it is.
var discoverPorts = []int{80, 443, 22, 445}

// Guard re-checks one address immediately before it is dialled, as the second line behind the service's
// authorisation. A non-nil error skips the address.
type Guard func(netip.Addr) error

// Dialer opens a TCP connection; net.Dialer satisfies it.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Engine is the connect-probe engine.
type Engine struct {
	guard  Guard
	dialer Dialer
}

// New builds the engine. A nil guard refuses every address, so forgetting to wire one scans nothing.
func New(guard Guard) *Engine {
	return &Engine{guard: guard, dialer: &net.Dialer{Timeout: dialTimeout}}
}

// Name labels results.
func (*Engine) Name() string { return EngineName }

// Available is always nil: the engine needs nothing installed.
func (*Engine) Available(context.Context) error { return nil }

// Supports is true for discovery and port scans; service detection needs nmap.
func (*Engine) Supports(m netaudit.Mode) bool {
	return m == netaudit.ModeDiscover || m == netaudit.ModePorts
}

type probe struct {
	addr netip.Addr
	port int
}

type outcome struct {
	probe
	open  bool
	alive bool
}

// Scan probes every address × port with a bounded set of workers. Addresses are dialled as literals, never as
// names, so nothing is resolved after authorisation.
func (e *Engine) Scan(ctx context.Context, req netaudit.ScanRequest) (netaudit.Result, error) {
	ports, err := e.portsFor(req)
	if err != nil {
		return netaudit.Result{}, err
	}
	if len(req.Targets)*len(ports) > maxProbes {
		return netaudit.Result{}, fmt.Errorf("%d addresses × %d ports is more than the %d probes the limited engine allows; narrow the scan or install nmap",
			len(req.Targets), len(ports), maxProbes)
	}
	var allowed []netip.Addr
	for _, a := range req.Targets {
		if e.guard != nil && e.guard(a) == nil {
			allowed = append(allowed, a)
		}
	}
	results := e.run(ctx, allowed, ports)
	return netaudit.Result{
		Hosts:    assemble(allowed, results, req.Mode),
		Warnings: []string{"the limited engine reports open ports only; it does not identify services or versions"},
	}, ctx.Err()
}

func (e *Engine) portsFor(req netaudit.ScanRequest) ([]int, error) {
	if req.Mode == netaudit.ModeDiscover {
		return discoverPorts, nil
	}
	ports, err := netaudit.ExpandPorts(req.Ports)
	if err != nil {
		return nil, err
	}
	if len(ports) == 0 {
		return defaultPorts, nil
	}
	return ports, nil
}

// run dials every probe with a fixed pool of workers and returns what each saw.
func (e *Engine) run(ctx context.Context, addrs []netip.Addr, ports []int) []outcome {
	jobs := make(chan probe)
	out := make(chan outcome)
	var wg sync.WaitGroup
	for range min(workers, max(1, len(addrs)*len(ports))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				out <- e.dial(ctx, p)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, a := range addrs {
			for _, port := range ports {
				select {
				case jobs <- probe{addr: a, port: port}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	var results []outcome
	for r := range out {
		results = append(results, r)
	}
	return results
}

// dial tries one connection. A refusal proves the address is alive even though the port is closed.
func (e *Engine) dial(ctx context.Context, p probe) outcome {
	conn, err := e.dialer.DialContext(ctx, "tcp", net.JoinHostPort(p.addr.String(), strconv.Itoa(p.port)))
	if err == nil {
		conn.Close()
		return outcome{probe: p, open: true, alive: true}
	}
	return outcome{probe: p, alive: errors.Is(err, syscall.ECONNREFUSED)}
}

// assemble folds probe outcomes into hosts, in address order and with ports sorted. A discover scan lists no ports.
func assemble(addrs []netip.Addr, results []outcome, mode netaudit.Mode) []netaudit.Host {
	byAddr := map[netip.Addr]*netaudit.Host{}
	order := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		byAddr[a] = &netaudit.Host{Addr: a}
		order = append(order, a)
	}
	for _, r := range results {
		h := byAddr[r.addr]
		h.Up = h.Up || r.alive
		if r.open && mode != netaudit.ModeDiscover {
			h.Ports = append(h.Ports, netaudit.Port{Number: r.port, Proto: protoTCP, State: stateOpen})
		}
	}
	hosts := make([]netaudit.Host, 0, len(order))
	for _, a := range order {
		h := byAddr[a]
		sort.Slice(h.Ports, func(i, j int) bool { return h.Ports[i].Number < h.Ports[j].Number })
		hosts = append(hosts, *h)
	}
	return hosts
}
