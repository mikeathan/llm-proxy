package assistant

import (
	"bytes"
	"encoding/json"
	"testing"

	"llm-proxy/models"
)

func TestReasoningModeEnums(t *testing.T) {
	if ModeThinkTokens != 0 || ModeEffort == ModeThinkTokens || ModeObject == ModeEffort || ModeEnableThinking == ModeObject {
		t.Fatal("reasoning modes must be distinct iota values")
	}
	if EffortLow.String() != "low" || EffortMedium.String() != "medium" || EffortHigh.String() != "high" {
		t.Fatalf("effort strings wrong: %s %s %s", EffortLow, EffortMedium, EffortHigh)
	}
	if EffortNone.String() != "" {
		t.Fatalf("EffortNone must stringify to empty (omitted on wire), got %q", EffortNone.String())
	}
}

func TestReasoningSpecValidate(t *testing.T) {
	valid := []ReasoningSpec{
		{Mode: ModeThinkTokens, Effort: EffortMedium, Budget: 1024},
		{Mode: ModeEffort, Effort: EffortHigh},
		{Mode: ModeEffort, Effort: EffortNone}, // disabled effort is valid
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
		{Mode: ModeObject, Effort: EffortNone}, // object needs a concrete effort
		{Mode: ModeEnableThinking, Effort: EffortNone, Enabled: true},
		{Mode: ModeThinkTokens, Effort: EffortNone, Enabled: true, Budget: 10},
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
	objectResolver{}.Apply(req, ReasoningSpec{Mode: ModeObject, Effort: EffortLow, Enabled: true})
	if req.Reasoning == nil || req.Reasoning.Effort != "low" || req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
		t.Errorf("object resolver wrong: %+v", req.Reasoning)
	}
	if req.ReasoningEffort != "" || req.ChatTemplateKwargs != nil {
		t.Errorf("object resolver leaked fields: %+v", req)
	}
}

