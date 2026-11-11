package routerconn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

const (
	// maxDevices and maxPlaces bound what one read keeps.
	maxDevices = 128
	maxPlaces  = 200
	// gap is the least time between the end of one read and the start of the next.
	gap = 5 * time.Second
)

var (
	// ErrBusy is returned while a read runs or has just ended.
	ErrBusy = errors.New("the router was just read; wait a few seconds")
	// ErrNotSet is returned when the router address, user or key is missing.
	ErrNotSet = errors.New("set the router's address, user and key first")
	// ErrBadUser is returned for a user name that is not a plain account name.
	ErrBadUser = errors.New("the user name may use letters, digits, dot, dash and underscore only")
	// ErrBadKey is returned for a key that cannot be used.
	ErrBadKey = errors.New("that is not a private key this program can use; it must not be protected by a passphrase")
	// ErrNotConfirmed is returned when the fingerprint given is not the one the router showed.
	ErrNotConfirmed = errors.New("the router's key is not the one shown; start again")
)

var userRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

// Settings keeps the router's address, user and pinned host key; Secret holds the private key.
type Settings interface {
	Get() (address, user, hostKey string)
	Set(address, user, hostKey string) error
}

// Secret is the stored private key: set, cleared and used, never read back to a screen.
type Secret interface {
	Value() string
	Set(string) error
	Clear() error
}

// Owners says who holds an internet address and where it is used, from the tables kept on this computer.
type Owners func(netip.Addr) (models.TrafficOwner, bool)

// Deps are what a Manager needs. Read and Probe are routerssh's functions; Owners may be nil or return nil.
type Deps struct {
	Settings Settings
	Secret   Secret
	Read     func(context.Context, routerssh.Target) ([]byte, error)
	Probe    func(ctx context.Context, address string) (line, fingerprint string, err error)
	Owners   func() Owners
	// Observe is given each read before it is kept, to be added to the router watch's history, which marks the places that are new in it.
	// It is nil, or does nothing, while the watch is off.
	Observe func(*models.RouterConnections)
	// Watching says whether the router watch is on, for the screen.
	Watching func() bool
	Now      func() time.Time
}

// View is what the screen shows of the setup and of the last read. The key is never in it.
type View struct {
	Address  string `json:"address"`
	User     string `json:"user"`
	KeySaved bool   `json:"key_saved"`
	// Fingerprint is the pinned router key's fingerprint, empty until pinned.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Watching says the router watch is on: a history is kept of each read and the nightly audit reads the router.
	Watching bool                      `json:"watching"`
	Last     *models.RouterConnections `json:"last,omitempty"`
}

// Manager keeps the setup and the last read, and reads the router one at a time.
type Manager struct {
	deps Deps

	mu      sync.Mutex
	running bool
	ended   time.Time
	last    *models.RouterConnections
}

// New builds a manager.
func New(deps Deps) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Manager{deps: deps}
}

// View is the setup and the last read.
func (m *Manager) View() View {
	addr, user, hostKey := m.deps.Settings.Get()
	v := View{Address: addr, User: user, KeySaved: m.deps.Secret.Value() != ""}
	if pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey)); err == nil {
		v.Fingerprint = ssh.FingerprintSHA256(pub)
	}
	if m.deps.Watching != nil {
		v.Watching = m.deps.Watching()
	}
	m.mu.Lock()
	v.Last = m.last
	m.mu.Unlock()
	return v
}

// SetTarget saves the router's address and user. A new address drops the pinned key and the last read, which belonged to another router.
func (m *Manager) SetTarget(address, user string) error {
	if _, err := routerssh.ParseAddress(address); err != nil {
		return err
	}
	if !userRE.MatchString(user) {
		return ErrBadUser
	}
	oldAddr, _, hostKey := m.deps.Settings.Get()
	if oldAddr != address {
		hostKey = ""
		m.mu.Lock()
		m.last = nil
		m.mu.Unlock()
	}
	if err := m.deps.Settings.Set(address, user, hostKey); err != nil {
		return fmt.Errorf("failed to save the router settings: %w", err)
	}
	return nil
}

// SetKey saves the private key after checking it can be used. It is never answered back.
func (m *Manager) SetKey(key string) error {
	if _, err := ssh.ParsePrivateKey([]byte(key)); err != nil {
		return ErrBadKey
	}
	if err := m.deps.Secret.Set(key); err != nil {
		return fmt.Errorf("failed to save the router key: %w", err)
	}
	return nil
}

