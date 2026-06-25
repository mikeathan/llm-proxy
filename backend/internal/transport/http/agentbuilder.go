package api

import (
	"context"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// ServiceProvider is the common interface for constructing an Agent.
// Both the assistant handler and automation executor implement it.
type ServiceProvider interface {
	ModelConfig(modelName string) (models.ModelConfig, bool)
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *assistant.GuardrailDecisionStore
	Orchestrator() *orchestrator.Orchestrator
	MemoryStore() *memory.Store
	Events() *automation.EventBus
}

// AgentBuilder constructs an assistant.Agent from shared options.
// The handler layer calls Build then sets context-specific fields
// (EnableHotMemory, history) before creating the Agent.
type AgentBuilder struct {
	svc  ServiceProvider
	opts assistant.AgentOptions
}

func NewAgentBuilder(svc ServiceProvider) *AgentBuilder {
	return &AgentBuilder{svc: svc}
}

func (b *AgentBuilder) WithLogger(log logging.Logger) *AgentBuilder {
	b.opts.Logger = log
	return b
}

func (b *AgentBuilder) WithGuardrails() *AgentBuilder {
	b.opts.Guardrails = b.svc.GuardrailEngine()
	return b
}

func (b *AgentBuilder) WithWorkspaceID(ws string) *AgentBuilder {
	b.opts.WorkspaceID = ws
	return b
}

func (b *AgentBuilder) WithModelName(name string) *AgentBuilder {
	b.opts.ModelName = name
	return b
}

func (b *AgentBuilder) WithObserver(obs assistant.Observer) *AgentBuilder {
	b.opts.Observer = obs
	return b
}

func (b *AgentBuilder) WithGuardrailDecisionHandler(h assistant.GuardrailDecisionCallback) *AgentBuilder {
	b.opts.GuardrailDecisionHandler = h
	return b
}

func (b *AgentBuilder) WithMemoryStore() *AgentBuilder {
	b.opts.MemoryStore = b.svc.MemoryStore()
	return b
}

func (b *AgentBuilder) WithOrchestrator() *AgentBuilder {
	b.opts.Orchestrator = b.svc.Orchestrator()
	return b
}

func (b *AgentBuilder) WithHotMemory(enabled bool) *AgentBuilder {
	b.opts.EnableHotMemory = enabled
	return b
}

// WithModelConfig loads the model config and applies overrides.
// If EnableExecutionPlan is true and tools are available, PlanStrategy is set.
// The client parameter is needed for PlanStrategy construction.
func (b *AgentBuilder) WithModelConfig(ctx context.Context, modelName string, tools assistant.ToolProvider, client proxy.Client) *AgentBuilder {
	cfg, ok := b.svc.ModelConfig(modelName)
	if !ok {
		return b
	}
	if b.opts.ApplyModelConfig(cfg) {
		toolList, err := tools.ListTools(ctx)
		if err == nil && len(toolList) > 0 {
			b.opts.PlanStrategy = assistant.NewExecutionPlanStrategy(client, toolList, b.opts.Logger)
		}
	}
	return b
}

func (b *AgentBuilder) Build(client proxy.Client, provider assistant.ToolProvider, engine assistant.Engine) *assistant.Agent {
	return assistant.NewAgent(client, provider, engine, b.opts)
}
