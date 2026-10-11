package handlers

import (
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

// TestBudgetOverridesToPersist verifies Phase 3: explicit cloud values that
// differ from the derived baseline persist; derived-matching or local values do
// not.
func TestBudgetOverridesToPersist(t *testing.T) {
	cases := []struct {
		name             string
		b                modelBudgetOverride
		wantMax, wantCtx int
	}{
		{
			name: "cloud explicit differs from baseline persists both",
			b: modelBudgetOverride{
				ExplicitMaxTokens: 4096, ExplicitCtxBudget: 10000,
				DerivedMaxTokens: 8192, DerivedCtxBudget: 30000,
				WorkloadClass: models.WorkloadCloud,
			},
			wantMax: 4096, wantCtx: 10000,
		},
		{
			name: "cloud explicit matches baseline not persisted",
			b: modelBudgetOverride{
				ExplicitMaxTokens: 8192, ExplicitCtxBudget: 30000,
				DerivedMaxTokens: 8192, DerivedCtxBudget: 30000,
				WorkloadClass: models.WorkloadCloud,
			},
			wantMax: 0, wantCtx: 0,
		},
		{
			name: "cloud explicit zero not persisted",
			b: modelBudgetOverride{
				ExplicitMaxTokens: 0, ExplicitCtxBudget: 0,
				DerivedMaxTokens: 8192, DerivedCtxBudget: 30000,
				WorkloadClass: models.WorkloadCloud,
			},
			wantMax: 0, wantCtx: 0,
		},
		{
			name: "local never persists even when explicit differs",
			b: modelBudgetOverride{
				ExplicitMaxTokens: 999, ExplicitCtxBudget: 999,
				DerivedMaxTokens: 2730, DerivedCtxBudget: 21848,
				WorkloadClass: models.WorkloadLocal,
			},
			wantMax: 0, wantCtx: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMax, gotCtx := c.b.budgetOverridesToPersist()
			if gotMax != c.wantMax || gotCtx != c.wantCtx {
				t.Fatalf("budgetOverridesToPersist = %d/%d, want %d/%d", gotMax, gotCtx, c.wantMax, c.wantCtx)
			}
		})
	}
}

// TestWriteModelOverridesClearsStaleEntry verifies that when a model's save no
// longer carries any override, a previously-persisted entry is REMOVED so
// "unset" actually restores the provider default (nullable semantics).
func TestWriteModelOverridesClearsStaleEntry(t *testing.T) {
	cfg := models.ModelConfig{Name: "m"}
	settings := &models.UserSettings{ModelOverrides: map[string]models.ModelOverride{"m": {MaxSteps: 5}}}
	calls := 0
	updateFn := func(fn func(*models.UserSettings)) error {
		calls++
		fn(settings)
		return nil
	}
	writeModelOverrides("m", cfg, modelBudgetOverride{WorkloadClass: models.WorkloadCloud}, true, updateFn)
	if calls != 1 {
		t.Fatalf("expected 1 update call to clear the stale entry, got %d", calls)
	}
	if _, ok := settings.ModelOverrides["m"]; ok {
		t.Fatalf("stale override must be removed when no override remains, got %+v", settings.ModelOverrides)
	}
}

// TestWriteModelOverridesSkipsWriteWhenNothingToChange verifies no settings
// write happens when the model has no overrides AND no stale entry exists.
func TestWriteModelOverridesSkipsWriteWhenNothingToChange(t *testing.T) {
	cfg := models.ModelConfig{Name: "m"}
	calls := 0
	updateFn := func(fn func(*models.UserSettings)) error {
		calls++
		fn(&models.UserSettings{})
		return nil
	}
	writeModelOverrides("m", cfg, modelBudgetOverride{WorkloadClass: models.WorkloadCloud}, false, updateFn)
	if calls != 0 {
		t.Fatalf("no stale entry and no override must skip the settings write, got %d calls", calls)
	}
}

// TestWriteModelOverridesPersistsExplicitFalseReasoning verifies an explicit
// false reasoning_enabled survives the settings.yml persistence path.
func TestWriteModelOverridesPersistsExplicitFalseReasoning(t *testing.T) {
	f := false
	cfg := models.ModelConfig{Name: "m", ReasoningEnabled: &f}
	var got models.UserSettings
	updateFn := func(fn func(*models.UserSettings)) error {
		fn(&got)
		return nil
	}
	writeModelOverrides("m", cfg, modelBudgetOverride{WorkloadClass: models.WorkloadCloud}, false, updateFn)
	entry, ok := got.ModelOverrides["m"]
	if !ok {
		t.Fatal("expected override entry persisted")
	}
	if entry.ReasoningEnabled == nil || *entry.ReasoningEnabled {
		t.Fatalf("expected ReasoningEnabled=false persisted, got %+v", entry.ReasoningEnabled)
	}
}

// TestConvertProviderTiers_TemperatureDefaults pins the provider temperature
// policy: cloud tiers carry 0 ("unset" — the provider's own default applies,
// the wire field is omitempty) while the local tier keeps the 0.1 automation
// default.
func TestConvertProviderTiers_TemperatureDefaults(t *testing.T) {
	out := convertProviderTiers(assistant.ProviderTiers())
	if len(out) == 0 {
		t.Fatal("expected provider tiers")
	}
	for provider, d := range out {
		want := 0.0
		if provider == models.ProviderLocal {
			want = assistant.DefaultAutomationTemperature
		}
		if d.Temperature != want {
			t.Errorf("%s: temperature = %v, want %v", provider, d.Temperature, want)
		}
	}
	if out[models.ProviderLocal].Temperature != 0.1 {
		t.Errorf("local temperature = %v, want 0.1", out[models.ProviderLocal].Temperature)
	}
}

// TestBaseAdminTuningDefaults_KeepsGlobalTemperature documents that the global
// agent_defaults fallback is unchanged (0.1); only provider tiers diverge.
func TestBaseAdminTuningDefaults_KeepsGlobalTemperature(t *testing.T) {
	if got := baseAdminTuningDefaults().Temperature; got != assistant.DefaultAutomationTemperature {
		t.Errorf("base temperature = %v, want %v", got, assistant.DefaultAutomationTemperature)
	}
}

// TestModelViewTuning_Temperature verifies the per-model view never
// fabricates a temperature: a stored 0 means "unset" (the request omits the
// field) for cloud and local alike, and an explicit value is always echoed.
func TestModelViewTuning_Temperature(t *testing.T) {
	cases := []struct {
		name string
		mc   models.ModelConfig
		want float64
	}{
		{"cloud unset stays 0", models.ModelConfig{WorkloadClass: models.WorkloadCloud}, 0},
		{"cloud explicit echoed", models.ModelConfig{WorkloadClass: models.WorkloadCloud, Temperature: 0.7}, 0.7},
		{"local unset stays 0", models.ModelConfig{WorkloadClass: models.WorkloadLocal}, 0},
		{"local explicit echoed", models.ModelConfig{WorkloadClass: models.WorkloadLocal, Temperature: 0.3}, 0.3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, got, _, _, _, _, _ := modelViewTuning(tc.mc)
			if got != tc.want {
				t.Errorf("temperature = %v, want %v", got, tc.want)
			}
		})
	}
}
