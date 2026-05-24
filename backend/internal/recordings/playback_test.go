package recordings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePlaybackFile(t *testing.T, dir string, lines []string) string {
	t.Helper()
	path := filepath.Join(dir, "test.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlaybackClient_SingleTurn(t *testing.T) {
	dir := t.TempDir()
	path := writePlaybackFile(t, dir, []string{
		`{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hello"}]}`,
		`{"type":"response","choices":[{"index":0,"message":{"role":"assistant","content":"hi there"}}]}`,
		`{"type":"done","total_chunks":1}`,
	})

	pc, err := NewPlaybackClient(path)
	if err != nil {
		t.Fatal(err)
	}

	if pc.TurnCount() != 1 {
		t.Fatalf("expected 1 turn, got %d", pc.TurnCount())
	}

	turn := pc.NextTurn()
	if turn == nil {
		t.Fatal("expected a turn")
	}
	if len(turn.Request.Messages) == 0 {
		t.Fatal("expected request messages")
	}
	if len(turn.Response.Choices) == 0 {
		t.Fatal("expected response choices")
	}

	next := pc.NextTurn()
	if next != nil {
		t.Fatal("expected nil after exhausting turns")
	}
}

func TestPlaybackClient_MultiTurn(t *testing.T) {
	dir := t.TempDir()
	path := writePlaybackFile(t, dir, []string{
		`{"type":"request","messages":[{"role":"user","content":"hello"}]}`,
		`{"type":"response","choices":[{"index":0,"message":{"role":"assistant","content":"first"}}]}`,
		`{"type":"done","total_chunks":1}`,
		`{"type":"request","messages":[{"role":"user","content":"again"}]}`,
		`{"type":"response","choices":[{"index":0,"message":{"role":"assistant","content":"second"}}]}`,
		`{"type":"done","total_chunks":1}`,
	})

	pc, err := NewPlaybackClient(path)
	if err != nil {
		t.Fatal(err)
	}

	if pc.TurnCount() != 2 {
		t.Fatalf("expected 2 turns, got %d", pc.TurnCount())
	}

	pc.NextTurn()
	pc.NextTurn()
	if pc.NextTurn() != nil {
		t.Fatal("expected nil on third call")
	}

	pc.Reset()
	if pc.NextTurn() == nil {
		t.Fatal("expected a turn after reset")
	}
}

func TestPlaybackClient_StreamingTurn(t *testing.T) {
	dir := t.TempDir()
	path := writePlaybackFile(t, dir, []string{
		`{"type":"request","messages":[{"role":"user","content":"hello"}]}`,
		`{"type":"chunk","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"type":"chunk","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"type":"done","total_chunks":2}`,
	})

	pc, err := NewPlaybackClient(path)
	if err != nil {
		t.Fatal(err)
	}

	turn := pc.NextTurn()
	if turn == nil {
		t.Fatal("expected a turn")
	}
	if len(turn.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(turn.Chunks))
	}
}

func TestPlaybackClient_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlaybackClient(path); err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestPlaybackClient_NoSuchFile(t *testing.T) {
	if _, err := NewPlaybackClient("/nonexistent/path.jsonl"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPlaybackClient_ErrorTurn(t *testing.T) {
	dir := t.TempDir()
	path := writePlaybackFile(t, dir, []string{
		`{"type":"request"}`,
		`{"type":"error","message":"rate limited"}`,
	})

	pc, err := NewPlaybackClient(path)
	if err != nil {
		t.Fatal(err)
	}

	turn := pc.NextTurn()
	if turn == nil {
		t.Fatal("expected a turn")
	}
	if turn.Error != "rate limited" {
		t.Fatalf("expected error 'rate limited', got %q", turn.Error)
	}
}
