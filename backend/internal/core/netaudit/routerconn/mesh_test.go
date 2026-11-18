package routerconn

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/platform/routerssh"
)

// fakeConn is a kept-open connection that answers the list and notes how it was used.
type fakeConn struct {
	mu     sync.Mutex
	reads  int
	closed bool
	fail   error
}

func (c *fakeConn) ReadClients(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("closed")
	}
	c.reads++
	if c.fail != nil {
		return nil, c.fail
	}
	return []byte(`{"24:00:00:00:00:60":{"5G":{"0a:00:00:00:00:01":{"ip":"192.168.50.9","rssi":"-57"}}}}`), nil
}

func (c *fakeConn) Close() error { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }

type meshRig struct {
	m      *Manager
	set    *memSettings
	secret *memSecret
	now    time.Time
	dials  []routerssh.Target
	conns  []*fakeConn
	fail   error
}

func newMeshRig(t *testing.T) *meshRig {
	t.Helper()
	r := &meshRig{set: &memSettings{addr: "192.168.50.1", user: "admin", host: "ssh-ed25519 AAAA"}, secret: &memSecret{v: "KEY"}, now: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)}
	r.m = New(Deps{
		Settings: r.set, Secret: r.secret,
		Dial: func(_ context.Context, tg routerssh.Target) (ClientReader, error) {
			r.dials = append(r.dials, tg)
			c := &fakeConn{fail: r.fail}
			r.conns = append(r.conns, c)
			return c, nil
		},
		Now: func() time.Time { return r.now },
	})
	return r
}

func TestTheListIsReadAtMostOnceEveryThirtySecondsOnOneConnection(t *testing.T) {
	r := newMeshRig(t)
	ctx := context.Background()
	for i := range 5 {
		got, err := r.m.MeshClients(ctx)
		if err != nil || len(got.Clients) != 1 || got.Clients[0].RSSI != -57 {
			t.Fatalf("ask %d: %+v %v", i, got, err)
		}
		r.now = r.now.Add(10 * time.Second)
	}
	if len(r.dials) != 1 || r.conns[0].reads != 2 {
		t.Errorf("five asks in 50 s: one sign-in and two reads (at 0 and 30 s), got %d sign-ins and %d reads", len(r.dials), r.conns[0].reads)
	}
	if r.dials[0].HostKey != "ssh-ed25519 AAAA" || string(r.dials[0].Key) != "KEY" {
		t.Errorf("signs in with the setup's pinned key and key: %+v", r.dials[0])
	}
}

func TestEverySetupChangeClosesTheListsConnection(t *testing.T) {
	ctx := context.Background()
	for name, change := range map[string]func(m *Manager) error{
		"remove the key": func(m *Manager) error { return m.ClearKey() },
		"new address":    func(m *Manager) error { return m.SetTarget("192.168.50.2", "admin") },
		"new user":       func(m *Manager) error { return m.SetTarget("192.168.50.1", "root") },
		"factory reset":  func(m *Manager) error { m.CloseMesh(); return nil },
	} {
		r := newMeshRig(t)
		if _, err := r.m.MeshClients(ctx); err != nil {
			t.Fatal(err)
		}
		if err := change(r.m); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !r.conns[0].closed {
			t.Errorf("%s: the open connection must close", name)
		}
		_, err := r.m.MeshClients(ctx)
		switch name {
		case "remove the key":
			if !errors.Is(err, ErrNotSet) || len(r.dials) != 1 {
				t.Errorf("%s: no key, no sign-in: %v (%d sign-ins)", name, err, len(r.dials))
			}
		default:
			if err != nil || len(r.dials) != 2 {
				t.Errorf("%s: the next read signs in again with the new setup: %v (%d sign-ins)", name, err, len(r.dials))
			}
		}
	}
}

func TestAFailedReadClosesTheConnectionAndTheWaitGrows(t *testing.T) {
	r := newMeshRig(t)
	r.fail = errors.New("router gone")
	ctx := context.Background()
	if _, err := r.m.MeshClients(ctx); err == nil {
		t.Fatal("a failed read is an error")
	}
	if !r.conns[0].closed {
		t.Error("a failed read closes the connection")
	}
	r.now = r.now.Add(45 * time.Second)
	if _, err := r.m.MeshClients(ctx); !errors.Is(err, ErrMeshWaiting) || len(r.dials) != 1 {
		t.Errorf("after one failure the wait is 60 s: %v (%d sign-ins)", err, len(r.dials))
	}
	r.fail = nil
	r.now = r.now.Add(20 * time.Second)
	if got, err := r.m.MeshClients(ctx); err != nil || len(got.Clients) != 1 || len(r.dials) != 2 {
		t.Errorf("then it tries again: %+v %v", got, err)
	}
	r.m.mesh.failures = 20
	if w := r.m.mesh.wait(); w != meshMaxWait {
		t.Errorf("the wait stops growing at %v: %v", meshMaxWait, w)
	}
}

func TestAnIdleConnectionIsClosed(t *testing.T) {
	r := newMeshRig(t)
	r.m.mesh.idleFor = 10 * time.Millisecond
	if _, err := r.m.MeshClients(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.conns[0].mu.Lock()
		closed := r.conns[0].closed
		r.conns[0].mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a connection with no read for the idle time must close")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.m.mesh.mu.Lock()
	defer r.m.mesh.mu.Unlock()
	if r.m.mesh.conn != nil {
		t.Error("the manager forgets the closed connection")
	}
}

func TestReadingTheListNeverMakesTheOperatorsReadBusy(t *testing.T) {
	r := newMeshRig(t)
	r.m.deps.Read = func(context.Context, routerssh.Target) ([]byte, error) { return []byte(table), nil }
	if _, err := r.m.MeshClients(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.Read(context.Background()); err != nil {
		t.Errorf("the operator's read of the table is its own: %v", err)
	}
}
