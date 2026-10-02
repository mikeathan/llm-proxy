package memory

import (
	"context"
	"time"

	"llm-proxy/internal/platform/logging"
)

// DefaultSessionReaperInterval is the sweep cadence used when wiring the reaper.
const DefaultSessionReaperInterval = time.Hour

// SessionReaper deletes session-scoped memories older than the retention
// window. Unattended runs write session entries by default
// (memory_update, Phase 3), so without a reaper they would accumulate forever.
// It mirrors the ledger Cleaner and run reaper: create with NewSessionReaper,
// call Start(ctx) in a goroutine tethered to the app lifecycle.
//
// Only MemoryType Session is ever deleted — long_term and user_profile entries
// are operator data regardless of age.
type SessionReaper struct {
	store     func() *Store
	retention func() time.Duration
	interval  time.Duration
}

// NewSessionReaper takes providers rather than values: the store is replaced on
// factory reset and may be nil when the database failed to open, and the
// retention comes from settings that can change while the server runs.
// A non-positive retention disables reaping.
func NewSessionReaper(store func() *Store, retention func() time.Duration, interval time.Duration) *SessionReaper {
	return &SessionReaper{store: store, retention: retention, interval: interval}
}

// Start sweeps once immediately, then on the interval, until ctx is cancelled.
func (r *SessionReaper) Start(ctx context.Context) {
	logging.Info("Memory session reaper started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer func() {
		ticker.Stop()
		logging.Info("Memory session reaper stopped")
	}()

	r.reap(ctx)
	for {
		select {
		case <-ticker.C:
			r.reap(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// reap performs one sweep and returns the number of entries deleted.
func (r *SessionReaper) reap(ctx context.Context) int64 {
	store := r.store()
	retention := r.retention()
	if store == nil || retention <= 0 || ctx.Err() != nil {
		return 0
	}
	n, err := store.DeleteOlderThan(ctx, Session, time.Now().Add(-retention))
	if err != nil {
		logging.Warn("Memory session reaper: delete failed", "error", err)
		return 0
	}
	if n > 0 {
		logging.Info("Memory session reaper: removed stale session entries", "count", n)
	}
	return n
}
