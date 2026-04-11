package mcp

import (
	"llm-proxy/internal/platform/logging"
	"strings"
	"testing"
)

// NoOpLogger for testing
type NoOpLogger struct{}

func (l *NoOpLogger) Debug(msg string, args ...any)   {}
func (l *NoOpLogger) Info(msg string, args ...any)    {}
func (l *NoOpLogger) Warn(msg string, args ...any)    {}
func (l *NoOpLogger) Error(msg string, args ...any)   {}
func (l *NoOpLogger) With(args ...any) logging.Logger { return l }
func (l *NoOpLogger) SetLevel(lvl logging.Level)      {}
func (l *NoOpLogger) Level() logging.Level            { return logging.LevelInfo }

func TestGetSystemPrompt_Fallback(t *testing.T) {
	logger := &NoOpLogger{}
	orchestrator := NewOrchestrator(logger)
	mirror := NewResourceMirror()
	adapter := NewMCPNodeHerder(orchestrator, mirror, logger)

	// Ensure mirror is empty
	if mirror.HasSystemPrompt() {
		t.Fatal("Mirror should be empty initially")
	}

	// Call GetSystemPrompt. Since orchestrator has no clients, it should fail to fetch and return fallback.
	prompt, err := adapter.GetSystemPrompt()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedPhrase := "disconnected"
	if !strings.Contains(prompt, expectedPhrase) {
		t.Errorf("Expected fallback prompt containing '%s', got: %s", expectedPhrase, prompt)
	}
}
