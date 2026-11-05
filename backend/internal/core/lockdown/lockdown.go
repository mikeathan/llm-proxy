// Package lockdown is the lock down switch: one step-up action the operator takes when something looks wrong. While it is on,
// automations do not start, agents may not use the terminal or the network, and connectors' inbound messages are refused.
// Switching it on also signs out every other session and, if asked, revokes every script token. It changes no setting, so
// lifting it returns everything as it was (the sessions and tokens it ended stay ended). It never switches on by itself.
package lockdown

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/models"
)

// Message is the fixed reason a refused action gives.
const Message = "Lock down is on: automations do not run and agents may not use the terminal, the network or tools from MCP servers until it is lifted in Settings, Safety."

// ErrLockedDown is the refusal while the switch is on.
var ErrLockedDown = errors.New(Message)

// Store keeps the switch. A missing file is off.
type Store interface {
	Read() (models.Lockdown, error)
	Write(models.Lockdown) error
}

// Auth ends what switching on ends: *auth.Service.
type Auth interface {
	RevokeOtherSessions(keep string) (int, error)
	RevokeAllTokens() (int, error)
}

// Manager holds the switch. On is read on every tool call, so it is held in memory and only written through Engage and Release.
type Manager struct {
	store Store
	auth  Auth
	now   func() time.Time

	on    atomic.Bool
	mu    sync.Mutex
	state models.Lockdown
}

func New(store Store, auth Auth, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{store: store, auth: auth, now: now}
}

// SetAuth gives the manager what ends sessions and tokens; it is wired once the sign-in service exists.
func (m *Manager) SetAuth(a Auth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth = a
}

// Load reads the switch at start. A record that cannot be read leaves the program locked and returns the error: a damaged
// switch must not open what the operator closed.
func (m *Manager) Load() error {
	st, err := m.store.Read()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.state = models.Lockdown{On: true, Since: m.now()}
		m.on.Store(true)
		return fmt.Errorf("failed to read the lock down switch, so it stays on: %w", err)
	}
	m.state = st
	m.on.Store(st.On)
	return nil
}

// On reports whether lock down is on. It is safe to call on every tool call.
func (m *Manager) On() bool { return m.on.Load() }

// State is the switch as the screen shows it.
func (m *Manager) State() models.Lockdown {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Engage switches lock down on, signs out every session but keepSession and, when revokeTokens is set, revokes every script
// token. Engaging while on ends sessions (and tokens) again and keeps the first time.
func (m *Manager) Engage(keepSession string, revokeTokens bool) (models.Lockdown, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.state
	if !st.On {
		st = models.Lockdown{On: true, Since: m.now()}
	}
	if err := m.store.Write(st); err != nil {
		return m.state, fmt.Errorf("failed to save the lock down switch: %w", err)
	}
	m.state = st
	m.on.Store(true)
	var errs []error
	if m.auth != nil {
		n, err := m.auth.RevokeOtherSessions(keepSession)
		st.SignedOut += n
		errs = append(errs, err)
		if revokeTokens {
			n, err := m.auth.RevokeAllTokens()
			st.TokensRevoked += n
			errs = append(errs, err)
		}
	}
	m.state = st
	if err := m.store.Write(st); err != nil {
		errs = append(errs, fmt.Errorf("failed to save what lock down ended: %w", err))
	}
	return st, errors.Join(errs...)
}

// Release lifts lock down.
func (m *Manager) Release() (models.Lockdown, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.Write(models.Lockdown{}); err != nil {
		return m.state, fmt.Errorf("failed to save the lock down switch: %w", err)
	}
	m.state = models.Lockdown{}
	m.on.Store(false)
	return m.state, nil
}