// ClearKey removes the key and the last read.
func (m *Manager) ClearKey() error {
	if err := m.deps.Secret.Clear(); err != nil {
		return fmt.Errorf("failed to remove the router key: %w", err)
	}
	m.mu.Lock()
	m.last = nil
	m.mu.Unlock()
	return nil
}

// Probe learns the router's host key without sending any credential and returns its fingerprint for the operator to compare with the router's
// own page. Nothing is saved.
func (m *Manager) Probe(ctx context.Context) (string, error) {
	addr, _, _ := m.deps.Settings.Get()
	if addr == "" {
		return "", ErrNotSet
	}
	_, fp, err := m.deps.Probe(ctx, addr)
	return fp, err
}

// Pin saves the router's host key when it still has the fingerprint the operator confirmed.
func (m *Manager) Pin(ctx context.Context, confirm string) error {
	addr, user, _ := m.deps.Settings.Get()
	if addr == "" {
		return ErrNotSet
	}
	line, fp, err := m.deps.Probe(ctx, addr)
	if err != nil {
		return err
	}
	if confirm == "" || fp != confirm {
		return ErrNotConfirmed
	}
	if err := m.deps.Settings.Set(addr, user, line); err != nil {
		return fmt.Errorf("failed to save the router's key: %w", err)
	}
	return nil
}

// Read reads the router's table once, on the operator's press, and keeps the result in memory.
func (m *Manager) Read(ctx context.Context) (*models.RouterConnections, error) {
	addr, user, hostKey := m.deps.Settings.Get()
	key := m.deps.Secret.Value()
	if addr == "" || user == "" || key == "" {
		return nil, ErrNotSet
	}
	m.mu.Lock()
	if m.running || (!m.ended.IsZero() && m.deps.Now().Sub(m.ended) < gap) {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.running = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running, m.ended = false, m.deps.Now()
		m.mu.Unlock()
	}()
	raw, err := m.deps.Read(ctx, routerssh.Target{Address: addr, User: user, Key: []byte(key), HostKey: hostKey})
	if err != nil {
		return nil, err
	}
	conns, lines, truncated := Parse(bytes.NewReader(raw))
	var owners Owners
	if m.deps.Owners != nil {
		owners = m.deps.Owners()
	}
	result := summarise(conns, owners)
	result.At, result.Lines, result.Truncated, result.Kept = m.deps.Now(), lines, truncated, len(conns)
	if m.deps.Observe != nil {
		m.deps.Observe(result)
	}
	m.mu.Lock()
	m.last = result
	m.mu.Unlock()
	return result, nil
}

type placeKey struct {
	addr  netip.Addr
	port  int
	proto string
}

// summarise groups connections by device and place, the busiest first, within the bounds, and names each place from the tables.
func summarise(conns []Conn, owners Owners) *models.RouterConnections {
	byDevice := map[netip.Addr]map[placeKey]*models.RouterPlace{}
	totals := map[netip.Addr]int{}
	for _, c := range conns {
		places := byDevice[c.Device]
		if places == nil {
			places = map[placeKey]*models.RouterPlace{}
			byDevice[c.Device] = places
		}
		k := placeKey{c.Remote, c.Port, c.Proto}
		p := places[k]
		if p == nil {
			p = &models.RouterPlace{Address: c.Remote.String(), Port: c.Port, Proto: c.Proto, State: c.State}
			places[k] = p
		}
		p.Count++
		totals[c.Device]++
	}
	out := &models.RouterConnections{Devices: []models.RouterDevice{}}
	for dev, places := range byDevice {
		d := models.RouterDevice{Address: dev.String(), Total: totals[dev]}
		for _, p := range places {
			if owners != nil {
				if o, ok := owners(netip.MustParseAddr(p.Address)); ok {
					p.Owner = &o
				}
			}
			d.Places = append(d.Places, *p)
		}
		slices.SortFunc(d.Places, func(a, b models.RouterPlace) int {
			if a.Count != b.Count {
				return b.Count - a.Count
			}
			return compareStrings(a.Address, b.Address)
		})
		if len(d.Places) > maxPlaces {
			d.Places = d.Places[:maxPlaces]
		}
		out.Devices = append(out.Devices, d)
	}
	slices.SortFunc(out.Devices, func(a, b models.RouterDevice) int {
		if a.Total != b.Total {
			return b.Total - a.Total
		}
		return compareStrings(a.Address, b.Address)
	})
	if len(out.Devices) > maxDevices {
		out.Devices = out.Devices[:maxDevices]
	}
	return out
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
