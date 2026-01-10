// Package pending stores unresolved tool-execution state while awaiting
// user disambiguation. It is NOT general conversation memory.
package pending

import (
	"fmt"
	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/proxy"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PendingToolCallState struct {
	ToolCall   proxy.ToolCall
	History    []proxy.Message
	Candidates []devices.Candidate
	Target     string
	Expose     string
}

type PendingToolCallStore interface {
	Get(conversationID string) (*PendingToolCallState, bool)
	Set(conversationID string, state PendingToolCallState)
	Clear(conversationID string)
}

type InMemoryPendingToolCallStore struct {
	mu       sync.Mutex
	store    map[string]pendingEntry
	ttl      time.Duration
	clock    tools.Clock
	interval time.Duration
}

type pendingEntry struct {
	state     PendingToolCallState
	expiresAt time.Time
}

const (
	defaultPendingTTL      = 10 * time.Minute
	defaultPendingInterval = 2 * time.Minute
)

func NewInMemoryPendingToolCallStore() *InMemoryPendingToolCallStore {
	// Default TTL/cleanup protects against abandoned conversations and memory leaks.
	return NewInMemoryPendingToolCallStoreWithOptions(defaultPendingTTL, tools.RealClock{}, defaultPendingInterval)
}

func NewInMemoryPendingToolCallStoreWithOptions(ttl time.Duration, clock tools.Clock, interval time.Duration) *InMemoryPendingToolCallStore {
	if ttl <= 0 {
		ttl = defaultPendingTTL
	}
	if interval <= 0 {
		interval = defaultPendingInterval
	}
	store := &InMemoryPendingToolCallStore{
		store:    make(map[string]pendingEntry),
		ttl:      ttl,
		clock:    clock,
		interval: interval,
	}
	go store.cleanupLoop()
	return store
}

func (s *InMemoryPendingToolCallStore) cleanupLoop() {
	// Periodic cleanup reclaims stale pending tool calls.
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanupExpired()
	}
}

func (s *InMemoryPendingToolCallStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	for key, entry := range s.store {
		if now.After(entry.expiresAt) {
			delete(s.store, key)
		}
	}
}

func (s *InMemoryPendingToolCallStore) pruneExpiredLocked(now time.Time) {
	for key, entry := range s.store {
		if now.After(entry.expiresAt) {
			delete(s.store, key)
		}
	}
}

func (s *InMemoryPendingToolCallStore) Get(conversationID string) (*PendingToolCallState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	s.pruneExpiredLocked(now)
	entry, ok := s.store[conversationID]
	if !ok {
		return nil, false
	}
	copyState := PendingToolCallState{
		ToolCall:   entry.state.ToolCall,
		History:    append([]proxy.Message(nil), entry.state.History...),
		Candidates: append([]devices.Candidate(nil), entry.state.Candidates...),
		Target:     entry.state.Target,
		Expose:     entry.state.Expose,
	}
	return &copyState, true
}

func (s *InMemoryPendingToolCallStore) Set(conversationID string, state PendingToolCallState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	s.pruneExpiredLocked(now)
	s.store[conversationID] = pendingEntry{
		state:     state,
		expiresAt: now.Add(s.ttl),
	}
}

func (s *InMemoryPendingToolCallStore) Clear(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, conversationID)
}

func ResolvePendingToolCall(input string, candidates []devices.Candidate) (*devices.Candidate, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, false
	}

	if idx, err := strconv.Atoi(trimmed); err == nil {
		if idx >= 1 && idx <= len(candidates) {
			c := candidates[idx-1]
			return &c, true
		}
	}

	lower := strings.ToLower(trimmed)
	for _, c := range candidates {
		if strings.ToLower(c.Name) == lower || strings.ToLower(c.ID) == lower {
			return &c, true
		}
	}

	var match *devices.Candidate
	for _, c := range candidates {
		name := strings.ToLower(c.Name)
		if strings.Contains(name, lower) {
			if match != nil {
				return nil, false
			}
			candidate := c
			match = &candidate
		}
	}

	if match != nil {
		return match, true
	}

	return nil, false
}

func FormatPendingPrompt(target, expose string, candidates []devices.Candidate) string {
	lines := make([]string, 0, len(candidates))
	for i, c := range candidates {
		label := fmt.Sprintf("%d) %s", i+1, c.Name)
		if c.ID != "" {
			label += fmt.Sprintf(" (%s)", c.ID)
		}
		lines = append(lines, label)
	}
	return fmt.Sprintf(
		"I found multiple devices matching %q for %q. Please choose one:\n%s",
		target,
		expose,
		strings.Join(lines, "\n"),
	)
}
