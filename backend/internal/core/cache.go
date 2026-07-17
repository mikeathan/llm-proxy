package core

import "sync"

type ContentCache struct {
	mu      sync.RWMutex
	entries map[string]string
}

func (c *ContentCache) Get(key string, readFn func() (string, error)) (string, error) {
	c.mu.RLock()
	if v, ok := c.entries[key]; ok {
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.entries[key]; ok {
		return v, nil
	}
	v, err := readFn()
	if err != nil {
		return "", err
	}
	if c.entries == nil {
		c.entries = make(map[string]string)
	}
	c.entries[key] = v
	return v, nil
}

func (c *ContentCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *ContentCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}
