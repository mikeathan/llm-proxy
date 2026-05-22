package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxSteps is the fallback when no per-model value is set.
	DefaultMaxSteps = 25
	// DefaultContextBudget is the character count that triggers the physical sieve.
	// For an 8K-token model (~16K chars at 2:1 char:token ratio), 8000 chars keeps
	// the input under ~4000 tokens and leaves room for a 2048-token response.
	DefaultContextBudget = 8000
	// DefaultMaxTokens limits the LLM response length to prevent context overflow
	// and truncation of tool call arguments (e.g. write_file with huge content).
	DefaultMaxTokens = 3072
	// DefaultReasoningStuckThreshold is the max characters of consecutive
	// reasoning content before the stream is aborted as stuck. At ~4
	// chars/token this allows ~500 thinking tokens before firing.
	DefaultReasoningStuckThreshold = 2000
	// DefaultStarvationLimit is the max consecutive no-tool turns before the agent
	// is considered stalled and fails.  Tool-call failures and retries still count
	// as "steps" but turns without ANY tool call are tracked separately so the
	// agent can't loop forever on text-only rambling.
	DefaultStarvationLimit = 15
	// AgentGlobalTimeout is the maximum duration for a complete agentic operation.
	AgentGlobalTimeout = 30 * time.Minute
	// AgentTurnTimeout is the maximum time allowed for a single LLM turn.
	AgentTurnTimeout = 10 * time.Minute
	// AgentRetryTimeout is the timeout used for fallback/retry logic.
	AgentRetryTimeout = 5 * time.Minute
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client          proxy.Client
	provider        ToolProvider
	engine          Engine
	guardrails      *guardrails.GuardrailEngine
	logger          logging.Logger
	maxSteps        int
	contextBudget   int
	maxTokens       int
	reasoningBudget int
	icuWeight       float64
	globalTimeout   time.Duration
	useNativeTools  bool
	observer        Observer
	workspaceID     string
	onGuardrail     GuardrailDecisionCallback
	prefillDisabled bool
	orch            *orchestrator.Orchestrator
	modelName       string
	providerType    string
}

type AgentOptions struct {
	MaxSteps                 int     // 0 = use DefaultMaxSteps
	ContextBudget            int     // 0 = use DefaultContextBudget
	MaxResponseTokens        int     // 0 = use DefaultMaxTokens
	ReasoningBudget          int     // 0 = use provider default (no cap)
	ICUWeight                float64 // 0 = default 1.0
	Logger                   logging.Logger
	Guardrails               *guardrails.GuardrailEngine
	Observer                 Observer
	WorkspaceID              string
	UseNativeTools           *bool // nil = delegate to provider; explicit true/false overrides
	UsePrefill               bool  // ignored; prefill is always on for automation + text tools
	GuardrailDecisionHandler GuardrailDecisionCallback
	Orchestrator             *orchestrator.Orchestrator
	ModelName                string
	ProviderType             string
	GlobalTimeout            time.Duration // 0 = use DefaultAgentGlobalTimeout
}

// applyDefaults fills in zero-valued fields with their global defaults
// so callers don't need to repeat this boilerplate for every agent instance.
func (o *AgentOptions) applyDefaults() {
	if o.MaxSteps <= 0 {
		o.MaxSteps = DefaultMaxSteps
	}
	if o.ContextBudget <= 0 {
		o.ContextBudget = DefaultContextBudget
	}
	if o.MaxResponseTokens <= 0 {
		o.MaxResponseTokens = DefaultMaxTokens
	}
	if o.ICUWeight <= 0 {
		o.ICUWeight = 1.0
	}
	if o.Logger == nil {
		o.Logger = logging.NewNopLogger()
	}
	if o.GlobalTimeout <= 0 {
		o.GlobalTimeout = AgentGlobalTimeout
	}
}

// NewAgent creates a new unified agent.
func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	opts.applyDefaults()

	// Guardrails: default engine if none provided.
	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
	}

	// Resolve UseNativeTools: explicit override takes precedence, otherwise ask provider.
	useNative := provider.UseNativeTools()
	if opts.UseNativeTools != nil {
		useNative = *opts.UseNativeTools
	}

	a := &Agent{
		client:          client,
		provider:        provider,
		engine:          engine,
		guardrails:      gr,
		logger:          opts.Logger,
		maxSteps:        opts.MaxSteps,
		contextBudget:   opts.ContextBudget,
		maxTokens:       opts.MaxResponseTokens,
		reasoningBudget: opts.ReasoningBudget,
		icuWeight:       opts.ICUWeight,
		globalTimeout:   opts.GlobalTimeout,
		useNativeTools:  useNative,
		observer:        opts.Observer,
		workspaceID:     opts.WorkspaceID,
		onGuardrail:     opts.GuardrailDecisionHandler,
		orch:            opts.Orchestrator,
		modelName:       opts.ModelName,
		providerType:    opts.ProviderType,
	}
	opts.Logger.Info("NewAgent: agent created", "max_tokens", a.maxTokens, "reasoning_budget", a.reasoningBudget, "max_steps", a.maxSteps)
	return a
}

// ── Repetition Detection ──────────────────────────────────────────────────

type toolKey struct {
	name string
	args string
}

type repetitionDetector struct {
	recentCalls     []toolKey
	duplicateStreak int
}

