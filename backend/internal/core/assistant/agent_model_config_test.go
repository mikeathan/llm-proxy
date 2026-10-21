package assistant

import (
	"testing"
	"time"

	"llm-proxy/models"
)

// ApplyModelConfig carries every per-model override onto the options, and leaves an option alone when its field is unset.
func TestApplyModelConfig_CopiesEveryOverride(t *testing.T) {
	yes, reasoning := true, false
	cfg := models.ModelConfig{
		Provider: "openai", WorkloadClass: models.WorkloadClass("cloud"), LoopStrategy: "react",
		MaxSteps: 7, ContextBudget: 9000, MaxTokens: 512, ReasoningBudget: 2048, ReasoningEnabled: &reasoning,
		Temperature: 0.4, ToolCallFormat: "native", Prefill: &yes,
		TimeoutMinutes: 3, ToolTimeoutSeconds: 11, FilesystemToolTimeoutSeconds: 12,
		MaxPlanDurationMinutes: 4, MaxPlanSteps: 13, GuardrailTimeoutSeconds: 6,
		GuardrailTimeoutBehavior: "fail-closed", GuardrailApprovalTimeoutSecs: 90,
	}
	var o AgentOptions
	o.ApplyModelConfig(cfg)

	checks := []struct {
		name      string
		got, want any
	}{
		{"provider", o.ProviderType, "openai"},
		{"workload", o.WorkloadClass, models.WorkloadClass("cloud")},
		{"max steps", o.MaxSteps, 7},
		{"context budget", o.ContextBudget, 9000},
		{"max response tokens", o.MaxResponseTokens, 512},
		{"reasoning budget", o.ReasoningBudget, 2048},
		{"temperature", o.Temperature, 0.4},
		{"prefill", o.UsePrefill, true},
		{"global timeout", o.GlobalTimeout, 3 * time.Minute},
		{"tool timeout", o.ToolTimeout, 11 * time.Second},
		{"filesystem timeout", o.FilesystemToolTimeout, 12 * time.Second},
		{"plan duration", o.MaxPlanDuration, 4 * time.Minute},
		{"plan steps", o.MaxPlanSteps, 13},
		{"guardrail timeout", o.GuardrailTimeout, 6 * time.Second},
		{"guardrail behavior", o.GuardrailTimeoutBehavior, "fail-closed"},
		{"approval timeout", o.GuardrailApprovalTimeout, 90 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
	if o.UseNativeTools == nil || !*o.UseNativeTools {
		t.Error("tool_call_format native must lock native tools on")
	}
	if o.ReasoningEnabled == nil || *o.ReasoningEnabled {
		t.Error("reasoning_enabled false must be carried, not dropped")
	}
	if o.LoopStrategy != ParseLoopStrategy("react") {
		t.Errorf("loop strategy %q", o.LoopStrategy)
	}

	keep := AgentOptions{MaxSteps: 3, ToolTimeout: time.Second, GuardrailTimeoutBehavior: "fail-open"}
	keep.ApplyModelConfig(models.ModelConfig{})
	if keep.MaxSteps != 3 || keep.ToolTimeout != time.Second || keep.GuardrailTimeoutBehavior != "fail-open" || keep.UseNativeTools != nil {
		t.Errorf("an empty config must change nothing: %+v", keep)
	}
}