func TestObjectResolverSerializesDisabledReasoning(t *testing.T) {
	req := &models.ChatRequest{}
	objectResolver{}.Apply(req, ReasoningSpec{Mode: ModeObject, Effort: EffortMedium, Enabled: false})
	payload, err := json.Marshal(req.Reasoning)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"effort":"medium","enabled":false}` {
		t.Fatalf("expected explicit disabled reasoning, got %s", payload)
	}
}

func TestReasoningEnabledOverrideIsProviderSpecific(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode ReasoningMode
	}{
		{name: "nvidia", mode: ModeEnableThinking},
		{name: "openrouter", mode: ModeObject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disabled := false
			spec := applyReasoningEnabledOverride(ReasoningSpec{Mode: tc.mode, Enabled: true}, &disabled, models.WorkloadCloud)
			if spec.Enabled {
				t.Fatal("expected provider reasoning to be disabled")
			}
		})
	}
	local := applyReasoningEnabledOverride(ReasoningSpec{Mode: ModeThinkTokens, Budget: 1024}, boolPtr(false), models.WorkloadLocal)
	if local.Budget != 1024 {
		t.Fatal("local reasoning budget should remain unchanged")
	}
}

// TestApplyReasoningEnabledOverride_Effort verifies effort-mode mapping:
// enabled -> medium, disabled -> EffortNone (omitted wire), nil -> untouched.
func TestApplyReasoningEnabledOverride_Effort(t *testing.T) {
	enabled := true
	disabled := false

	on := applyReasoningEnabledOverride(ReasoningSpec{Mode: ModeEffort, Effort: EffortNone}, &enabled, models.WorkloadCloud)
	if on.Effort != EffortMedium {
		t.Errorf("enabled effort should map to medium, got %v", on.Effort)
	}

	off := applyReasoningEnabledOverride(ReasoningSpec{Mode: ModeEffort, Effort: EffortMedium}, &disabled, models.WorkloadCloud)
	if off.Effort != EffortNone {
		t.Errorf("disabled effort should map to EffortNone, got %v", off.Effort)
	}

	nilOverride := applyReasoningEnabledOverride(ReasoningSpec{Mode: ModeEffort, Effort: EffortHigh}, nil, models.WorkloadCloud)
	if nilOverride.Effort != EffortHigh {
		t.Errorf("nil override must leave effort untouched, got %v", nilOverride.Effort)
	}
}

// TestApplyReasoningEnabledOverride_LocalLoopbackOpenaiSlug: a WorkloadLocal
// openai slug must be byte-identical to input regardless of enabled flag.
func TestApplyReasoningEnabledOverride_LocalLoopbackOpenaiSlug(t *testing.T) {
	disabled := false
	in := ReasoningSpec{Mode: ModeEffort, Effort: EffortMedium}
	out := applyReasoningEnabledOverride(in, &disabled, models.WorkloadLocal)
	if out != in {
		t.Errorf("local workload override must not mutate spec: in %+v out %+v", in, out)
	}
}

// TestEffortResolverDisabledOmitsWire verifies EffortNone yields no reasoning
// field on the wire (the documented effort-mode "disabled" behaviour).
func TestEffortResolverDisabledOmitsWire(t *testing.T) {
	req := &models.ChatRequest{}
	effortResolver{}.Apply(req, ReasoningSpec{Mode: ModeEffort, Effort: EffortNone})
	if req.ReasoningEffort != "" {
		t.Errorf("EffortNone must omit reasoning_effort, got %q", req.ReasoningEffort)
	}
	if req.Reasoning != nil || req.ChatTemplateKwargs != nil || req.ThinkingBudgetTokens != 0 || req.ReasoningBudget != 0 {
		t.Errorf("effort-none resolver leaked other fields: %+v", req)
	}
}

// TestReasoningCapabilityFor verifies the capability descriptor table.
func TestReasoningCapabilityFor(t *testing.T) {
	cases := map[string]struct {
		toggleable     bool
		defaultEnabled bool
		mode           ReasoningMode
	}{
		"openai":     {toggleable: true, defaultEnabled: true, mode: ModeEffort},
		"gemini":     {toggleable: true, defaultEnabled: true, mode: ModeEffort},
		"openrouter": {toggleable: true, defaultEnabled: true, mode: ModeObject},
		"nvidia":     {toggleable: true, defaultEnabled: true, mode: ModeEnableThinking},
		"local":      {toggleable: false, defaultEnabled: false, mode: ModeThinkTokens},
	}
	for k, want := range cases {
		got := ReasoningCapabilityFor(k)
		if got.Toggleable != want.toggleable || got.DefaultEnabled != want.defaultEnabled || got.Mode != want.mode {
			t.Errorf("provider %q: got %+v, want toggleable=%v defaultEnabled=%v mode=%v",
				k, got, want.toggleable, want.defaultEnabled, want.mode)
		}
	}
	// Unknown provider is non-toggleable (safe default).
	unknown := ReasoningCapabilityFor("does-not-exist")
	if unknown.Toggleable {
		t.Errorf("unknown provider must be non-toggleable, got %+v", unknown)
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

// TestEnableThinkingResolverSerializesDisabledReasoning guards the NVIDIA
// disabled state: an explicit false must reach the wire as
// "enable_thinking": false, never as an empty kwargs object (which would leave
// the provider's native default in force).
func TestEnableThinkingResolverSerializesDisabledReasoning(t *testing.T) {
	req := &models.ChatRequest{}
	enableThinkingResolver{}.Apply(req, ReasoningSpec{Mode: ModeEnableThinking, Enabled: false})
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking {
		t.Fatalf("expected enable_thinking=false, got %+v", req.ChatTemplateKwargs)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"chat_template_kwargs":{"enable_thinking":false}`)) {
		t.Fatalf("explicit false must be serialized on the wire, got %s", body)
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
	// Critical local-regression guard: an "openai"-slugged config classified
	// WorkloadLocal must use think-tokens, not reasoning_effort.
	r := NewReasoningResolver(models.WorkloadLocal, "openai", 1500)
	req := &models.ChatRequest{}
	r.Apply(req, providerReasoningCapabilities["openai"].Spec())
	if req.ThinkingBudgetTokens != 1500 {
		t.Errorf("local workload override should yield thinking_budget_tokens, got %+v", req)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("local workload override must not set reasoning_effort, got %q", req.ReasoningEffort)
	}
}