// check returns a nag prompt at streak 1-2, a fatal error at streak >= 3.
// submit_final_answer and system_error are excluded from tracking.
func (rd *repetitionDetector) check(logger logging.Logger, toolCalls []proxy.ToolCall) (bool, string, error) {
	for _, tc := range toolCalls {
		key := toolKey{tc.Function.Name, tc.Function.Arguments}
		if tc.Function.Name != models.ToolSubmitFinalAnswer && tc.Function.Name != models.ToolSystemError {
			if len(rd.recentCalls) > 0 && rd.recentCalls[len(rd.recentCalls)-1] == key {
				rd.duplicateStreak++
				logger.Warn("duplicate action detected", "tool", key.name, "args", key.args, "streak", rd.duplicateStreak)
				if rd.duplicateStreak >= 3 {
					return true, "", fmt.Errorf("infinite loop detected: model keeps calling %s(%s) after %d nags", key.name, key.args, rd.duplicateStreak)
				}
				return true, prompts.AutomationDuplicateNagPrompt, nil
			}
			rd.duplicateStreak = 0
			if len(rd.recentCalls) >= 3 {
				rd.recentCalls = rd.recentCalls[1:]
			}
			rd.recentCalls = append(rd.recentCalls, key)
		}
	}
	return false, "", nil
}

// ── Main Agentic Loop ─────────────────────────────────────────────────────
// Execute runs the agentic loop until maxSteps, submit_final_answer, or error.
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	execCtx, cancel := context.WithTimeout(ctx, a.globalTimeout)
	defer cancel()

	steps := 0
	sieveStreak := 0
	starvationCount := 0
	warnedAdvisory := false
	currentHistory := append([]proxy.Message{}, history...)
	rd := repetitionDetector{}

	parseErrorStreak := 0
	lastParseErrorKind := ""
	totalErrorStreak := 0
	modelCompatNotified := false

	isAutomation := false
	for _, m := range currentHistory {
		if prompts.IsAutomationTask(m.Content) {
			isAutomation = true
			break
		}
	}

	for {
		steps++
		if err := execCtx.Err(); err != nil {
			return "", currentHistory, fmt.Errorf("agent execution halted: %w", err)
		}

		if steps >= a.maxSteps && !warnedAdvisory {
			warnedAdvisory = true
			a.logger.Warn("agent exceeded advisory step limit, continuing", "steps", steps)
		}

		a.notifyStepStart(steps)
		a.notifyThinking()

		// ── Execute One Turn ────────────────────────────────────────────
		// Calls the LLM, parses tool calls from the response, validates them.
		turnMsg, parseErr, toolsList, err := a.executeTurn(execCtx, &currentHistory)
		if err != nil {
			starvationCount++
			if starvationCount >= DefaultStarvationLimit {
				return "", currentHistory, fmt.Errorf("agent stalled: %w", err)
			}
			if isContextSizeError(err) {
				sieveStreak++
				if sieveStreak >= 3 {
					return "", currentHistory, fmt.Errorf("agent execution failed: model stuck in reasoning loop after %d sieve retries", sieveStreak)
				}
				if sieveStreak == 1 {
					currentHistory = a.applyReactiveSieve(currentHistory)
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: prompts.ReasoningStuckNag,
					})
				} else {
					if len(currentHistory) <= 2 {
						return "", currentHistory, fmt.Errorf("agent execution failed: cannot recover from reasoning loop")
					}
					currentHistory = a.applyAggressiveSieve(currentHistory)
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: prompts.ReasoningStuckEscalatedNag,
					})
				}
				continue
			}

			if isToolCallParseError(err) {
				a.logger.Warn("server-side tool call JSON parse error, sending length feedback to model", "error", err)
				totalErrorStreak++
				if totalErrorStreak >= 5 && !modelCompatNotified {
					modelCompatNotified = true
					a.notifyModelCompatWarning(a.useNativeTools)
				}
				currentHistory = append(currentHistory, proxy.Message{
					Role:    proxy.UserRole,
					Content: prompts.AutomationContentTooLongPrompt,
				})
				continue
			}

			return "", currentHistory, err
		}

		sieveStreak = 0

		// ── Tool Calls Produced ─────────────────────────────────────────
		// Deduplicate, check for loops, execute, check for submit_final_answer.
		if len(turnMsg.ToolCalls) > 0 {
			starvationCount = 0
			parseErrorStreak = 0
			lastParseErrorKind = ""
			totalErrorStreak = 0
			modelCompatNotified = false

			isDuplicate, nagPrompt, dupErr := rd.check(a.logger, turnMsg.ToolCalls)
			if dupErr != nil {
				return "", currentHistory, dupErr
			}
			if isDuplicate {
				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				currentHistory = append(currentHistory, proxy.Message{
					Role:    proxy.UserRole,
					Content: nagPrompt,
				})
				continue
			}

			if len(turnMsg.Content) > 1000 {
				hasWrite := false
				for _, tc := range turnMsg.ToolCalls {
					if tc.Function.Name == models.ToolFileWrite {
						hasWrite = true
						break
					}
				}
				if hasWrite {
					turnMsg.Content = "[Response trimmed — write_file content too long. See tool result feedback.]"
				}
			}

			currentHistory = append(currentHistory, turnMsg)
			a.notify(EventMessage, turnMsg)
			if err := a.processToolCalls(execCtx, turnMsg, &currentHistory); err != nil {
				return "", currentHistory, err
			}

			hasFinalSubmit := false
			for _, tc := range turnMsg.ToolCalls {
				if tc.Function.Name == models.ToolSubmitFinalAnswer {
					summary := extractTaskSummary(tc.Function.Arguments)
					if turnMsg.Content == "" || (summary != "Task complete." && summary != "") {
						turnMsg.Content = summary
					}
					hasFinalSubmit = true
				}
			}
			if hasFinalSubmit {
				return turnMsg.Content, currentHistory, nil
			}
			// ── No Tool Calls ──────────────────────────────────────────────
			// Exit heuristics for pure-text responses: premature termination,
			// parse error feedback, or generic nag.
		} else {
			starvationCount++
			if starvationCount >= DefaultStarvationLimit {
				return "", currentHistory, fmt.Errorf("agent stalled: no tool calls in %d consecutive turns", starvationCount)
			}
			reply, shouldExit, err := a.handleNoToolCalls(
				turnMsg,
				&currentHistory,
				isAutomation,
				parseErr,
				toolsList,
				steps,
				&parseErrorStreak,
				&lastParseErrorKind,
				&totalErrorStreak,
				&modelCompatNotified,
			)
			if err != nil {
				return "", currentHistory, err
			}
			if shouldExit {
				return reply, currentHistory, nil
			}
		}
	}
	return "", currentHistory, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

