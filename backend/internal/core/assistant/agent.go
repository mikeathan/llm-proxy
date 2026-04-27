package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"strings"
	"sync"
	"time"
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client      proxy.Client
	provider    ToolProvider
	engine      Engine
	guardrails  *guardrails.GuardrailEngine
	logger      logging.Logger
	maxSteps    int
	observer    Observer
	workspaceID string
}

type AgentOptions struct {
	MaxSteps    int
	Logger      logging.Logger
	Guardrails  *guardrails.GuardrailEngine
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
	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
	}

	return &Agent{
		client:      client,
		provider:    provider,
		engine:      engine,
		guardrails:  gr,
		logger:      opts.Logger,
		maxSteps:    opts.MaxSteps,
		observer:    opts.Observer,
		workspaceID: opts.WorkspaceID,
	}
}

// Execute runs the agentic loop for a given conversation history.
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	steps := 0
	currentHistory := append([]proxy.Message{}, history...)

	type toolKey struct{ name, args string }
	recentCalls := make([]toolKey, 0, 5)

	for steps < a.maxSteps {
		steps++
		if err := execCtx.Err(); err != nil {
			return "", currentHistory, fmt.Errorf("agent execution halted: %w", err)
		}

		a.notifyStepStart(steps)
		a.notifyThinking()

		var turnMsg proxy.Message
		err := func() error {
			turnCtx, turnCancel := context.WithTimeout(execCtx, 5*time.Minute)
			defer turnCancel()

			toolsList, err := a.provider.ListTools(turnCtx)
			if err != nil {
				return fmt.Errorf("failed to list tools: %w", err)
			}

			msg, err := a.computeNextResponse(turnCtx, currentHistory, toolsList)
			if err != nil {
				return err
			}

			a.handleContentToolCalls(&msg)
			msg.Content = normalizeContent(msg.Content)
			turnMsg = msg

			// Process tool calls within the turn context if they exist
			if len(turnMsg.ToolCalls) > 0 {
				// Loop detection
				for _, tc := range turnMsg.ToolCalls {
					key := toolKey{tc.Function.Name, tc.Function.Arguments}
					count := 0
					for _, prev := range recentCalls {
						if prev == key {
							count++
						}
					}
					if count >= 3 {
						return fmt.Errorf("infinite loop detected: agent repeated tool call '%s'. User Intervention Required", key.name)
					}
					if len(recentCalls) >= 5 {
						recentCalls = recentCalls[1:]
					}
					recentCalls = append(recentCalls, key)
				}

				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				return a.processToolCalls(turnCtx, turnMsg, &currentHistory)
			}
			return nil
		}()

		if err != nil {
			return "", currentHistory, err
		}

		// If no tool calls were generated in the turn, handle termination
		if len(turnMsg.ToolCalls) == 0 {
			if a.isPrematureTermination(turnMsg, currentHistory) {
				retries := a.countRetries(currentHistory)
				if retries >= 2 {
					return turnMsg.Content, currentHistory, fmt.Errorf("model repeatedly returned incomplete responses")
				}

				// If it's the first retry, do it silently to avoid UI noise
				if retries == 0 {
					a.logger.Info("model returned empty response, performing silent retry", "step", steps)
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: "You returned an empty response. Please continue the task or provide your final report.",
					})
					continue
				}

				// For subsequent retries, notify the user
				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				a.notifyPrematureTerminationNag(&currentHistory)
				continue
			}

			currentHistory = append(currentHistory, turnMsg)
			a.notify(EventMessage, turnMsg)
			return turnMsg.Content, currentHistory, nil
		}
	}

	return "", currentHistory, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	llmTools := tools
	if !a.provider.UseNativeTools() {
		llmTools = nil
	}

	ch, err := a.client.Stream(ctx, proxy.ChatRequest{
		Messages: history,
		Tools:    llmTools,
	})

	if err != nil {
		a.logger.Warn("streaming not supported or failed, falling back to non-streaming", "error", err)
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	a.processStream(ch, &fullMsg)

	// If streaming failed with a 500 error related to tool parsing, fallback to non-streaming retry.
	// (Note: processStream consumes the channel, so we rely on computeNextResponseNonStreaming for the retry)
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
				displayContent, hasToolCall := FilterStreamingMarkup(fullMsg.Content)

				// If we've reached a tool call, provide a small visual hint in the stream
				// so the user knows the agent is still active but is now processing technical steps.
				if hasToolCall {
					a.notify(EventToolStream, displayContent+"\n\n🛠️ *Agent is initiating tool calls...*")
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

	llmTools := tools
	if !a.provider.UseNativeTools() {
		llmTools = nil
	}

	resp, err := a.client.Chat(chatCtx, proxy.ChatRequest{
		Messages: history,
		Tools:    llmTools,
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
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		// If there are tool calls, it's definitely NOT a termination.
		if len(msg.ToolCalls) > 0 {
			return false
		}
		return true
	}

	// Basic repetition detection for short content-only responses
	// if the same message is returned 3 times in a row, it's premature.
	if len(history) >= 2 {
		last := history[len(history)-1]
		if last.Role == proxy.AssistantRole && strings.TrimSpace(last.Content) == content {
			// Found one repeat, check if it's a pattern
			prev := history[len(history)-2]
			if prev.Role == proxy.AssistantRole && strings.TrimSpace(prev.Content) == content {
				return true
			}
		}
	}

	return false
}

func (a *Agent) countRetries(history []proxy.Message) int {
	count := 0
	for _, h := range history {
		if h.Role == "user" && (strings.Contains(h.Content, "incomplete response") || strings.Contains(h.Content, "empty response")) {
			count++
		}
	}
	return count
}

func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message) error {
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Limit concurrent tool executions to prevent resource exhaustion.
	// As per Antigravity Constitution II.1: "implement a Semaphore (currently capped at 10)"
	const maxConcurrentTools = 10
	sem := make(chan struct{}, maxConcurrentTools)

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}

		wg.Add(1)
		go func(tc proxy.ToolCall) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			a.logger.Info("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)
			a.notifyToolCall(tc)

			if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
				a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
				a.notifyGuardrailViolation(tc.Function.Name, err)

				mu.Lock()
				a.appendToolResult(history, tc, formatGuardrailError(err))
				mu.Unlock()
				return
			}

			// Inject workspace ID into context so tools can resolve contextual guardrails
			toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
			result, err := a.engine.ExecuteTool(toolCtx, tc)
			if err != nil {
				a.logger.Warn("tool execution failed", "name", tc.Function.Name, "error", err)
				result = formatToolError(err)
			}

			mu.Lock()
			a.appendToolResult(history, tc, result)
			a.notifyToolResult(tc.ID, tc.Function.Name, result)
			mu.Unlock()
		}(tc)
	}

	wg.Wait()
	return nil
}

func formatToolError(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
}

// appendToolResult helper with memory-sensitive pruning
func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) {
	raw, _ := json.Marshal(result)
	content := string(raw)

	// If tool result is very large (e.g. > 16KB), truncate it to save memory and context tokens.
	// Agents often don't need the full output of e.g. a huge directory listing or file read
	// if they've already seen it or if it's too much to process at once.
	const maxToolResultSize = 16384
	if len(content) > maxToolResultSize {
		a.logger.Info("truncating large tool result for history", "name", tc.Function.Name, "original_size", len(content))
		content = fmt.Sprintf("%s\n\n... (result truncated for memory efficiency. Total size: %d bytes)", content[:maxToolResultSize], len(content))
	}

	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    content,
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
