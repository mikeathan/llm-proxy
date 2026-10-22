package delegation

import (
	"context"
	"errors"
	"sync"
	"time"
)

// errPoolBusy is a task that waited its full turn for a sub-agent slot.
var errPoolBusy = errors.New("every sub-agent slot is in use")

// pool bounds how many sub-agents run at once across the app. It stands beside the run lanes: a child never claims a
// lane slot its parent may hold, so it can neither wait on its parent nor pre-empt it. The limit is read on every
// acquire, so a settings change applies to the next task.
type pool struct {
	mu    sync.Mutex
	inUse int
	limit func() int
	freed chan struct{} // closed and replaced on every release, waking every waiter to try again
}

func newPool(limit func() int) *pool {
	return &pool{limit: limit, freed: make(chan struct{})}
}

// acquire takes a slot, waiting at most wait for one; release must be called exactly once.
func (p *pool) acquire(ctx context.Context, wait time.Duration) (release func(), err error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		p.mu.Lock()
		if p.inUse < p.limit() {
			p.inUse++
			p.mu.Unlock()
			return sync.OnceFunc(p.release), nil
		}
		freed := p.freed
		p.mu.Unlock()
		select {
		case <-freed:
		case <-timer.C:
			return nil, errPoolBusy
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *pool) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inUse--
	close(p.freed)
	p.freed = make(chan struct{})
}
