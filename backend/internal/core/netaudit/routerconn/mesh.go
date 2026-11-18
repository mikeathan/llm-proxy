package routerconn

import (
	"context"
	"errors"
	"sync"
	"time"

	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

// The mesh router's list of joined devices, read while a radio watch runs (Constitution I.15, ADR-0059): at most once every meshEvery however
// often it is asked, on one connection kept open between reads and closed after meshIdle without one, and at once whenever the setup changes.
// It has its own lock and state, so a read of the list never makes the operator's read of the table wait or answer "busy".
const (
	meshEvery = 30 * time.Second
	meshIdle  = 2 * time.Minute
	// meshMaxWait is the longest wait after failures; each failure doubles the wait from meshEvery.
	meshMaxWait = 10 * time.Minute
)

// ErrMeshWaiting is returned when the list was read too recently and there is no earlier read to answer with.
var ErrMeshWaiting = errors.New("the router's list of devices was just read")

// ClientReader is a connection to the router that reads the list of joined devices (routerssh.Conn).
type ClientReader interface {
	ReadClients(ctx context.Context) ([]byte, error)
	Close() error
}

// mesh is the list's state, guarded by mu.
type mesh struct {
	mu       sync.Mutex
	conn     ClientReader
	idle     *time.Timer
	last     *models.MeshClients
	tried    time.Time
	failures int
	idleFor  time.Duration
}

// MeshClients answers the latest list of joined devices, reading the router only when the last try is older than the wait. A list that could
// not be read is an error; an earlier read is answered while the wait runs.
func (m *Manager) MeshClients(ctx context.Context) (*models.MeshClients, error) {
	m.mesh.mu.Lock()
	defer m.mesh.mu.Unlock()
	now := m.deps.Now()
	if !m.mesh.tried.IsZero() && now.Sub(m.mesh.tried) < m.mesh.wait() {
		if m.mesh.last == nil {
			return nil, ErrMeshWaiting
		}
		return m.mesh.last, nil
	}
	m.mesh.tried = now
	list, err := m.readMesh(ctx)
	if err != nil {
		m.mesh.failures++
		m.mesh.closeConn()
		return nil, err
	}
	m.mesh.failures = 0
	m.mesh.last = &models.MeshClients{At: now, Clients: list}
	m.mesh.armIdle()
	return m.mesh.last, nil
}

// readMesh reads the list once, signing in first when no connection is open. mu is held.
func (m *Manager) readMesh(ctx context.Context) ([]models.MeshClient, error) {
	if m.deps.Dial == nil {
		return nil, ErrNotSet
	}
	if m.mesh.conn == nil {
		addr, user, hostKey := m.deps.Settings.Get()
		key := m.deps.Secret.Value()
		if addr == "" || user == "" || key == "" {
			return nil, ErrNotSet
		}
		c, err := m.deps.Dial(ctx, routerssh.Target{Address: addr, User: user, Key: []byte(key), HostKey: hostKey})
		if err != nil {
			return nil, err
		}
		m.mesh.conn = c
	}
	raw, err := m.mesh.conn.ReadClients(ctx)
	if err != nil {
		return nil, err
	}
	return ParseClients(raw)
}

// CloseMesh closes the list's connection and forgets the last list: the setup changed, the key was removed, the data was reset or the
// service is stopping. The next read signs in again with whatever the setup is then.
func (m *Manager) CloseMesh() {
	m.mesh.mu.Lock()
	defer m.mesh.mu.Unlock()
	m.mesh.closeConn()
	m.mesh.last, m.mesh.tried, m.mesh.failures = nil, time.Time{}, 0
}

// wait is how long after a try the next may be: meshEvery, doubled for each failure in a row, at most meshMaxWait. mu is held.
func (s *mesh) wait() time.Duration {
	w := meshEvery
	for i := 0; i < s.failures && w < meshMaxWait; i++ {
		w *= 2
	}
	return min(w, meshMaxWait)
}

// armIdle closes the connection after idleFor without a read. mu is held.
func (s *mesh) armIdle() {
	idle := s.idleFor
	if idle == 0 {
		idle = meshIdle
	}
	if s.idle != nil {
		s.idle.Stop()
	}
	conn := s.conn
	s.idle = time.AfterFunc(idle, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.conn == conn {
			s.closeConn()
		}
	})
}

// closeConn closes the open connection, if there is one. mu is held.
func (s *mesh) closeConn() {
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}
