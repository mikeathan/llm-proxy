package newsroom

import (
	"sync"
	"time"
)

// The look-up route reads public sites on the owner's behalf while a watch form is open, so it is kept polite: an answer is remembered for a while (the same
// channel typed three ways is one read), at most lookupCacheMax answers are kept, and Telegram, whose preview page asks for a gap between reads, is
// limited across all channels.
const (
	lookupCacheTTL           = 10 * time.Minute
	lookupCacheMax           = 64
	telegramLookupsPerMinute = 6
)

type lookupEntry struct {
	at   time.Time
	body []byte
}

// lookupCache holds recent look-up answers by key and the times of recent Telegram reads. It is safe for concurrent use.
type lookupCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]lookupEntry
	tg      []time.Time
}

func newLookupCache() *lookupCache {
	return &lookupCache{now: time.Now, entries: map[string]lookupEntry{}}
}

// get returns a remembered answer that is still fresh.
func (c *lookupCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.at) > lookupCacheTTL {
		delete(c.entries, key)
		return nil, false
	}
	return e.body, true
}

// put remembers an answer, dropping expired ones and then the oldest while the cache is over its bound.
func (c *lookupCache) put(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.at) > lookupCacheTTL {
			delete(c.entries, k)
		}
	}
	for len(c.entries) >= lookupCacheMax {
		oldest, first := "", true
		var at time.Time
		for k, e := range c.entries {
			if first || e.at.Before(at) {
				oldest, at, first = k, e.at, false
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = lookupEntry{at: now, body: body}
}

func (c *lookupCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// allowTelegram takes one of the Telegram reads allowed in the last minute, or says there is none left.
func (c *lookupCache) allowTelegram() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	kept := c.tg[:0]
	for _, at := range c.tg {
		if now.Sub(at) < time.Minute {
			kept = append(kept, at)
		}
	}
	c.tg = kept
	if len(c.tg) >= telegramLookupsPerMinute {
		return false
	}
	c.tg = append(c.tg, now)
	return true
}
