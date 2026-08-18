package assistant

import (
	"context"
	"testing"

	"llm-proxy/internal/core/proxy"
)

type stubProvider struct {
	tools       []proxy.Tool
	nativeTools bool
}

func (s *stubProvider) ListTools(_ context.Context) ([]proxy.Tool, error) {
	return s.tools, nil
}
func (s *stubProvider) GetSystemPrompt() (string, error) { return "system", nil }
func (s *stubProvider) UseNativeTools() bool             { return s.nativeTools }

// The provider tests exercise resolveToolProvider — the single narrow waist for
// tool availability. A nil guardrail engine isolates the pure allowed/excluded
// semantics from guardrail-derived exclusions (covered separately in
// tool_availability_test.go).

func TestFilteredToolProvider_ExcludesTools(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "notify_user"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	fp := resolveToolProvider(inner, nil, "", nil, []string{"notify_user"})

	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
	for _, tool := range tools {
		if tool.Function.Name == "notify_user" {
			t.Error("notify_user should be excluded from filtered provider")
		}
	}
}

func TestFilteredToolProvider_EmptyExclude(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "notify_user"}},
		},
	}
	fp := resolveToolProvider(inner, nil, "", nil, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("nil exclude should not filter, got %d tools", len(tools))
	}
}

func TestFilteredToolProvider_PreservesOrder(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "a"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "b"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "c"}},
		},
	}
	fp := resolveToolProvider(inner, nil, "", nil, []string{"b"})
	tools, _ := fp.ListTools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Function.Name != "a" || tools[1].Function.Name != "c" {
		t.Errorf("order should be preserved: got [%s, %s]", tools[0].Function.Name, tools[1].Function.Name)
	}
}

func TestFilteredToolProvider_Delegates(t *testing.T) {
	inner := &stubProvider{nativeTools: true}
	fp := resolveToolProvider(inner, nil, "", nil, []string{"notify_user"})
	if !fp.UseNativeTools() {
		t.Error("UseNativeTools should delegate to inner provider")
	}
	prompt, err := fp.GetSystemPrompt()
	if err != nil {
		t.Fatalf("GetSystemPrompt: %v", err)
	}
	if prompt != "system" {
		t.Errorf("GetSystemPrompt should delegate, got %q", prompt)
	}
}

func TestAllowedTools_RestrictsAccess(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "terminal_execute"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "directory_list"}},
		},
	}
	fp := resolveToolProvider(inner, nil, "", []string{"read_file", "directory_list"}, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 allowed tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Function.Name] = true
	}
	if names["terminal_execute"] {
		t.Error("terminal_execute should be blocked by allowlist")
	}
	if !names["read_file"] || !names["directory_list"] {
		t.Error("read_file and directory_list should be in allowlist")
	}
}

func TestAllowedTools_EmptyAllowsAll(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "terminal_execute"}},
		},
	}
	fp := resolveToolProvider(inner, nil, "", nil, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("empty allowed should pass all tools through, got %d", len(tools))
	}
}
