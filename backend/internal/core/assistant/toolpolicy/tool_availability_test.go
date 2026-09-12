package toolpolicy

import (
	"context"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type stubProvider struct {
	tools       []proxy.Tool
	nativeTools bool
}

func (s *stubProvider) ListTools(_ context.Context) ([]proxy.Tool, error) { return s.tools, nil }
func (s *stubProvider) GetSystemPrompt() (string, error)                  { return "system", nil }
func (s *stubProvider) UseNativeTools() bool                              { return s.nativeTools }

func toolNames(tools []proxy.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Function.Name] = true
	}
	return out
}

// ---------------------------------------------------------------------------
// Resolve — the narrow waist (allow ∩ exclude ∩ guardrail-disabled)
// ---------------------------------------------------------------------------

func TestResolve_Intersection(t *testing.T) {
	base := &stubProvider{tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolNotifyUser}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "extra_tool"}},
	}}
	// Communication disabled → notify_user excluded by guardrail policy.
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	// allow {read_file, notify_user} ∩ exclude {read_file} ∩ guardrail-disabled {notify_user} = {}.
	resolved := Resolve(base, gr, "ws-1", []string{"read_file", models.ToolNotifyUser}, []string{"read_file"})
	tools, err := resolved.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty intersection, got %v", toolNames(tools))
	}

	// allow {read_file, extra_tool} ∩ guardrail-disabled {notify_user} = {read_file, extra_tool}.
	resolved = Resolve(base, gr, "ws-1", []string{"read_file", "extra_tool"}, nil)
	tools, err = resolved.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	names := toolNames(tools)
	if !names["read_file"] || !names["extra_tool"] || names[models.ToolNotifyUser] {
		t.Errorf("expected {read_file, extra_tool} (notify_user filtered by guardrail), got %v", names)
	}
}

func TestResolve_GuardrailDisabledOnly(t *testing.T) {
	base := &stubProvider{tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolNotifyUser}},
	}}
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	resolved := Resolve(base, gr, "ws-1", nil, nil)
	if _, ok := resolved.(*filteredProvider); !ok {
		t.Fatal("expected a filtered provider when guardrails disable a tool")
	}
	tools, err := resolved.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	names := toolNames(tools)
	if !names["test_tool"] {
		t.Error("expected test_tool to remain available")
	}
	if names[models.ToolNotifyUser] {
		t.Error("notify_user must be excluded when communication is disabled")
	}
}

func TestResolve_NoFilteringReturnsBase(t *testing.T) {
	base := &stubProvider{tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}}}
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Communication: models.CommunicationGuardrailsConfig{Enabled: true},
			Search:        models.SearchGuardrailsConfig{Enabled: true},
			Network:       models.NetworkGuardrailsConfig{Enabled: true},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	resolved := Resolve(base, gr, "ws-1", nil, nil)
	if resolved != base {
		t.Error("expected the base provider unwrapped when nothing needs filtering")
	}
}

// ---------------------------------------------------------------------------
// filteredProvider — allowed/excluded semantics
// ---------------------------------------------------------------------------

func TestFilteredProvider_ExcludesTools(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "notify_user"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	fp := Resolve(inner, nil, "", nil, []string{"notify_user"})

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

func TestFilteredProvider_EmptyExclude(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "notify_user"}},
		},
	}
	fp := Resolve(inner, nil, "", nil, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("nil exclude should not filter, got %d tools", len(tools))
	}
}

func TestFilteredProvider_PreservesOrder(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "a"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "b"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "c"}},
		},
	}
	fp := Resolve(inner, nil, "", nil, []string{"b"})
	tools, _ := fp.ListTools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Function.Name != "a" || tools[1].Function.Name != "c" {
		t.Errorf("order should be preserved: got [%s, %s]", tools[0].Function.Name, tools[1].Function.Name)
	}
}

func TestFilteredProvider_Delegates(t *testing.T) {
	inner := &stubProvider{nativeTools: true}
	fp := Resolve(inner, nil, "", nil, []string{"notify_user"})
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
	fp := Resolve(inner, nil, "", []string{"read_file", "directory_list"}, nil)
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
	fp := Resolve(inner, nil, "", nil, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("empty allowed should pass all tools through, got %d", len(tools))
	}
}

// TestAllowedTools_ReadOnlyAutomation pins the read-only automation allowlist
// (allow ∩ exclude) end-to-end through Resolve.
func TestAllowedTools_ReadOnlyAutomation(t *testing.T) {
	inner := &stubProvider{
		tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "directory_list"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "terminal_execute"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "grep"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "write_file"}},
		},
	}
	fp := Resolve(inner, nil, "", []string{"read_file", "directory_list"}, nil)
	tools, err := fp.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("read-only automation should expose only 2 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Function.Name] = true
	}
	if names["terminal_execute"] || names["grep"] || names["write_file"] {
		t.Error("terminal_execute, grep, write_file should be blocked for read-only automation")
	}
	if !names["read_file"] || !names["directory_list"] {
		t.Error("read_file and directory_list should be available")
	}

	fpAll := Resolve(inner, nil, "", nil, nil)
	toolsAll, err := fpAll.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools (all): %v", err)
	}
	if len(toolsAll) != 5 {
		t.Errorf("nil AllowedTools should pass all 5 through, got %d", len(toolsAll))
	}
}
