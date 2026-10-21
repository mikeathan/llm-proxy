package recorder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockClient struct {
	chatFn   func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
	streamFn func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error)
}

func (m *mockClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	return m.chatFn(ctx, req)
}
func (m *mockClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return m.streamFn(ctx, req)
}

func (m *mockClient) ReasoningField() string { return proxy.ReasoningFieldBudget }

// localStub is a minimal proxy.Client that reports the llama.cpp wire field,
// used to prove RecordingClient delegates ReasoningField to its underlying.
type localStub struct {
	proxy.Client
}

func (localStub) ReasoningField() string { return proxy.ReasoningFieldThinkTokens }

func TestRecordingClient_ReasoningFieldDelegates(t *testing.T) {
	rc := New(localStub{}, "", "m")
	if got := rc.ReasoningField(); got != proxy.ReasoningFieldThinkTokens {
		t.Errorf("recording client ReasoningField = %q, want %q (delegated from underlying)", got, proxy.ReasoningFieldThinkTokens)
	}

	rcCloud := New(&mockClient{}, "", "m")
	if got := rcCloud.ReasoningField(); got != proxy.ReasoningFieldBudget {
		t.Errorf("recording client ReasoningField = %q, want %q (delegated from mock underlying)", got, proxy.ReasoningFieldBudget)
	}
}

func readJSONL(path string) ([]recordLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []recordLine
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r recordLine
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			return nil, err
		}
		lines = append(lines, r)
	}
	return lines, scanner.Err()
}

func TestRecordingClient_Chat(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "hello world"}},
				},
			}, nil
		},
	}

	rc := New(underlying, dir, "gemma4")
	resp, err := rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "hello world" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 recording file, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (request, response, done), got %d", len(lines))
	}
	if lines[0].Type != "request" || lines[0].Model != "gemma4" {
		t.Fatalf("unexpected first line: %+v", lines[0])
	}
	if len(lines[0].Messages) != 1 || lines[0].Messages[0].Content != "hi" {
		t.Fatalf("unexpected messages: %+v", lines[0].Messages)
	}
	if lines[1].Type != "response" {
		t.Fatalf("expected response line, got %s", lines[1].Type)
	}
	if lines[1].Choices[0].Message.Content != "hello world" {
		t.Fatalf("unexpected response content: %s", lines[1].Choices[0].Message.Content)
	}
	if lines[2].Type != "done" || lines[2].TotalChunks != 1 {
		t.Fatalf("unexpected done line: %+v", lines[2])
	}
}

func TestRecordingClient_Chat_Error(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, fmt.Errorf("rate limited")
		},
	}

	rc := New(underlying, dir, "gemma4")
	_, err := rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected error, got %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 recording file, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (request, error), got %d", len(lines))
	}
	if lines[0].Type != "request" {
		t.Fatalf("expected request, got %s", lines[0].Type)
	}
	if lines[1].Type != "error" || lines[1].Message != "rate limited" {
		t.Fatalf("unexpected error line: %+v", lines[1])
	}
}

func TestRecordingClient_Stream(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		streamFn: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 3)
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "hello "}}}}
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "world"}}}}
			close(ch)
			return ch, nil
		},
	}

	rc := New(underlying, dir, "gemma4")
	ch, err := rc.Stream(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var got []string
	for chunk := range ch {
		got = append(got, chunk.Choices[0].Delta.Content)
	}
	if len(got) != 2 || got[0] != "hello " || got[1] != "world" {
		t.Fatalf("unexpected chunks: %v", got)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 recording file, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (request, chunk, chunk, done), got %d: %+v", len(lines), lines)
	}
	if lines[0].Type != "request" {
		t.Fatalf("expected request, got %s", lines[0].Type)
	}
	if lines[1].Type != "chunk" || lines[1].Choices[0].Delta.Content != "hello " {
		t.Fatalf("unexpected chunk 1: %+v", lines[1])
	}
	if lines[2].Type != "chunk" || lines[2].Choices[0].Delta.Content != "world" {
		t.Fatalf("unexpected chunk 2: %+v", lines[2])
	}
	if lines[3].Type != "done" || lines[3].TotalChunks != 2 {
		t.Fatalf("unexpected done: %+v", lines[3])
	}
}

func TestRecordingClient_Stream_Error(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		streamFn: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	rc := New(underlying, dir, "gemma4")
	_, err := rc.Stream(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected connection refused error, got %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 recording file, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (request, error), got %d", len(lines))
	}
	if lines[1].Type != "error" || !strings.Contains(lines[1].Message, "connection refused") {
		t.Fatalf("unexpected error line: %+v", lines[1])
	}
}

func TestRecordingClient_MultiTurn(t *testing.T) {
	dir := t.TempDir()

	callCount := 0
	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: fmt.Sprintf("turn-%d", callCount)}},
				},
			}, nil
		},
	}

	rc := New(underlying, dir, "gemma4")
	resp1, _ := rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "first"}},
	})
	resp2, _ := rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "second"}},
	})

	if resp1.Choices[0].Message.Content != "turn-1" || resp2.Choices[0].Message.Content != "turn-2" {
		t.Fatalf("unexpected responses: %s, %s", resp1.Choices[0].Message.Content, resp2.Choices[0].Message.Content)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 file for multi-turn, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}

	if len(lines) != 6 {
		t.Fatalf("expected 6 lines (2 requests + 2 responses + 2 done), got %d", len(lines))
	}
}

