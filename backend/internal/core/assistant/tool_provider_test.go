package assistant

import (
	"context"
	"fmt"
	"testing"

	"llm-proxy/internal/core/proxy"
)

var errTest = fmt.Errorf("test error")

func TestDeduplicateTools_NoDuplicates(t *testing.T) {
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_b"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_c"}},
	}
	result := deduplicateTools(tools)
	if len(result) != 3 {
		t.Errorf("expected 3 tools, got %d", len(result))
	}
	for i, tr := range result {
		if tr.Function.Name != tools[i].Function.Name {
			t.Errorf("tool %d: expected %s, got %s", i, tools[i].Function.Name, tr.Function.Name)
		}
	}
}

func TestDeduplicateTools_AllDuplicates(t *testing.T) {
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
	}
	result := deduplicateTools(tools)
	if len(result) != 1 {
		t.Errorf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Name != "tool_a" {
		t.Errorf("expected 'tool_a', got '%s'", result[0].Function.Name)
	}
}

func TestDeduplicateTools_PartialOverlap(t *testing.T) {
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_b"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_c"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_b"}},
	}
	result := deduplicateTools(tools)
	if len(result) != 3 {
		t.Errorf("expected 3 unique tools, got %d", len(result))
	}
}

func TestDeduplicateTools_FirstOccurrenceWins(t *testing.T) {
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a", Description: "first"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a", Description: "second"}},
	}
	result := deduplicateTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Description != "first" {
		t.Errorf("expected first occurrence to win, got description '%s'", result[0].Function.Description)
	}
}

func TestDeduplicateTools_Empty(t *testing.T) {
	result := deduplicateTools([]proxy.Tool{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d tools", len(result))
	}
}

func TestMultiToolProvider_Deduplication(t *testing.T) {
	prov1 := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "tool_a"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "tool_b"}},
		},
	}
	prov2 := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "tool_b"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "tool_c"}},
		},
	}

	multi := NewMultiToolProvider(true, prov1, prov2)
	tools, err := multi.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 3 {
		t.Errorf("expected 3 unique tools, got %d", len(tools))
	}
}

func TestCompositeEngine_PrimarySuccess(t *testing.T) {
	primary := &MockEngine{Result: "primary_result", Err: nil}
	secondary := &MockEngine{Result: "should_not_be_called", Err: nil}
	engine := NewCompositeEngine(primary, secondary)

	res, err := engine.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "tool_a"}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != "primary_result" {
		t.Errorf("expected 'primary_result', got %v", res)
	}
}

func TestCompositeEngine_PrimaryNotFound_FallsBackToSecondary(t *testing.T) {
	primary := &MockEngine{Result: nil, Err: ErrToolNotInternal}
	secondary := &MockEngine{Result: "secondary_result", Err: nil}
	engine := NewCompositeEngine(primary, secondary)

	res, err := engine.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "tool_a"}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != "secondary_result" {
		t.Errorf("expected 'secondary_result', got %v", res)
	}
}

func TestCompositeEngine_PrimaryNotFound_SecondaryFails(t *testing.T) {
	primary := &MockEngine{Result: nil, Err: ErrToolNotInternal}
	secondary := &MockEngine{Result: nil, Err: errTest}
	engine := NewCompositeEngine(primary, secondary)

	_, err := engine.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "tool_a"}})

	if err == nil {
		t.Fatal("expected error from secondary, got nil")
	}
}

func TestCompositeEngine_PrimaryError_PropagatesResultWithError(t *testing.T) {
	primary := &MockEngine{Result: "stderr: File exists", Err: errTest}
	secondary := &MockEngine{Result: "should_not_be_called", Err: nil}
	engine := NewCompositeEngine(primary, secondary)

	res, err := engine.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "tool_a"}})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != "stderr: File exists" {
		t.Errorf("expected 'stderr: File exists' to propagate, got %v", res)
	}
}

func TestCompositeEngine_PrimaryError_NilResult(t *testing.T) {
	primary := &MockEngine{Result: nil, Err: errTest}
	secondary := &MockEngine{Result: "should_not_be_called", Err: nil}
	engine := NewCompositeEngine(primary, secondary)

	res, err := engine.ExecuteTool(context.Background(), proxy.ToolCall{Function: proxy.FunctionCall{Name: "tool_a"}})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %v", res)
	}
}
