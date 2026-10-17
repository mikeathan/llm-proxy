package memory

import (
	"context"
	"time"

	"llm-proxy/internal/platform/logging"
)

// DefaultExpiryReaperInterval is the sweep cadence used when wiring the reaper.
const DefaultExpiryReaperInterval = time.Hour

// ExpiryReaper deletes facts whose expiry has passed. A fact gets an expiry when it is written (a session fact, or one
// the model or the operator dated), so the reaper needs no retention setting of its own and reaches every scope: a
// user-scope session fact used to be stored as a user profile entry and never reaped. Facts without an expiry are
// operator data and are never deleted by age.
//
// It mirrors the ledger Cleaner and run reaper: create with NewExpiryReaper, call Start(ctx) in a goroutine tethered
// to the app lifecycle.
type ExpiryReaper struct {
	store    func() *Store
	interval time.Duration
}

// NewExpiryReaper takes a provider rather than a value: the store is replaced on factory reset and may be nil when the
// database failed to open.
func NewExpiryReaper(store func() *Store, interval time.Duration) *ExpiryReaper {
	return &ExpiryReaper{store: store, interval: interval}
}

// Start sweeps once immediately, then on the interval, until ctx is cancelled.
func (r *ExpiryReaper) Start(ctx context.Context) {
	logging.Info("Memory expiry reaper started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer func() {
		ticker.Stop()
		logging.Info("Memory expiry reaper stopped")
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

// reap performs one sweep and returns the number of facts deleted.
func (r *ExpiryReaper) reap(ctx context.Context) int64 {
	store := r.store()
	if store == nil || ctx.Err() != nil {
		return 0
	}
	n, err := store.DeleteExpired(ctx, time.Now())
	if err != nil {
		logging.Warn("Memory expiry reaper: delete failed", "error", err)
		return 0
	}
	if n > 0 {
		logging.Info("Memory expiry reaper: removed expired facts", "count", n)
	}
	return n
}
