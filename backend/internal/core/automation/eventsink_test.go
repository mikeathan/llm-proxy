package automation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
)

func TestEventSink_WritesAndSyncsOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	s, err := NewEventSink(path)
	if err != nil {
		t.Fatalf("NewEventSink: %v", err)
	}

	ev := assistant.AgentEvent{
		Type:      assistant.EventMessage,
		Channel:   assistant.ChannelAutomation,
		Payload:   "hello",
		Timestamp: time.Now(),
	}
	if err := s.Write(ev); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v (content %q)", err, string(b))
	}
	if decoded.Type != "message" {
		t.Errorf("expected type message, got %q", decoded.Type)
	}
}

func TestEventSink_MultipleWritesSyncedOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	s, err := NewEventSink(path)
	if err != nil {
		t.Fatalf("NewEventSink: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Write(assistant.AgentEvent{
			Type:      assistant.EventReasoning,
			Channel:   assistant.ChannelAssistant,
			Payload:   "chunk",
			Timestamp: time.Now(),
		}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	s.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := 0
	for _, l := range splitLines(string(b)) {
		if l == "" {
			continue
		}
		lines++
	}
	if lines != 5 {
		t.Errorf("expected 5 events, got %d", lines)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
