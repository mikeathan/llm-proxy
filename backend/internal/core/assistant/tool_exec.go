// tool_exec.go — Tool call validation, guardrail resolution, execution via
// Engine, and result appending.  Also holds ExecutionPlan, ExecutionPlanStrategy
// (moved from strategy_plan.go), formatGuardrailError, toolCategory,
// validateToolArgs.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

const planGenMaxTokens = 4096 // max tokens for plan generation LLM call

const (
	planErrMaxSteps  = "plan exceeds max steps: %d > %d"             // pre-check at executePlan entry
	planErrStepLimit = "plan aborted: exceeded max steps limit (%d)" // in-loop belt-and-suspenders check
)

// guardrailDenialReason classifies why a guardrail block became a denial, so
// the model is told the right guidance (hard policy block vs explicit user deny
// vs an approval prompt that expired without consent).
type guardrailDenialReason int

const (
	denialSecurity guardrailDenialReason = iota // hard policy block; no approval prompt was offered
	denialUser                                  // user explicitly denied the approval
	denialTimeout                               // approval prompt expired without a response
)

const (
	// guardrailDeniedByPolicy is appended when a security-boundary block is
	// denied without an approval prompt (path outside workspace, blocked system
	// file). No user consent was involved — it is a hard policy denial.
	guardrailDeniedByPolicy = "Action blocked by security policy. Do NOT retry, rephrase, or attempt the same outcome via a different path."
	// guardrailDeniedByUser is appended when the user explicitly denies an
	// approval so the model does not retry, rephrase, or route around the block.
	guardrailDeniedByUser = "Action denied by the user. Do NOT retry, rephrase, or attempt the same outcome via a different path."
	// guardrailDeniedByTimeout is appended when the approval prompt expired
	// without a response. Silence is not consent.
	guardrailDeniedByTimeout = "Action timed out without user response; silence is not consent. Do NOT retry, rephrase, or attempt the same outcome via a different path."
)

type ExecutionPlan struct {
	Description string          `json:"description"`
	Steps       []ExecutionStep `json:"steps"`
}

type ExecutionStep struct {
	ToolName    string                 `json:"tool"`
	Description string                 `json:"description"`
	Input       string                 `json:"input,omitempty"`
	Parameters  map[string]interface{} `json:"args,omitempty"`
}

type ExecutionPlanStrategy struct {
	llm    proxy.Client
	tools  []proxy.Tool
	logger logging.Logger

	// applyRequest applies per-model request config (temperature + reasoning
	// wire params) to the plan-generation request so it matches normal turns.
	applyRequest func(req *proxy.ChatRequest)
	// onReasoning relays plan-generation reasoning deltas to the UI.
	onReasoning func(reasoning string)
	// onLifecycle relays plan-generation liveness events (still_thinking).
	onLifecycle func(phase string, extra map[string]any)
}

// ExecutionPlanStrategyOption configures an ExecutionPlanStrategy. The hooks
// keep the strategy decoupled from Agent internals — session wiring binds them
// to the shared primitives (Agent.applyRequestConfig, notify).
type ExecutionPlanStrategyOption func(*ExecutionPlanStrategy)

// withApplyRequest binds the shared per-model request-config application so
// plan generation sends the same temperature and reasoning wire params as
// normal turns (single source of truth: Agent.applyRequestConfig).
func withApplyRequest(fn func(req *proxy.ChatRequest)) ExecutionPlanStrategyOption {
	return func(s *ExecutionPlanStrategy) { s.applyRequest = fn }
}

// withOnReasoning binds a reasoning-delta relay (Agent.notify(EventReasoning)).
func withOnReasoning(fn func(reasoning string)) ExecutionPlanStrategyOption {
	return func(s *ExecutionPlanStrategy) { s.onReasoning = fn }
}

// withOnLifecycle binds a lifecycle relay (Agent.notifyLifecycle) used by the
// plan-gen liveness heartbeat (still_thinking).
func withOnLifecycle(fn func(phase string, extra map[string]any)) ExecutionPlanStrategyOption {
	return func(s *ExecutionPlanStrategy) { s.onLifecycle = fn }
}

