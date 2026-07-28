package assistant

import (
	"context"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// resolverTestClient is a minimal proxy.Client for resolver tests.
type resolverTestClient struct {
	field string
}

func (c *resolverTestClient) ReasoningField() string { return c.field }

func (c *resolverTestClient) Chat(_ context.Context, _ models.ChatRequest) (*models.ChatResponse, error) {
	return nil, nil
}

func (c *resolverTestClient) Stream(_ context.Context, _ models.ChatRequest) (<-chan *models.ChatResponse, error) {
	return nil, nil
}

func newResolverTestClient(field string) *resolverTestClient {
	return &resolverTestClient{field: field}
}

func TestReasoningModeEnums(t *testing.T) {
	if ModeThinkTokens != 0 || ModeEffort == ModeThinkTokens || ModeObject == ModeEffort || ModeEnableThinking == ModeObject {
		t.Fatal("reasoning modes must be distinct iota values")
	}
	if EffortLow.String() != "low" || EffortMedium.String() != "medium" || EffortHigh.String() != "high" {
		t.Fatalf("effort strings wrong: %s %s %s", EffortLow, EffortMedium, EffortHigh)
	}
}

func TestReasoningSpecValidate(t *testing.T) {
	valid := []ReasoningSpec{
		{Mode: ModeThinkTokens, Effort: EffortMedium, Budget: 1024},
		{Mode: ModeEffort, Effort: EffortHigh},
		{Mode: ModeObject, Effort: EffortLow},
		{Mode: ModeEnableThinking, Effort: EffortMedium, Enabled: true},
	}
	for i, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("case %d: expected valid, got %v", i, err)
		}
	}
	invalid := []ReasoningSpec{
		{Mode: ModeEffort, Effort: EffortMedium, Enabled: true},
		{Mode: ModeEnableThinking, Effort: EffortHigh, Enabled: true},
		{Mode: ModeThinkTokens, Effort: EffortLow, Enabled: true, Budget: 10},
	}
	for i, s := range invalid {
		if err := s.Validate(); err == nil {
			t.Errorf("case %d: expected invalid, got nil", i)
		}
	}
}

func TestEffortResolver(t *testing.T) {
	req := &models.ChatRequest{}
	effortResolver{}.Apply(req, ReasoningSpec{Mode: ModeEffort, Effort: EffortHigh})
	if req.ReasoningEffort != "high" {
		t.Errorf("expected reasoning_effort=high, got %q", req.ReasoningEffort)
	}
	if req.Reasoning != nil || req.ChatTemplateKwargs != nil || req.ThinkingBudgetTokens != 0 || req.ReasoningBudget != 0 {
		t.Errorf("effort resolver leaked other fields: %+v", req)
	}
}

func TestObjectResolver(t *testing.T) {
	req := &models.ChatRequest{}
	objectResolver{}.Apply(req, ReasoningSpec{Mode: ModeObject, Effort: EffortLow})
	if req.Reasoning == nil || req.Reasoning.Effort != "low" || !req.Reasoning.Enabled {
		t.Errorf("object resolver wrong: %+v", req.Reasoning)
	}
	if req.ReasoningEffort != "" || req.ChatTemplateKwargs != nil {
		t.Errorf("object resolver leaked fields: %+v", req)
	}
}

func TestEnableThinkingResolver(t *testing.T) {
	req := &models.ChatRequest{}
	enableThinkingResolver{}.Apply(req, ReasoningSpec{Mode: ModeEnableThinking, Enabled: true})
	if req.ChatTemplateKwargs == nil || !req.ChatTemplateKwargs.EnableThinking {
		t.Errorf("expected enable_thinking=true, got %+v", req.ChatTemplateKwargs)
	}
	if req.ReasoningBudget != 0 || req.ThinkingBudgetTokens != 0 {
		t.Errorf("enable_thinking leaked budget fields: %+v", req)
	}
}

func TestThinkTokensResolver(t *testing.T) {
	req := &models.ChatRequest{}
	thinkTokensResolver{}.Apply(req, ReasoningSpec{Mode: ModeThinkTokens, Budget: 1500})
	if req.ThinkingBudgetTokens != 1500 {
		t.Errorf("expected thinking_budget_tokens=1500, got %d", req.ThinkingBudgetTokens)
	}
	if req.ReasoningBudget != 0 || req.ReasoningEffort != "" {
		t.Errorf("think_tokens leaked fields: %+v", req)
	}
}

func TestNewReasoningResolver_LocalHostOverride(t *testing.T) {
	// Critical local-regression guard: an "openai"-slugged config pointing at a
	// local llama.cpp host must use think-tokens, not reasoning_effort.
	client := newResolverTestClient(proxy.ReasoningFieldThinkTokens)
	r := NewReasoningResolver("openai", client, 1500)
	req := &models.ChatRequest{}
	r.Apply(req, providerReasoningTable["openai"])
	if req.ThinkingBudgetTokens != 1500 {
		t.Errorf("local host override should yield thinking_budget_tokens, got %+v", req)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("local host override must not set reasoning_effort, got %q", req.ReasoningEffort)
	}
}

func TestNewReasoningResolver_Cloud(t *testing.T) {
	client := newResolverTestClient(proxy.ReasoningFieldBudget)
	req := &models.ChatRequest{}
	NewReasoningResolver("openai", client, 0).Apply(req, providerReasoningTable["openai"])
	if req.ReasoningEffort != "medium" {
		t.Errorf("openai should set reasoning_effort, got %q", req.ReasoningEffort)
	}

	req2 := &models.ChatRequest{}
	NewReasoningResolver("nvidia", client, 0).Apply(req2, providerReasoningTable["nvidia"])
	if req2.ChatTemplateKwargs == nil || !req2.ChatTemplateKwargs.EnableThinking {
		t.Errorf("nvidia should set enable_thinking, got %+v", req2.ChatTemplateKwargs)
	}

	req3 := &models.ChatRequest{}
	NewReasoningResolver("openrouter", client, 0).Apply(req3, providerReasoningTable["openrouter"])
	if req3.Reasoning == nil {
		t.Errorf("openrouter should set reasoning object")
	}
}

func TestNewReasoningResolver_UnknownNoop(t *testing.T) {
	client := newResolverTestClient(proxy.ReasoningFieldBudget)
	req := &models.ChatRequest{} // fresh request — noop must not add any reasoning param
	NewReasoningResolver("does-not-exist", client, 0).Apply(req, ReasoningSpec{})
	if req.ReasoningBudget != 0 || req.ReasoningEffort != "" || req.ThinkingBudgetTokens != 0 || req.Reasoning != nil || req.ChatTemplateKwargs != nil {
		t.Errorf("noop resolver should leave reasoning params empty, got %+v", req)
	}
}