func TestRecordingClient_Concurrent(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	rc := New(underlying, dir, "gemma4")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rc.Chat(context.Background(), proxy.ChatRequest{
				Messages: []proxy.Message{{Role: "user", Content: "hi"}},
			})
			if err != nil {
				t.Errorf("concurrent chat failed: %v", err)
			}
		}()
	}
	wg.Wait()

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
}

func TestRecordingClient_SameFileForAllCalls(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	rc := New(underlying, dir, "gemma4")

	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hello"}},
	})

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 file for all calls, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "gemma4", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines for 2 calls (2 request + 2 response + 2 done), got %d", len(lines))
	}
}

func TestRecordingClient_ModelNameFromConstructor(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	// Model name set in constructor, not from request
	rc := New(underlying, dir, "qwen3.5")
	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	entries, _ := os.ReadDir(filepath.Join(dir, "qwen3.5"))
	if len(entries) != 1 {
		t.Fatalf("expected file in qwen3.5 dir, got entries in qwen3.5: %d", len(entries))
	}

	// Request Model field should be ignored
	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Model:    "some-other-model",
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	entries, _ = os.ReadDir(filepath.Join(dir, "qwen3.5"))
	if len(entries) != 1 {
		t.Fatalf("expected single file in qwen3.5, got %d (Model from request must be ignored)", len(entries))
	}

	// No directory created for the other model name
	_, err := os.ReadDir(filepath.Join(dir, "some-other-model"))
	if err == nil {
		t.Fatal("directory should not exist for model name from request")
	}
}

func TestRecordingClient_TaskSubdirectory(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	// Chat with task name → file goes to {model}/{task}/
	rc := New(underlying, dir, "gemma4")
	ctx := models.WithTaskName(context.Background(), "smoke-test")
	_, _ = rc.Chat(ctx, proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4", "smoke-test"))
	if len(entries) != 1 {
		t.Fatalf("expected file in gemma4/smoke-test, got %d", len(entries))
	}
}

func TestRecordingClient_NoTaskNoSubdirectory(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	// Chat without task name → file goes to {model}/ root
	rc := New(underlying, dir, "gemma4")
	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	entries, _ := os.ReadDir(filepath.Join(dir, "gemma4"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in gemma4 root (no-task), got %d", len(entries))
	}
	if entries[0].IsDir() {
		t.Fatal("expected a file, got a subdirectory")
	}
}

func TestRecordingClient_DefaultModelName(t *testing.T) {
	dir := t.TempDir()

	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	// Empty model name defaults to "unknown"
	rc := New(underlying, dir, "")
	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})

	entries, _ := os.ReadDir(filepath.Join(dir, "unknown"))
	if len(entries) != 1 {
		t.Fatalf("expected file in unknown dir, got %d", len(entries))
	}

	lines, err := readJSONL(filepath.Join(dir, "unknown", entries[0].Name()))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	if len(lines) < 1 || lines[0].Model != "unknown" {
		t.Fatalf("expected model 'unknown' in line, got %q", lines[0].Model)
	}
}

