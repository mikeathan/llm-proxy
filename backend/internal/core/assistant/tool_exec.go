// tool_exec.go — Tool call validation, guardrail resolution, execution via
// Engine, and result appending.  Also holds ExecutionPlan, ExecutionPlanStrategy
// (moved from strategy_plan.go), formatGuardrailError, toolCategory,
// extractTaskSummary, validateToolArgs.
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

const planGenMaxTokens = 4096 // max tokens for plan generation LLM call

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
}

func NewExecutionPlanStrategy(llm proxy.Client, tools []proxy.Tool, logger logging.Logger) *ExecutionPlanStrategy {
	return &ExecutionPlanStrategy{llm: llm, tools: tools, logger: logger}
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

	resp, err := s.llm.Chat(ctx, req)
	if err != nil {
		s.logger.Info("plan generation LLM call failed", "error", err)
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		s.logger.Info("plan generation returned no choices")
		return nil, fmt.Errorf("plan generation returned no choices")
	}

	content := resp.Choices[0].Message.Content
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
		s.logger.Info("plan parse failed", "error", err, "raw_length", len(content))
		return nil, fmt.Errorf("plan parse failed: %w", err)
	}

	if len(plan.Steps) == 0 {
		s.logger.Info("plan generated with zero steps", "description", plan.Description)
		return nil, fmt.Errorf("plan has no steps")
	}

	s.logger.Debug("execution plan generated", "steps", len(plan.Steps), "description", plan.Description)
	return &plan, nil
}

// processToolCalls validates tool args, resolves guardrails, and executes
// via Engine.  Batched submit_final_answer is rejected (only single submission
// allowed).  A guardrail denial stops the entire batch.
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
		a.deps.Logger.Warn("rejected batched submission", "count", len(msg.ToolCalls))
		mu.Lock()
		errorMsg := prompts.AutomationRejectedSubmissionPrompt
		for _, tc := range msg.ToolCalls {
			a.appendToolResult(history, tc, map[string]string{"error": errorMsg})
		}
		mu.Unlock()
		return nil
	}

	toolsList, listErr := a.deps.Provider.ListTools(ctx)
	if listErr != nil {
		a.deps.Logger.Error("failed to list tools for validation", "error", listErr)
		return fmt.Errorf("list tools: %w", listErr)
	}

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tc.Type != "" && tc.Type != "function" {
			continue
		}

		a.deps.Logger.Debug("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)
		a.deps.Logger.Info("executing tool", "name", tc.Function.Name)

		if err := validateToolArgs(tc, toolsList); err != nil {
			a.deps.Logger.Warn("tool argument validation failed", "name", tc.Function.Name, "error", err)
			errMsg := fmt.Sprintf("INVALID ARGUMENTS: %v", err)
			if isTruncationError(err.Error()) {
				errMsg = prompts.AutomationContentTooLongPrompt
			}
			mu.Lock()
			a.appendToolResult(history, tc, map[string]string{"error": errMsg})
			mu.Unlock()
			return nil
		}

		approved, stopBatch := a.resolveGuardrail(ctx, tc, history, &mu)
		if stopBatch {
			return nil
		}

		a.notifyToolCall(tc)
		toolCtx := models.WithWorkspaceID(ctx, a.config.WorkspaceID)
		if approved {
			toolCtx = models.WithGuardrailApproved(toolCtx)
		}
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
		a.appendToolResult(history, tc, finalResult)
		resultStr, _ := json.Marshal(finalResult)
		a.deps.Logger.Debug("tool execution completed", "name", tc.Function.Name, "error", err, "result", string(resultStr))
		a.deps.Logger.Info("tool execution completed", "name", tc.Function.Name, "error", err)
		a.notifyToolResult(tc.ID, tc.Function.Name, finalResult)
		if t := GetUsageTracker(ctx); t != nil {
			t.AddToolCall(tc.Function.Name)
		}
		mu.Unlock()

		if err != nil {
			a.deps.Logger.Warn("tool execution failed - stopping batch", "name", tc.Function.Name, "error", err)
			return nil
		}
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			return nil
		}

	}
	return nil
}

