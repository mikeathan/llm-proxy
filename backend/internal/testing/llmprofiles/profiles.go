package llmprofiles

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/core/proxy"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type FixtureClient struct {
	calls []RecordedCall
	index int
	mu    sync.Mutex
}

type RecordedCall struct {
	Request     proxy.ChatRequest
	Response    *proxy.ChatResponse
	Chunks      []proxy.ChatResponse
	Error       error
	ToolResults []ToolRecord
}

type ToolRecord struct {
	ToolCallID string
	Name       string
	Arguments  string
	Result     string
}

type fixtureLine struct {
	Type        string          `json:"type"`
	Model       string          `json:"model,omitempty"`
	Messages    []proxy.Message `json:"messages,omitempty"`
	Tools       []proxy.Tool    `json:"tools,omitempty"`
	Choices     []proxy.Choice  `json:"choices,omitempty"`
	StatusCode  int             `json:"status_code,omitempty"`
	Message     string          `json:"message,omitempty"`
	TotalChunks int             `json:"total_chunks,omitempty"`
}

func NewFixtureClient(path string) (*FixtureClient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fixture: open %s: %w", path, err)
	}
	defer f.Close()

	return parseFixture(f)
}

func parseFixture(r io.Reader) (*FixtureClient, error) {
	var calls []RecordedCall
	scanner := bufio.NewScanner(r)
	var current *RecordedCall
	for scanner.Scan() {
		var line fixtureLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, fmt.Errorf("fixture: parse line: %w", err)
		}

		switch line.Type {
		case "request":
			if current != nil {
				calls = append(calls, *current)
			}
			current = &RecordedCall{
				Request: proxy.ChatRequest{
					Model:    line.Model,
					Messages: line.Messages,
					Tools:    line.Tools,
				},
			}
		case "chunk":
			if current != nil {
				current.Chunks = append(current.Chunks, proxy.ChatResponse{
					Choices: line.Choices,
				})
			}
		case "response":
			if current != nil {
				current.Response = &proxy.ChatResponse{}
				if len(line.Choices) > 0 {
					current.Response.Choices = line.Choices
				}
			}
		case "error":
			if current != nil {
				current.Error = fmt.Errorf("%s", line.Message)
			}
		}
	}
	if current != nil {
		calls = append(calls, *current)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("fixture: scan: %w", err)
	}

	extractToolResults(calls)

	return &FixtureClient{calls: calls}, nil
}

func extractToolResults(calls []RecordedCall) {
	for i := 0; i < len(calls)-1; i++ {
		prevMsgs := calls[i].Request.Messages
		currMsgs := calls[i+1].Request.Messages

		if len(currMsgs) <= len(prevMsgs) {
			continue
		}
		newMsgs := currMsgs[len(prevMsgs):]

		var assistantMsg *proxy.Message
		for i := range newMsgs {
			if newMsgs[i].Role == proxy.AssistantRole {
				assistantMsg = &newMsgs[i]
				break
			}
		}
		if assistantMsg == nil || len(assistantMsg.ToolCalls) == 0 {
			continue
		}

		tcIndex := 0
		for _, msg := range newMsgs {
			if msg.Role == proxy.ToolRole {
				if tcIndex < len(assistantMsg.ToolCalls) {
					tc := assistantMsg.ToolCalls[tcIndex]
					calls[i].ToolResults = append(calls[i].ToolResults, ToolRecord{
						ToolCallID: msg.ToolCallID,
						Name:       tc.Function.Name,
						Arguments:  tc.Function.Arguments,
						Result:     msg.Content,
					})
					tcIndex++
				}
			}
		}
	}
}

func (f *FixtureClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.index >= len(f.calls) {
		return nil, fmt.Errorf("fixture: no more recorded calls (index %d, total %d)", f.index, len(f.calls))
	}
	call := f.calls[f.index]
	f.index++

	if call.Error != nil {
		return nil, call.Error
	}
	if call.Response != nil {
		return call.Response, nil
	}
	if len(call.Chunks) > 0 {
		return synthesizeResponse(call.Chunks), nil
	}
	return nil, fmt.Errorf("fixture: no response or chunks for this call")
}

func synthesizeResponse(chunks []proxy.ChatResponse) *proxy.ChatResponse {
	msg := proxy.Message{Role: "assistant"}
	for _, ch := range chunks {
		for _, choice := range ch.Choices {
			msg.Content += choice.Delta.Content
			msg.ReasoningContent += choice.Delta.ReasoningContent
			for _, tc := range choice.Delta.ToolCalls {
				if tc.ID != "" {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				} else if len(msg.ToolCalls) > 0 {
					last := &msg.ToolCalls[len(msg.ToolCalls)-1]
					last.Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}
	return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}
}

func (f *FixtureClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	f.mu.Lock()
	if f.index >= len(f.calls) {
		f.mu.Unlock()
		return nil, fmt.Errorf("fixture: no more recorded calls (index %d, total %d)", f.index, len(f.calls))
	}
	call := f.calls[f.index]
	f.index++
	f.mu.Unlock()

	if call.Error != nil {
		return nil, call.Error
	}

	if len(call.Chunks) > 0 {
		ch := make(chan *proxy.ChatResponse, len(call.Chunks))
		for i := range call.Chunks {
			ch <- &call.Chunks[i]
		}
		close(ch)
		return ch, nil
	}

	if call.Response != nil && len(call.Response.Choices) > 0 {
		ch := make(chan *proxy.ChatResponse, 1)
		chunk := &proxy.ChatResponse{}
		for _, choice := range call.Response.Choices {
			delta := proxy.Message{
				Role:             choice.Message.Role,
				Content:          choice.Message.Content,
				ReasoningContent: choice.Message.ReasoningContent,
			}
			if len(choice.Message.ToolCalls) > 0 {
				delta.ToolCalls = choice.Message.ToolCalls
			}
			chunk.Choices = append(chunk.Choices, proxy.Choice{Delta: delta})
		}
		ch <- chunk
		close(ch)
		return ch, nil
	}

	return nil, fmt.Errorf("fixture: no response or chunks recorded for this call")
}

func (f *FixtureClient) Calls() []RecordedCall {
	return f.calls
}

func LoadFixture(path string) (*FixtureClient, error) {
	return NewFixtureClient(path)
}

func RunAgainstFixtures(t *testing.T, fixturesDir string, fn func(t *testing.T, client proxy.Client, name string)) {
	t.Helper()

	var files []string
	_ = filepath.WalkDir(fixturesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(path) == ".jsonl" {
			rel, _ := filepath.Rel(fixturesDir, path)
			files = append(files, rel)
		}
		return nil
	})

	if len(files) == 0 {
		t.Skip("no .jsonl fixture files found in " + fixturesDir)
	}

	for _, rel := range files {
		path := filepath.Join(fixturesDir, rel)
		t.Run(rel, func(t *testing.T) {
			client, err := NewFixtureClient(path)
			if err != nil {
				t.Fatalf("failed to load fixture %s: %v", rel, err)
			}
			fn(t, client, rel)
		})
	}
}

var _ proxy.Client = (*FixtureClient)(nil)
