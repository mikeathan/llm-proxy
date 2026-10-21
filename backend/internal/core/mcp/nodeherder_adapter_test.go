package mcp

import (
	"encoding/json"
	mcp_sdk "github.com/mark3labs/mcp-go/mcp"
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

func textResult(isError bool, texts ...string) *mcp_sdk.CallToolResult {
	res := &mcp_sdk.CallToolResult{IsError: isError}
	for _, text := range texts {
		res.Content = append(res.Content, mcp_sdk.TextContent{Type: "text", Text: text})
	}
	return res
}

func TestToolResultToAny(t *testing.T) {
	structured, err := toolResultToAny(textResult(false, `{"temp":8,"ok":true}`))
	if m, ok := structured.(map[string]any); err != nil || !ok || m["temp"] != float64(8) {
		t.Errorf("JSON text must come back structured: %v %v", structured, err)
	}
	plain, err := toolResultToAny(textResult(false, "just words"))
	if err != nil || plain != "just words" {
		t.Errorf("plain text is returned as it is: %v %v", plain, err)
	}
	if _, err := toolResultToAny(textResult(false)); err == nil || !strings.Contains(err.Error(), "empty tool result") {
		t.Errorf("no content is an error: %v", err)
	}
	failed, err := toolResultToAny(textResult(true, "device offline"))
	if err == nil || !strings.Contains(err.Error(), "device offline") || failed != "device offline" {
		t.Errorf("a tool error carries its message: %v %v", failed, err)
	}
	noText, err := toolResultToAny(&mcp_sdk.CallToolResult{IsError: true})
	if err == nil || noText != "unknown tool error" {
		t.Errorf("an error with no content still says so: %v %v", noText, err)
	}
	image := mcp_sdk.ImageContent{Type: "image", Data: "abc", MIMEType: "image/png"}
	got, err := toolResultToAny(&mcp_sdk.CallToolResult{Content: []mcp_sdk.Content{image}})
	if err != nil || got != image {
		t.Errorf("non-text content is returned as it is: %v %v", got, err)
	}
}
