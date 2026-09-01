package ledger

import (
	"context"
	"time"

	"llm-proxy/internal/platform/logging"
)

// Cleaner periodically removes stale data from the SQLite ledger.
// Create with NewCleaner, call Start(ctx) to run in a background
// goroutine. Stops automatically when ctx is cancelled.
type Cleaner struct {
	store     *Store
	interval  time.Duration
	retention time.Duration
}

func NewCleaner(store *Store, interval, retention time.Duration) *Cleaner {
	return &Cleaner{
		store:     store,
		interval:  interval,
		retention: retention,
	}
}

func (c *Cleaner) Start(ctx context.Context) {
	logging.Info("Ledger cleaner started", "interval", c.interval, "retention", c.retention)
	ticker := time.NewTicker(c.interval)
	defer func() {
		ticker.Stop()
		logging.Info("Ledger cleaner stopped")
	}()

	c.clean(ctx)

	for {
		select {
		case <-ticker.C:
			c.clean(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (c *Cleaner) clean(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	logging.Debug("Ledger cleaner: running cleanup")
	if err := c.store.ExpireSlots(ctx); err != nil {
		logging.Warn("Ledger cleaner: expire slots failed", "error", err)
	}

	cutoff := time.Now().UTC().Add(-c.retention)
	if err := c.store.PurgeTransactions(ctx, cutoff); err != nil {
		logging.Warn("Ledger cleaner: purge transactions failed", "error", err)
	}
	if err := c.store.PurgeBalances(ctx, cutoff); err != nil {
		logging.Warn("Ledger cleaner: purge balances failed", "error", err)
	}
}
