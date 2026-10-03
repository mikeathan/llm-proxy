package eventbus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
)

func TestSink_WritesAndSyncsOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	s, err := NewSink(path)
	if err != nil {
		t.Fatalf("NewSink: %v", err)
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

func TestSink_MultipleWritesSyncedOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	s, err := NewSink(path)
	if err != nil {
		t.Fatalf("NewSink: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Write(assistant.AgentEvent{
			Type:      assistant.EventToolResult,
			Channel:   assistant.ChannelAssistant,
			Payload:   "chunk",
			Timestamp: time.Now(),
		}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	s.Close()

	if got := len(readSinkEvents(t, path)); got != 5 {
		t.Errorf("expected 5 events, got %d", got)
	}
}

// Reasoning and tool_stream events carry the full text so far, so a run wrote
// thousands of them (11 MB for a four-minute run). The file keeps the newest
// snapshot of each stream, in order with the events around it.
func TestSink_KeepsOnlyNewestSnapshotOfAStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewSink(path)
	if err != nil {
		t.Fatalf("NewSink: %v", err)
	}
	write := func(typ assistant.AgentEventType, conv string, payload string) {
		t.Helper()
		if err := s.Write(assistant.AgentEvent{Type: typ, Channel: assistant.ChannelAssistant, ConversationID: conv, Payload: payload}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	write(assistant.EventLifecycle, "c1", "started")
	for _, p := range []string{"a", "ab", "abc"} {
		write(assistant.EventReasoning, "c1", p)
	}
	write(assistant.EventToolCall, "c1", "call")
	write(assistant.EventReasoning, "c1", "x")
	write(assistant.EventReasoning, "c2", "other conversation")
	s.Close()

	var got []string
	for _, ev := range readSinkEvents(t, path) {
		got = append(got, string(ev.Type)+":"+ev.Payload.(string))
	}
	want := []string{"lifecycle:started", "reasoning:abc", "tool_call:call", "reasoning:x", "reasoning:other conversation"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// A long stream is checkpointed so a crash mid-run still leaves recent
// reasoning on disk for debugging.
func TestSink_CheckpointsALongStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewSink(path)
	if err != nil {
		t.Fatalf("NewSink: %v", err)
	}
	clock := time.Unix(1000, 0)
	s.now = func() time.Time { return clock }

	write := func(payload string) {
		t.Helper()
		if err := s.Write(assistant.AgentEvent{Type: assistant.EventReasoning, Channel: assistant.ChannelAssistant, ConversationID: "c1", Payload: payload}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	write("a")
	write("ab")
	clock = clock.Add(snapshotCheckpointInterval + time.Second)
	write("abc")
	write("abcd")
	s.Close()

	var got []string
	for _, ev := range readSinkEvents(t, path) {
		got = append(got, ev.Payload.(string))
	}
	if want := []string{"abc", "abcd"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("payloads = %v, want %v", got, want)
	}
}

func readSinkEvents(t *testing.T, path string) []assistant.AgentEvent {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var out []assistant.AgentEvent
	for _, l := range splitLines(string(b)) {
		if l == "" {
			continue
		}
		var ev assistant.AgentEvent
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("unmarshal %q: %v", l, err)
		}
		out = append(out, ev)
	}
	return out
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

func TestSink_WriteAfterCloseReturnsError(t *testing.T) {
	s, err := NewSink(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatalf("NewSink: %v", err)
	}
	s.Close()
	for _, typ := range []assistant.AgentEventType{assistant.EventReasoning, assistant.EventMessage} {
		if err := s.Write(assistant.AgentEvent{Type: typ, Payload: "late"}); err == nil {
			t.Errorf("Write(%s) after Close returned nil, want an error", typ)
		}
	}
}

// A stream that stalls still gets its latest text on disk: the periodic sync
// writes a snapshot that has been held longer than the checkpoint interval.
func TestSink_SyncWritesAStaleHeldSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewSink(path)
	if err != nil {
		t.Fatalf("NewSink: %v", err)
	}
	clock := time.Unix(1000, 0)
	s.now = func() time.Time { return clock }
	if err := s.Write(assistant.AgentEvent{Type: assistant.EventReasoning, ConversationID: "c1", Payload: "latest text"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	s.sync()
	if got := len(readSinkEvents(t, path)); got != 0 {
		t.Fatalf("a fresh snapshot was written early (%d events)", got)
	}
	clock = clock.Add(snapshotCheckpointInterval + time.Second)
	s.sync()
	if got := readSinkEvents(t, path); len(got) != 1 || got[0].Payload != "latest text" {
		t.Fatalf("stale snapshot not written by the sync: %+v", got)
	}
	s.Close()
	if got := len(readSinkEvents(t, path)); got != 1 {
		t.Errorf("the snapshot was written twice (%d events)", got)
	}
}
