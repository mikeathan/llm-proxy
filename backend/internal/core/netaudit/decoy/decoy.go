// Package decoy runs fake network services that nothing legitimate ever uses. A connection to one means a device on the
// network is probing for services, which is what an intruder or a piece of malware does first. The listeners are
// low-interaction by construction: they read at most a few bytes and throw them away, send nothing but a fixed short string,
// and keep only who connected, to which decoy, and when.
package decoy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// Spec is one decoy: the port it listens on and the fixed string it sends first, if any.
type Spec struct {
	Number int
	// Name says what the decoy pretends to be, in plain words for the screen.
	Name   string
	Banner string
}

// Specs are the decoys. Their ports are in neither of the scan port lists (netaudit.DefaultPorts, ThoroughPorts), so the
// audit's own scans of the host that runs them never touch them; a test pins that. All are unprivileged.
var Specs = []Spec{
	{Number: 2222, Name: "SSH login", Banner: "SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u3\r\n"},
	{Number: 4445, Name: "File sharing"},
	{Number: 8023, Name: "Telnet"},
}

const (
	readLimit      = 64
	readWait       = 2 * time.Second
	writeWait      = time.Second
	defaultMax     = 16
	maxTouches     = 200
	alertEvery     = 5 * time.Minute
	reconcileEvery = 15 * time.Second
	maxCount       = 1_000_000
	sourceGap      = time.Second
)

// Store keeps the touches. A missing file is none.
type Store interface {
	Read() ([]models.DecoyTouch, error)
	Write([]models.DecoyTouch) error
}

// Deps are what the manager needs; the application wires them in one place.
type Deps struct {
	Specs   []Spec
	Enabled func() bool
	// InScope says whether a source belongs to the declared scan networks; others are ignored.
	InScope func(netip.Addr) bool
	// IsOwn says whether an address is this computer's own, whose connections (its own audit and sweep) are ignored.
	IsOwn func(netip.Addr) bool
	Store Store
	// Notify tells the operator which sources touched a decoy, in address order. Wording is the caller's.
	Notify  func(ctx context.Context, sources []string) error
	OnError func(error)
	Now     func() time.Time
}

// Status says what is listening and what is wrong.
type Status struct {
	Listening []int  `json:"listening"`
	Problem   string `json:"problem,omitempty"`
}

// Manager owns the listeners. Reconcile makes them match the setting; Run does that until its context ends.
type Manager struct {
	deps     Deps
	maxConns int

	mu        sync.Mutex
	listeners map[int]net.Listener
	order     []int // the ports, in the order the decoys are defined
	problem   string
	lastSent  time.Time
	lastTrbl  string
	seen      map[string]time.Time
	wg        sync.WaitGroup
	poke      chan struct{}
}

// New builds the manager. Nothing listens until Reconcile.
func New(deps Deps) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Manager{deps: deps, maxConns: defaultMax, listeners: map[int]net.Listener{}, seen: map[string]time.Time{}, poke: make(chan struct{}, 1)}
}

// Run keeps the listeners matching the operator's switch, and stops them when ctx ends.
func (m *Manager) Run(ctx context.Context) {
	for {
		m.Reconcile(ctx)
		select {
		case <-ctx.Done():
			m.stop()
			m.wg.Wait()
			return
		case <-time.After(reconcileEvery):
		case <-m.poke:
		}
	}
}

// Poke makes Run look at the switch now instead of at its next turn, so a change takes effect at once.
func (m *Manager) Poke() {
	select {
	case m.poke <- struct{}{}:
	default:
	}
}

// Reconcile starts the listeners when the switch is on and none run, and stops them when it is off.
func (m *Manager) Reconcile(ctx context.Context) {
	m.mu.Lock()
	running := len(m.listeners) > 0
	m.mu.Unlock()
	switch enabled := m.deps.Enabled(); {
	case enabled && !running:
		m.start(ctx)
	case !enabled && running:
		m.stop()
	}
}

func (m *Manager) start(ctx context.Context) {
	slots := make(chan struct{}, m.maxConns)
	m.mu.Lock()
	m.problem = ""
	m.mu.Unlock()
	var failed []string
	for _, spec := range m.deps.Specs {
		ln, err := net.Listen("tcp4", ":"+strconv.Itoa(spec.Number))
		if err != nil {
			failed = append(failed, fmt.Sprintf("port %d is in use or not allowed", spec.Number))
			continue
		}
		port := ln.Addr().(*net.TCPAddr).Port
		m.mu.Lock()
		m.listeners[port] = ln
		m.order = append(m.order, port)
		m.mu.Unlock()
		m.wg.Add(1)
		go m.serve(ctx, spec, ln, slots)
	}
	if len(failed) > 0 {
		m.setProblem(strings.Join(failed, "; "))
		return
	}
	m.mu.Lock()
	m.lastTrbl = ""
	m.mu.Unlock()
}

func (m *Manager) setProblem(p string) {
	m.mu.Lock()
	m.problem = p
	m.mu.Unlock()
	m.trouble(fmt.Errorf("decoy services: %s", p))
}

func (m *Manager) stop() {
	m.mu.Lock()
	for port, ln := range m.listeners {
		_ = ln.Close()
		delete(m.listeners, port)
	}
	m.order = nil
	m.problem = ""
	m.mu.Unlock()
}

// Addrs lists the loopback-reachable addresses of the running listeners, in the order the decoys are defined.
func (m *Manager) Addrs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.order))
	for i, p := range m.order {
		out[i] = "127.0.0.1:" + strconv.Itoa(p)
	}
	return out
}

