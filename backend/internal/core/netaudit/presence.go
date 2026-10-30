package netaudit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Presence is the light, frequent look at who is on the network. It makes the host talk to every address in the declared scan
// scope (the sweep: quick connection attempts, no program, no model) and reads the neighbour table, so a device that was not
// there before is noticed in minutes and not at the next nightly audit. It never starts a model and never scans a port beyond
// what the sweep tries.
type Presence struct {
	deps PresenceDeps

	mu        sync.Mutex
	last      string // the trouble reported last, so a repeating failure is logged once
	lastCheck time.Time
}

// PresenceStatus says when the check last completed and what, if anything, is wrong with it now.
type PresenceStatus struct {
	LastCheck time.Time `json:"last_check,omitempty"`
	// Problem is plain words for why the check cannot look, or empty. It is never "nobody is there".
	Problem string `json:"problem,omitempty"`
}

// Status reports the last completed cycle and the trouble that stopped the latest one.
func (p *Presence) Status() PresenceStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PresenceStatus{LastCheck: p.lastCheck, Problem: p.last}
}

// PresenceSettings are the operator's choices, read for every cycle so a change applies to the next one.
type PresenceSettings struct {
	// Enabled is off until the operator turns it on.
	Enabled bool
	Every   time.Duration
	// Targets are the declared scan ranges.
	Targets []string
}

// PresenceInventory is the part of *Inventory the check uses.
type PresenceInventory interface {
	Refresh(ctx context.Context) (Snapshot, error)
	Pending() (Pending, error)
	MarkNotified(Pending) error
}

// PresenceDeps are what the check needs; the application wires them in one place.
type PresenceDeps struct {
	Settings func() PresenceSettings
	// Sweep makes the host talk to every address of the targets. ErrScanBusy means a scan is running, which skips the cycle.
	Sweep     func(ctx context.Context, targets []string) error
	Inventory PresenceInventory
	// Notify sends one short message to the operator's connectors.
	Notify func(ctx context.Context, message string) error
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// OnError is told about trouble, once per distinct message until the trouble ends.
	OnError func(error)
}

// idlePoll is how often a disabled check looks again at its setting.
const idlePoll = 30 * time.Second

// NewPresence builds the check. Nothing runs until Run.
func NewPresence(deps PresenceDeps) *Presence {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Presence{deps: deps}
}

// Run does a cycle every Every while enabled, and stops when ctx ends.
func (p *Presence) Run(ctx context.Context) {
	for {
		wait := idlePoll
		if s := p.deps.Settings(); s.Enabled {
			p.Tick(ctx)
			wait = max(s.Every, time.Millisecond)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Tick is one cycle: sweep, read the neighbour table, and tell the operator about anything new.
func (p *Presence) Tick(ctx context.Context) {
	s := p.deps.Settings()
	if !s.Enabled {
		return
	}
	if err := p.deps.Sweep(ctx, s.Targets); err != nil {
		if !errors.Is(err, ErrScanBusy) {
			p.trouble(fmt.Errorf("presence check: %w", err))
		}
		return
	}
	if _, err := p.deps.Inventory.Refresh(ctx); err != nil {
		p.trouble(fmt.Errorf("presence check: %w", err))
		return
	}
	pending, err := p.deps.Inventory.Pending()
	if err != nil {
		p.trouble(fmt.Errorf("presence check: %w", err))
		return
	}
	if !pending.Empty() {
		if err := p.deps.Notify(ctx, PresenceMessage(pending)); err != nil {
			p.trouble(fmt.Errorf("presence check: could not send the alert: %w", err))
			return
		}
		if err := p.deps.Inventory.MarkNotified(pending); err != nil {
			p.trouble(fmt.Errorf("presence check: %w", err))
			return
		}
	}
	p.mu.Lock()
	p.last, p.lastCheck = "", p.deps.Now()
	p.mu.Unlock()
}

func (p *Presence) trouble(err error) {
	p.mu.Lock()
	same := err.Error() == p.last
	p.last = err.Error()
	p.mu.Unlock()
	if same {
		return
	}
	if p.deps.OnError != nil {
		p.deps.OnError(err)
	}
}

// PresenceMessage is the alert: counts and fixed words, never what a device calls itself or what the operator named it.
func PresenceMessage(p Pending) string {
	var parts []string
	if p.NewDevices > 0 {
		parts = append(parts, plural(p.NewDevices, "new device", "new devices"))
	}
	if p.Changes > 0 {
		parts = append(parts, plural(p.Changes, "address now used by a different device", "addresses now used by a different device"))
	}
	if p.Gateway {
		parts = append(parts, "the router's hardware address changed")
	}
	return "Network audit: " + strings.Join(parts, ", ") + ". Open the network audit to review."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
