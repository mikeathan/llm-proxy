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

func listenUDP4(addr string) (*net.UDPConn, error) {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp4", a)
}

// Port is the UDP port the program listens on, unprivileged and fixed in code. The router's own setting is 514, so the
// computer redirects 514 to this port (the screen shows the command); the redirect keeps the sender's address.
const Port = 5514

const (
	reconcileEvery = 30 * time.Second
	// perSecond is how many datagrams from the router are looked at in one second; the rest are dropped unread.
	perSecond = 10
	// queriesPerSecond is how many DNS query lines are read in one second, under a counter of their own: they are far more numerous
	// than the device messages, and they must neither use that allowance nor be starved by it. The owner's capture on 2026-11-05
	// (10 minutes of everything the router sends to port 514: 362 lines, none dropped, about 0.6 a second) puts the ordinary rate
	// far below this; the limit leaves room for a burst of page loads, which one capture of a quiet network does not show.
	queriesPerSecond = 50
	// watchedPerSecond is how many failed sign-ins and disconnects are read in one second, under a counter of their own. A forged
	// flood can make the count wrong and nothing else; the limit only keeps the work bounded.
	watchedPerSecond = 20
	// defaultWakeGap is the least time between two wakes this listener asks for.
	defaultWakeGap = 5 * time.Second
	// receiveBuffer is the kernel's queue for this socket. It is small on purpose: when lines come faster than they are read
	// the kernel drops the surplus, so memory use is fixed whatever is sent.
	receiveBuffer = 64 << 10
	// A name is offered for at most suggestBurst devices in suggestWindow; the same device and name is not offered again within
	// suggestSame. The name store is rewritten by an offer, so this keeps a stream of forged leases from rewriting it.
	suggestBurst  = 5
	suggestWindow = 10 * time.Second
	suggestSame   = time.Hour
	maxOffered    = 256
)

// Deps are what the manager needs; the application wires them in one place.
type Deps struct {
	Enabled func() bool
	// Gateway is the router's address. Datagrams from any other address, and all datagrams while it is unknown, are dropped.
	Gateway func() netip.Addr
	// Device says whether the device list holds this hardware address and whether the operator has named it.
	Device func(mac string) (known, named bool, err error)
	// Suggest offers a host name for a device to the operator, who confirms it. It cleans the name itself and must never
	// show or send it as it is. May be nil.
	Suggest func(mac, name string) error
	// Drop counts one firewall drop from the internet (its protocol and destination port, nothing else) at the time it was heard.
	// It must not block and may be nil, in which case drop lines are ignored.
	Drop func(d Drop, at time.Time)
	// Query is handed one DNS query line from the router (its type, vetted name and sender, nothing else) at the time it was heard.
	// It must not block. Nil means query lines are not read at all.
	Query func(q Query, at time.Time)
	// Watch is handed one failed router sign-in or one Wi-Fi disconnect (its kind and, for a disconnect, its reason code, nothing
	// else) at the time it was heard. It must not block. Nil means these lines are not read at all.
	Watch func(w Watched, at time.Time)
	// Wake asks the presence check to look now. It must not block.
	Wake    func()
	OnError func(error)
	Now     func() time.Time
	// Listen opens the socket; nil means a UDP socket on Port. Tests replace it.
	Listen func(addr string) (*net.UDPConn, error)
}

// Status says whether the program is listening, what is wrong, and when the router was last heard.
type Status struct {
	Listening bool      `json:"listening"`
	Problem   string    `json:"problem,omitempty"`
	LastHeard time.Time `json:"last_heard,omitzero"`
	// QueriesRead, QueriesLimited and QueriesPeak describe the DNS query lines since QueriesSince (the listener's start): how many
	// were read, how many the limit turned away, and the most read in one second. They are how the limit is tuned from the real rate.
	QueriesRead    int64     `json:"queries_read"`
	QueriesLimited int64     `json:"queries_limited"`
	QueriesPeak    int       `json:"queries_peak_per_second"`
	QueriesSince   time.Time `json:"queries_since,omitzero"`
	// QueriesLast is when the latest DNS query line was read; zero until one was.
	QueriesLast time.Time `json:"queries_last,omitzero"`
}

// Manager owns the socket. Reconcile makes it match the operator's switch; Run does that until its context ends.
type Manager struct {
	deps    Deps
	wakeGap time.Duration

	mu         sync.Mutex
	conn       *net.UDPConn
	problem    string
	lastTrbl   string
	lastHeard  time.Time
	lastWake   time.Time
	windowFrom time.Time
	inWindow   int
	queryFrom  time.Time
	inQueries  int
	watchFrom  time.Time
	inWatched  int
	qRead      int64
	qLimited   int64
	qPeak      int
	qSince     time.Time
	qLast      time.Time
	offerFrom  time.Time
	offerCount int
	offered    map[string]time.Time // device and name offered lately, so a repeat is not offered again; bounded by maxOffered
	wg         sync.WaitGroup
	poke       chan struct{}
}

// New builds the manager. Nothing listens until Reconcile.
func New(deps Deps) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Listen == nil {
		deps.Listen = listenUDP4
	}
	return &Manager{deps: deps, wakeGap: defaultWakeGap, poke: make(chan struct{}, 1), offered: map[string]time.Time{}}
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
	_ = conn.SetReadBuffer(receiveBuffer) // a failure leaves the system default, which is still bounded
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
	return Status{
		Listening: m.conn != nil, Problem: m.problem, LastHeard: m.lastHeard,
		QueriesRead: m.qRead, QueriesLimited: m.qLimited, QueriesPeak: m.qPeak, QueriesSince: m.qSince, QueriesLast: m.qLast,
	}
}

