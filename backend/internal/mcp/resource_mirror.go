package mcp

import (
	"sync"
	"time"
)

// ResourceMirror maintains a local cache of MCP resources.
// It is updated via push notifications from the MCP server.
type ResourceMirror struct {
	mu           sync.RWMutex
	systemPrompt string
	lastUpdated  time.Time
}

func NewResourceMirror() *ResourceMirror {
	return &ResourceMirror{}
}

func (m *ResourceMirror) SetSystemPrompt(prompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemPrompt = prompt
	m.lastUpdated = time.Now()
}

func (m *ResourceMirror) GetSystemPrompt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemPrompt
}

func (m *ResourceMirror) LastUpdated() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastUpdated
}

func (m *ResourceMirror) HasSystemPrompt() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemPrompt != ""
}
