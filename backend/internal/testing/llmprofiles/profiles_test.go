package llmprofiles

import (
	"context"
	"llm-proxy/internal/core/proxy"
	"strings"
	"testing"
)

func TestFixtureClient_Chat(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"hello world"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	resp, err := client.Chat(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Choices[0].Message.Content != "hello world" {
		t.Fatalf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
}

func TestFixtureClient_Stream(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"chunk","choices":[{"delta":{"content":"hello "}}]}
{"type":"chunk","choices":[{"delta":{"content":"world"}}]}
{"type":"done","total_chunks":2}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	ch, err := client.Stream(context.Background(), proxy.ChatRequest{})
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
}

func TestFixtureClient_Chat_Error(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"error","message":"rate limit exceeded"}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	_, err = client.Chat(context.Background(), proxy.ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestFixtureClient_Stream_Error(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"error","message":"connection timeout"}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	_, err = client.Stream(context.Background(), proxy.ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "connection timeout") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestFixtureClient_MultiTurn(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"first"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"turn-1"}}]}
{"type":"done","total_chunks":1}
{"type":"request","model":"gemma4","messages":[{"role":"user","content":"second"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"turn-2"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	resp1, _ := client.Chat(context.Background(), proxy.ChatRequest{})
	resp2, _ := client.Chat(context.Background(), proxy.ChatRequest{})

	if resp1.Choices[0].Message.Content != "turn-1" {
		t.Fatalf("expected turn-1, got %s", resp1.Choices[0].Message.Content)
	}
	if resp2.Choices[0].Message.Content != "turn-2" {
		t.Fatalf("expected turn-2, got %s", resp2.Choices[0].Message.Content)
	}
}

func TestFixtureClient_Exhausted(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4"}
{"type":"response","choices":[{"message":{"role":"assistant","content":"only one"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	_, _ = client.Chat(context.Background(), proxy.ChatRequest{})
	_, err = client.Chat(context.Background(), proxy.ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "no more recorded calls") {
		t.Fatalf("expected 'no more recorded calls', got %v", err)
	}
}

func TestFixtureClient_Stream_FromResponse(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"hello world"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	ch, err := client.Stream(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var content string
	for chunk := range ch {
		content += chunk.Choices[0].Delta.Content
	}
	if content != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", content)
	}
}

func TestFixtureClient_Stream_FromResponse_WithToolCalls(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"weather"}]}
{"type":"response","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"London\"}"}}]}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	ch, err := client.Stream(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var toolCalls []proxy.ToolCall
	for chunk := range ch {
		toolCalls = append(toolCalls, chunk.Choices[0].Delta.ToolCalls...)
	}
	if len(toolCalls) != 1 || toolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("expected 1 tool call get_weather, got %+v", toolCalls)
	}
}

func TestFixtureClient_RecordsRequest(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"test_tool"}}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"ok"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(client.calls))
	}
	call := client.calls[0]
	if call.Request.Model != "gemma4" {
		t.Fatalf("expected model gemma4, got %s", call.Request.Model)
	}
	if len(call.Request.Messages) != 1 || call.Request.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", call.Request.Messages)
	}
	if len(call.Request.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(call.Request.Tools))
	}
}

func TestChat_SynthesizesFromChunks(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"hi"}]}
{"type":"chunk","choices":[{"delta":{"content":"Hello"}}]}
{"type":"chunk","choices":[{"delta":{"content":" world"}}]}
{"type":"done","total_chunks":2}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	resp, err := client.Chat(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "Hello world" {
		t.Fatalf("expected 'Hello world', got '%s'", resp.Choices[0].Message.Content)
	}
}

func TestChat_SynthesizesToolCallsFromChunks(t *testing.T) {
	jsonl := `{"type":"request","model":"gemma4","messages":[{"role":"user","content":"weather"}]}
{"type":"chunk","choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":"}}]}}]}
{"type":"chunk","choices":[{"delta":{"tool_calls":[{"id":"","type":"","function":{"name":"","arguments":"\"London\"}"}}]}}]}
{"type":"done","total_chunks":2}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	resp, err := client.Chat(context.Background(), proxy.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	tc := resp.Choices[0].Message.ToolCalls
	if len(tc) != 1 || tc[0].Function.Name != "get_weather" {
		t.Fatalf("expected 1 tool call get_weather, got %d", len(tc))
	}
	if tc[0].Function.Arguments != `{"city":"London"}` {
		t.Fatalf("expected args {\"city\":\"London\"}, got %s", tc[0].Function.Arguments)
	}
}

func TestExtractToolResults(t *testing.T) {
	jsonl := `{"type":"request","model":"test-model","messages":[{"role":"user","content":"weather?"}]}
{"type":"response","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"London\"}"}}]}}]}
{"type":"done","total_chunks":1}
{"type":"request","model":"test-model","messages":[{"role":"user","content":"weather?"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"London\"}"}}]},{"role":"tool","content":"Sunny, 22°C","tool_call_id":"call_1"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"The weather is sunny."}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	if len(client.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(client.calls))
	}

	tr := client.calls[0].ToolResults
	if len(tr) != 1 {
		t.Fatalf("expected 1 tool result for call 0, got %d", len(tr))
	}
	if tr[0].Name != "get_weather" {
		t.Fatalf("expected tool name get_weather, got %s", tr[0].Name)
	}
	if tr[0].Arguments != `{"city":"London"}` {
		t.Fatalf("expected args Sydney, got %s", tr[0].Arguments)
	}
	if tr[0].Result != "Sunny, 22°C" {
		t.Fatalf("expected result Sunny, 22°C, got %s", tr[0].Result)
	}
	if tr[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id call_1, got %s", tr[0].ToolCallID)
	}
}

func TestExtractToolResults_MultiCall(t *testing.T) {
	jsonl := `{"type":"request","model":"test-model","messages":[{"role":"user","content":"do both"}]}
{"type":"response","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"tool_a","arguments":"{}"}},{"id":"c2","type":"function","function":{"name":"tool_b","arguments":"{}"}}]}}]}
{"type":"done","total_chunks":1}
{"type":"request","model":"test-model","messages":[{"role":"user","content":"do both"},{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"tool_a","arguments":"{}"}},{"id":"c2","type":"function","function":{"name":"tool_b","arguments":"{}"}}]},{"role":"tool","content":"result_a","tool_call_id":"c1"},{"role":"tool","content":"result_b","tool_call_id":"c2"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"done"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	tr := client.calls[0].ToolResults
	if len(tr) != 2 {
		t.Fatalf("expected 2 tool results, got %d", len(tr))
	}
	if tr[0].Name != "tool_a" || tr[1].Name != "tool_b" {
		t.Fatalf("unexpected tool order: %s, %s", tr[0].Name, tr[1].Name)
	}
	if tr[0].Result != "result_a" || tr[1].Result != "result_b" {
		t.Fatalf("unexpected results: %s, %s", tr[0].Result, tr[1].Result)
	}
}

func TestExtractToolResults_NoToolCalls(t *testing.T) {
	jsonl := `{"type":"request","model":"test-model","messages":[{"role":"user","content":"hi"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"hello"}}]}
{"type":"done","total_chunks":1}
{"type":"request","model":"test-model","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"how can I help"}}]}
{"type":"done","total_chunks":1}
`

	client, err := parseFixture(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("parseFixture failed: %v", err)
	}

	if len(client.calls[0].ToolResults) != 0 {
		t.Fatalf("expected 0 tool results for text-only turn, got %d", len(client.calls[0].ToolResults))
	}
}