// ── Context Pressure Sieves ───────────────────────────────────────────────
// These keep the LLM's input under the context window by pruning old messages.

// applyPhysicalSieve is called before EVERY LLM turn.  Keeps first 2 + last 10
// messages when total characters exceed a.contextBudget.
func (a *Agent) applyPhysicalSieve(history []proxy.Message) []proxy.Message {
	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}
	if totalChars <= a.contextBudget {
		return history
	}

	a.logger.Warn("critical context pressure - activating physical sieve", "chars", totalChars)
	if len(history) <= 10 {
		return history
	}

	newHistory := make([]proxy.Message, 0, len(history))
	newHistory = append(newHistory, history[0], history[1])
	newHistory = append(newHistory, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	newHistory = append(newHistory, history[len(history)-10:]...)
	newHistory = append(newHistory, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.ContextSieveWarning,
	})
	return newHistory
}

// applyReactiveSieve prunes history aggressively in response to an LLM context size overflow error.
func (a *Agent) applyReactiveSieve(history []proxy.Message) []proxy.Message {
	a.logger.Warn("context size overflow detected, applying reactive sieve")
	if len(history) <= 8 {
		return history
	}
	sieved := make([]proxy.Message, 0, len(history))
	sieved = append(sieved, history[0], history[1])
	sieved = append(sieved, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	tail := 6
	if len(history) < tail+2 {
		tail = len(history) - 2
	}
	return append(sieved, history[len(history)-tail:]...)
}

// applyAggressiveSieve prunes even more aggressively for the 2nd consecutive
// stuck event — keeps only the first 2 messages + the last 3.
func (a *Agent) applyAggressiveSieve(history []proxy.Message) []proxy.Message {
	a.logger.Warn("aggressive sieve applied — model stuck after prior recovery attempt")
	sieved := make([]proxy.Message, 0, 6)
	sieved = append(sieved, history[0], history[1])
	sieved = append(sieved, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.SieveSystemNote,
	})
	tail := 3
	if len(history) < tail+2 {
		tail = len(history) - 2
	}
	return append(sieved, history[len(history)-tail:]...)
}

// ── Single Turn ───────────────────────────────────────────────────────────
// executeTurn runs one LLM call, parses tool calls, validates & deduplicates.
func (a *Agent) executeTurn(ctx context.Context, history *[]proxy.Message) (proxy.Message, *proxy.ParseError, []proxy.Tool, error) {
	turnCtx, turnCancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer turnCancel()

	toolsList, err := a.provider.ListTools(turnCtx)
	if err != nil {
		return proxy.Message{}, nil, nil, fmt.Errorf("failed to list tools: %w", err)
	}

	*history = a.applyPhysicalSieve(*history)

	msg, err := a.computeNextResponse(turnCtx, *history, toolsList)
	if err != nil {
		return proxy.Message{}, nil, nil, err
	}

	a.logger.Debug("raw model response",
		"content_len", len(msg.Content),
		"content", msg.Content,
		"native_tool_calls", len(msg.ToolCalls),
	)

	parseErr := a.handleContentToolCalls(&msg)
	turnMsg := msg

	if parseErr != nil && (parseErr.XMLFound || len(turnMsg.ToolCalls) == 0) {
		contentPreview := ""
		if len(msg.Content) > 0 {
			contentPreview = msg.Content
			if len(contentPreview) > 500 {
				contentPreview = contentPreview[:500]
			}
		}
		a.logger.Warn("tool call parse error",
			"xml_found", parseErr.XMLFound,
			"json_error", parseErr.JSONError,
			"tool_name", parseErr.ToolName,
			"content_preview", contentPreview,
		)
	}

	if len(turnMsg.ToolCalls) > 0 {
		parseErr = nil
		for _, tc := range turnMsg.ToolCalls {
			if valErr := proxy.ValidateToolCall(tc, toolsList); valErr != nil {
				if pe, ok := valErr.(*proxy.ParseError); ok {
					parseErr = pe
				}
				break
			}
		}
		if parseErr != nil {
			parseErr.XMLFound = true
			turnMsg.ToolCalls = nil
			if len(turnMsg.Content) > 400 && isTruncationError(parseErr.JSONError) {
				turnMsg.Content = "[Large response truncated — see next message for guidance.]"
			}
			return turnMsg, parseErr, toolsList, nil
		}

		uniqueCalls := make([]proxy.ToolCall, 0, len(turnMsg.ToolCalls))
		seenInTurn := make(map[string]bool)
		for _, tc := range turnMsg.ToolCalls {
			callKey := tc.Function.Name + ":" + tc.Function.Arguments
			if !seenInTurn[callKey] {
				seenInTurn[callKey] = true
				uniqueCalls = append(uniqueCalls, tc)
			}
		}
		turnMsg.ToolCalls = uniqueCalls
	}

	return turnMsg, parseErr, toolsList, nil
}

