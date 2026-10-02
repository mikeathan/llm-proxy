package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"llm-proxy/internal/platform/logging"
)

// Usage counters tell the operator which facts the model actually gets: how
// many runs a fact was sent in (injected) and how many times memory_search
// returned it (searched), plus when it was last used.
//
// They must never put a database write on a run's hot path, so recording is an
// in-memory increment under a mutex and a background UsageFlusher writes the
// accumulated deltas in one transaction. A crash loses at most one flush
// interval of counts; a failed flush keeps them for the next attempt.

// DefaultUsageFlushInterval is how often the flusher writes pending counts.
const DefaultUsageFlushInterval = 30 * time.Second

// usageFinalFlushTimeout bounds the flush done on shutdown, after the root
// context is already cancelled.
const usageFinalFlushTimeout = 5 * time.Second

const flushUsageSQL = `UPDATE memories
SET injected_count = injected_count + ?, searched_count = searched_count + ?,
    last_used_at = MAX(COALESCE(last_used_at, ''), ?)
WHERE workspace_id = ? AND id = ?`

type usageKey struct {
	workspaceID string
	id          int64
}

type usageDelta struct {
	injected int
	searched int
	last     time.Time
}

// usageRecorder accumulates pending deltas. The zero value is ready to use.
type usageRecorder struct {
	mu      sync.Mutex
	pending map[usageKey]*usageDelta
}

func (r *usageRecorder) record(entries []MemoryEntry, apply func(*usageDelta)) {
	if len(entries) == 0 {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = make(map[usageKey]*usageDelta, len(entries))
	}
	for _, e := range entries {
		k := usageKey{e.WorkspaceID, e.ID}
		d := r.pending[k]
		if d == nil {
			d = &usageDelta{}
			r.pending[k] = d
		}
		apply(d)
		d.last = now
	}
}

// take removes and returns everything pending.
func (r *usageRecorder) take() map[usageKey]*usageDelta {
	r.mu.Lock()
	defer r.mu.Unlock()
	batch := r.pending
	r.pending = nil
	return batch
}

// restore merges a batch that could not be written back into the pending set.
func (r *usageRecorder) restore(batch map[usageKey]*usageDelta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = make(map[usageKey]*usageDelta, len(batch))
	}
	for k, d := range batch {
		cur := r.pending[k]
		if cur == nil {
			r.pending[k] = d
			continue
		}
		cur.injected += d.injected
		cur.searched += d.searched
		if d.last.After(cur.last) {
			cur.last = d.last
		}
	}
}

// RecordInjected notes that these facts were sent to the model in a run. It is
// in-memory only; nothing reaches the database until FlushUsage.
func (s *Store) RecordInjected(entries []MemoryEntry) {
	s.usage.record(entries, func(d *usageDelta) { d.injected++ })
}

// RecordSearched notes that memory_search returned these facts to the model.
func (s *Store) RecordSearched(entries []MemoryEntry) {
	s.usage.record(entries, func(d *usageDelta) { d.searched++ })
}

// FlushUsage writes the pending counts in a single transaction. On any failure
// the counts are kept for the next flush.
func (s *Store) FlushUsage(ctx context.Context) error {
	batch := s.usage.take()
	if len(batch) == 0 {
		return nil
	}
	if err := s.writeUsage(ctx, batch); err != nil {
		s.usage.restore(batch)
		return err
	}
	return nil
}

func (s *Store) writeUsage(ctx context.Context, batch map[usageKey]*usageDelta) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory usage begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	stmt, err := tx.PrepareContext(ctx, flushUsageSQL)
	if err != nil {
		return fmt.Errorf("memory usage prepare: %w", err)
	}
	defer stmt.Close()
	for k, d := range batch {
		if _, err := stmt.ExecContext(ctx, d.injected, d.searched, d.last.Format(sqliteDateTimeFormat), k.workspaceID, k.id); err != nil {
			return fmt.Errorf("memory usage update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("memory usage commit: %w", err)
	}
	return nil
}

// listUnusedSQL lists facts never sent to a model and never returned by search.
const listUnusedSQL = selectColumnsSQL + ` FROM memories WHERE workspace_id = ? AND injected_count = 0 AND searched_count = 0 ORDER BY updated_at DESC LIMIT ? OFFSET ?`

// ListUnused returns the workspace's facts that have never been used. Counting
// started when this feature was introduced, so older facts read as unused until
// their first use.
func (s *Store) ListUnused(ctx context.Context, workspaceID string, limit, offset int) ([]MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, listUnusedSQL, workspaceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("memory list unused: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// UsageFlusher periodically writes pending usage counts. Create with
// NewUsageFlusher and run Start in a goroutine tethered to the app lifecycle.
type UsageFlusher struct {
	store    func() *Store
	interval time.Duration
}

// NewUsageFlusher takes a store provider: the store is replaced on factory
// reset and may be nil when the database failed to open.
func NewUsageFlusher(store func() *Store, interval time.Duration) *UsageFlusher {
	return &UsageFlusher{store: store, interval: interval}
}

// Start flushes on the interval until ctx is cancelled, then once more so the
// last window of counts is not lost on shutdown.
func (f *UsageFlusher) Start(ctx context.Context) {
	logging.Info("Memory usage flusher started", "interval", f.interval)
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.flush(ctx)
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), usageFinalFlushTimeout)
			defer cancel()
			f.flush(final)
			logging.Info("Memory usage flusher stopped")
			return
		}
	}
}

func (f *UsageFlusher) flush(ctx context.Context) {
	store := f.store()
	if store == nil {
		return
	}
	if err := store.FlushUsage(ctx); err != nil {
		logging.Warn("Memory usage flush failed; counts kept for the next attempt", "error", err)
	}
}
