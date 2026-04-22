package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"regexp"
	"strings"
	"time"
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client      proxy.Client
	provider    ToolProvider
	engine      Engine
	guardrails  *GuardrailEngine
	logger      logging.Logger
	maxSteps    int
	observer    Observer
	workspaceID string
}

type AgentEventType string

const (
	EventStepStart          AgentEventType = "step_start"
	EventMessage            AgentEventType = "message"
	EventToolCall           AgentEventType = "tool_call"
	EventToolResult         AgentEventType = "tool_result"
	EventGuardrailViolation AgentEventType = "guardrail_violation"
	EventError              AgentEventType = "error"
	EventToolStream         AgentEventType = "tool_stream"
)

type AgentEvent struct {
	Type      AgentEventType `json:"type"`
	Payload   any            `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

type Observer func(AgentEvent)

type AgentOptions struct {
	MaxSteps    int
	Logger      logging.Logger
	Guardrails  *GuardrailEngine
	Observer    Observer
	WorkspaceID string
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
		guardrails = NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
	}

	return &Agent{
		client:      client,
		provider:    provider,
		engine:      engine,
		guardrails:  guardrails,
		logger:      opts.Logger,
		maxSteps:    opts.MaxSteps,
		observer:    opts.Observer,
		workspaceID: opts.WorkspaceID,
	}
}

// Execute runs the agentic loop for a given conversation history.
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	steps := 0
	currentHistory := append([]proxy.Message{}, history...)

	for steps < a.maxSteps {
		steps++
		a.logger.Debug("agent loop step", "step", steps)
		a.notifyStepStart(steps)
		a.notifyThinking()

		// 1. Get LLM Tools
		tools, err := a.provider.ListTools(ctx)
		if err != nil {
			return "", nil, fmt.Errorf("failed to list tools: %w", err)
		}

		// 2. Get Next Response (handles fallbacks/tool support internally)
		msg, err := a.computeNextResponse(ctx, currentHistory, tools)
		if err != nil {
			return "", nil, err
		}

		// Handle embedded tool calls in content (common with local models like Qwen)
		a.handleContentToolCalls(&msg)

		// Content Normalization (Handle common model glitches)
		msg.Content = normalizeContent(msg.Content)

		currentHistory = append(currentHistory, msg)
		a.notify(EventMessage, msg)

		// 3. Termination Check
		if len(msg.ToolCalls) == 0 {
			if a.isPrematureTermination(msg, currentHistory) {
				if a.countRetries(currentHistory) >= 2 {
					return msg.Content, currentHistory, fmt.Errorf("model repeatedly returned incomplete responses")
				}

				a.logger.Warn("model attempted to terminate prematurely, forcing retry", "content", msg.Content)
				a.notifyPrematureTerminationNag(&currentHistory)
				continue
			}
			return msg.Content, currentHistory, nil
		}

		// 4. Execution Step: Process Tool Calls
		if err := a.processToolCalls(ctx, msg, &currentHistory); err != nil {
			return "", nil, err
		}
	}

	return "", nil, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	// Attempt streaming first to provide immediate feedback
	ch, err := a.client.Stream(ctx, proxy.ChatRequest{
		Messages: history,
		Tools:    tools,
	})

	if err != nil {
		a.logger.Warn("streaming not supported or failed, falling back to non-streaming", "error", err)
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole

	a.processStream(ch, &fullMsg)

	return fullMsg, nil
}

func (a *Agent) processStream(ch <-chan *proxy.ChatResponse, fullMsg *proxy.Message) {
	for resp := range ch {
		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			
			// Accumulate Content
			if choice.Delta.Content != "" {
				fullMsg.Content += choice.Delta.Content
				
				// UI Smoothing: If we detect the start of a tool call signature, 
				// stop streaming to the text-content bus to avoid showing raw technical markup.
				displayContent := fullMsg.Content
				hasToolCall := false
				cutoffPatterns := []string{
					"<function-name>", "<tools>", "functions.", 
					"```json", "```", 
					"{\"name\":", "[{\"name\":", 
					"{\"target\":", "{\"mode\":", "{\"command\":",
					"[{'type':", "{\"type\":",
				}
				for _, p := range cutoffPatterns {
					if idx := strings.Index(displayContent, p); idx != -1 {
						displayContent = displayContent[:idx]
						hasToolCall = true
						break
					}
				}
				
				// If we've reached a tool call, provide a small visual hint in the stream
				// so the user knows the agent is still active but is now processing technical steps.
				if hasToolCall {
					a.notify(EventToolStream, displayContent + "\n\n🛠️ *Agent is initiating tool calls...*")
				} else {
					a.notify(EventToolStream, displayContent)
				}
			}

			// Accumulate Tool Calls
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					// Stream-based tool calls often come in chunks
					if tc.ID != "" {
						fullMsg.ToolCalls = append(fullMsg.ToolCalls, tc)
					} else if len(fullMsg.ToolCalls) > 0 {
						// Append arguments to the last tool call
						last := &fullMsg.ToolCalls[len(fullMsg.ToolCalls)-1]
						last.Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}
	}
}

func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	resp, err := a.client.Chat(chatCtx, proxy.ChatRequest{
		Messages: history,
		Tools:    tools,
	})

	if err != nil && isToolSupportError(err) {
		a.logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)

		// Try again without tools
		chatCtx2, cancel2 := context.WithTimeout(ctx, 180*time.Second)
		defer cancel2()
		resp, err = a.client.Chat(chatCtx2, proxy.ChatRequest{
			Messages: history,
		})
	}

	if err != nil {
		return proxy.Message{}, fmt.Errorf("llm completion failed: %w", err)
	}

	return resp.Choices[0].Message, nil
}

func (a *Agent) handleContentToolCalls(msg *proxy.Message) {
	if msg.Content != "" {
		if cleanedContent, calls, ok := proxy.ParseContentToolCalls(msg.Content); ok {
			msg.ToolCalls = append(msg.ToolCalls, calls...)
			msg.Content = cleanedContent
			a.logger.Debug("detected embedded tool calls in content", "count", len(calls))
		}
	}
}

func (a *Agent) isPrematureTermination(msg proxy.Message, history []proxy.Message) bool {
	if msg.Content != "" && (len(msg.Content) >= 60 || strings.Contains(msg.Content, "```") || strings.Contains(msg.Content, "#")) {
		return false
	}
	return true
}