// ── No-Tool-Call Handling ─────────────────────────────────────────────────
// When the LLM produces text only (no tool calls), decide whether to exit
// (premature termination) or nag the model to produce a tool call.
func (a *Agent) handleNoToolCalls(
	turnMsg proxy.Message,
	history *[]proxy.Message,
	isAutomation bool,
	parseErr *proxy.ParseError,
	toolsList []proxy.Tool,
	steps int,
	parseErrorStreak *int,
	lastParseErrorKind *string,
	totalErrorStreak *int,
	modelCompatNotified *bool,
) (string, bool, error) {
	if len(*history) == 0 || (*history)[len(*history)-1].Content != turnMsg.Content {
		*history = append(*history, turnMsg)
		a.notify(EventMessage, turnMsg)
	}

	if isAutomation {
		if a.isPrematureTermination(turnMsg, *history) {
			a.logger.Warn("automation task — premature termination detected", "step", steps)
			return turnMsg.Content, true, nil
		}

		if parseErr != nil {
			errKind := parseErrorKind(parseErr)
			if errKind == *lastParseErrorKind {
				*parseErrorStreak++
			} else {
				*parseErrorStreak = 0
				*lastParseErrorKind = errKind
			}
			*totalErrorStreak++
			if *totalErrorStreak >= 5 && !*modelCompatNotified {
				*modelCompatNotified = true
				a.notifyModelCompatWarning(a.useNativeTools)
			}

			availableNames := proxy.AvailableToolNames(toolsList)
			feedback := parseErr.Feedback(availableNames)

			if *parseErrorStreak >= 2 {
				feedback = fmt.Sprintf(prompts.ParseErrorEscalationPrefix, feedback)
			}

			a.logger.Debug("injecting specific parse-error feedback",
				"error", parseErr.Error(),
				"streak", *parseErrorStreak,
				"feedback", feedback,
			)
			*history = append(*history, proxy.Message{
				Role:    proxy.UserRole,
				Content: feedback,
			})
		} else {
			a.logger.Warn("turn resulted in no action - nagging model", "step", steps, "nag", prompts.AutomationNagPrompt)
			*history = append(*history, proxy.Message{
				Role:    proxy.UserRole,
				Content: prompts.AutomationNagPrompt,
			})
		}
		return "", false, nil
	}

	if a.isPrematureTermination(turnMsg, *history) {
		a.logger.Info("premature termination detected — model is repeating or producing empty output")
		return turnMsg.Content, true, nil
	}
	if steps == 1 {
		return turnMsg.Content, true, nil
	}
	if a.countConsecutiveChat(*history) >= 2 {
		return turnMsg.Content, true, nil
	}
	if a.precededByToolResult(*history) {
		return turnMsg.Content, true, nil
	}

	return "", false, nil
}

// parseErrorKind classifies a ParseError into a stable category so the
// escalation logic can detect when the model keeps making the same mistake.
func parseErrorKind(e *proxy.ParseError) string {
	if e == nil {
		return ""
	}
	if !e.XMLFound {
		return "no_xml"
	}
	if e.JSONError != "" {
		return "json"
	}
	if e.ToolName != "" {
		return "tool_name"
	}
	return ""
}

func isTruncationError(errStr string) bool {
	low := strings.ToLower(errStr)
	return strings.Contains(low, "unexpected end") || strings.Contains(low, "missing closing")
}

func isToolCallParseError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "failed to parse tool call arguments")
}

func isContextSizeError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "context size") ||
		strings.Contains(low, "context_length_exceeded") ||
		strings.Contains(low, "maximum context length") ||
		strings.Contains(low, "reduce the length") ||
		strings.Contains(low, "too many tokens") ||
		strings.Contains(low, "reasoning stuck")
}

// ── Content Tool Call Parsing ─────────────────────────────────────────────
// Parses <tool_call> XML blocks from generated text.  Native tool calls from
// API deltas are already in msg.ToolCalls by this point.
func (a *Agent) handleContentToolCalls(msg *proxy.Message) *proxy.ParseError {
	if msg.Content == "" {
		return nil
	}
	cleaned, calls, parseErr := proxy.ParseContentToolCalls(msg.Content)
	msg.Content = cleaned
	if parseErr == nil && len(calls) > 0 {
		msg.ToolCalls = append(msg.ToolCalls, calls...)
		a.logger.Debug("xml tool calls parsed from content", "count", len(calls))
		return nil
	}

	if a.useNativeTools {
		nativeCleaned, nativeCalls, nativeErr := proxy.ParseNativeToolCalls(msg.Content)
		if nativeErr == nil && len(nativeCalls) > 0 {
			msg.Content = nativeCleaned
			msg.ToolCalls = append(msg.ToolCalls, nativeCalls...)
			a.logger.Debug("native format tool calls parsed from content", "count", len(nativeCalls))
			return nil
		}
	}

	return parseErr
}

// precededByToolResult checks whether the last assistant message (no tool calls)
// follows a tool result, indicating that the model has just completed a tool loop.
func (a *Agent) precededByToolResult(history []proxy.Message) bool {
	if len(history) < 2 {
		return false
	}
	last := history[len(history)-1]
	prev := history[len(history)-2]
	return last.Role == proxy.AssistantRole &&
		len(last.ToolCalls) == 0 &&
		strings.TrimSpace(last.Content) != "" &&
		prev.Role == proxy.ToolRole
}

func (a *Agent) countConsecutiveChat(history []proxy.Message) int {
	chatCount := 0
	var lastContent string
	for j := len(history) - 1; j >= 0; j-- {
		if history[j].Role == proxy.AssistantRole && len(history[j].ToolCalls) == 0 {
			// Check for near-identical monologue repetition
			current := strings.TrimSpace(history[j].Content)
			if lastContent != "" && len(current) > 10 && strings.Contains(lastContent, current[:len(current)/2]) {
				chatCount += 2 // Penalize similar monologues heavily
			} else {
				chatCount++
			}
			lastContent = current
		} else {
			break
		}
	}
	return chatCount
}

