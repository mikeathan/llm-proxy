package assistant

import (
	"context"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func toolNames(tools []proxy.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Function.Name] = true
	}
	return out
}

func TestResolveToolProvider_Intersection(t *testing.T) {
	base := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolNotifyUser}},
		{Type: "function", Function: proxy.FunctionSchema{Name: "extra_tool"}},
	}}
	// Communication disabled → notify_user excluded by guardrail policy.
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	// allow {read_file, notify_user} ∩ exclude {read_file} ∩ guardrail-disabled {notify_user} = {}.
	resolved := resolveToolProvider(base, gr, "ws-1", []string{"read_file", models.ToolNotifyUser}, []string{"read_file"})
	tools, err := resolved.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty intersection, got %v", toolNames(tools))
	}

	// allow {read_file, extra_tool} ∩ guardrail-disabled {notify_user} = {read_file, extra_tool}.
	resolved = resolveToolProvider(base, gr, "ws-1", []string{"read_file", "extra_tool"}, nil)
	tools, err = resolved.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	names := toolNames(tools)
	if !names["read_file"] || !names["extra_tool"] || names[models.ToolNotifyUser] {
		t.Errorf("expected {read_file, extra_tool} (notify_user filtered by guardrail), got %v", names)
	}
}

func TestResolveToolProvider_GuardrailDisabledOnly(t *testing.T) {
	base := &MockProvider{Tools: []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolNotifyUser}},
	}}
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	resolved := resolveToolProvider(base, gr, "ws-1", nil, nil)
	if _, ok := resolved.(*filteredToolProvider); !ok {
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

func TestResolveToolProvider_NoFilteringReturnsBase(t *testing.T) {
	base := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}}}
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Communication: models.CommunicationGuardrailsConfig{Enabled: true},
			Search:        models.SearchGuardrailsConfig{Enabled: true},
			Network:       models.NetworkGuardrailsConfig{Enabled: true},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	resolved := resolveToolProvider(base, gr, "ws-1", nil, nil)
	if resolved != base {
		t.Error("expected the base provider unwrapped when nothing needs filtering")
	}
}

// TestPlanExecute_NotifyUserSchemaFollowsGuardrail pins the plan-execute
// regression: a statically-disabled tool (notify_user with communication off)
// must never be exposed to the plan generator; enabling communication restores
// it.
func TestPlanExecute_NotifyUserSchemaFollowsGuardrail(t *testing.T) {
	client := &MockClient{}
	baseTools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolNotifyUser}},
	}
	engine := &MockEngine{Result: "ok"}

	grDisabled := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: false}}
	}, storage.NewPathResolver("", "", ""), nil, nil)
	agentDisabled := NewAgent(client, &MockProvider{Tools: baseTools}, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		Guardrails:  grDisabled,
	})
	tools, err := agentDisabled.deps.Provider.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	names := toolNames(tools)
	if names[models.ToolNotifyUser] {
		t.Error("plan tool set must exclude notify_user when communication is disabled")
	}
	if !names["test_tool"] {
		t.Error("non-gated tools must remain in the plan tool set")
	}

	grEnabled := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Communication: models.CommunicationGuardrailsConfig{Enabled: true}}
	}, storage.NewPathResolver("", "", ""), nil, nil)
	agentEnabled := NewAgent(client, &MockProvider{Tools: baseTools}, engine, AgentOptions{
		MaxSteps:    5,
		WorkspaceID: "ws-1",
		Guardrails:  grEnabled,
	})
	tools, err = agentEnabled.deps.Provider.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if !toolNames(tools)[models.ToolNotifyUser] {
		t.Error("plan tool set must include notify_user when communication is enabled")
	}
}
