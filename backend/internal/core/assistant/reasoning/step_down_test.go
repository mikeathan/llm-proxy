package reasoning

import (
	"testing"

	"llm-proxy/models"
)

// StepDown lowers one request's reasoning by one conservative step, whatever the wire mechanism: a local think-token
// budget is switched off (the existing recovery), an effort level goes down one notch and never to none, and a
// plain on/off toggle is left alone because switching a cloud model's reasoning off is not ours to do.
func TestStepDown(t *testing.T) {
	t.Run("think tokens: thinking is switched off and the budget dropped", func(t *testing.T) {
		spec := ReasoningSpec{Mode: ModeThinkTokens, Effort: EffortMedium, Budget: 1820}
		req := &models.ChatRequest{}
		resolverThinkTokens.Apply(req, spec)
		if !StepDown(req, spec) {
			t.Fatal("think tokens must step down")
		}
		if req.ThinkingBudgetTokens != 0 || req.ReasoningBudgetMessage != "" ||
			req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking == nil || *req.ChatTemplateKwargs.EnableThinking {
			t.Errorf("thinking not switched off: %+v", req)
		}
	})

	t.Run("effort: one level down, never to none", func(t *testing.T) {
		for _, c := range []struct {
			from      ReasoningEffort
			wire      string // the effort on the wire after the step
			steppable bool
		}{{EffortHigh, "medium", true}, {EffortMedium, "low", true}, {EffortLow, "low", false}, {EffortNone, "", false}} {
			spec := ReasoningSpec{Mode: ModeEffort, Effort: c.from}
			req := &models.ChatRequest{}
			resolverEffort.Apply(req, spec)
			if got := StepDown(req, spec); got != c.steppable {
				t.Errorf("effort %v: stepped = %v, want %v", c.from, got, c.steppable)
			}
			if req.ReasoningEffort != c.wire {
				t.Errorf("effort %v: wire effort %q, want %q", c.from, req.ReasoningEffort, c.wire)
			}
		}
	})

	t.Run("object: its effort goes down and it stays enabled", func(t *testing.T) {
		spec := ReasoningSpec{Mode: ModeObject, Effort: EffortHigh, Enabled: true}
		req := &models.ChatRequest{}
		resolverObject.Apply(req, spec)
		if !StepDown(req, spec) {
			t.Fatal("object mode must step down from high")
		}
		if req.Reasoning == nil || req.Reasoning.Effort != "medium" || req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
			t.Errorf("object not lowered conservatively: %+v", req.Reasoning)
		}
	})

	t.Run("a toggle has no lower level and is left as it was", func(t *testing.T) {
		spec := ReasoningSpec{Mode: ModeEnableThinking, Effort: EffortMedium, Enabled: true}
		req := &models.ChatRequest{}
		resolverEnableThinking.Apply(req, spec)
		if StepDown(req, spec) {
			t.Error("enable_thinking must not step down")
		}
		if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking == nil || !*req.ChatTemplateKwargs.EnableThinking {
			t.Errorf("the toggle was changed: %+v", req.ChatTemplateKwargs)
		}
	})
}

// Describe names the reasoning control a request really carries, for the request log: the think-token budget, the
// effort, or the on/off toggle, whichever the resolver set.
func TestDescribe(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		req  models.ChatRequest
		want string
	}{
		{"nothing", models.ChatRequest{}, "none"},
		{"think tokens", models.ChatRequest{ThinkingBudgetTokens: 1820}, "thinking_budget_tokens=1820"},
		{"effort", models.ChatRequest{ReasoningEffort: "medium"}, "reasoning_effort=medium"},
		{"object", models.ChatRequest{Reasoning: &models.ReasoningObject{Effort: "high", Enabled: &yes}}, "reasoning{effort=high,enabled=true}"},
		{"toggle off", models.ChatRequest{ChatTemplateKwargs: &models.ChatTemplateKwargs{EnableThinking: &no}}, "enable_thinking=false"},
		{"preserve only is not a reasoning control", models.ChatRequest{ChatTemplateKwargs: &models.ChatTemplateKwargs{PreserveThinking: &yes}}, "none"},
		{"budget and toggle off together", models.ChatRequest{ThinkingBudgetTokens: 5, ChatTemplateKwargs: &models.ChatTemplateKwargs{EnableThinking: &no}}, "thinking_budget_tokens=5,enable_thinking=false"},
	}
	for _, c := range cases {
		if got := Describe(&c.req); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
