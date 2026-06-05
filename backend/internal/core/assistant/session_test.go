package assistant

import (
	"context"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
)

type testLogger struct {
	T *testing.T
}

func (l *testLogger) Debug(msg string, args ...any) { l.T.Logf("DEBUG: "+msg, args...) }
func (l *testLogger) Info(msg string, args ...any)  { l.T.Logf("INFO: "+msg, args...) }
func (l *testLogger) Warn(msg string, args ...any)  { l.T.Logf("WARN: "+msg, args...) }
func (l *testLogger) Error(msg string, args ...any) { l.T.Logf("ERROR: "+msg, args...) }
func (l *testLogger) With(args ...any) logging.Logger { return l }
func (l *testLogger) SetLevel(logging.Level)        {}
func (l *testLogger) Level() logging.Level          { return logging.LevelDebug }

var _ logging.Logger = (*testLogger)(nil)

func TestTokenCache_ExactMatch(t *testing.T) {
	cache := NewTokenCache(100)
	cache.Add("uname -a", "Darwin Kernel 25.5.0")

	got, ok := cache.Get("uname -a")
	if !ok {
		t.Fatal("expected cache hit for uname -a")
	}
	if got != "Darwin Kernel 25.5.0" {
		t.Errorf("expected 'Darwin Kernel 25.5.0', got %q", got)
	}
}

func TestTokenCache_DifferentKey(t *testing.T) {
	cache := NewTokenCache(100)
	cache.Add("uname -a", "Darwin Kernel 25.5.0")

	_, ok := cache.Get("npx tsc --version")
	if ok {
		t.Error("expected cache miss for different key")
	}
}

func TestTokenCache_Eviction(t *testing.T) {
	cache := NewTokenCache(3)
	cache.Add("a", "1")
	cache.Add("b", "2")
	cache.Add("c", "3")
	cache.Add("d", "4") // evicts "a"

	_, ok := cache.Get("a")
	if ok {
		t.Error("expected 'a' to be evicted")
	}
}

func TestTokenCache_Flush(t *testing.T) {
	cache := NewTokenCache(100)
	cache.Add("uname -a", "Darwin Kernel 25.5.0")
	cache.Flush()

	_, ok := cache.Get("uname -a")
	if ok {
		t.Error("expected cache miss after flush")
	}
}

func TestTokenCache_EmptyCache(t *testing.T) {
	cache := NewTokenCache(100)

	_, ok := cache.Get("anything")
	if ok {
		t.Error("expected cache miss on empty cache")
	}
}

func TestInterceptRedundantToolCalls_MemoryMatch(t *testing.T) {
	store := newTestMemory(t)
	_, err := store.Insert(context.Background(), "ws-1", memory.LongTerm,
		"tool_versions", "TypeScript version installed: 6.0.3", "agent")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	// Agent with a nil TokenCache means MBTCP is disabled — tool runs normally.
	// The LRU cache only works from exact matches, not FTS5 semantic search.
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		MemoryStore: store,
		WorkspaceID: "ws-1",
		Logger:      &testLogger{T: t},
	})
	session := newRunSession(agent, context.Background(), nil)

	msg := proxy.Message{
		ToolCalls: []proxy.ToolCall{
			{
				ID: "call_1", Type: "function",
				Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: `npx tsc --version`},
			},
		},
	}

	history := []proxy.Message{}
	session.interceptRedundantToolCalls(&msg, &history)

	// Without a cache, MBTCP lets the tool through
	if len(msg.ToolCalls) != 1 {
		t.Errorf("expected tool calls to pass through without cache, got %d", len(msg.ToolCalls))
	}
}

func TestInterceptRedundantToolCalls_WithCacheMatch(t *testing.T) {
	store := newTestMemory(t)
	_, err := store.Insert(context.Background(), "ws-1", memory.LongTerm,
		"tool_versions", "TypeScript version installed: 6.0.3", "agent")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		MemoryStore: store,
		WorkspaceID: "ws-1",
		Logger:      &testLogger{T: t},
	})
	agent.toolCache = NewTokenCache(100)
	// Pre-populate the cache with the command result
	agent.toolCache.Add("npx tsc --version", "Version 6.0.3")

	session := newRunSession(agent, context.Background(), nil)

	msg := proxy.Message{
		ToolCalls: []proxy.ToolCall{
			{
				ID: "call_1", Type: "function",
				Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: `npx tsc --version`},
			},
		},
	}

	history := []proxy.Message{}
	session.interceptRedundantToolCalls(&msg, &history)

	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected tool calls to be intercepted with cached result, got %d", len(msg.ToolCalls))
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 synthetic result, got %d", len(history))
	}
	if !strings.Contains(history[0].Content, "Version 6.0.3") {
		t.Errorf("expected cached content in result, got: %s", history[0].Content)
	}
}

func TestInterceptRedundantToolCalls_NilCache(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		Logger: &testLogger{T: t},
	})
	session := newRunSession(agent, context.Background(), nil)

	msg := proxy.Message{
		ToolCalls: []proxy.ToolCall{
			{
				ID: "call_1", Type: "function",
				Function: proxy.FunctionCall{Name: "execute_terminal_command", Arguments: `uname -a`},
			},
		},
	}

	history := []proxy.Message{}
	session.interceptRedundantToolCalls(&msg, &history)

	if len(msg.ToolCalls) != 1 {
		t.Errorf("expected tool calls to pass through with nil cache, got %d", len(msg.ToolCalls))
	}
}

func TestInterceptRedundantToolCalls_NonTerminalIgnored(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		Logger: &testLogger{T: t},
	})
	agent.toolCache = NewTokenCache(100)
	agent.toolCache.Add("list_directory", "file list")

	session := newRunSession(agent, context.Background(), nil)

	msg := proxy.Message{
		ToolCalls: []proxy.ToolCall{
			{
				ID: "call_1", Type: "function",
				Function: proxy.FunctionCall{Name: "list_directory", Arguments: `.`},
			},
		},
	}

	history := []proxy.Message{}
	session.interceptRedundantToolCalls(&msg, &history)

	// Non-terminal tools should not be intercepted
	if len(msg.ToolCalls) != 1 {
		t.Errorf("expected non-terminal tool calls untouched, got %d", len(msg.ToolCalls))
	}
}

func newTestMemory(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.DB().Close() })

	memStore, err := memory.New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return memStore
}
