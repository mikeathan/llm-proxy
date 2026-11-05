package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// The follow-up scan: a device that just appeared gets one ordinary service scan of its own address, so what it offers is known
// within minutes rather than at the next nightly audit. It is off until the operator turns it on. It goes through the scan
// service (the declared scope, one scan at a time), never runs the deep check, and involves no model: the presence check that
// finds the device is a model-free loop (ADR-0014). Its findings are told as counts in one alert and are not stored: the nightly
// audit scans the device again and compares it with the rest.
const (
	// followUpFresh is how recently a device must have first appeared to count as just arrived.
	followUpFresh = time.Hour
	// followUpEvery is how long a device waits before it can be followed up again, and followUpPerHour how many scans one hour
	// may hold, so a burst of arrivals (or a phone that keeps looking new) cannot keep the scanner busy.
	followUpEvery   = 24 * time.Hour
	followUpPerHour = 3
	// followUpPending bounds the devices waiting for their scan.
	followUpPending = 16
	// followUpTimeout bounds one scan; followUpRetry is how often a deferred scan is tried again.
	followUpTimeout = 10 * time.Minute
	followUpRetry   = time.Minute
	// followUpNamedMost bounds the finding titles named in the alert.
	followUpNamedMost = 3
)

// FollowUpDeps are what the follow-up needs; the application wires them in one place.
type FollowUpDeps struct {
	Enabled func() bool
	// Scan runs one ordinary service scan of one address through the scan service; ErrScanBusy defers it.
	Scan    func(ctx context.Context, addr string) (Result, error)
	Notify  func(ctx context.Context, message string) error
	Now     func() time.Time
	OnError func(error)
}

type followUpItem struct{ mac, ip string }

// FollowUp keeps the arrivals waiting for their scan and runs them one at a time from its own loop.
type FollowUp struct {
	deps FollowUpDeps
	wake chan struct{}

	mu      sync.Mutex
	pending []followUpItem
	scanned map[string]time.Time // hardware address -> when it was last followed up
	recent  []time.Time          // the scans of the last hour
}

func NewFollowUp(deps FollowUpDeps) *FollowUp {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &FollowUp{deps: deps, wake: make(chan struct{}, 1), scanned: map[string]time.Time{}}
}

// Offer looks at one presence look and queues the devices that just arrived. It returns at once; it rides on the presence look.
// A device with a private (randomized) hardware address, one the operator approved or named, and one seen before are left out.
func (f *FollowUp) Offer(snap Snapshot) {
	if f.deps.Enabled == nil || !f.deps.Enabled() {
		return
	}
	now := f.deps.Now()
	f.mu.Lock()
	for mac, at := range f.scanned {
		if now.Sub(at) >= followUpEvery {
			delete(f.scanned, mac)
		}
	}
	for _, d := range snap.Inventory.Devices {
		if f.wanted(d, now) && len(f.pending) < followUpPending {
			f.pending = append(f.pending, followUpItem{mac: d.MAC, ip: d.IP})
		}
	}
	queued := len(f.pending) > 0
	f.mu.Unlock()
	if queued {
		select {
		case f.wake <- struct{}{}:
		default:
		}
	}
}

// wanted reports whether a device is a fresh arrival not followed up or waiting already. f.mu is held.
func (f *FollowUp) wanted(d models.NetworkDevice, now time.Time) bool {
	if d.Approved || d.Name != "" || d.Randomized || now.Sub(d.FirstSeen) > followUpFresh {
		return false
	}
	if _, err := netip.ParseAddr(d.IP); err != nil {
		return false
	}
	if _, done := f.scanned[d.MAC]; done {
		return false
	}
	return !slices.ContainsFunc(f.pending, func(p followUpItem) bool { return p.mac == d.MAC })
}

// Run takes the waiting scans one at a time until ctx ends. It is started from the one place the Network loops start
// (Constitution II.2, II.14).
func (f *FollowUp) Run(ctx context.Context) {
	ticker := time.NewTicker(followUpRetry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.wake:
		case <-ticker.C:
		}
		f.Step(ctx)
	}
}

// Step runs the first waiting scan, if the hour has room for it. A scan that finds another one running stays first in line.
func (f *FollowUp) Step(ctx context.Context) {
	item, ok := f.next()
	if !ok || !f.deps.Enabled() {
		return
	}
	scanCtx, cancel := context.WithTimeout(ctx, followUpTimeout)
	res, err := f.deps.Scan(scanCtx, item.ip)
	cancel()
	if errors.Is(err, ErrScanBusy) || ctx.Err() != nil {
		return // deferred: tried again on the next turn
	}
	f.done(item)
	if err != nil {
		f.report(fmt.Errorf("follow-up scan: %w", err))
		return
	}
	if f.deps.Notify != nil {
		if err := f.deps.Notify(ctx, FollowUpMessage(item.ip, FindingsOf(res.Hosts))); err != nil && !errors.Is(err, ErrNoConnector) {
			f.report(fmt.Errorf("follow-up alert: %w", err))
		}
	}
}

// next is the first waiting scan, when fewer than followUpPerHour ran in the last hour.
func (f *FollowUp) next() (followUpItem, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.deps.Now()
	f.recent = slices.DeleteFunc(f.recent, func(at time.Time) bool { return now.Sub(at) >= time.Hour })
	if len(f.pending) == 0 || len(f.recent) >= followUpPerHour {
		return followUpItem{}, false
	}
	return f.pending[0], true
}

// done takes a scan out of the line and counts it, whatever its outcome, so a failing one is not retried at once.
func (f *FollowUp) done(item followUpItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.deps.Now()
	f.pending = slices.DeleteFunc(f.pending, func(p followUpItem) bool { return p.mac == item.mac })
	f.scanned[item.mac] = now
	f.recent = append(f.recent, now)
}

func (f *FollowUp) report(err error) {
	if f.deps.OnError != nil {
		f.deps.OnError(err)
	}
}

// FollowUpMessage is the alert for one follow-up scan: the address the network gave the device, how many ports it has open, and
// the fixed titles of what is worth a look on them. Nothing the device sent is in it.
func FollowUpMessage(addr string, findings []Finding) string {
	open := 0
	var worth []string
	for _, f := range findings {
		switch {
		case f.Check == CheckOpenPort:
			open++
		case f.Severity != SeverityInfo:
			if title := CheckTitle(f.Check); title != "" && !slices.Contains(worth, title) {
				worth = append(worth, title)
			}
		}
	}
	msg := fmt.Sprintf("Network audit: the new device at %s was scanned on arrival: it has %s.", addr, plural(open, "open port", "open ports"))
	if len(worth) > 0 {
		shown := worth[:min(len(worth), followUpNamedMost)]
		msg += " Worth a look: " + strings.Join(shown, "; ") + "."
	}
	return msg + " The nightly audit compares it with the rest."
}

// FollowUpConfig reads and changes whether new devices are scanned on arrival. It is off until the operator turns it on.
type FollowUpConfig struct{ store PolicyStore }

func NewFollowUpConfig(store PolicyStore) *FollowUpConfig { return &FollowUpConfig{store: store} }

// Enabled reports whether new devices are scanned on arrival.
func (c *FollowUpConfig) Enabled() bool { return c.store.Get().FollowUpScan }

// Set switches the scan on arrival on or off.
func (c *FollowUpConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.FollowUpScan = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the scan-on-arrival setting: %w", err)
	}
	return nil
}