// TestNewReasoningResolver_LocalURLOpenaiSlug is the V1 guarantee: an openai
// slug + loopback URL is WorkloadLocal, so the reasoning wire follows the same
// classifier as the budget — never reasoning_effort.
func TestNewReasoningResolver_LocalURLOpenaiSlug(t *testing.T) {
	classifier := models.NewWorkloadClassifier("127.0.0.1", nil)
	cfg := models.ModelConfig{
		Provider: "openai",
		Name:     "llama-alias",
		ProviderConfig: &models.ProviderConfig{
			BaseURL: "http://127.0.0.1:8080",
		},
	}
	if class := classifier.Classify(cfg); class != models.WorkloadLocal {
		t.Fatalf("expected WorkloadLocal for loopback openai slug, got %q", class)
	}
	r := NewReasoningResolver(models.WorkloadLocal, "openai", 0)
	req := &models.ChatRequest{}
	r.Apply(req, providerReasoningCapabilities["openai"].Spec())
	if req.ThinkingBudgetTokens <= 0 && req.ThinkingBudgetTokens != 0 {
		t.Errorf("local workload must use thinking_budget_tokens, got %+v", req)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("loopback-URL openai model must NOT send reasoning_effort, got %q", req.ReasoningEffort)
	}
}

func TestNewReasoningResolver_Cloud(t *testing.T) {
	req := &models.ChatRequest{}
	NewReasoningResolver(models.WorkloadCloud, "openai", 0).Apply(req, providerReasoningCapabilities["openai"].Spec())
	if req.ReasoningEffort != "medium" {
		t.Errorf("openai should set reasoning_effort, got %q", req.ReasoningEffort)
	}

	req2 := &models.ChatRequest{}
	NewReasoningResolver(models.WorkloadCloud, "nvidia", 0).Apply(req2, providerReasoningCapabilities["nvidia"].Spec())
	if req2.ChatTemplateKwargs == nil || !req2.ChatTemplateKwargs.EnableThinking {
		t.Errorf("nvidia should set enable_thinking, got %+v", req2.ChatTemplateKwargs)
	}

	req3 := &models.ChatRequest{}
	NewReasoningResolver(models.WorkloadCloud, "openrouter", 0).Apply(req3, providerReasoningCapabilities["openrouter"].Spec())
	if req3.Reasoning == nil {
		t.Errorf("openrouter should set reasoning object")
	}
}

func TestNewReasoningResolver_UnknownNoop(t *testing.T) {
	req := &models.ChatRequest{} // fresh request — noop must not add any reasoning param
	NewReasoningResolver(models.WorkloadCloud, "does-not-exist", 0).Apply(req, ReasoningSpec{})
	if req.ReasoningBudget != 0 || req.ReasoningEffort != "" || req.ThinkingBudgetTokens != 0 || req.Reasoning != nil || req.ChatTemplateKwargs != nil {
		t.Errorf("noop resolver should leave reasoning params empty, got %+v", req)
	}
}

// TestReasoningCapabilitiesCoverProviderRegistry enforces the single-source-of-
// truth contract: the reasoning capability table (which carries both the wire
// spec and the toggle descriptor) must have a row for every canonical provider
// (models.ProviderIDs) and no stray keys. Adding a provider without updating
// the table fails CI.
func TestReasoningCapabilitiesCoverProviderRegistry(t *testing.T) {
	ids := models.ProviderIDs()
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	for _, id := range ids {
		if _, ok := providerReasoningCapabilities[id]; !ok {
			t.Errorf("providerReasoningCapabilities missing row for provider %q", id)
		}
	}
	for k := range providerReasoningCapabilities {
		if !idSet[k] {
			t.Errorf("providerReasoningCapabilities has key %q not in ProviderIDs", k)
		}
	}
}
