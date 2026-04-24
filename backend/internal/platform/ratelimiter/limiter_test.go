package ratelimiter

import (
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) NowUtc() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func TestLimiter_FirstCallAllowed(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	limiter := NewLimiter(clock)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected first call to be allowed")
	}
}

func TestLimiter_SecondCallBlockedWithinInterval(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	limiter := NewLimiter(clock)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected first call to be allowed")
	}
	if limiter.Allow("key", time.Second) {
		t.Fatalf("expected second call to be blocked")
	}
}

func TestLimiter_AllowedAfterInterval(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	limiter := NewLimiter(clock)

	limiter.Allow("key", time.Second)

	clock.Advance(1 * time.Second)

	if !limiter.Allow("key", time.Second) {
		t.Fatalf("expected call to be allowed after interval")
	}
}

func TestLimiter_PerKeyIsolation(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	limiter := NewLimiter(clock)

	if !limiter.Allow("keyA", time.Second) {
		t.Fatalf("expected keyA to be allowed")
	}
	if !limiter.Allow("keyB", time.Second) {
		t.Fatalf("expected keyB to be allowed independently")
	}
}

func TestLimiter_ClearResetsState(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	limiter := NewLimiter(clock)

	limiter.Allow("key", time.Hour)
	limiter.Clear()

	if !limiter.Allow("key", time.Hour) {
		t.Fatalf("expected call to be allowed after clear")
	}
}

func TestLimiter_Sweep(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	// We cast to *rateLimiter to access unexported sweep method
	l := &rateLimiter{
		calls: make(map[string]time.Time),
		clock: clock,
	}

	// 1. Add some entries
	l.Allow("fresh", time.Hour)
	l.Allow("stale", time.Hour)

	// 2. Advance clock so "stale" is 14 mins old
	clock.Advance(14 * time.Minute)
	// Use a very small interval here so Allow updates the timestamp
	l.Allow("fresh", time.Nanosecond)

	// 3. Run sweep with 10 min maxAge
	l.sweep(10 * time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.calls["fresh"]; !exists {
		t.Errorf("expected 'fresh' entry to remain")
	}
	if _, exists := l.calls["stale"]; exists {
		t.Errorf("expected 'stale' entry to be evicted")
	}
}
