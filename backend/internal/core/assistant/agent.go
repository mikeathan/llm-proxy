package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"strings"
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client     proxy.Client
	provider   ToolProvider
	engine     Engine
	guardrails *GuardrailEngine // Renamed from safety
	logger     logging.Logger
	maxSteps   int
}

type AgentOptions struct {
	MaxSteps   int
	Logger     logging.Logger
	Guardrails *GuardrailEngine
}

// NewAgent creates a new unified agent.
func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 10
	}
	if opts.Logger == nil {
		opts.Logger = logging.NewNopLogger()
	}
	// Default guardrail engine if none provided
	guardrails := opts.Guardrails
	if guardrails == nil {
		guardrails = NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} })
	}

	return &Agent{
		client:     client,
		provider:   provider,
		engine:     engine,
		guardrails: guardrails,
		logger:     opts.Logger,
		maxSteps:   opts.MaxSteps,
	}
}

// Execute runs the agentic loop for a given conversation history.
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	steps := 0
	currentHistory := append([]proxy.Message{}, history...)

	for steps < a.maxSteps {
		steps++
		a.logger.Debug("agent loop step", "step", steps)

		// 1. Get LLM Tools
		tools, err := a.provider.ListTools(ctx)
		if err != nil {
			return "", nil, fmt.Errorf("failed to list tools: %w", err)
		}

		// 2. Predictive Step: Next message/tool call
		resp, err := a.client.Chat(ctx, proxy.ChatRequest{
			Messages: currentHistory,
			Tools:    tools,
		})
		if err != nil {
			// If the model or API doesn't support tools, retry once without them
			if isToolSupportError(err) {
				a.logger.Warn("model does not support tools, retrying without them", "error", err)
				resp, err = a.client.Chat(ctx, proxy.ChatRequest{
					Messages: currentHistory,
				})
			}

			if err != nil {
				return "", nil, fmt.Errorf("llm completion failed: %w", err)
			}
		}

		msg := resp.Choices[0].Message
		currentHistory = append(currentHistory, msg)

		// 3. Termination Check
		if len(msg.ToolCalls) == 0 {
			return msg.Content, currentHistory, nil
		}

		// 4. Execution Step: Process Tool Calls
		if err := a.processToolCalls(ctx, msg, &currentHistory); err != nil {
			return "", nil, err
		}
	}

	return "", nil, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message) error {
	for _, tc := range msg.ToolCalls {
		a.logger.Debug("executing tool", "name", tc.Function.Name)

		// Guardrail check
		if err := a.guardrails.ValidateToolCall(ctx, tc); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.appendToolResult(history, tc, map[string]string{"error": "Guardrail violation: " + err.Error()})
			continue
		}

		result, err := a.engine.ExecuteTool(ctx, tc)
		if err != nil {
			a.logger.Warn("tool error", "name", tc.Function.Name, "error", err)
			a.appendToolResult(history, tc, map[string]string{"error": err.Error()})
			continue
		}

		a.appendToolResult(history, tc, result)
	}
	return nil
}

// appendToolResult helper
func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) {
	raw, _ := json.Marshal(result)
	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    string(raw),
		ToolCallID: tc.ID,
	})
}

func isToolSupportError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "tools is not currently supported") ||
		strings.Contains(lowErr, "tool_choice is not supported") ||
		strings.Contains(lowErr, "auto tool choice requires") ||
		strings.Contains(lowErr, "parameter `tools`")
}
