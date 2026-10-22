package assistant

import (
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// runOptionDepsStub is the smallest RunOptionDeps: one model config, settings and a memory store.
type runOptionDepsStub struct {
	cfg    models.ModelConfig
	store  *memory.Store
	memory *models.MemoryConfig
	agent  *models.AgentConfig
}

func (d *runOptionDepsStub) ModelConfig(name string) (models.ModelConfig, bool) {
	return d.cfg, d.cfg.Name != "" && d.cfg.Name == name
}
func (d *runOptionDepsStub) GuardrailEngine() *guardrails.GuardrailEngine { return nil }
func (d *runOptionDepsStub) GuardrailDecisionStore() *GuardrailDecisionStore {
	return NewGuardrailDecisionStore()
}
func (d *runOptionDepsStub) Orchestrator() *orchestrator.Orchestrator { return nil }
func (d *runOptionDepsStub) MemoryStore() *memory.Store               { return d.store }
func (d *runOptionDepsStub) MemorySettings() *models.MemoryConfig     { return d.memory }
func (d *runOptionDepsStub) AgentSettings() *models.AgentConfig       { return d.agent }

// One precedence chain for every kind of run: the run's own choice, then the profile, then the model config, then
// the global default.
func TestResolveRunOptions(t *testing.T) {
	store := newTestMemoryStore(t)
	cfg := models.ModelConfig{Name: "m", LoopStrategy: models.LoopStrategyPlanExecute, MaxSteps: 9}
	off, on := new(false), new(true)
	cases := []struct {
		name    string
		deps    *runOptionDepsStub
		spec    RunSpec
		hot     bool
		store   bool
		loop    LoopStrategyName
		channel EventChannel
	}{
		{name: "chat: the global assistant default turns hot memory on",
			deps: &runOptionDepsStub{store: store, cfg: cfg}, spec: RunSpec{Kind: profile.KindAssistant, Model: "m"},
			hot: true, store: true, loop: LoopPlanExecute, channel: ChannelAssistant},
		{name: "chat keeps the store for memory_search when hot memory is off",
			deps: &runOptionDepsStub{store: store, memory: &models.MemoryConfig{AssistantHot: off}},
			spec: RunSpec{Kind: profile.KindAssistant}, hot: false, store: true, channel: ChannelAssistant},
		{name: "automation: no memory unless something turns it on",
			deps: &runOptionDepsStub{store: store}, spec: RunSpec{Kind: profile.KindAutomation},
			hot: false, store: false, channel: ChannelAutomation},
		{name: "automation: the profile beats the global default",
			deps: &runOptionDepsStub{store: store}, spec: RunSpec{Kind: profile.KindAutomation, Profile: profile.Profile{Memory: models.MemoryModeOn}},
			hot: true, store: true, channel: ChannelAutomation},
		{name: "the run's own memory choice beats the profile",
			deps: &runOptionDepsStub{store: store, memory: &models.MemoryConfig{AutomationHot: on}},
			spec: RunSpec{Kind: profile.KindAutomation, Profile: profile.Profile{Memory: models.MemoryModeOn}, Overrides: RunOverrides{Memory: models.MemoryModeOff}},
			hot:  false, store: false, channel: ChannelAutomation},
		{name: "no memory store means no hot memory",
			deps: &runOptionDepsStub{}, spec: RunSpec{Kind: profile.KindAssistant}, hot: false, store: false, channel: ChannelAssistant},
		{name: "the run's loop beats the model's",
			deps: &runOptionDepsStub{cfg: cfg}, spec: RunSpec{Kind: profile.KindAutomation, Model: "m", Overrides: RunOverrides{LoopStrategy: LoopReact}},
			loop: LoopReact, channel: ChannelAutomation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.Logger = logging.NewNopLogger()
			got := ResolveRunOptions(tc.deps, tc.spec)
			if got.EnableHotMemory != tc.hot || (got.MemoryStore != nil) != tc.store {
				t.Errorf("hot memory = %v, store set = %v; want %v, %v", got.EnableHotMemory, got.MemoryStore != nil, tc.hot, tc.store)
			}
			if got.LoopStrategy != tc.loop || got.Channel != tc.channel {
				t.Errorf("loop = %q, channel = %q; want %q, %q", got.LoopStrategy, got.Channel, tc.loop, tc.channel)
			}
			if got.GuardrailDecisionHandler == nil || got.MaxSteps == 0 {
				t.Errorf("every run gets a decision handler and a step budget: %+v", got)
			}
		})
	}
}
