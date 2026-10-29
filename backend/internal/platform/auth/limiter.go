package auth

import (
	"sync"
	"time"
)

const (
	maxFailuresPerAddress = 5
	failureWindow         = 15 * time.Minute
	maxFailuresGlobal     = 30
	globalWindow          = time.Hour
)

// limiter slows down guessing. It is in memory: a restart clears it, which an attacker cannot cause and the operator can.
// Failures count per client address and across all addresses, so many addresses cannot add up to unlimited guesses.
type limiter struct {
	mu     sync.Mutex
	byAddr map[string][]time.Time
	global []time.Time
}

func newLimiter() *limiter { return &limiter{byAddr: map[string][]time.Time{}} }

// wait is how long the address must wait before trying again; zero means it may try now.
func (l *limiter) wait(addr string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := within(l.byAddr[addr], now, failureWindow)
	if len(recent) == 0 {
		delete(l.byAddr, addr) // an address that has stopped failing is forgotten, so the map cannot grow for ever
	} else {
		l.byAddr[addr] = recent
	}
	l.global = within(l.global, now, globalWindow)
	var wait time.Duration
	if len(recent) >= maxFailuresPerAddress {
		wait = recent[0].Add(failureWindow).Sub(now)
	}
	if len(l.global) >= maxFailuresGlobal {
		wait = max(wait, l.global[0].Add(globalWindow).Sub(now))
	}
	return wait
}

func (l *limiter) fail(addr string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byAddr[addr] = append(l.byAddr[addr], now)
	l.global = append(l.global, now)
}

// succeed forgets one address's failures: a right passphrase proves it is the operator.
func (l *limiter) succeed(addr string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byAddr, addr)
}

// within keeps the times that are still inside the window.
func within(times []time.Time, now time.Time, window time.Duration) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	return kept
}
