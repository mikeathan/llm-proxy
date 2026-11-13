package stackport

import "sync"

// KeyCache keeps the master key in memory after the first successful read, so a stack that tags thousands of values a minute does not read the
// key file for each one. Drop forgets it (and zeroes it): the factory reset calls Drop, so the next call reads the new key. A failed read is not
// remembered.
type KeyCache struct {
	load func() ([]byte, error)
	mu   sync.Mutex
	key  []byte
}

// NewKeyCache returns a cache over load.
func NewKeyCache(load func() ([]byte, error)) *KeyCache { return &KeyCache{load: load} }

// Get returns the cached key, reading it first when there is none. The caller must not change it.
func (c *KeyCache) Get() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key != nil {
		return c.key, nil
	}
	k, err := c.load()
	if err != nil {
		return nil, err
	}
	c.key = k
	return k, nil
}

// Drop forgets the key and overwrites it.
func (c *KeyCache) Drop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.key)
	c.key = nil
}