func NewExecutionPlanStrategy(llm proxy.Client, tools []proxy.Tool, logger logging.Logger, opts ...ExecutionPlanStrategyOption) *ExecutionPlanStrategy {
	s := &ExecutionPlanStrategy{llm: llm, tools: tools, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *ExecutionPlanStrategy) Generate(ctx context.Context, task string) (*ExecutionPlan, error) {
	s.logger.Debug("generating execution plan", "tools", len(s.tools), "task_len", len(task))

	toolInfos := make([]prompts.ToolInfo, len(s.tools))
	for i, t := range s.tools {
		toolInfos[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	userPrompt := prompts.BuildExecutionPlanPrompt(toolInfos, task)

	req := proxy.ChatRequest{
		Messages: []proxy.Message{
			{Role: proxy.SystemRole, Content: prompts.ExecutionPlanSystemPrompt},
			{Role: proxy.UserRole, Content: userPrompt},
		},
		MaxTokens: planGenMaxTokens,
	}
	if s.applyRequest != nil {
		s.applyRequest(&req)
	}

	content, err := s.generatePlanContent(ctx, req)
	if err != nil {
		return nil, err
	}
	plan, err := parsePlanContent(content)
	if err != nil {
		s.logger.Info("plan parse failed", "error", err, "raw_length", len(content))
		return nil, fmt.Errorf("plan parse failed: %w", err)
	}

	s.logger.Debug("execution plan generated", "steps", len(plan.Steps), "description", plan.Description)
	return plan, nil
}

// generatePlanContent runs the plan-generation LLM call, streaming first with a
// non-streaming Chat fallback when the provider cannot stream (mirrors
// computeNextResponse's streaming→non-streaming fallback). Returns the raw
// plan JSON text. Mid-stream errors are returned, not retried — matching
// processStream semantics.
func (s *ExecutionPlanStrategy) generatePlanContent(ctx context.Context, req proxy.ChatRequest) (string, error) {
	ch, streamErr := s.llm.Stream(ctx, req)
	if streamErr != nil {
		// User cancel — bail out, do not fall back to a retry path.
		if isUserCanceled(streamErr) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "", streamErr
		}
		s.logger.Info("plan generation streaming unavailable, falling back to non-streaming", "error", streamErr)
		return s.planContentViaChat(ctx, req)
	}

	var content, reasoning strings.Builder
	streamStart := time.Now()

	// Liveness heartbeat: emits still_thinking only while the plan-gen stream
	// is silent (no content/reasoning advanced since the last tick), so the UI
	// never shows a dead bubble during a long planning TTFT. Same emit
	// semantics as processStream.
	hb := core.NewHeartbeat()
	hb.Start(ctx, streamHeartbeatInterval)
	defer hb.Stop()
	var lastTickContent, lastTickReasoning int

	// Reasoning relay: full-snapshot + coalesce + dedupe, matching
	// processStream's notify semantics — the UI replaces the live reasoning
	// inset, never appends, and the bus stays quiet between coalesce ticks.
	// The snapshot is only re-taken when the reasoning text actually grew, so
	// the pure-content phase never re-copies or re-compares the (possibly
	// large) static reasoning string on every chunk.
	var pendingReasoning, lastEmittedReasoning string
	var lastPendingLen int
	var lastNotifyAt time.Time
	flushPendingReasoning := func() {
		if pendingReasoning != "" && pendingReasoning != lastEmittedReasoning && s.onReasoning != nil {
			s.onReasoning(pendingReasoning)
			lastEmittedReasoning = pendingReasoning
		}
		pendingReasoning = ""
		lastNotifyAt = time.Now()
	}
	defer flushPendingReasoning()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-hb.C:
			contentLen, reasoningLen := content.Len(), reasoning.Len()
			if contentLen == lastTickContent && reasoningLen == lastTickReasoning && s.onLifecycle != nil {
				s.onLifecycle(PhaseStillThinking, map[string]any{
					"elapsed": time.Since(streamStart).Round(time.Second).String(),
				})
			}
			lastTickContent, lastTickReasoning = contentLen, reasoningLen
		case resp, ok := <-ch:
			if !ok {
				return content.String(), nil
			}
			if len(resp.Choices) == 0 {
				continue
			}
			choice := resp.Choices[0]
			// Resolve content/reasoning with the shared Delta→Message field
			// semantics (same source as processStream).
			chunk := resolveStreamChunk(choice)

			if chunk.Content != "" {
				content.WriteString(chunk.Content)
			}
			if chunk.ReasoningContent != "" {
				reasoning.WriteString(chunk.ReasoningContent)
			}
			if chunk.Reasoning != "" {
				reasoning.WriteString(chunk.Reasoning)
			}
			// Join multi-part reasoning details with "\n" like
			// Message.ExtractReasoning does for the turn loop.
			for _, d := range chunk.ReasoningDetails {
				if d.Text == "" {
					continue
				}
				if reasoning.Len() > 0 {
					reasoning.WriteString("\n")
				}
				reasoning.WriteString(d.Text)
			}
			if reasoning.Len() > lastPendingLen {
				lastPendingLen = reasoning.Len()
				pendingReasoning = reasoning.String()
			}

			if time.Since(lastNotifyAt) >= streamNotifyCoalesceInterval {
				flushPendingReasoning()
			}
		}
	}
}

