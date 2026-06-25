package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGBNFConstraint_EmptyTools(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	if c.Apply(req, nil) {
		t.Error("expected false for nil tools")
	}
	if c.Apply(req, []Tool{}) {
		t.Error("expected false for empty tools")
	}
	if req.Grammar != nil {
		t.Error("expected Grammar to stay nil")
	}
}

func TestGBNFConstraint_NoSchema(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	ok := c.Apply(req, []Tool{
		{Function: FunctionSchema{Name: "no_schema"}},
	})
	if ok {
		t.Error("expected false for tool without parameters schema")
	}
}

func TestGBNFConstraint_StringParam(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "simple_tool",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []any{"query"},
				},
			},
		},
	}
	if !c.Apply(req, tools) {
		t.Fatal("expected true for valid schema")
	}
	if req.Grammar == nil {
		t.Fatal("expected non-nil Grammar")
	}
	if !strings.Contains(*req.Grammar, "string-lit") {
		t.Error("expected string-lit rule in grammar")
	}
	if !strings.Contains(*req.Grammar, "\"query\"") {
		t.Error("expected query field in grammar")
	}
}

func TestGBNFConstraint_AllTypes(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "all_types",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  map[string]any{"type": "string"},
						"count": map[string]any{"type": "integer"},
						"flag":  map[string]any{"type": "boolean"},
						"tags": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"mode": map[string]any{
							"type": "string",
							"enum": []any{"fast", "deep"},
						},
					},
					"required": []any{"name"},
				},
			},
		},
	}
	if !c.Apply(req, tools) {
		t.Fatal("expected true for all-types schema")
	}
	g := *req.Grammar
	for _, want := range []string{"string-lit", "int-lit", "bool-lit", "\"mode\"", "\"tags\"", "\"fast\"", "\"deep\""} {
		if !strings.Contains(g, want) {
			t.Errorf("expected %q in grammar", want)
		}
	}
}

func TestGBNFConstraint_MultiTool(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "tool_a",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"x": map[string]any{"type": "string"},
					},
					"required": []any{"x"},
				},
			},
		},
		{
			Function: FunctionSchema{
				Name: "tool_b",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"y": map[string]any{"type": "integer"},
					},
					"required": []any{"y"},
				},
			},
		},
	}
	if !c.Apply(req, tools) {
		t.Fatal("expected true for multi-tool")
	}
	g := *req.Grammar
	if !strings.Contains(g, "|") {
		t.Error("expected disjunctive | for multi-tool grammar")
	}
}

func TestGBNFConstraint_OptionalField(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "mixed",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":    map[string]any{"type": "string"},
						"comment": map[string]any{"type": "string"},
					},
					"required": []any{"name"},
				},
			},
		},
	}
	if !c.Apply(req, tools) {
		t.Fatal("expected true for schema with optionals")
	}
}

func TestGBNFConstraint_AllOptional(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "search",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"limit": map[string]any{"type": "integer"},
					},
					"required": []any{},
				},
			},
		},
	}
	if !c.Apply(req, tools) {
		t.Fatal("expected true for all-optional schema")
	}
	g := *req.Grammar
	// The grammar should produce valid JSON with no leading comma.
	// Check that empty object is allowed (no leading comma in pattern).
	if !strings.Contains(g, "(") {
		t.Error("expected optional group for all-optional fields")
	}
}

func TestGBNFConstraint_ApplyIdempotent(t *testing.T) {
	c := &GBNFConstraint{}
	req := &ChatRequest{}
	tools := []Tool{
		{
			Function: FunctionSchema{
				Name: "a",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []any{"query"},
				},
			},
		},
	}
	g1 := ""
	if c.Apply(req, tools) {
		g1 = *req.Grammar
	}
	if c.Apply(req, tools) {
		g2 := *req.Grammar
		if g1 != g2 {
			t.Error("Apply should produce identical grammar for same tools")
		}
	}
}

func TestClientChat_GrammarFieldInBody(t *testing.T) {
	var capturedBody []byte
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(r.Body)
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}
	grammar := `root ::= "{" "x" ":" string-lit "}"`
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{
		Model: "test",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
		Grammar: &grammar,
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(capturedBody, &bodyMap); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if _, ok := bodyMap["grammar"]; !ok {
		t.Error("expected grammar in request body")
	}
}

func TestClientChat_GrammarFieldNotSet(t *testing.T) {
	var capturedBody []byte
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(r.Body)
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{
		Model: "test",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(capturedBody, &bodyMap); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if _, ok := bodyMap["grammar"]; ok {
		t.Error("expected no grammar in body when not set")
	}
}
