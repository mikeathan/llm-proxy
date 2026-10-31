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
	// OnLook, if set, is given each look the check took (after the inventory is updated), so other work that belongs to a
	// look can ride on it, such as asking unnamed devices their names. It must stop when ctx ends and must not alert.
	OnLook func(ctx context.Context, snap Snapshot)
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
	snap, err := p.deps.Inventory.Refresh(ctx)
	if err != nil {
		p.trouble(fmt.Errorf("presence check: %w", err))
		return
	}
	// The alert goes first: whatever rides on a look (naming devices, the weekly summary) may be slow, and must never hold
	// an alert back. It runs even when the alert could not be sent, because it does not depend on a connector.
	alertErr := p.alert(ctx)
	if p.deps.OnLook != nil {
		p.deps.OnLook(ctx, snap)
	}
	if alertErr != nil {
		return
	}
	p.mu.Lock()
	p.last, p.lastCheck = "", p.deps.Now()
	p.mu.Unlock()
}

// alert tells the operator about whatever is new and marks it told only once it was sent. A failure is reported once per
// cause and leaves the alert pending.
func (p *Presence) alert(ctx context.Context) error {
	pending, err := p.deps.Inventory.Pending()
	if err != nil {
		err = fmt.Errorf("presence check: %w", err)
		p.trouble(err)
		return err
	}
	if pending.Empty() {
		return nil
	}
	if err := p.deps.Notify(ctx, PresenceMessage(pending)); err != nil {
		err = fmt.Errorf("presence check: could not send the alert: %w", err)
		p.trouble(err)
		return err
	}
	if err := p.deps.Inventory.MarkNotified(pending); err != nil {
		err = fmt.Errorf("presence check: %w", err)
		p.trouble(err)
		return err
	}
	return nil
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

// maxListed is how many addresses one message names; the rest are counted.
const maxListed = 5

// PresenceMessage is the alert: counts and fixed words, and where the devices are. Addresses come from this computer's own
// neighbour table; what a device calls itself, and what the operator named a device, are never in it.
func PresenceMessage(p Pending) string {
	var parts []string
	if p.NewDevices > 0 {
		parts = append(parts, plural(p.NewDevices, "new device", "new devices")+listed(p.newAddrs))
	}
	if p.Changes > 0 {
		parts = append(parts, plural(p.Changes, "address now used by a different device", "addresses now used by a different device")+listed(p.movedAddrs))
	}
	if p.Gateway {
		parts = append(parts, "the router's hardware address changed")
	}
	return "Network audit: " + strings.Join(parts, "; ") + ". Open the network audit to review."
}

// listed names up to maxListed addresses after a colon, then counts the rest. No address list gives no suffix.
func listed(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	shown := addrs
	if len(shown) > maxListed {
		shown = shown[:maxListed]
	}
	out := ": " + strings.Join(shown, ", ")
	if rest := len(addrs) - len(shown); rest > 0 {
		out += fmt.Sprintf(" and %d more", rest)
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