func TestRecordingClient_Disabled(t *testing.T) {
	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "ok"}},
				},
			}, nil
		},
	}

	rc := New(underlying, "", "gemma4")
	resp, err := rc.Chat(context.Background(), proxy.ChatRequest{
		Messages: []proxy.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat failed when disabled: %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
}

func maskKeys(s string) string { return strings.ReplaceAll(s, "SECRET-123", "[REDACTED]") }

// A recording is every prompt of a run, kept on disk and readable through the admin API, so what is written is masked
// while what the model and the agent see is not.
func TestRecordingClient_RedactsWhatItWritesAndNotWhatItReturns(t *testing.T) {
	dir := t.TempDir()
	underlying := &mockClient{
		chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{Choices: []proxy.Choice{{
				Message: proxy.Message{Role: "assistant", Content: "the key is SECRET-123", ReasoningContent: "thinking about SECRET-123",
					ReasoningDetails: []models.ReasoningDetail{{Type: "reasoning.text", Text: "SECRET-123 again"}},
					ToolCalls:        []proxy.ToolCall{{ID: "c1", Function: proxy.FunctionCall{Name: "write_file", Arguments: `{"content":"SECRET-123"}`}}}},
			}}}, nil
		},
	}
	rc := New(underlying, dir, "m", WithRedactor(maskKeys))
	messages := []proxy.Message{
		{Role: "system", Content: "memory: the token is SECRET-123"},
		{Role: "assistant", ToolCalls: []proxy.ToolCall{{ID: "c0", Function: proxy.FunctionCall{Name: "fetch_url", Arguments: `{"url":"https://x.test/?k=SECRET-123"}`}}}},
		{Role: "tool", ToolCallID: "c0", Content: "page says SECRET-123"},
	}
	resp, err := rc.Chat(context.Background(), proxy.ChatRequest{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Choices[0].Message.Content; got != "the key is SECRET-123" {
		t.Errorf("the caller must get the unmasked reply, got %q", got)
	}
	if messages[0].Content != "memory: the token is SECRET-123" || messages[1].ToolCalls[0].Function.Arguments != `{"url":"https://x.test/?k=SECRET-123"}` {
		t.Error("the request the caller passed in must not be modified")
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "m"))
	raw, err := os.ReadFile(filepath.Join(dir, "m", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET-123") {
		t.Errorf("a secret reached the recording file:\n%s", raw)
	}
	if got := strings.Count(string(raw), "[REDACTED]"); got < 7 {
		t.Errorf("expected the request messages, tool arguments and the reply to be masked, found %d markers", got)
	}
}

func TestRecordingClient_RedactsStreamedChunksAndErrors(t *testing.T) {
	dir := t.TempDir()
	underlying := &mockClient{
		streamFn: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 2)
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "leaked SECRET-123"}}}}
			close(ch)
			return ch, nil
		},
	}
	rc := New(underlying, dir, "m", WithRedactor(maskKeys))
	out, err := rc.Stream(context.Background(), proxy.ChatRequest{Messages: []proxy.Message{{Role: "user", Content: "SECRET-123"}}})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for chunk := range out {
		got += chunk.Choices[0].Delta.Content
	}
	if got != "leaked SECRET-123" {
		t.Errorf("the caller must get the unmasked stream, got %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "m"))
	raw, _ := os.ReadFile(filepath.Join(dir, "m", entries[0].Name()))
	if strings.Contains(string(raw), "SECRET-123") {
		t.Errorf("a secret reached the recording file:\n%s", raw)
	}

	failing := &mockClient{streamFn: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		return nil, fmt.Errorf("upstream rejected key SECRET-123")
	}}
	dir2 := t.TempDir()
	rc2 := New(failing, dir2, "m", WithRedactor(maskKeys))
	_, _ = rc2.Stream(context.Background(), proxy.ChatRequest{})
	entries2, _ := os.ReadDir(filepath.Join(dir2, "m"))
	raw2, _ := os.ReadFile(filepath.Join(dir2, "m", entries2[0].Name()))
	if strings.Contains(string(raw2), "SECRET-123") {
		t.Errorf("an error line must be masked too:\n%s", raw2)
	}
}

func TestRecordingClient_WithoutARedactorRecordsAsBefore(t *testing.T) {
	dir := t.TempDir()
	underlying := &mockClient{chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		return &proxy.ChatResponse{}, nil
	}}
	rc := New(underlying, dir, "m")
	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{Messages: []proxy.Message{{Role: "user", Content: "SECRET-123"}}})
	entries, _ := os.ReadDir(filepath.Join(dir, "m"))
	raw, _ := os.ReadFile(filepath.Join(dir, "m", entries[0].Name()))
	if !strings.Contains(string(raw), "SECRET-123") {
		t.Error("with no redactor the recording must be unchanged (replay tests rely on it)")
	}
}

func TestSyncOnce_FlushesOpenRecordingsAndSaysWhenNoneRemain(t *testing.T) {
	dir := t.TempDir()
	rc := New(&mockClient{chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		return &proxy.ChatResponse{}, nil
	}}, dir, "m")

	if !rc.syncOnce() {
		t.Error("with no recording open the sync pass reports idle")
	}
	if rc.syncRunning {
		t.Error("an idle pass must clear syncRunning so the loop can start again later")
	}

	_, _ = rc.Chat(context.Background(), proxy.ChatRequest{Messages: []proxy.Message{{Role: "user", Content: "hi"}}})
	if rc.syncOnce() {
		t.Error("with a recording open the pass is not idle")
	}
	rc.Stop()
}

func TestSyncLoop_ExitsWhenIdleAndWhenStopped(t *testing.T) {
	idle := New(&mockClient{}, t.TempDir(), "m")
	idle.syncEvery = 5 * time.Millisecond
	idle.startSyncLoopIfNeeded()
	waitFor(t, func() bool { idle.mu.Lock(); defer idle.mu.Unlock(); return !idle.syncRunning }, "an idle loop to end itself")

	busy := New(&mockClient{chatFn: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		return &proxy.ChatResponse{}, nil
	}}, t.TempDir(), "m")
	busy.syncEvery = 5 * time.Millisecond
	_, _ = busy.Chat(context.Background(), proxy.ChatRequest{})
	time.Sleep(30 * time.Millisecond) // several passes with a recording open
	busy.mu.Lock()
	running := busy.syncRunning
	busy.mu.Unlock()
	if !running {
		t.Error("the loop must keep running while a recording is open")
	}
	busy.Stop()
	waitFor(t, func() bool { busy.Stop(); return true }, "Stop to be safe to repeat")
}

func waitFor(t *testing.T, ok func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