// ── LLM Calls (Streaming + Non-Streaming) ─────────────────────────────────
// computeNextResponse tries streaming first, with these fallback layers:
//  1. prefill rejected by server (thinking mode) → retry streaming without prefill
//  2. stream error → non-streaming with same tools
//  3. stream returned empty with native tools → non-streaming with tools (NOT nil)
//  4. stream returned empty without native tools → non-streaming with tools
func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	// Decide whether to pass tool definitions via API (native) or embed them
	// as XML instructions in the system prompt (text-based).
	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	prepared := a.prepareMessages(history)

	isAutomationCtx := false
	for _, m := range history {
		if prompts.IsAutomationTask(m.Content) {
			isAutomationCtx = true
			break
		}
	}
	if llmTools == nil && len(tools) > 0 {
		prepared = a.injectToolInstructions(prepared, tools)
	} else if llmTools != nil && len(tools) > 0 {
		prepared = a.injectNativeToolReference(prepared, tools)
	}

	// Prefill: in automation mode with text-based tools, inject a synthetic
	// assistant message (<tool_call>\n{"tool":") so the model completes a tool
	// call instead of deciding whether to think or act.  NOT used with native
	// tools — llama.cpp handles native tool deltas natively.
	var prefill string
	if a.shouldPrefill(isAutomationCtx) {
		prefill = prompts.AutomationPrefline
		prepared = append(prepared, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}

	req := proxy.ChatRequest{
		Messages:  prepared,
		Tools:     llmTools,
		MaxTokens: a.maxTokens,
	}
	if a.useNativeTools && isAutomationCtx {
		req.ToolChoice = proxy.ToolChoiceRequired
	}
	if isAutomationCtx {
		req.Temperature = 0.1
		if a.reasoningBudget > 0 {
			req.ReasoningBudget = a.reasoningBudget
		} else {
			req.ReasoningBudget = a.maxTokens / 4
		}
	}

	// Pre-flight budget check: debit projected ICU cost before the LLM
	// call. If the stream fails, the defer block below refunds it so
	// transient errors don't consume the workspace budget.
	var txnID string
	a.logger.Info("computeNextResponse: before budget check", "max_tokens", a.maxTokens, "reasoning_budget", a.reasoningBudget)
	if a.orch != nil && a.orch.Budget != nil {
		totalChars := 0
		for _, m := range history {
			totalChars += len(m.Content)
		}
		preflight, pfErr := a.orch.Budget.PreFlightCheck(ctx, a.workspaceID, orchestrator.PreFlightRequest{
			ModelName:       a.modelName,
			ProviderType:    a.providerType,
			ContextChars:    totalChars,
			MaxTokens:       a.maxTokens,
			ReasoningBudget: a.reasoningBudget,
			ICUWeight:       a.icuWeight,
		})
		if pfErr != nil {
			return proxy.Message{}, fmt.Errorf("budget error: %w", pfErr)
		}
		if !preflight.Allowed {
			return proxy.Message{}, fmt.Errorf("budget exceeded: %s", preflight.Reason)
		}
		txnID = preflight.TransactionID
		if preflight.SqueezeFactor < 1.0 {
			a.maxTokens = preflight.AdjustedMaxTokens
			a.reasoningBudget = preflight.AdjustedReasoning
			req.MaxTokens = a.maxTokens
			a.logger.Warn("budget squeeze applied", "factor", preflight.SqueezeFactor, "adjusted_max_tokens", a.maxTokens, "adjusted_reasoning", a.reasoningBudget)
		}
		a.logger.Info("computeNextResponse: after budget check", "max_tokens", a.maxTokens, "squeeze_factor", preflight.SqueezeFactor, "budget_exists", a.orch != nil)
	}

	ch, streamErr := a.client.Stream(ctx, req)
	a.logger.Info("stream request sent", "model", a.modelName, "max_tokens", a.maxTokens, "tool_choice", req.ToolChoice, "temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)
	if a.orch != nil && a.orch.Budget != nil && txnID != "" {
		defer func() {
			if streamErr != nil {
				if refErr := a.orch.Budget.Refund(ctx, txnID); refErr != nil {
					a.logger.Warn("ICU refund failed", "txn", txnID, "error", refErr)
				} else {
					a.logger.Warn("ICU refunded due to stream failure", "txn", txnID)
				}
			}
		}()
	}

	if streamErr != nil {
		if prefill != "" && isPrefillThinkingError(streamErr) {
			a.logger.Info("prefill rejected by server (thinking mode active), retrying without prefill in XML mode",
				"prefill_len", len(prefill),
				"prefill_preview", prefill,
			)
			a.prefillDisabled = true
			a.notifyPrefillDisabled()
			prefill = ""
			prepared = a.prepareMessages(history)
			if len(tools) > 0 {
				prepared = a.injectToolInstructions(prepared, tools)
			}
			req = proxy.ChatRequest{
				Messages:        prepared,
				Tools:           nil,
				MaxTokens:       a.maxTokens,
				Temperature:     0.1,
				ReasoningBudget: a.maxTokens / 4,
			}
			ch, streamErr = a.client.Stream(ctx, req)
		}
		if streamErr != nil {
			a.logger.Warn("streaming not supported or failed, falling back to non-streaming", "error", streamErr)
			return a.computeNextResponseNonStreaming(ctx, history, tools)
		}
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if streamErr = a.processStream(ctx, ch, &fullMsg); streamErr != nil {
		return proxy.Message{}, streamErr
	}

	if prefill != "" {
		fullMsg.Content = prefill + fullMsg.Content
	}

	a.logger.Info("stream completed", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent), "tool_calls", len(fullMsg.ToolCalls))

	// Fallback 1: native tools + empty stream + no native tool deltas.
	// Common when reasoning consumes the token budget before the tool call
	// starts. Retry non-streaming WITH tools (not nil) so the model still
	// has tool context and can produce a valid tool call.
	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 && llmTools != nil {
		a.logger.Info("empty response with native tools, retrying without them")
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	// Fallback 2: non-native mode, empty stream. Retry non-streaming with
	// XML tool instructions (tools are injected into the system prompt).
	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
		a.logger.Info("stream returned no content, falling back to non-streaming retry")
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	a.logger.Debug("raw model response received", "content_preview", fmt.Sprintf("%.50s", fullMsg.Content), "tool_calls", len(fullMsg.ToolCalls))
	return fullMsg, nil
}

// ── Stream Processing ─────────────────────────────────────────────────────
// Consumes SSE chunks from the LLM stream.  Content goes to fullMsg.Content,
// reasoning to fullMsg.ReasoningContent, and native tool deltas to
// fullMsg.ToolCalls.  Both content and reasoning are sent to the UI via
// EventToolStream but only Content and ToolCalls enter the conversation
// history — ReasoningContent is stripped by SanitizeHistory.
func (a *Agent) processStream(ctx context.Context, ch <-chan *proxy.ChatResponse, fullMsg *proxy.Message) error {
	var tokUsed, reasonUsed int

	// Progress indicator: log every 30s during long streams so the user
	// knows the agent is still working and not hung.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.logger.Info("stream still generating", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent))
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-ch:
			if !ok {
				return nil
			}
			if len(resp.Choices) > 0 {
				choice := resp.Choices[0]
				chunkContent := choice.Delta.Content
				if chunkContent == "" && choice.Message.Content != "" {
					chunkContent = choice.Message.Content
				}
				reasoningChunk := choice.Delta.ReasoningContent
				if reasoningChunk == "" && choice.Message.ReasoningContent != "" {
					reasoningChunk = choice.Message.ReasoningContent
				}

				// Budget tracking: count tokens used vs. maxTokens + reasoningBudget.
				if a.orch != nil && a.orch.Interceptor != nil {
					result := a.orch.Interceptor.InterceptChunk(orchestrator.StreamChunk{
						Content:          chunkContent,
						ReasoningContent: reasoningChunk,
						ProviderType:     a.providerType,
					})
					tokUsed += result.TokensUsed
					reasonUsed += result.ReasoningUsed

					term := a.orch.Interceptor.InterceptChunkWithBudget(ctx,
						orchestrator.StreamChunk{},
						tokUsed, reasonUsed, a.maxTokens, a.reasoningBudget,
					)
					if term.ShouldTerminate {
						return fmt.Errorf("stream terminated: token budget exceeded (%d tokens used)", tokUsed)
					}
				}

				// Reasoning stuck check: scales with max_tokens so larger models
				// get more room to reason before being cut off. Defaults to 2000
				// chars (~500 tokens at 4 chars/token) when max_tokens is not set.
				stuckThreshold := a.maxTokens * 2
				if stuckThreshold <= 0 || stuckThreshold < DefaultReasoningStuckThreshold {
					stuckThreshold = DefaultReasoningStuckThreshold
				}
				if len(fullMsg.ReasoningContent) > stuckThreshold && len(fullMsg.Content) == 0 && len(fullMsg.ToolCalls) == 0 {
					a.logger.Warn("reasoning stuck detected, aborting stream early to trigger fallback", "reasoning_chars", len(fullMsg.ReasoningContent), "stuck_threshold", stuckThreshold)
					return nil
				}

				if chunkContent != "" {
					fullMsg.Content += chunkContent
				}
				if reasoningChunk != "" {
					fullMsg.ReasoningContent += reasoningChunk
				}
				if chunkContent != "" || reasoningChunk != "" {
					displayText := fullMsg.ReasoningContent + fullMsg.Content
					displayContent, hasToolCall := FilterStreamingMarkup(displayText)
					if hasToolCall {
						a.notify(EventToolStream, displayContent+"\n\n🛠️ *Agent is initiating tool calls...*")
					} else {
						a.notify(EventToolStream, displayContent)
					}
				}
				if len(choice.Delta.ToolCalls) > 0 {
					// Native tool-call delta accumulation.  This path only activates
					// when useNativeTools is true (cloud models using native tool
					// schemas).  When useNativeTools is false the LLM server never
					// sends tool-call deltas, and tool extraction happens via the
					// XML text parser in handleContentToolCalls instead.
					for _, tc := range choice.Delta.ToolCalls {
						if tc.ID != "" {
							fullMsg.ToolCalls = append(fullMsg.ToolCalls, tc)
						} else if len(fullMsg.ToolCalls) > 0 {
							last := &fullMsg.ToolCalls[len(fullMsg.ToolCalls)-1]
							last.Function.Arguments += tc.Function.Arguments
						}
					}
				}
			}
		}
	}
}