// planContentViaChat is the non-streaming plan-generation path used when the
// provider cannot stream (SSE-less providers / stream-start failure).
func (s *ExecutionPlanStrategy) planContentViaChat(ctx context.Context, req proxy.ChatRequest) (string, error) {
	resp, err := s.llm.Chat(ctx, req)
	if err != nil {
		s.logger.Info("plan generation LLM call failed", "error", err)
		return "", fmt.Errorf("plan generation failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		s.logger.Info("plan generation returned no choices")
		return "", fmt.Errorf("plan generation returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}

// parsePlanContent parses the raw plan-generation output into an
// ExecutionPlan. Shared by the streaming and non-streaming plan paths so the
// fence-stripping + unmarshal logic exists exactly once.
func parsePlanContent(content string) (*ExecutionPlan, error) {
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		if idx := strings.LastIndex(content, "```"); idx >= 0 {
			content = content[:idx]
		}
		content = strings.TrimSpace(content)
	}

	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, err
	}

	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	return &plan, nil
}

// executeSingleToolStep resolves guardrails, executes one tool, appends
// the result to history, and fires notifications.
// Returns stopBatch (guardrail denied) and execErr (execution failed).
func (a *Agent) executeSingleToolStep(
	ctx context.Context,
	tc proxy.ToolCall,
	history *[]proxy.Message,
	mu *sync.Mutex,
) (stopBatch bool, execErr error) {
	approved, stopBatch := a.resolveGuardrail(ctx, tc, history, mu)
	if stopBatch {
		return true, nil
	}

	a.notifyToolCall(tc)
	toolCtx := models.WithWorkspaceID(ctx, a.config.WorkspaceID)
	if approved {
		toolCtx = models.WithGuardrailApproved(toolCtx)
	}

	toolCtx, cancel := a.toolCtxWithTimeout(toolCtx, tc.Function.Name)
	defer cancel()

	result, err := a.deps.Engine.ExecuteTool(toolCtx, tc)

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
	resultStr := a.appendToolResult(history, tc, finalResult)
	a.deps.Logger.Debug("tool execution completed", "name", tc.Function.Name, "error", err, "result", resultStr)
	a.notifyToolResult(tc.ID, tc.Function.Name, finalResult)
	if t := GetUsageTracker(ctx); t != nil {
		t.AddToolCall(tc.Function.Name)
	}
	mu.Unlock()

	if err != nil {
		return false, err
	}
	return false, nil
}

// processToolCalls validates tool args, resolves guardrails, and executes
// via Engine.  A guardrail denial stops the entire batch.
// Returns (salvagedReport, err). salvagedReport is non-empty when a truncated
// write_file/append_file content field was recovered as the task deliverable.
func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message, toolsList []proxy.Tool) (string, error) {
	var mu sync.Mutex

	if len(toolsList) == 0 {
		var listErr error
		toolsList, listErr = a.deps.Provider.ListTools(ctx)
		if listErr != nil {
			a.deps.Logger.Error("failed to list tools for validation", "error", listErr)
			return "", fmt.Errorf("list tools: %w", listErr)
		}
	}

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if tc.Type != "" && tc.Type != "function" {
			continue
		}

		a.deps.Logger.Debug("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)
		a.deps.Logger.Info("executing tool", "name", tc.Function.Name)

		if err := validateToolArgs(tc, toolsList); err != nil {
			a.deps.Logger.Warn("tool argument validation failed", "name", tc.Function.Name, "error", err)
			if report, handled := a.salvageTruncatedWrite(ctx, tc, history, &mu); handled {
				return report, nil
			}
			errMsg := fmt.Sprintf("INVALID ARGUMENTS: %v", err)
			if isTruncationError(err.Error()) {
				errMsg = prompts.AutomationContentTooLongPrompt
			}
			mu.Lock()
			a.appendToolResult(history, tc, map[string]string{"error": errMsg})
			mu.Unlock()
			return "", nil
		}

		stopBatch, execErr := a.executeSingleToolStep(ctx, tc, history, &mu)
		a.deps.Logger.Info("tool execution completed", "name", tc.Function.Name, "error", execErr)
		if stopBatch || execErr != nil {
			if execErr != nil {
				a.deps.Logger.Warn("tool execution failed - stopping batch", "name", tc.Function.Name, "error", execErr)
			}
			return "", nil
		}
	}
	return "", nil
}

func (a *Agent) resolveGuardrail(ctx context.Context, tc proxy.ToolCall, history *[]proxy.Message, mu *sync.Mutex) (approved, stopBatch bool) {
	grCtx, grCancel := a.guardrailCtxWithTimeout(ctx)
	defer grCancel()
	// denial records why the block became a denial. Defaults to an explicit
	// user denial; set to timeout when the approval prompt expires so the model
	// is told the block was "no response", never consent.
	denial := denialUser
	if err := a.deps.Guardrails.ValidateToolCall(grCtx, tc, a.config.WorkspaceID); err != nil {
		// A timeout (or other context error) during guardrail evaluation is not a
		// policy decision. Apply the configured failure behavior instead of
		// treating it as a guardrail violation (which would wrongly prompt for an
		// allow/deny decision).
		if errors.Is(grCtx.Err(), context.DeadlineExceeded) {
			switch a.config.GuardrailTimeoutBehavior {
			case "fail-closed":
				a.deps.Logger.Error("guardrail evaluation timed out; failing closed (tool denied)",
					"name", tc.Function.Name, "timeout", a.config.GuardrailTimeout)
				mu.Lock()
				a.appendToolResult(history, tc, map[string]string{"error": "guardrail evaluation timed out"})
				mu.Unlock()
				return false, true
			default: // fail-open (and any unrecognized value)
				a.deps.Logger.Warn("guardrail evaluation timed out; failing open (tool allowed)",
					"name", tc.Function.Name, "timeout", a.config.GuardrailTimeout)
				return false, false
			}
		}
		a.deps.Logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
		a.notifyGuardrailViolation(tc.Function.Name, err)

		// Security boundary violations (path outside workspace, blocked system files)
		// are denied immediately — no approval dialog.
		if isGuardrailSecurityBoundary(err) {
			mu.Lock()
			a.appendToolResult(history, tc, formatGuardrailError(err, denialSecurity))
			mu.Unlock()
			return false, true
		}

		// Unattended automation runs have no interactive user to answer an
		// approval prompt (Constitution II.10: in automation mode the callback
		// is nil and violations fail immediately). Deny right away with hard
		// policy guidance so the model adapts instead of stalling the run for
		// the approval bound — an unanswered prompt previously burned the full
		// GuardrailApprovalTimeout and pushed the run past its deadline.
		if a.config.Channel == ChannelAutomation {
			mu.Lock()
			a.appendToolResult(history, tc, formatGuardrailError(err, denialSecurity))
			mu.Unlock()
			return false, true
		}

		if a.deps.OnGuardrail != nil {
			// The approval wait is bounded (Constitution II.10 / SPEC
			// guardrails) so an unanswered prompt cannot stall the run
			// indefinitely. The bound is configurable per-model
			// (GuardrailApprovalTimeout, default 5 min — Hermes parity; 60s
			// proved too tight in practice). On expiry the callback returns an
			// error and the call is treated as denied below — the violation is
			// recorded and the run continues without the tool.
			approvalCtx, approvalCancel := context.WithTimeout(ctx, a.config.GuardrailApprovalTimeout)
			defer approvalCancel()
			decision, decErr := a.deps.OnGuardrail(approvalCtx, GuardrailBlockedPayload{
				DecisionID:  fmt.Sprintf("gr_%d", time.Now().UnixNano()),
				Tool:        tc.Function.Name,
				Args:        tc.Function.Arguments,
				Reason:      err.Error(),
				Category:    toolCategory(tc.Function.Name),
				WorkspaceID: a.config.WorkspaceID,
			})
			if decErr != nil {
				// No decision arrived before the approval bound — the prompt
				// expired. Reported as "no response", never as user consent.
				a.deps.Logger.Warn("guardrail approval wait ended without a decision, treating as denied",
					"name", tc.Function.Name, "error", decErr)
				denial = denialTimeout
			} else if decision.Allow {
				if decision.Persist {
					if pErr := a.deps.Guardrails.PersistOverride(a.config.WorkspaceID, toolCategory(tc.Function.Name), tc.Function.Name, tc.Function.Arguments); pErr != nil {
						a.deps.Logger.Warn("failed to persist guardrail override", "error", pErr)
					}
				} else {
					a.deps.Guardrails.MarkOverride(a.config.WorkspaceID, tc.Function.Name)
				}
				return true, false
			}
		}

		mu.Lock()
		a.appendToolResult(history, tc, formatGuardrailError(err, denial))
		mu.Unlock()
		return false, true
	}
	return false, false
}

func isGuardrailSecurityBoundary(err error) bool {
	s := err.Error()
	return strings.Contains(s, "path access denied") ||
		strings.Contains(s, "security violation")
}

func formatGuardrailError(err error, reason guardrailDenialReason) map[string]string {
	// Hermes-aligned denial guidance: the model must not silently work around
	// a block by rephrasing or taking a different path to the same outcome.
	// A timeout is reported as "no response", never as consent.
	var guidance string
	switch reason {
	case denialTimeout:
		guidance = guardrailDeniedByTimeout
	case denialSecurity:
		guidance = guardrailDeniedByPolicy
	default:
		guidance = guardrailDeniedByUser
	}
	return map[string]string{"error": fmt.Sprintf("Guardrail violation: %s. %s", err.Error(), guidance)}
}

func (a *Agent) executePlan(ctx context.Context, history []proxy.Message, plan *ExecutionPlan) (string, []proxy.Message, error) {
	a.deps.Logger.Debug("executing plan", "steps", len(plan.Steps), "description", plan.Description)
	currentHistory := append([]proxy.Message{}, history...)

	if a.config.MaxPlanSteps > 0 && len(plan.Steps) > a.config.MaxPlanSteps {
		return "", currentHistory, fmt.Errorf(planErrMaxSteps, len(plan.Steps), a.config.MaxPlanSteps)
	}

	planCtx := ctx
	var planCancel context.CancelFunc
	if a.config.MaxPlanDuration > 0 {
		planCtx, planCancel = context.WithTimeout(ctx, a.config.MaxPlanDuration)
		defer planCancel()
	}

	toolsList, listErr := a.deps.Provider.ListTools(planCtx)
	if listErr != nil {
		return "", currentHistory, fmt.Errorf("list tools: %w", listErr)
	}

	var mu sync.Mutex
	for i, step := range plan.Steps {
		if a.config.MaxPlanSteps > 0 && i >= a.config.MaxPlanSteps {
			a.deps.Logger.Info("plan execution aborted: max steps exceeded", "step", i, "limit", a.config.MaxPlanSteps)
			return "", currentHistory, fmt.Errorf(planErrStepLimit, a.config.MaxPlanSteps)
		}

		if err := planCtx.Err(); err != nil {
			a.deps.Logger.Info("plan execution halted", "step", i, "error", err)
			return "", currentHistory, fmt.Errorf("plan execution halted: %w", err)
		}

		a.deps.Logger.Debug("plan step", "step", i, "tool", step.ToolName, "description", step.Description)

		argsJSON, err := json.Marshal(step.Parameters)
		if err != nil {
			a.deps.Logger.Info("plan step marshal failed", "step", i, "tool", step.ToolName, "error", err)
			return "", currentHistory, fmt.Errorf("plan step %d: marshal args: %w", i, err)
		}

		tc := proxy.ToolCall{
			ID:   fmt.Sprintf("plan_%d", i),
			Type: "function",
			Function: proxy.FunctionCall{
				Name:      step.ToolName,
				Arguments: string(argsJSON),
			},
		}

		// Enforce the tool manifest schema exactly like the react loop
		// (processToolCalls → validateToolArgs): required parameters present and
		// non-empty. Plan steps bypass the react loop, so the name-only check is
		// not enough — a plan that guesses a parameter name (e.g. file_path vs
		// path) must fail fast instead of executing a tool with an empty value.
		if valErr := validateToolArgs(tc, toolsList); valErr != nil {
			a.deps.Logger.Info("plan step validation failed", "step", i, "tool", step.ToolName, "error", valErr)
			return "", currentHistory, fmt.Errorf("plan step %d: invalid tool call: %w", i, valErr)
		}

		turnMsg := proxy.Message{
			Role:      proxy.AssistantRole,
			ToolCalls: []proxy.ToolCall{tc},
		}
		currentHistory = append(currentHistory, turnMsg)

		stopBatch, execErr := a.executeSingleToolStep(planCtx, tc, &currentHistory, &mu)
		if stopBatch {
			// Guardrail-denied step: the denial is already recorded as a
			// tool-result error by resolveGuardrail. Continue with the
			// remaining plan steps (mirrors processToolCalls) rather than
			// aborting the whole plan on a single denial.
			a.deps.Logger.Warn("plan step guardrail denied, continuing", "step", i, "tool", step.ToolName)
			continue
		}
		if execErr != nil {
			// A tool execution failure (e.g. a shell command exiting non-zero,
			// a compile error, a missing file) is a step outcome, not a plan
			// bug: executeSingleToolStep already appended the error as a tool
			// result, so the finalization turn can report it. Record and
			// continue — mirroring the react loop (processToolCalls) so every
			// strategy tolerates step errors identically and a single failed
			// step never discards the rest of the run. Only structural plan
			// errors (marshal, validation, limits, deadline) abort above.
			a.deps.Logger.Warn("plan step failed, continuing", "step", i, "tool", step.ToolName, "error", execErr)
			continue
		}
	}

	a.deps.Logger.Debug("plan execution complete", "steps", len(plan.Steps))
	return "[Plan execution complete]", currentHistory, nil
}

func toolCategory(toolName string) string {
	switch toolName {
	case models.ToolTerminalExecute:
		return "terminal"
	case models.ToolDirectoryList, models.ToolFileRead, models.ToolFileWrite, models.ToolFileAppend:
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

// toolCtxWithTimeout wraps ctx with the per-tool timeout derived from
// toolCategory and AgentConfig. Returns ctx unchanged when the timeout is 0
// (disabled). Filesystem tools use FilesystemToolTimeout; all others use
// ToolTimeout.
func (a *Agent) toolCtxWithTimeout(ctx context.Context, toolName string) (context.Context, context.CancelFunc) {
	timeout := a.config.ToolTimeout
	if toolCategory(toolName) == "filesystem" && a.config.FilesystemToolTimeout > 0 {
		timeout = a.config.FilesystemToolTimeout
	}
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}

// guardrailCtxWithTimeout wraps ctx with GuardrailTimeout so a slow guardrail
// evaluation (external policy service, DB, expensive computation) cannot block
// the agent loop past the configured bound. Returns ctx unchanged when the
// timeout is 0 (disabled) so guardrail evaluation remains unbounded (legacy).
func (a *Agent) guardrailCtxWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.config.GuardrailTimeout > 0 {
		return context.WithTimeout(ctx, a.config.GuardrailTimeout)
	}
	return ctx, func() {}
}

const salvageMinContentLen = 100 // min chars of recovered content to treat as final report

// salvageTruncatedWrite recovers write/append content from invalid (often truncated)
// tool args, optionally best-effort persists to the intended path when recoverable,
// and always returns the report for natural completion. Persist failure never aborts.
// handled=false means this call is not a salvageable write/append.
func (a *Agent) salvageTruncatedWrite(
	ctx context.Context,
	tc proxy.ToolCall,
	history *[]proxy.Message,
	mu *sync.Mutex,
) (report string, handled bool) {
	report = trySalvageWriteContent(tc)
	if report == "" {
		return "", false
	}

	path := strings.TrimSpace(extractToolArgField(tc.Function.Arguments, "path"))
	resultMeta := map[string]string{
		"warning":  "arguments truncated; report recovered from partial content",
		"chars":    fmt.Sprintf("%d", len(report)),
		"salvaged": "true",
	}

	a.deps.Logger.Info("salvaged truncated write content as final report",
		"name", tc.Function.Name, "chars", len(report), "path", path)

	if path == "" {
		a.deps.Logger.Warn("salvaged without path; workspace file not updated",
			"name", tc.Function.Name)
		resultMeta["persisted"] = "false"
		resultMeta["reason"] = "path not recoverable from truncated args"
	} else {
		resultMeta["path"] = path
		if err := a.persistSalvagedWrite(ctx, tc, path, report); err != nil {
			a.deps.Logger.Warn("salvaged write persist failed; completing with recovered report",
				"name", tc.Function.Name, "path", path, "error", err)
			resultMeta["persisted"] = "false"
			resultMeta["persist_error"] = err.Error()
		} else {
			a.deps.Logger.Info("salvaged write persisted to workspace",
				"name", tc.Function.Name, "path", path, "chars", len(report))
			resultMeta["persisted"] = "true"
		}
	}

	mu.Lock()
	a.appendToolResult(history, tc, resultMeta)
	mu.Unlock()
	a.notifyToolCall(tc)
	a.notifyToolResult(tc.ID, tc.Function.Name, resultMeta)
	return report, true
}

// persistSalvagedWrite rebuilds valid write/append args and executes via Engine.
// Guardrail denial or FS errors are returned to the caller; completion still proceeds.
func (a *Agent) persistSalvagedWrite(ctx context.Context, tc proxy.ToolCall, path, content string) error {
	argsJSON, err := json.Marshal(map[string]string{
		"path":    path,
		"content": content,
	})
	if err != nil {
		return fmt.Errorf("marshal salvaged args: %w", err)
	}
	fixed := proxy.ToolCall{
		ID:   tc.ID,
		Type: tc.Type,
		Function: proxy.FunctionCall{
			Name:      tc.Function.Name,
			Arguments: string(argsJSON),
		},
	}
	if fixed.Type == "" {
		fixed.Type = "function"
	}

	if a.deps.Guardrails != nil {
		if grErr := a.deps.Guardrails.ValidateToolCall(ctx, fixed, a.config.WorkspaceID); grErr != nil {
			return fmt.Errorf("guardrail: %w", grErr)
		}
	}
	if a.deps.Engine == nil {
		return fmt.Errorf("engine not configured")
	}

	toolCtx := models.WithWorkspaceID(ctx, a.config.WorkspaceID)
	_, execErr := a.deps.Engine.ExecuteTool(toolCtx, fixed)
	if execErr != nil {
		return execErr
	}
	if t := GetUsageTracker(ctx); t != nil {
		t.AddToolCall(tc.Function.Name)
	}
	return nil
}

// trySalvageWriteContent recovers a truncated write_file/append_file content
// field so long Markdown reports still complete when native tool-arg JSON is cut.
func trySalvageWriteContent(tc proxy.ToolCall) string {
	switch tc.Function.Name {
	case models.ToolFileWrite, models.ToolFileAppend:
	default:
		return ""
	}
	content := extractToolArgField(tc.Function.Arguments, "content")
	if len(strings.TrimSpace(content)) < salvageMinContentLen {
		return ""
	}
	return content
}

// extractToolArgField returns a string field from tool-call args JSON.
// Uses normal unmarshal first, then truncated-JSON walk for incomplete args.
func extractToolArgField(raw, field string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err == nil {
		if val, ok := args[field].(string); ok {
			return val
		}
		return ""
	}
	return extractTruncatedJSONField(raw, field)
}

// extractTruncatedJSONField extracts a string field from JSON that may be cut
// mid-value (missing closing quote/brace). Decodes escapes via strconv.Unquote.
func extractTruncatedJSONField(raw, field string) string {
	start := indexJSONStringValueStart(raw, field)
	if start < 0 || start >= len(raw) {
		return ""
	}
	body := scanJSONStringBody(raw[start:])
	if body == "" {
		return ""
	}
	decoded, err := strconv.Unquote(`"` + body + `"`)
	if err == nil {
		return decoded
	}
	// Truncation mid-escape (e.g. trailing \): drop incomplete tail and retry.
	if trimmed := trimIncompleteJSONEscape(body); trimmed != body {
		if decoded, err = strconv.Unquote(`"` + trimmed + `"`); err == nil {
			return decoded
		}
		body = trimmed
	}
	// Last resort: return body with common complete escapes applied.
	return fallbackDecodeJSONString(body)
}

// indexJSONStringValueStart finds the byte offset of a JSON string value for field.
// Accepts "field":", "field": ", and "field" : " forms used by models/servers.
func indexJSONStringValueStart(raw, field string) int {
	prefixes := []string{
		`"` + field + `":"`,
		`"` + field + `": "`,
		`"` + field + `" : "`,
	}
	for _, p := range prefixes {
		if i := strings.Index(raw, p); i != -1 {
			return i + len(p)
		}
	}
	return -1
}

// scanJSONStringBody returns the raw JSON string contents (still escaped) up to
// an unescaped closing quote, or the remainder if the value was truncated.
func scanJSONStringBody(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip escaped byte; may be past end if truncated
			continue
		}
		if s[i] == '"' {
			return s[:i]
		}
	}
	return s
}

