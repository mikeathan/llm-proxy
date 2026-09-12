// Package sizewatch provides best-effort workspace disk accounting for the
// sandboxing plan's max_storage_gb control (Phase 3). It is ACCOUNTING, not a
// kernel-enforced quota: sizes are computed by directory walk on a cached,
// TTL-bounded basis and checked before shell spawns. A single `dd`/`npm
// install` between checks can exceed the boundary (TOCTOU window) — callers
// label it as accounting, never as a hard quota.
package sizewatch

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// sizer measures a directory tree in bytes.
type sizer func(root string) (int64, error)

// SumDir returns the total size in bytes of every regular file under root.
// An unreadable root is an error (so the cache can fall back to the last known
// size); unreadable entries below an otherwise-walkable root are skipped.
func SumDir(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err // root itself unreadable → report, let the cache decide
			}
			return nil // skip unreadable entries rather than failing the scan
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// Cache memoizes per-workspace directory sizes for a TTL so checks are cheap
// after the first walk. Concurrent misses for the SAME root are collapsed to a
// single walk (a full workspace walk is expensive; two shell spawns racing the
// TTL must not both traverse the tree). Safe for concurrent use.
type Cache struct {
	ttl     time.Duration
	measure sizer

	mu       sync.Mutex
	sizes    map[string]sizeEntry
	inflight map[string]*sync.Mutex
}

type sizeEntry struct {
	bytes int64
	at    time.Time
}

// NewCache returns a size cache whose entries expire after ttl; measurement
// uses SumDir. A non-positive ttl falls back to 60s.
func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Cache{
		ttl:      ttl,
		measure:  SumDir,
		sizes:    map[string]sizeEntry{},
		inflight: map[string]*sync.Mutex{},
	}
}

// Bytes returns the cached size of root, measuring it when the entry is stale
// or absent. Errors (unreadable root) return the last known size, or 0 when
// never measured.
func (c *Cache) Bytes(root string, now time.Time) int64 {
	if size, ok := c.fresh(root, now); ok {
		return size
	}

	// Serialize measurement per root so concurrent misses share one walk.
	gate := c.gate(root)
	gate.Lock()
	defer gate.Unlock()

	// Re-check: another goroutine may have measured while we waited.
	if size, ok := c.fresh(root, now); ok {
		return size
	}

	previous := c.last(root)
	size, err := c.measure(root)
	if err != nil {
		return previous
	}

	c.mu.Lock()
	c.sizes[root] = sizeEntry{bytes: size, at: now}
	c.mu.Unlock()
	return size
}

// fresh returns the cached size when the entry is within the TTL.
func (c *Cache) fresh(root string, now time.Time) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.sizes[root]
	if !ok || now.Sub(e.at) >= c.ttl {
		return 0, false
	}
	return e.bytes, true
}

// last returns the most recent measured size (0 when never measured).
func (c *Cache) last(root string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sizes[root].bytes
}

// gate returns the per-root measurement mutex (created on first use). Entries
// are bounded by the number of distinct roots (workspaces) and dropped by
// Forget on workspace deletion.
func (c *Cache) gate(root string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	gate, ok := c.inflight[root]
	if !ok {
		gate = &sync.Mutex{}
		c.inflight[root] = gate
	}
	return gate
}

// Forget drops a cached entry (workspace deletion).
func (c *Cache) Forget(root string) {
	c.mu.Lock()
	delete(c.sizes, root)
	delete(c.inflight, root)
	c.mu.Unlock()
}