// computeNextResponseNonStreaming is the fallback when streaming fails or
// returns empty.  Receives the same tools list so the model still has tool
// context (API-level native tools or injected XML instructions).
func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	preparedHistory := a.prepareMessages(history)

	isAutomationCtx := false
	for _, m := range history {
		if prompts.IsAutomationTask(m.Content) {
			isAutomationCtx = true
			break
		}
	}
	if llmTools == nil && len(tools) > 0 {
		preparedHistory = a.injectToolInstructions(preparedHistory, tools)
	} else if llmTools != nil && len(tools) > 0 {
		preparedHistory = a.injectNativeToolReference(preparedHistory, tools)
	}

	// Prefill assistant response in automation mode so the model
	// never has to choose between thinking and acting.  This is
	// on by default for all text-based tool calling.
	var prefill string
	if a.shouldPrefill(isAutomationCtx) {
		prefill = prompts.AutomationPrefline
		preparedHistory = append(preparedHistory, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}

	req := proxy.ChatRequest{
		Messages:  preparedHistory,
		Tools:     llmTools,
		MaxTokens: a.maxTokens,
	}
	if a.useNativeTools && isAutomationCtx {
		req.ToolChoice = proxy.ToolChoiceRequired
	}
	if isAutomationCtx {
		req.Temperature = 0.1
		if a.reasoningBudget > 0 {
			req.ReasoningBudget = a.reasoningBudget
		} else {
			req.ReasoningBudget = a.maxTokens / 4
		}
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	a.logger.Info("non-stream request sent", "model", a.modelName, "max_tokens", a.maxTokens, "tool_choice", req.ToolChoice, "temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)

	resp, err := a.client.Chat(chatCtx, req)
	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.logger.Info("prefill rejected by server (thinking mode), retrying without prefill in XML mode (non-stream)")
		a.prefillDisabled = true
		a.notifyPrefillDisabled()
		prefill = ""
		preparedHistory = a.prepareMessages(history)
		if len(tools) > 0 {
			preparedHistory = a.injectToolInstructions(preparedHistory, tools)
		}
		req = proxy.ChatRequest{
			Messages:        preparedHistory,
			Tools:           nil,
			MaxTokens:       a.maxTokens,
			Temperature:     0.1,
			ReasoningBudget: a.maxTokens / 4,
		}
		resp, err = a.client.Chat(chatCtx, req)
	}
	if err != nil && isToolSupportError(err) {
		a.logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)
		chatCtx2, cancel2 := context.WithTimeout(ctx, AgentRetryTimeout)
		defer cancel2()
		resp, err = a.client.Chat(chatCtx2, proxy.ChatRequest{Messages: history, MaxTokens: a.maxTokens})
	}

	if err != nil {
		return proxy.Message{}, fmt.Errorf("llm completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return proxy.Message{}, fmt.Errorf("llm returned no choices")
	}

	msg := resp.Choices[0].Message
	if prefill != "" {
		msg.Content = prefill + msg.Content
	}
	return msg, nil
}

// ── Termination Heuristics ────────────────────────────────────────────────

// isPrematureTermination detects when the model produced empty or repetitive
// output (3 identical assistant messages in a row).
func (a *Agent) isPrematureTermination(msg proxy.Message, history []proxy.Message) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		// Model generated only reasoning (no content, no tool calls). This is
		// not premature termination — the model was thinking but needs a nag
		// to stop thinking and produce an action.
		if len(msg.ReasoningContent) > 0 {
			return false
		}
		return len(msg.ToolCalls) == 0
	}
	if len(history) >= 2 {
		last := history[len(history)-1]
		if last.Role == proxy.AssistantRole && strings.TrimSpace(last.Content) == content {
			prev := history[len(history)-2]
			if prev.Role == proxy.AssistantRole && strings.TrimSpace(prev.Content) == content {
				return true
			}
		}
	}
	return false
}