func trimIncompleteJSONEscape(s string) string {
	if strings.HasSuffix(s, `\`) && !strings.HasSuffix(s, `\\`) {
		return strings.TrimSuffix(s, `\`)
	}
	// Incomplete \uXXXX
	if i := strings.LastIndex(s, `\u`); i >= 0 && len(s)-i < 6 {
		// Ensure the \u is not itself escaped (odd number of backslashes before u is rare; simple check)
		if i == 0 || s[i-1] != '\\' {
			return s[:i]
		}
	}
	return s
}

func fallbackDecodeJSONString(s string) string {
	replacer := strings.NewReplacer(
		`\n`, "\n",
		`\t`, "\t",
		`\r`, "\r",
		`\"`, `"`,
		`\\`, `\`,
	)
	return replacer.Replace(s)
}

// validateToolArgs checks required parameters from the tool's JSON schema
// against the actual call arguments.  Returns a descriptive error so the
// model can self-correct in the next turn.
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
		// Try to fix common JSON issues (unescaped quotes in string values)
		// before rejecting.  The model often produces valid tool names but
		// malformed JSON with unescaped quotes in content fields.
		sanitized := sanitizeToolArgs(tc.Function.Arguments, err)
		if sanitized != "" {
			if err2 := json.Unmarshal([]byte(sanitized), &args); err2 == nil {
				tc.Function.Arguments = sanitized
				goto validated
			}
		}
		return fmt.Errorf("failed to parse arguments as JSON: %w", err)
	}
validated:
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

// sanitizeToolArgs attempts to fix common JSON syntax issues in model-produced
// tool call arguments.  Returns the sanitized string or "" if no fix is possible.
// Handles the most frequent case: unescaped double quotes inside string values.
func sanitizeToolArgs(raw string, originalErr error) string {
	if raw == "" {
		return ""
	}
	if isTruncationError(originalErr.Error()) {
		return ""
	}

	// Walk the JSON tracking string boundaries with a simple state machine.
	// When we encounter a '"' that lies inside a value string rather than
	// at a structural position (object key, array element), escape it.
	var result strings.Builder
	result.Grow(len(raw) + 64)
	inKey := false      // currently inside an object key
	inStr := false      // currently inside a string (key or value)
	afterColon := false // the last structural token was ':'

	for i := 0; i < len(raw); i++ {
		b := raw[i]

		if b == '\\' {
			result.WriteByte(b)
			if i+1 < len(raw) {
				i++
				result.WriteByte(raw[i])
			}
			continue
		}

		if b == '"' {
			if !inStr {
				// Opening quote — determine if this is a key or value
				inStr = true
				inKey = !afterColon
				afterColon = false
				result.WriteByte(b)
				continue
			}
			// Closing quote or embedded quote inside value
			inStr = false
			if inKey {
				inKey = false
				result.WriteByte(b)
				continue
			}
			// We were in a value string.  Peek ahead to see if this quote
			// is structural (closing the value) or embedded (needs escaping).
			peekEnd := i + 1
			for peekEnd < len(raw) && raw[peekEnd] == ' ' {
				peekEnd++
			}
			if peekEnd >= len(raw) || raw[peekEnd] == ',' || raw[peekEnd] == '}' || raw[peekEnd] == ']' {
				// Structural closing quote — keep as-is
				afterColon = false
				result.WriteByte(b)
				continue
			}
			// Embedded quote — escape it
			result.WriteByte('\\')
			result.WriteByte(b)
			inStr = true // still inside the string
			continue
		}

		if b == ':' && !inStr {
			afterColon = true
			result.WriteByte(b)
			continue
		}

		if (b == ',' || b == '{' || b == '[') && !inStr {
			afterColon = false
			result.WriteByte(b)
			continue
		}

		result.WriteByte(b)
	}

	sanitized := result.String()
	if sanitized == raw {
		return ""
	}
	if err := json.Unmarshal([]byte(sanitized), &map[string]any{}); err != nil {
		return ""
	}
	return sanitized
}

// appendToolResult marshals the tool result, truncates it (proxy.TruncateResult
// caps at ~8KB to avoid blowing the context window), and appends as a tool-role
// message linked to the original call ID. It returns the truncated content so
// callers can reuse it (e.g. for logging) without re-marshaling (O5).
func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) string {
	raw, _ := json.Marshal(result)
	strContent := proxy.TruncateResultDefault(string(raw))
	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    strContent,
		ToolCallID: tc.ID,
	})
	return strContent
}
