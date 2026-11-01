package routerlog

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// Port is the UDP port the program listens on, unprivileged and fixed in code. The router's own setting is 514, so the
// computer redirects 514 to this port (the screen shows the command); the redirect keeps the sender's address.
const Port = 5514

const (
	reconcileEvery = 30 * time.Second
	// perSecond is how many datagrams from the router are looked at in one second; the rest are dropped unread.
	perSecond = 10
	// defaultWakeGap is the least time between two wakes this listener asks for.
	defaultWakeGap = 5 * time.Second
)

// Deps are what the manager needs; the application wires them in one place.
type Deps struct {
	Enabled func() bool
	// Gateway is the router's address. Datagrams from any other address, and all datagrams while it is unknown, are dropped.
	Gateway func() netip.Addr
	// Known says whether the device inventory already has this hardware address.
	Known func(mac string) (bool, error)
	// Wake asks the presence check to look now. It must not block.
	Wake    func()
	OnError func(error)
	Now     func() time.Time
	// Listen opens the socket; nil means a UDP socket on Port. Tests replace it.
	Listen func(addr string) (net.PacketConn, error)
}

// Status says whether the program is listening, what is wrong, and when the router was last heard.
type Status struct {
	Listening bool      `json:"listening"`
	Problem   string    `json:"problem,omitempty"`
	LastHeard time.Time `json:"last_heard,omitzero"`
}

// Manager owns the socket. Reconcile makes it match the operator's switch; Run does that until its context ends.
type Manager struct {
	deps    Deps
	wakeGap time.Duration

	mu         sync.Mutex
	conn       net.PacketConn
	problem    string
	lastTrbl   string
	lastHeard  time.Time
	lastWake   time.Time
	windowFrom time.Time
	inWindow   int
	wg         sync.WaitGroup
	poke       chan struct{}
}

// New builds the manager. Nothing listens until Reconcile.
func New(deps Deps) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Listen == nil {
		deps.Listen = func(addr string) (net.PacketConn, error) { return net.ListenPacket("udp4", addr) }
	}
	return &Manager{deps: deps, wakeGap: defaultWakeGap, poke: make(chan struct{}, 1)}
}

// Run keeps the socket matching the operator's switch, and closes it when ctx ends.
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

// Poke makes Run look at the switch now.
func (m *Manager) Poke() {
	select {
	case m.poke <- struct{}{}:
	default:
	}
}

// Reconcile opens the socket when the switch is on and it is closed, and closes it when the switch is off.
func (m *Manager) Reconcile(ctx context.Context) {
	m.mu.Lock()
	running := m.conn != nil
	m.mu.Unlock()
	switch enabled := m.deps.Enabled(); {
	case enabled && !running:
		m.start(ctx)
	case !enabled && running:
		m.stop()
	}
}

func (m *Manager) start(ctx context.Context) {
	conn, err := m.deps.Listen(":" + strconv.Itoa(Port))
	if err != nil {
		m.setProblem(fmt.Sprintf("port %d is in use or not allowed", Port))
		return
	}
	m.mu.Lock()
	m.conn, m.problem, m.lastTrbl = conn, "", ""
	m.mu.Unlock()
	m.wg.Add(1)
	go m.serve(ctx, conn)
}

func (m *Manager) setProblem(p string) {
	m.mu.Lock()
	m.problem = p
	m.mu.Unlock()
	m.trouble(errors.New("router log: " + p))
}

func (m *Manager) stop() {
	m.mu.Lock()
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
	m.problem = ""
	m.mu.Unlock()
}

// Addr is the address the socket listens on, or nil when it is closed.
func (m *Manager) Addr() *net.UDPAddr {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn == nil {
		return nil
	}
	a, _ := m.conn.LocalAddr().(*net.UDPAddr)
	return a
}

// Status reports whether the socket is open, the trouble opening it, and when the router was last heard.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{Listening: m.conn != nil, Problem: m.problem, LastHeard: m.lastHeard}
}

// serve reads datagrams until the socket is closed. The buffer is one byte longer than the longest line read, so a longer
// datagram is seen as too long and dropped; nothing is kept between datagrams except two times and a counter.
func (m *Manager) serve(ctx context.Context, conn net.PacketConn) {
	defer m.wg.Done()
	buf := make([]byte, maxDatagram+1)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}
			continue
		}
		m.handle(buf[:n], from)
	}
}

func (m *Manager) handle(datagram []byte, from net.Addr) {
	udp, ok := from.(*net.UDPAddr)
	if !ok {
		return
	}
	src := udp.AddrPort().Addr().Unmap()
	if gw := m.deps.Gateway(); !gw.IsValid() || src != gw {
		return
	}
	now := m.deps.Now()
	if !m.within(now) {
		return
	}
	m.mu.Lock()
	m.lastHeard = now
	m.mu.Unlock()
	ev, ok := Parse(datagram)
	if !ok {
		return
	}
	known, err := m.deps.Known(ev.MAC)
	if err != nil {
		m.trouble(fmt.Errorf("router log: could not check the device list: %w", err))
		return
	}
	if known {
		return
	}
	m.wake(now)
}

// within counts a datagram against this second's allowance.
func (m *Manager) within(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if now.Sub(m.windowFrom) >= time.Second {
		m.windowFrom, m.inWindow = now, 0
	}
	if m.inWindow >= perSecond {
		return false
	}
	m.inWindow++
	return true
}

// wake asks for a look, at most once per gap.
func (m *Manager) wake(now time.Time) {
	m.mu.Lock()
	due := now.Sub(m.lastWake) >= m.wakeGap
	if due {
		m.lastWake = now
	}
	m.mu.Unlock()
	if due {
		m.deps.Wake()
	}
}

// trouble reports a problem once until it changes.
func (m *Manager) trouble(err error) {
	m.mu.Lock()
	same := err.Error() == m.lastTrbl
	m.lastTrbl = err.Error()
	m.mu.Unlock()
	if !same && m.deps.OnError != nil {
		m.deps.OnError(err)
	}
}
