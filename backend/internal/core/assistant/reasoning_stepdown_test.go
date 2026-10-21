package assistant

import (
	"testing"

	"llm-proxy/models"
)

func stepDownAgent(provider string, workload models.WorkloadClass) *Agent {
	return NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{ProviderType: provider, WorkloadClass: workload, MaxResponseTokens: 5461})
}

// After a stream was cut as a reasoning loop, the next request asks for less reasoning, through whichever control the
// model's provider uses, once; the request after that is as before. A local model answers without thinking, an effort
// model drops one effort level, an object model lowers its effort and stays enabled, and an on/off model is left alone.
func TestLoopCutLowersTheNextRequestsReasoning(t *testing.T) {
	t.Run("local: thinking is switched off for one request", func(t *testing.T) {
		a := stepDownAgent(models.ProviderLocal, models.WorkloadLocal)
		var req models.ChatRequest
		a.applyRequestConfig(&req)
		if req.ThinkingBudgetTokens == 0 {
			t.Fatal("a local request should carry its think budget")
		}
		a.noteStuckThinking()
		req = models.ChatRequest{}
		a.applyRequestConfig(&req)
		if req.ThinkingBudgetTokens != 0 || req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking == nil || *req.ChatTemplateKwargs.EnableThinking {
			t.Errorf("local request after a loop cut: %+v", req)
		}
		req = models.ChatRequest{}
		a.applyRequestConfig(&req)
		if req.ThinkingBudgetTokens == 0 {
			t.Error("the step is one-shot: the next request must carry the budget again")
		}
	})

	t.Run("effort model: one level lower, once", func(t *testing.T) {
		a := stepDownAgent(models.ProviderOpenAI, models.WorkloadCloud)
		var req models.ChatRequest
		a.applyRequestConfig(&req)
		if req.ReasoningEffort != "medium" {
			t.Fatalf("effort before = %q", req.ReasoningEffort)
		}
		a.noteStuckThinking()
		req = models.ChatRequest{}
		a.applyRequestConfig(&req)
		if req.ReasoningEffort != "low" {
			t.Errorf("effort after a loop cut = %q, want low", req.ReasoningEffort)
		}
		req = models.ChatRequest{}
		a.applyRequestConfig(&req)
		if req.ReasoningEffort != "medium" {
			t.Errorf("effort on the request after = %q, want medium again", req.ReasoningEffort)
		}
	})

	t.Run("object model: effort lowered, still enabled", func(t *testing.T) {
		a := stepDownAgent(models.ProviderOpenRouter, models.WorkloadCloud)
		a.noteStuckThinking()
		var req models.ChatRequest
		a.applyRequestConfig(&req)
		if req.Reasoning == nil || req.Reasoning.Effort != "low" || req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
			t.Errorf("object after a loop cut: %+v", req.Reasoning)
		}
	})

	t.Run("on/off model: reasoning is not ours to switch off", func(t *testing.T) {
		a := stepDownAgent(models.ProviderNVIDIA, models.WorkloadCloud)
		a.noteStuckThinking()
		var req models.ChatRequest
		a.applyRequestConfig(&req)
		if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking == nil || !*req.ChatTemplateKwargs.EnableThinking {
			t.Errorf("the toggle was switched off: %+v", req.ChatTemplateKwargs)
		}
	})
}
