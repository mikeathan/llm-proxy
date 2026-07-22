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

func TestFilteredToolProvider_ExcludesTools(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "notify_user"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	fp := NewFilteredToolProvider(inner, []string{"notify_user"})

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
	fp := NewFilteredToolProvider(inner, nil)
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
	fp := NewFilteredToolProvider(inner, []string{"b"})
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
	fp := NewFilteredToolProvider(inner, []string{"notify_user"})
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
