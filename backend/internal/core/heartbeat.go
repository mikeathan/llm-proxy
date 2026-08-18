// heartbeat.go — A generic, mechanism-only periodic ticker for long-running
// operations that must signal liveness to an owner loop (agent stream
// processing, fallback waits, transport keepalive, …). It is a single-file
// reusable component in the parent `core` package (precedent: ttl_cache.go).
//
// The owner starts the heartbeat with a context and interval, then consumes
// ticks from the buffered C channel in its own select. If the owner is busy
// (not reading C), ticks are dropped rather than queued — the heartbeat is a
// "still alive" signal, not a work queue. The goroutine exits on ctx.Done() or
// Stop(), satisfying the goroutine-lifecycle discipline (Constitution II.14).
package core

import (
	"context"
	"sync"
	"time"
)

// Heartbeat emits one tick per interval into a buffered size-1 channel that the
// owner consumes via its select loop. It is mechanism-only: the phase/payload
// meaning of a tick is supplied at each call site.
type Heartbeat struct {
	// C yields one tick per interval when the owner is ready to receive;
	// ticks are dropped when the owner is busy.
	C         <-chan time.Time
	c         chan time.Time
	stop      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewHeartbeat allocates a Heartbeat. It performs no I/O; call Start to begin.
func NewHeartbeat() *Heartbeat {
	c := make(chan time.Time, 1)
	return &Heartbeat{
		C:    c,
		c:    c,
		stop: make(chan struct{}),
	}
}

// Start launches the ticker goroutine. It is idempotent; a second call is a
// no-op. An interval <= 0 disables the heartbeat (no goroutine, no ticks).
// Start returns h so it can be chained, but callers typically just call it.
func (h *Heartbeat) Start(ctx context.Context, interval time.Duration) *Heartbeat {
	if interval <= 0 {
		return h
	}
	h.startOnce.Do(func() {
		go h.run(ctx, interval)
	})
	return h
}

// Stop terminates the ticker goroutine. It is idempotent and safe to call from
// any goroutine. Pair with defer hb.Stop() at each call site.
func (h *Heartbeat) Stop() {
	h.stopOnce.Do(func() {
		close(h.stop)
	})
}

func (h *Heartbeat) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stop:
			return
		case t := <-ticker.C:
			select {
			case h.c <- t:
			default:
				// Owner busy — drop the tick rather than queue it.
			}
		}
	}
}
