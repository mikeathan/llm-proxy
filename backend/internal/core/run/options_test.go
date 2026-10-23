package run

import (
	"context"
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/testing/assistanttest"
	"llm-proxy/models"
)

// optionDepsStub is the smallest OptionDeps: one model config, settings and a memory store.
type optionDepsStub struct {
	cfg    models.ModelConfig
	store  *memory.Store
	memory *models.MemoryConfig
	agent  *models.AgentConfig
	runner assistant.SubRunner
}

func (d *optionDepsStub) ModelConfig(name string) (models.ModelConfig, bool) {
	return d.cfg, d.cfg.Name != "" && d.cfg.Name == name
}
func (d *optionDepsStub) GuardrailEngine() *guardrails.GuardrailEngine { return nil }
func (d *optionDepsStub) GuardrailDecisionStore() *assistant.GuardrailDecisionStore {
	return assistant.NewGuardrailDecisionStore()
}
func (d *optionDepsStub) Orchestrator() *orchestrator.Orchestrator { return nil }
func (d *optionDepsStub) MemoryStore() *memory.Store               { return d.store }
func (d *optionDepsStub) MemorySettings() *models.MemoryConfig     { return d.memory }
func (d *optionDepsStub) AgentSettings() *models.AgentConfig       { return d.agent }
func (d *optionDepsStub) SubRunner() assistant.SubRunner           { return d.runner }

// One precedence chain for every kind of run: the run's own choice, then the profile, then the model config, then
// the global default.
func TestResolveRunOptions(t *testing.T) {
	store := newMemoryStore(t)
	cfg := models.ModelConfig{Name: "m", LoopStrategy: models.LoopStrategyPlanExecute, MaxSteps: 9}
	off, on := new(false), new(true)
	cases := []struct {
		name    string
		deps    *optionDepsStub
		spec    Spec
		hot     bool
		store   bool
		loop    assistant.LoopStrategyName
		channel assistant.EventChannel
	}{
		{name: "chat: the global assistant default turns hot memory on",
			deps: &optionDepsStub{store: store, cfg: cfg}, spec: Spec{Kind: profile.KindAssistant, Model: "m"},
			hot: true, store: true, loop: assistant.LoopPlanExecute, channel: assistant.ChannelAssistant},
		{name: "chat keeps the store for memory_search when hot memory is off",
			deps: &optionDepsStub{store: store, memory: &models.MemoryConfig{AssistantHot: off}},
			spec: Spec{Kind: profile.KindAssistant}, hot: false, store: true, channel: assistant.ChannelAssistant},
		{name: "automation: no memory unless something turns it on",
			deps: &optionDepsStub{store: store}, spec: Spec{Kind: profile.KindAutomation},
			hot: false, store: false, channel: assistant.ChannelAutomation},
		{name: "automation: the profile beats the global default",
			deps: &optionDepsStub{store: store}, spec: Spec{Kind: profile.KindAutomation, Profile: profile.Profile{Memory: models.MemoryModeOn}},
			hot: true, store: true, channel: assistant.ChannelAutomation},
		{name: "the run's own memory choice beats the profile",
			deps: &optionDepsStub{store: store, memory: &models.MemoryConfig{AutomationHot: on}},
			spec: Spec{Kind: profile.KindAutomation, Profile: profile.Profile{Memory: models.MemoryModeOn}, Overrides: Overrides{Memory: models.MemoryModeOff}},
			hot:  false, store: false, channel: assistant.ChannelAutomation},
		{name: "no memory store means no hot memory",
			deps: &optionDepsStub{}, spec: Spec{Kind: profile.KindAssistant}, hot: false, store: false, channel: assistant.ChannelAssistant},
		{name: "the run's loop beats the model's",
			deps: &optionDepsStub{cfg: cfg}, spec: Spec{Kind: profile.KindAutomation, Model: "m", Overrides: Overrides{LoopStrategy: assistant.LoopReact}},
			loop: assistant.LoopReact, channel: assistant.ChannelAutomation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.Logger = logging.NewNopLogger()
			got := ResolveOptions(tc.deps, tc.spec)
			if got.EnableHotMemory != tc.hot || (got.MemoryStore != nil) != tc.store {
				t.Errorf("hot memory = %v, store set = %v; want %v, %v", got.EnableHotMemory, got.MemoryStore != nil, tc.hot, tc.store)
			}
			if got.LoopStrategy != tc.loop || got.Channel != tc.channel {
				t.Errorf("loop = %q, channel = %q; want %q, %q", got.LoopStrategy, got.Channel, tc.loop, tc.channel)
			}
			if got.GuardrailDecisionHandler == nil || got.MaxSteps == 0 {
				t.Errorf("every chat and automation gets a decision handler and a step budget: %+v", got)
			}
		})
	}
}

// A sub-agent asks no one for approval (a blocked call fails at once, Constitution II.10), has no memory, cannot
// delegate in turn, and runs unattended.
func TestResolveRunOptions_SubagentRules(t *testing.T) {
	deps := &optionDepsStub{store: newMemoryStore(t), memory: &models.MemoryConfig{AssistantHot: new(true), AutomationHot: new(true)},
		runner: onSubRunner{}}
	got := ResolveOptions(deps, Spec{Kind: profile.KindSubagent, Logger: logging.NewNopLogger(),
		Profile: profile.Profile{ID: "web-researcher", Memory: models.MemoryModeOn, Delegates: []string{"other"}}})
	if got.GuardrailDecisionHandler != nil || got.MemoryStore != nil || got.EnableHotMemory || got.SubRunner != nil {
		t.Errorf("sub-agent options: handler %v, store %v, hot %v, runner %v; want none of them",
			got.GuardrailDecisionHandler != nil, got.MemoryStore != nil, got.EnableHotMemory, got.SubRunner != nil)
	}
	if lead := ResolveOptions(deps, Spec{Kind: profile.KindAssistant, Logger: logging.NewNopLogger()}); lead.SubRunner == nil {
		t.Error("a chat gets the app's sub-runner")
	}
}

// The workspace's adaptive result cap setting is what a run's options carry.
func TestResolveOptions_AdaptiveResultCap(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		deps := &optionDepsStub{agent: &models.AgentConfig{AdaptiveResultCap: new(enabled)}}
		if got := ResolveOptions(deps, Spec{Logger: logging.NewNopLogger()}); got.AdaptiveResultCap != enabled {
			t.Errorf("setting %v -> options %v", enabled, got.AdaptiveResultCap)
		}
	}
}

// onSubRunner is a delegation package whose helpers are on.
type onSubRunner struct{}

func (onSubRunner) Enabled() bool                    { return true }
func (onSubRunner) Problem() assistant.HelperProblem { return assistant.HelperNone }
func (onSubRunner) Run(context.Context, assistant.SubRunRequest) assistant.SubRunResult {
	return assistant.SubRunResult{}
}

func newMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	store, _ := assistanttest.NewMemoryStoreAndDB(t)
	return store
}
