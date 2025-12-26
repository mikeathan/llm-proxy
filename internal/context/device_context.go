package context

import (
	"llm-proxy/internal/ratelimiter"
)

type DeviceContextProvider interface {
	GetDeviceContext(deviceID string) (*DeviceContextResponse, error)
}

type deviceContextProvider struct {
	cache   *DeviceContextCache
	limiter *ratelimiter.Limiter
	client  *NodeHerderClient
}

func NewDeviceContextProvider(cache *DeviceContextCache, limiter *ratelimiter.Limiter, client *NodeHerderClient) *deviceContextProvider {
	return &deviceContextProvider{cache: cache, limiter: limiter, client: client}
}

func (p *deviceContextProvider) GetDeviceContext(deviceID string) (*DeviceContextResponse, error) {
	// TODO
	return nil, nil
}