// ── Tool Execution ────────────────────────────────────────────────────────
// Validates against schema, runs guardrails, executes via Engine.
func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message) error {
	var mu sync.Mutex
	hasSubmit := false
	for _, tc := range msg.ToolCalls {
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			hasSubmit = true
			break
		}
	}
	if hasSubmit && len(msg.ToolCalls) > 1 {
		a.logger.Warn("rejected batched submission", "count", len(msg.ToolCalls))
		mu.Lock()
		errorMsg := prompts.AutomationRejectedSubmissionPrompt
		for _, tc := range msg.ToolCalls {
			a.appendToolResult(history, tc, map[string]string{"error": errorMsg})
		}
		mu.Unlock()
		return nil
	}

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tc.Type != "" && tc.Type != "function" {
			continue
		}

		a.logger.Debug("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)
		a.logger.Info("executing tool", "name", tc.Function.Name)

		toolsList, _ := a.provider.ListTools(ctx)
		if err := validateToolArgs(tc, toolsList); err != nil {
			a.logger.Warn("tool argument validation failed", "name", tc.Function.Name, "error", err)
			errMsg := fmt.Sprintf("INVALID ARGUMENTS: %v", err)
			if tc.Function.Name == models.ToolFileWrite && isTruncationError(err.Error()) {
				errMsg = prompts.AutomationContentTooLongPrompt
			}
			mu.Lock()
			a.appendToolResult(history, tc, map[string]string{"error": errMsg})
			mu.Unlock()
			return nil
		}
		guardrailApproved := false

		if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.notifyGuardrailViolation(tc.Function.Name, err)

			if a.onGuardrail != nil {
				decision, decErr := a.onGuardrail(ctx, GuardrailBlockedPayload{
					DecisionID: fmt.Sprintf("gr_%d", time.Now().UnixNano()),
					Tool:       tc.Function.Name,
					Args:       tc.Function.Arguments,
					Reason:     err.Error(),
					Category:   toolCategory(tc.Function.Name),
				})
				if decErr == nil && decision.Allow {
					guardrailApproved = true
					if decision.Persist {
						if err := a.guardrails.PersistOverride(a.workspaceID, toolCategory(tc.Function.Name), tc.Function.Name, tc.Function.Arguments); err != nil {
							a.logger.Warn("failed to persist guardrail override", "error", err)
						}
					}
				} else {
					mu.Lock()
					a.appendToolResult(history, tc, formatGuardrailError(err))
					mu.Unlock()
					return nil
				}
			} else {
				mu.Lock()
				a.appendToolResult(history, tc, formatGuardrailError(err))
				mu.Unlock()
				return nil
			}
		}

		a.notifyToolCall(tc)
		toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
		if guardrailApproved {
			toolCtx = models.WithGuardrailApproved(toolCtx)
		}
		result, err := a.engine.ExecuteTool(toolCtx, tc)
		mu.Lock()
		var finalResult any
		if err != nil {
			if str, ok := result.(string); ok && strings.TrimSpace(str) != "" {
				finalResult = str
			} else {
				finalResult = map[string]string{"error": err.Error()}
			}
		} else {
			finalResult = result
		}
		a.appendToolResult(history, tc, finalResult)
		resultStr, _ := json.Marshal(finalResult)
		a.logger.Debug("tool execution completed", "name", tc.Function.Name, "error", err, "result", string(resultStr))
		a.logger.Info("tool execution completed", "name", tc.Function.Name, "error", err)
		a.notifyToolResult(tc.ID, tc.Function.Name, finalResult)
		mu.Unlock()

		if err != nil {
			a.logger.Warn("tool execution failed - stopping batch", "name", tc.Function.Name, "error", err)
			return nil
		}
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			return nil
		}
	}
	return nil
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
}