// Status reports the ports that listen and the trouble with the others.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Problem: m.problem, Listening: []int{}}
	for p := range m.listeners {
		st.Listening = append(st.Listening, p)
	}
	slices.Sort(st.Listening)
	return st
}

// serve accepts connections on one decoy. The slots belong to this start of the listeners: a connection still being served
// when the decoys are switched off and on again releases a slot of the old set, never of the new one.
func (m *Manager) serve(ctx context.Context, spec Spec, ln net.Listener, slots chan struct{}) {
	defer m.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // closed by stop
		}
		select {
		case slots <- struct{}{}:
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				defer func() { <-slots }()
				m.handle(ctx, spec, ln, conn)
			}()
		default:
			_ = conn.Close() // more at once than allowed
		}
	}
}

// handle serves one connection: it sends the decoy's fixed string, reads a few bytes and discards them, closes, and records
// who connected.
func (m *Manager) handle(ctx context.Context, spec Spec, ln net.Listener, conn net.Conn) {
	defer conn.Close()
	src, ok := m.sourceOf(conn)
	if !ok || !m.allow(src, ln) {
		return
	}
	// The banner goes first, as a real server's does: an SSH client sends nothing until it has seen the server's banner, so
	// waiting for the visitor to speak first would delay it by the whole read deadline.
	if spec.Banner != "" {
		_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
		_, _ = conn.Write([]byte(spec.Banner))
	}
	_ = conn.SetReadDeadline(time.Now().Add(readWait))
	_, _ = conn.Read(make([]byte, readLimit))
	m.record(ctx, src, ln.Addr().(*net.TCPAddr).Port)
}

func (m *Manager) sourceOf(conn net.Conn) (netip.Addr, bool) {
	tcp, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return netip.Addr{}, false
	}
	addr = addr.Unmap()
	if m.deps.IsOwn(addr) || !m.deps.InScope(addr) {
		return netip.Addr{}, false
	}
	return addr, true
}

// allow limits one source to one connection a second per decoy, so a flood cannot grow the store or the work.
func (m *Manager) allow(src netip.Addr, ln net.Listener) bool {
	key := src.String() + "|" + ln.Addr().String()
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.seen[key]; ok && time.Since(last) < sourceGap {
		return false
	}
	if len(m.seen) > 4096 {
		clear(m.seen)
	}
	m.seen[key] = time.Now()
	return true
}

func (m *Manager) record(ctx context.Context, src netip.Addr, port int) {
	now := m.deps.Now()
	m.mu.Lock()
	items, err := m.deps.Store.Read()
	if err != nil {
		m.mu.Unlock()
		m.trouble(fmt.Errorf("decoy services: %w", err))
		return
	}
	i := slices.IndexFunc(items, func(t models.DecoyTouch) bool { return t.Source == src.String() && t.Port == port })
	if i < 0 {
		items = append(items, models.DecoyTouch{Source: src.String(), Port: port, First: now, Last: now, Count: 1})
	} else {
		items[i].Last, items[i].Count = now, min(items[i].Count+1, maxCount)
	}
	slices.SortStableFunc(items, func(a, b models.DecoyTouch) int { return b.Last.Compare(a.Last) })
	if len(items) > maxTouches {
		items = items[:maxTouches]
	}
	err = m.deps.Store.Write(items)
	m.mu.Unlock()
	if err != nil {
		m.trouble(fmt.Errorf("decoy services: %w", err))
		return
	}
	m.alert(ctx, now)
}

// alert tells the operator about touches they have not yet been told of, at most every alertEvery. The addresses it hands on
// are the kernel's account of who connected (the connection's remote address), never bytes the visitor sent.
func (m *Manager) alert(ctx context.Context, now time.Time) {
	sources := m.untold(now)
	if len(sources) == 0 || m.deps.Notify == nil {
		return
	}
	if err := m.deps.Notify(ctx, sortedSources(sources)); err != nil {
		m.trouble(fmt.Errorf("decoy services: could not send the alert: %w", err))
		return
	}
	m.markTold(sources, now)
}

// untold returns the sources with a touch the operator has not been told of, or none while the last alert is recent.
func (m *Manager) untold(now time.Time) map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.lastSent.IsZero() && now.Sub(m.lastSent) < alertEvery {
		return nil
	}
	items, err := m.deps.Store.Read()
	if err != nil {
		return nil
	}
	sources := map[string]bool{}
	for _, t := range items {
		if !t.Notified {
			sources[t.Source] = true
		}
	}
	return sources
}

// markTold records that the operator was told about these sources, after the message went out.
func (m *Manager) markTold(sources map[string]bool, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSent = now
	items, err := m.deps.Store.Read()
	if err != nil {
		return
	}
	for i := range items {
		if sources[items[i].Source] {
			items[i].Notified = true
		}
	}
	_ = m.deps.Store.Write(items)
}

// sortedSources lists the sources in address order, so the same touches always make the same message.
func sortedSources(sources map[string]bool) []string {
	out := make([]string, 0, len(sources))
	for s := range sources {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b string) int {
		x, errX := netip.ParseAddr(a)
		y, errY := netip.ParseAddr(b)
		if errX != nil || errY != nil {
			return strings.Compare(a, b)
		}
		return x.Compare(y)
	})
	return out
}

func (m *Manager) trouble(err error) {
	m.mu.Lock()
	same := err.Error() == m.lastTrbl
	m.lastTrbl = err.Error()
	m.mu.Unlock()
	if !same && m.deps.OnError != nil {
		m.deps.OnError(err)
	}
}
