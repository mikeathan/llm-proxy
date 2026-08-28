package assistant

import (
	"testing"

	"llm-proxy/internal/core/assistant/reasoning"
	"llm-proxy/models"
)

func TestReasoningEnabledOverrideIsProviderSpecific(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode reasoning.ReasoningMode
	}{
		{name: "nvidia", mode: reasoning.ModeEnableThinking},
		{name: "openrouter", mode: reasoning.ModeObject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disabled := false
			spec := applyReasoningEnabledOverride(reasoning.ReasoningSpec{Mode: tc.mode, Enabled: true}, &disabled, models.WorkloadCloud)
			if spec.Enabled {
				t.Fatal("expected provider reasoning to be disabled")
			}
		})
	}
	local := applyReasoningEnabledOverride(reasoning.ReasoningSpec{Mode: reasoning.ModeThinkTokens, Budget: 1024}, boolPtr(false), models.WorkloadLocal)
	if local.Budget != 1024 {
		t.Fatal("local reasoning budget should remain unchanged")
	}
}

// TestApplyReasoningEnabledOverride_Effort verifies effort-mode mapping:
// enabled -> medium, disabled -> EffortNone (omitted wire), nil -> untouched.
func TestApplyReasoningEnabledOverride_Effort(t *testing.T) {
	enabled := true
	disabled := false

	on := applyReasoningEnabledOverride(reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortNone}, &enabled, models.WorkloadCloud)
	if on.Effort != reasoning.EffortMedium {
		t.Errorf("enabled effort should map to medium, got %v", on.Effort)
	}

	off := applyReasoningEnabledOverride(reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortMedium}, &disabled, models.WorkloadCloud)
	if off.Effort != reasoning.EffortNone {
		t.Errorf("disabled effort should map to EffortNone, got %v", off.Effort)
	}

	nilOverride := applyReasoningEnabledOverride(reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortHigh}, nil, models.WorkloadCloud)
	if nilOverride.Effort != reasoning.EffortHigh {
		t.Errorf("nil override must leave effort untouched, got %v", nilOverride.Effort)
	}
}

// TestApplyReasoningEnabledOverride_LocalLoopbackOpenaiSlug: a WorkloadLocal
// openai slug must be byte-identical to input regardless of enabled flag.
func TestApplyReasoningEnabledOverride_LocalLoopbackOpenaiSlug(t *testing.T) {
	disabled := false
	in := reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortMedium}
	out := applyReasoningEnabledOverride(in, &disabled, models.WorkloadLocal)
	if out != in {
		t.Errorf("local workload override must not mutate spec: in %+v out %+v", in, out)
	}
}