func (a *Agent) resolveGuardrail(ctx context.Context, tc proxy.ToolCall, history *[]proxy.Message, mu *sync.Mutex) (approved, stopBatch bool) {
	if err := a.deps.Guardrails.ValidateToolCall(ctx, tc, a.config.WorkspaceID); err != nil {
		a.deps.Logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
		a.notifyGuardrailViolation(tc.Function.Name, err)

		// Security boundary violations (path outside workspace, blocked system files)
		// are denied immediately — no approval dialog.
		if isGuardrailSecurityBoundary(err) {
			mu.Lock()
			a.appendToolResult(history, tc, formatGuardrailError(err))
			mu.Unlock()
			return false, true
		}

		if a.deps.OnGuardrail != nil {
			decision, decErr := a.deps.OnGuardrail(ctx, GuardrailBlockedPayload{
				DecisionID: fmt.Sprintf("gr_%d", time.Now().UnixNano()),
				Tool:       tc.Function.Name,
				Args:       tc.Function.Arguments,
				Reason:     err.Error(),
				Category:   toolCategory(tc.Function.Name),
			})
			if decErr == nil && decision.Allow {
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
		a.appendToolResult(history, tc, formatGuardrailError(err))
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

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
}

func (a *Agent) executePlan(ctx context.Context, history []proxy.Message, plan *ExecutionPlan) (string, []proxy.Message, error) {
	a.deps.Logger.Debug("executing plan", "steps", len(plan.Steps), "description", plan.Description)
	currentHistory := append([]proxy.Message{}, history...)

	toolsList, listErr := a.deps.Provider.ListTools(ctx)
	if listErr != nil {
		return "", currentHistory, fmt.Errorf("list tools: %w", listErr)
	}

	var mu sync.Mutex
	for i, step := range plan.Steps {
		if err := ctx.Err(); err != nil {
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

		if valErr := proxy.ValidateToolCall(tc, toolsList); valErr != nil {
			a.deps.Logger.Info("plan step validation failed", "step", i, "tool", step.ToolName, "error", valErr)
			return "", currentHistory, fmt.Errorf("plan step %d: invalid tool call: %w", i, valErr)
		}

		turnMsg := proxy.Message{
			Role:      proxy.AssistantRole,
			ToolCalls: []proxy.ToolCall{tc},
		}
		currentHistory = append(currentHistory, turnMsg)

		approved, stopBatch := a.resolveGuardrail(ctx, tc, &currentHistory, &mu)
		if stopBatch {
			a.deps.Logger.Warn("plan step guardrail denied", "step", i, "tool", step.ToolName)
			return "", currentHistory, fmt.Errorf("plan step %d: guardrail denied %s", i, step.ToolName)
		}

		a.notifyToolCall(tc)

		toolCtx := models.WithWorkspaceID(ctx, a.config.WorkspaceID)
		if approved {
			toolCtx = models.WithGuardrailApproved(toolCtx)
		}

		result, execErr := a.deps.Engine.ExecuteTool(toolCtx, tc)
		var finalResult any
		if execErr != nil {
			finalResult = map[string]string{"error": execErr.Error()}
		} else {
			finalResult = result
		}
		a.appendToolResult(&currentHistory, tc, finalResult)
		a.notifyToolResult(tc.ID, tc.Function.Name, finalResult)
		if t := GetUsageTracker(ctx); t != nil {
			t.AddToolCall(step.ToolName)
		}

		if execErr != nil {
			a.deps.Logger.Info("plan step failed", "step", i, "tool", step.ToolName, "error", execErr)
			return "", currentHistory, fmt.Errorf("plan step %d failed: %w", i, execErr)
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

// extractTaskSummary walks submit_final_answer args looking for a human-readable
// summary.  Priority is: summary > message > report > findings > content > result,
// then falls back to "Task complete." if nothing meaningful is found.
func extractTaskSummary(rawArgs string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		for _, key := range []string{"summary", "message", "report", "findings", "content", "result"} {
			if s := extractTruncatedJSONField(rawArgs, key); s != "" {
				return s
			}
		}
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

// extractTruncatedJSONField extracts a string field value from truncated JSON
// where the closing quote or brace may be missing.  Decodes JSON escape
// sequences (\n, \t, \", \\, etc.) in the extracted content.
func extractTruncatedJSONField(raw, field string) string {
	prefix := `"` + field + `": "`
	idx := strings.Index(raw, prefix)
	if idx == -1 {
		prefix = `"` + field + `" : "`
		idx = strings.Index(raw, prefix)
		if idx == -1 {
			return ""
		}
	}
	start := idx + len(prefix)
	if start >= len(raw) {
		return ""
	}
	content := raw[start:]
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		b := content[i]
		if b == '\\' && i+1 < len(content) {
			next := content[i+1]
			switch next {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			case 'r':
				out.WriteByte('\r')
			case '\\':
				out.WriteByte('\\')
			case '"':
				out.WriteByte('"')
			default:
				out.WriteByte('\\')
				out.WriteByte(next)
			}
			i++
			continue
		}
		if b == '"' {
			break
		}
		out.WriteByte(b)
	}
	return out.String()
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
// message linked to the original call ID.
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
