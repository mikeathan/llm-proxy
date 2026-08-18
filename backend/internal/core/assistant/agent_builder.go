package assistant

import (
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// ServiceProvider is the common interface for constructing an Agent.
// Both the assistant handler and automation executor reference it.
type ServiceProvider interface {
	ModelConfig(modelName string) (models.ModelConfig, bool)
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *GuardrailDecisionStore
	Orchestrator() *orchestrator.Orchestrator
	MemoryStore() *memory.Store
	Events() EventPublisher
}

// AgentBuilder constructs an Agent from shared options.
type AgentBuilder struct {
	svc  ServiceProvider
	opts AgentOptions
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

// WithExcludedTools constrains the exposed tool schema for this agent (e.g.
// channel-scoped exclusions). Combined with guardrail policy in NewAgent.
func (b *AgentBuilder) WithExcludedTools(tools []string) *AgentBuilder {
	b.opts.ExcludedTools = tools
	return b
}

// WithAllowedTools restricts the exposed tool schema to the given allowlist for
// this agent (e.g. unattended automation tool sets). Combined with guardrail
// policy in NewAgent.
func (b *AgentBuilder) WithAllowedTools(tools []string) *AgentBuilder {
	b.opts.AllowedTools = tools
	return b
}

func (b *AgentBuilder) WithModelName(name string) *AgentBuilder {
	b.opts.ModelName = name
	return b
}

// WithChannel sets the event stream this agent publishes to. Defaults to
// ChannelAssistant; automation passes ChannelAutomation.
func (b *AgentBuilder) WithChannel(ch EventChannel) *AgentBuilder {
	b.opts.Channel = ch
	return b
}

// WithConversationID scopes this agent's events to a specific chat session.
func (b *AgentBuilder) WithConversationID(id string) *AgentBuilder {
	b.opts.ConversationID = id
	return b
}

func (b *AgentBuilder) WithObserver(obs Observer) *AgentBuilder {
	b.opts.Observer = obs
	return b
}

func (b *AgentBuilder) WithGuardrailDecisionHandler(h GuardrailDecisionCallback) *AgentBuilder {
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

func (b *AgentBuilder) WithModelConfig(modelName string) *AgentBuilder {
	cfg, ok := b.svc.ModelConfig(modelName)
	if !ok {
		return b
	}
	b.opts.ApplyModelConfig(cfg)
	return b
}

func (b *AgentBuilder) Build(client proxy.Client, provider ToolProvider, engine Engine) *Agent {
	return NewAgent(client, provider, engine, b.opts)
}
