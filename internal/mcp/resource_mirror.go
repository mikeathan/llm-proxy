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

// NewResourceMirror creates a new resource mirror.
func NewResourceMirror() *ResourceMirror {
	return &ResourceMirror{}
}

// SetSystemPrompt updates the cached system prompt.
func (m *ResourceMirror) SetSystemPrompt(prompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemPrompt = prompt
	m.lastUpdated = time.Now()
}

// GetSystemPrompt retrieves the cached system prompt.
func (m *ResourceMirror) GetSystemPrompt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemPrompt
}

// LastUpdated returns the time of the last update.
func (m *ResourceMirror) LastUpdated() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastUpdated
}

// HasSystemPrompt returns true if system prompt has been loaded.
func (m *ResourceMirror) HasSystemPrompt() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemPrompt != ""
}
