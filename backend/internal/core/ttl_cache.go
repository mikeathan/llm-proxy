package core

import (
	"errors"
	"sync"
	"time"
)

// ErrNoCache is returned by a Get loader to deliver a value without storing
// it. Use it when the loaded value is usable for this call but must not be
// cached (e.g. a side-channel check failed and the entry would never be
// servable as a hit, which would otherwise occupy a bounded cache slot).
var ErrNoCache = errors.New("ttlcache: loader declined to cache result")

// ttlEntry wraps a cached value with its creation time for TTL + eviction.
type ttlEntry[V any] struct {
	value     V
	createdAt time.Time
}

// TTLCache is a concurrency-safe bounded cache with lazy TTL expiry and an
// optional validity predicate. It is the single shared implementation behind
// the previously duplicated bounded-TTL caches (PL-3 config cache, PL-5
// guardrail override cache, PL-6 agents-file cache).
//
// An entry is considered valid when BOTH hold:
//   - ttl <= 0, or time since creation is within ttl
//   - valid == nil, or valid(key, value) returns true
//
// The optional valid predicate carries domain-specific invalidation that TTL
// alone cannot express (e.g. PL-3's file-mtime check). When valid returns
// false the entry is treated as a miss and reloaded.
type TTLCache[K comparable, V any] struct {
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int // <=0 means unbounded
	valid      func(key K, value V) bool
	entries    map[K]ttlEntry[V]
	stopReaper chan struct{}
}

// NewTTLCache returns a TTLCache bounded by maxEntries (0 or negative = no
// bound) and entries that expire after ttl (0 disables expiry). valid, when
// non-nil, is consulted on every read; a false result forces a reload.
func NewTTLCache[K comparable, V any](maxEntries int, ttl time.Duration, valid func(key K, value V) bool) *TTLCache[K, V] {
	return &TTLCache[K, V]{
		ttl:        ttl,
		maxEntries: maxEntries,
		valid:      valid,
		entries:    make(map[K]ttlEntry[V]),
	}
}

// validEntry reports whether e is still usable under TTL + valid predicate.
func (c *TTLCache[K, V]) validEntry(key K, e ttlEntry[V]) bool {
	if c.ttl > 0 && time.Since(e.createdAt) > c.ttl {
		return false
	}
	if c.valid != nil && !c.valid(key, e.value) {
		return false
	}
	return true
}

// Get returns the cached value for key, loading it via load on miss, expiry,
// or invalid predicate. The loaded value is stored before being returned,
// unless load returns ErrNoCache (value returned, not stored).
func (c *TTLCache[K, V]) Get(key K, load func() (V, error)) (V, error) {
	c.mu.RLock()
	if e, ok := c.entries[key]; ok && c.validEntry(key, e) {
		v := e.value
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check under write lock: a concurrent load may have populated it.
	if e, ok := c.entries[key]; ok && c.validEntry(key, e) {
		return e.value, nil
	}
	delete(c.entries, key)

	v, err := load()
	if err != nil {
		if err == ErrNoCache {
			// Value is usable for this call but must not be cached.
			return v, nil
		}
		var zero V
		return zero, err
	}
	c.putLocked(key, v)
	return v, nil
}

// Put inserts or replaces key with value, evicting the oldest entry when at
// capacity.
func (c *TTLCache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(key, value)
}

func (c *TTLCache[K, V]) putLocked(key K, value V) {
	if c.maxEntries > 0 && len(c.entries) >= c.maxEntries {
		var oldestKey K
		var oldest time.Time
		first := true
		for k, e := range c.entries {
			if first || e.createdAt.Before(oldest) {
				oldestKey, oldest, first = k, e.createdAt, false
			}
		}
		if !first {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, createdAt: time.Now()}
}

// Contains reports whether a valid entry exists for key, deleting any stale
// entry it encounters. It does not load.
func (c *TTLCache[K, V]) Contains(key K) bool {
	c.mu.RLock()
	e, ok := c.entries[key]
	if ok && c.validEntry(key, e) {
		c.mu.RUnlock()
		return true
	}
	c.mu.RUnlock()

	if ok {
		c.mu.Lock()
		if e, ok := c.entries[key]; ok && !c.validEntry(key, e) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	return false
}

// Invalidate drops a single key.
func (c *TTLCache[K, V]) Invalidate(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries.
func (c *TTLCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[K]ttlEntry[V])
}

// Len returns the current number of entries (including stale ones pending
// lazy or reaper cleanup).
func (c *TTLCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Start launches a background goroutine that periodically drops expired or
// invalid entries. Safe to call once; pair with Stop on shutdown.
func (c *TTLCache[K, V]) Start(interval time.Duration) {
	if interval <= 0 {
		return
	}
	c.mu.Lock()
	if c.stopReaper != nil {
		c.mu.Unlock()
		return
	}
	c.stopReaper = make(chan struct{})
	stop := c.stopReaper
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.reap()
			case <-stop:
				return
			}
		}
	}()
}

// Stop terminates the reaper goroutine if running. Safe to call multiple times.
func (c *TTLCache[K, V]) Stop() {
	c.mu.Lock()
	stop := c.stopReaper
	c.stopReaper = nil
	c.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// reap removes all currently invalid entries.
func (c *TTLCache[K, V]) reap() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if !c.validEntry(k, e) {
			delete(c.entries, k)
		}
	}
}
