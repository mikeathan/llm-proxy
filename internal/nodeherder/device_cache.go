package nodeherder

import (
	"llm-proxy/utils"
	"sync/atomic"
	"time"
)

type DeviceContextCache struct {
	ttl   time.Duration
	value atomic.Value
	clock utils.Clock
}

type cacheEntry struct {
	ctx       *LLMDeviceContext
	expiresAt time.Time
}

func NewDeviceContextCache(ttl time.Duration, clock utils.Clock) *DeviceContextCache {
	return &DeviceContextCache{ttl: ttl, clock: clock, value: atomic.Value{}}
}

func (c *DeviceContextCache) Get() (*LLMDeviceContext, bool) {
	v := c.value.Load()
	if v == nil {
		return nil, false
	}
	entry := v.(*cacheEntry)
	if c.clock.NowUtc().After(entry.expiresAt) {
		return nil, false
	}
	return entry.ctx, true
}

func (c *DeviceContextCache) Set(ctx *LLMDeviceContext) {
	entry := &cacheEntry{
		ctx:       ctx,
		expiresAt: c.clock.NowUtc().Add(c.ttl),
	}

	c.value.Store(entry)
}

func (c *DeviceContextCache) Invalidate() {
	c.value = atomic.Value{}
}