func toolCategory(toolName string) string {
	switch toolName {
	case models.ToolTerminalExecute:
		return "terminal"
	case models.ToolDirectoryList, models.ToolFileRead, models.ToolFileWrite:
		return "filesystem"
	case models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo:
		return "network"
	case models.ToolInternetSearch:
		return "search"
	case models.ToolNotifyUser:
		return "communication"
	default:
		return "general"
	}
}

func extractTaskSummary(rawArgs string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "Task complete."
	}
	for _, key := range []string{"summary", "message", "report", "findings", "content", "result"} {
		if val, ok := args[key].(string); ok && val != "" {
			return val
		}
	}
	for _, val := range args {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return "Task complete."
}

func validateToolArgs(tc proxy.ToolCall, tools []proxy.Tool) error {
	var targetTool *proxy.Tool
	for _, t := range tools {
		if t.Function.Name == tc.Function.Name {
			targetTool = &t
			break
		}
	}
	if targetTool == nil {
		return fmt.Errorf("tool '%s' not found", tc.Function.Name)
	}
	params, ok := targetTool.Function.Parameters.(map[string]any)
	if !ok {
		return nil
	}
	requiredRaw, ok := params["required"]
	if !ok {
		return nil
	}
	var required []string
	switch r := requiredRaw.(type) {
	case []any:
		for _, v := range r {
			if s, ok := v.(string); ok {
				required = append(required, s)
			}
		}
	case []string:
		required = r
	}
	if len(required) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse arguments as JSON: %w", err)
	}
	for _, field := range required {
		val, ok := args[field]
		if !ok {
			return fmt.Errorf("missing required parameter '%s'", field)
		}
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("parameter '%s' cannot be empty", field)
		}
	}
	return nil
}

func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) {
	raw, _ := json.Marshal(result)
	strContent := string(raw)
	strContent = proxy.TruncateResult(strContent)
	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    strContent,
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

// isPrefillThinkingError detects llama.cpp's rejection of assistant
// response prefill when enable_thinking is active on the server side.
func isPrefillThinkingError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "prefill") &&
		strings.Contains(lowErr, "thinking")
}

// ── Message Preparation ───────────────────────────────────────────────────

// shouldPrefill returns true when:
//   - prefill hasn't been disabled by a server-side thinking-mode rejection
//   - we're in automation context (not interactive chat)
//   - native tools are NOT active (prefill is an XML-only mechanism)
func (a *Agent) shouldPrefill(isAutomationCtx bool) bool {
	return !a.prefillDisabled && isAutomationCtx && !a.useNativeTools
}

// prepareMessages normalizes history roles and strips fields incompatible
// with the current tool-calling mode (native vs XML).
func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.useNativeTools)
}

// injectToolInstructions adds the XML tool-call format manual to the system
// prompt.  Used when native tools are disabled (text-mode tool calling via
// <tool_call> blocks in generated text).
func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	instructions := prompts.BuildToolManual(info)
	a.logger.Debug("injecting XML tool manual into system prompt",
		"tool_count", len(info),
		"manual_chars", len(instructions),
		"has_manual", len(instructions) > 0,
	)
	newHistory := make([]proxy.Message, 0, len(history)+1)
	foundSystem := false
	for _, msg := range history {
		if !foundSystem && msg.Role == proxy.SystemRole {
			newMsg := msg
			hadManualBefore := prompts.HasToolManual(newMsg.Content)
			newMsg.Content = prompts.InjectToolManual(newMsg.Content, instructions)
			newHistory = append(newHistory, newMsg)
			foundSystem = true
			a.logger.Debug("tool manual injection result",
				"had_manual_before", hadManualBefore,
				"sys_prompt_chars", len(newMsg.Content),
			)
		} else {
			newHistory = append(newHistory, msg)
		}
	}
	if !foundSystem {
		newHistory = append([]proxy.Message{{
			Role:    proxy.SystemRole,
			Content: prompts.InjectToolManual("You are a powerful agentic AI.", instructions),
		}}, newHistory...)
	}
	return newHistory
}

// injectNativeToolReference adds a lightweight tool-name reference to the
// system prompt.  Used when native tools are active — the LLM receives tool
// definitions via the API but still benefits from textual context.
func (a *Agent) injectNativeToolReference(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	reference := prompts.BuildNativeToolReference(info)
	newHistory := make([]proxy.Message, 0, len(history)+1)
	foundSystem := false
	for _, msg := range history {
		if !foundSystem && msg.Role == proxy.SystemRole {
			newMsg := msg
			newMsg.Content = prompts.InjectToolReference(newMsg.Content, reference)
			newHistory = append(newHistory, newMsg)
			foundSystem = true
		} else {
			newHistory = append(newHistory, msg)
		}
	}
	if !foundSystem {
		newHistory = append([]proxy.Message{{
			Role:    proxy.SystemRole,
			Content: prompts.InjectToolReference("You are a powerful agentic AI.", reference),
		}}, newHistory...)
	}
	return newHistory
}
