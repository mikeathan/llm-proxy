package mcp

import (
	"encoding/json"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
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

// An MCP server's schema reaches us already decoded into a map, so the server's own order is gone. A model writes a call
// in the order the schema lists the arguments, and with the required ones last (alphabetical order) a local model left
// out optional arguments it meant to pass; required first used them 10 times in 10.
func TestSchemaRequiredFirst_ListsRequiredArgumentsFirst(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"limit": map[string]any{"type": "integer"}, "path": map[string]any{"type": "string"}, "offset": map[string]any{"type": "integer"}},
		"required":   []any{"path"},
	}
	schemaRequiredFirst(schema)
	wire, _ := json.Marshal(schema)
	text := string(wire)
	if !(strings.Index(text, `"path"`) < strings.Index(text, `"limit"`) && strings.Index(text, `"limit"`) < strings.Index(text, `"offset"`)) {
		t.Errorf("not required-first on the wire: %s", text)
	}
	if !strings.Contains(text, `"required":["path"]`) {
		t.Errorf("the rest of the schema changed: %s", text)
	}
	if props, ok := models.SchemaProperties(schema); !ok || len(props) != 3 {
		t.Errorf("readers must still find the properties: %v", props)
	}
}

func TestSchemaRequiredFirst_LeavesOtherShapesAlone(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"nil":            nil,
		"no properties":  {"type": "object"},
		"odd properties": {"properties": 5, "required": []any{"x"}},
		"odd required":   {"properties": map[string]any{"b": 1, "a": 2}, "required": "x"},
	} {
		before, _ := json.Marshal(schema)
		schemaRequiredFirst(schema)
		after, _ := json.Marshal(schema)
		if name != "odd required" && string(before) != string(after) {
			t.Errorf("%s changed: %s -> %s", name, before, after)
		}
	}
}
