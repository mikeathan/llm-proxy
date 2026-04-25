package ratelimiter

import (
	"llm-proxy/utils"
	"sync"
	"time"
)

type Limiter interface {
	Allow(key string, interval time.Duration) bool
	Clear()
}

type rateLimiter struct {
	mu    sync.Mutex
	calls map[string]time.Time
	clock utils.Clock
}

func NewLimiter(clock utils.Clock) Limiter {
	l := &rateLimiter{
		calls: make(map[string]time.Time),
		clock: clock,
	}
	// Start background cleaner to prevent memory leak (MEM-H1)
	go l.runCleaner(5*time.Minute, 10*time.Minute)
	return l
}

func (l *rateLimiter) Allow(key string, interval time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.NowUtc()
	last, exists := l.calls[key]
	if exists && now.Sub(last) < interval {
		return false
	}

	l.calls[key] = now
	return true
}

func (l *rateLimiter) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = make(map[string]time.Time)
}

func (l *rateLimiter) runCleaner(interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		l.sweep(maxAge)
	}
}

func (l *rateLimiter) sweep(maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.NowUtc()
	for k, last := range l.calls {
		if now.Sub(last) > maxAge {
			delete(l.calls, k)
		}
	}
}
