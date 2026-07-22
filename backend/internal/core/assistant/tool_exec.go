// tool_exec.go — Tool call validation, guardrail resolution, execution via
// Engine, and result appending.  Also holds ExecutionPlan, ExecutionPlanStrategy
// (moved from strategy_plan.go), formatGuardrailError, toolCategory,
// validateToolArgs.
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	resultBytes, _ := json.Marshal(finalResult)
	a.deps.Logger.Debug("tool execution completed", "name", tc.Function.Name, "error", err, "result", string(resultBytes))
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

		stopBatch, execErr := a.executeSingleToolStep(ctx, tc, &currentHistory, &mu)
		if stopBatch {
			a.deps.Logger.Warn("plan step guardrail denied", "step", i, "tool", step.ToolName)
			return "", currentHistory, fmt.Errorf("plan step %d: guardrail denied %s", i, step.ToolName)
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

// salvageReportFromToolCalls returns the first recoverable write/append report
// from a tool-call batch. Shared by processToolCalls and history fallback.
func salvageReportFromToolCalls(calls []proxy.ToolCall) string {
	for _, tc := range calls {
		if report := trySalvageWriteContent(tc); report != "" {
			return report
		}
	}
	return ""
}

// salvageReportFromHistory walks history newest-first and salvages truncated
// write_file/append_file args from assistant tool calls.
func salvageReportFromHistory(history []proxy.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != proxy.AssistantRole {
			continue
		}
		if report := salvageReportFromToolCalls(history[i].ToolCalls); report != "" {
			return report
		}
	}
	return ""
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