// serve reads datagrams until the socket is closed. The buffer is one byte longer than the longest line read, so a longer
// datagram is seen as too long and dropped; nothing is kept between datagrams except two times and a counter.
func (m *Manager) serve(ctx context.Context, conn *net.UDPConn) {
	defer m.wg.Done()
	buf := make([]byte, maxDatagram+1)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf) // no allocation per datagram
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}
			continue
		}
		m.handle(buf[:n], from)
	}
}

func (m *Manager) handle(datagram []byte, from netip.AddrPort) {
	now := m.deps.Now()
	if ev, ok := m.accept(datagram, from, now); ok {
		m.act(ev, now)
		return
	}
	m.countDrop(datagram, from, now)
	m.readQuery(datagram, from, now)
	m.readWatched(datagram, from, now)
}

// readWatched hands on a failed router sign-in or a Wi-Fi disconnect from the router's own address, under its own limit apart from
// the device allowance. The cheap search turns every other line away before anything is parsed.
func (m *Manager) readWatched(datagram []byte, from netip.AddrPort, now time.Time) {
	if m.deps.Watch == nil || !IsWatched(datagram) {
		return
	}
	if gw := m.deps.Gateway(); !gw.IsValid() || from.Addr().Unmap() != gw || !m.withinWatched(now) {
		return
	}
	if w, ok := ParseWatched(datagram); ok {
		m.deps.Watch(w, now)
	}
}

// withinWatched counts a watched line against this second's allowance.
func (m *Manager) withinWatched(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if now.Sub(m.watchFrom) >= time.Second {
		m.watchFrom, m.inWatched = now, 0
	}
	if m.inWatched >= watchedPerSecond {
		return false
	}
	m.inWatched++
	return true
}

// countDrop counts a firewall drop line from the router's own address. It is not held to the allowance of device messages:
// parsing one costs a few dozen string operations, the socket's small queue bounds how many arrive, and what it keeps is two
// numbers in a counter whose size is fixed.
func (m *Manager) countDrop(datagram []byte, from netip.AddrPort, now time.Time) {
	if m.deps.Drop == nil || !IsDrop(datagram) {
		return
	}
	if gw := m.deps.Gateway(); !gw.IsValid() || from.Addr().Unmap() != gw {
		return
	}
	if d, ok := ParseDrop(datagram); ok {
		m.deps.Drop(d, now)
	}
}

// readQuery hands on a DNS query line from the router's own address. It has its own limit, apart from the device allowance, and
// the cheap search turns every other line away before anything is parsed.
func (m *Manager) readQuery(datagram []byte, from netip.AddrPort, now time.Time) {
	if m.deps.Query == nil || !IsQuery(datagram) {
		return
	}
	if gw := m.deps.Gateway(); !gw.IsValid() || from.Addr().Unmap() != gw || !m.withinQueries(now) {
		return
	}
	if q, ok := ParseQuery(datagram); ok {
		m.deps.Query(q, now)
	}
}

// withinQueries counts a query line against this second's allowance for query lines.
func (m *Manager) withinQueries(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.qSince.IsZero() {
		m.qSince = now
	}
	if now.Sub(m.queryFrom) >= time.Second {
		m.queryFrom, m.inQueries = now, 0
	}
	if m.inQueries >= queriesPerSecond {
		m.qLimited++
		return false
	}
	m.inQueries++
	m.qRead++
	m.qLast = now
	m.qPeak = max(m.qPeak, m.inQueries)
	return true
}

// accept is everything that happens to a datagram before it can do anything: the sender check, the cheap search that
// turns the firewall lines away without using any of the allowance, the allowance, and the parse.
func (m *Manager) accept(datagram []byte, from netip.AddrPort, now time.Time) (Event, bool) {
	if gw := m.deps.Gateway(); !gw.IsValid() || from.Addr().Unmap() != gw {
		return Event{}, false
	}
	m.mu.Lock()
	m.lastHeard = now // anything from the router shows that its log is arriving
	m.mu.Unlock()
	if !Interesting(datagram) || !m.within(now) {
		return Event{}, false
	}
	return Parse(datagram)
}

// act applies an event: offer the device's name if it is worth offering, and wake the check if the device is new.
func (m *Manager) act(ev Event, now time.Time) {
	known, named, err := m.deps.Device(ev.MAC)
	if err != nil {
		m.trouble(fmt.Errorf("router log: could not check the device list: %w", err))
		return
	}
	if ev.Name != "" && m.deps.Suggest != nil && (!known || !named) && m.mayOffer(ev, now) {
		if err := m.deps.Suggest(ev.MAC, ev.Name); err != nil {
			m.trouble(fmt.Errorf("router log: could not offer a device name: %w", err))
		}
	}
	if !known {
		m.wake(now)
	}
}

// mayOffer says whether a name may be offered now: not the same device and name as lately, and within the burst allowance.
func (m *Manager) mayOffer(ev Event, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ev.MAC + " " + ev.Name
	if at, ok := m.offered[key]; ok && now.Sub(at) < suggestSame {
		return false
	}
	if now.Sub(m.offerFrom) >= suggestWindow {
		m.offerFrom, m.offerCount = now, 0
	}
	if m.offerCount >= suggestBurst {
		return false
	}
	m.offerCount++
	if len(m.offered) >= maxOffered {
		clear(m.offered)
	}
	m.offered[key] = now
	return true
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
