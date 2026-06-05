package assistant

import (
	"container/list"
	"sync"
)

type cacheEntry struct {
	key   string
	value string
}

type TokenCache struct {
	mu        sync.RWMutex
	capacity  int
	items     map[string]*list.Element
	evictList *list.List
}

func NewTokenCache(capacity int) *TokenCache {
	return &TokenCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

func (c *TokenCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, hit := c.items[key]
	if !hit {
		return "", false
	}
	c.evictList.MoveToFront(elem)
	return elem.Value.(*cacheEntry).value, true
}

func (c *TokenCache) Add(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, hit := c.items[key]; hit {
		c.evictList.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = value
		return
	}

	elem := c.evictList.PushFront(&cacheEntry{key, value})
	c.items[key] = elem

	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

func (c *TokenCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.evictList = list.New()
}

func (c *TokenCache) removeOldest() {
	elem := c.evictList.Back()
	if elem == nil {
		return
	}
	c.evictList.Remove(elem)
	kv := elem.Value.(*cacheEntry)
	delete(c.items, kv.key)
}