func (a *Agent) countRetries(history []proxy.Message) int {
	count := 0
	for _, h := range history {
		if h.Role == "user" && strings.Contains(h.Content, "You returned an incomplete response") {
			count++
		}
	}
	return count
}

func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message) error {
	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.logger.Info("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)
		a.notifyToolCall(tc)

		if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.notifyGuardrailViolation(tc.Function.Name, err)
			a.appendToolResult(history, tc, formatGuardrailError(err))
			continue
		}

		// Inject workspace ID into context so tools can resolve contextual guardrails
		toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
		result, err := a.engine.ExecuteTool(toolCtx, tc)
		if err != nil {
			a.logger.Warn("tool execution failed", "name", tc.Function.Name, "error", err)
			result = formatToolError(err)
		}

		a.appendToolResult(history, tc, result)
		a.notifyToolResult(tc.ID, tc.Function.Name, result)
	}
	return nil
}

func formatToolError(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
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

func (a *Agent) notify(t AgentEventType, payload any) {
	if a.observer != nil {
		a.observer(AgentEvent{
			Type:      t,
			Payload:   payload,
			Timestamp: time.Now(),
		})
	}
}

// Named Notification Wrappers

func (a *Agent) notifyThinking() {
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "🤖 Agent is thinking...",
	})
}

func (a *Agent) notifyStepStart(step int) {
	a.notify(EventStepStart, map[string]int{"step": step})
}

func (a *Agent) notifyFallbackWarning(err error) {
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "⚠️ WARNING: The selected model does not support tool calling. Fallback mode engaged (tools disabled). " + err.Error(),
	})
}

func (a *Agent) notifyPrematureTerminationNag(history *[]proxy.Message) {
	nagMsg := proxy.Message{
		Role:    "user",
		Content: "You returned an incomplete response. You MUST continue using tools or reply with the final comprehensive Markdown report as requested.",
	}
	*history = append(*history, nagMsg)
	a.notify(EventMessage, nagMsg)
}

func (a *Agent) notifyToolCall(tc proxy.ToolCall) {
	a.notify(EventToolCall, tc)
}

func (a *Agent) notifyToolResult(id, name string, result any) {
	a.notify(EventToolResult, map[string]any{"id": id, "name": name, "result": result})
}

func (a *Agent) notifyGuardrailViolation(tool string, err error) {
	a.notify(EventGuardrailViolation, map[string]string{
		"tool":  tool,
		"error": err.Error(),
	})
}

func normalizeContent(content string) string {
	content = strings.TrimSpace(content)

	// Detect and extract text from common "structured noise" blocks
	// e.g. [{'type': 'text', 'text': 'Hello'}] -> Hello
	// We handle both complete and incomplete/truncated blocks
	extractionPatterns := []struct {
		pattern     string
		replacement string
	}{
		{`\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"](.*?)['"]\s*\}?\s*\]?`, "$1"},
		{`\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"]([^'"]*)`, "$1"}, 
	}

	for _, p := range extractionPatterns {
		re := regexp.MustCompile("(?s)" + p.pattern) // Use (?s) to allow matching across newlines in $1
		content = re.ReplaceAllString(content, p.replacement)
	}

	return strings.TrimSpace(content)
}
