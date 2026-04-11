package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// TaskExecutor executes automations.
type TaskExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
}

type ExecuteRequest struct {
	WorkspaceID    string
	AutomationName string
	TaskFile       string
	TaskContent    string
	Strategy       ExecutionStrategy
	State          *models.AgentState
	Model          string // Optional model override
}

type ExecuteResponse struct {
	Output string
	Error  error
	State  *models.AgentState
}

// LLMServiceProvider is the minimal interface needed by LLMTaskExecutor.
// It avoids an import cycle by not depending on the api or nodeherder packages.
type LLMServiceProvider interface {
	ClientProvider() proxy.LLMClientProvider
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	Logger() logging.Logger
}

// DefaultTaskExecutor is a placeholder that marks execution as running.
// Kept for cases where LLM execution is not required.
type DefaultTaskExecutor struct{}

func NewDefaultTaskExecutor() *DefaultTaskExecutor {
	return &DefaultTaskExecutor{}
}

func (e *DefaultTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	resp.State.IsRunning = true
	resp.Output = "dispatcher placeholder output (Phase 3: wire LLM)"

	return resp, nil
}

// LLMTaskExecutor is a TaskExecutor that uses an LLM client for execution.
type LLMTaskExecutor struct {
	svc LLMServiceProvider
}

func NewLLMTaskExecutor(svc LLMServiceProvider) TaskExecutor {
	return &LLMTaskExecutor{svc: svc}
}

func (e *LLMTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	startTime := time.Now()
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	// Mark as running
	resp.State.IsRunning = true

	// Get LLM client
	var client proxy.Client
	var err error
	if req.Model != "" {
		client, err = e.svc.GetClientForModel(ctx, req.Model)
	} else {
		clientProvider := e.svc.ClientProvider()
		client, err = clientProvider.GetClient(ctx)
	}
	
	if err != nil {
		errStr := fmt.Sprintf("failed to get llm client: %v", err)
		resp.State.LastError = errStr
		resp.State.IsRunning = false
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime))
		return resp, fmt.Errorf("failed to get llm client: %w", err)
	}

	// Build messages - simple user prompt for automation execution
	var history []proxy.Message

	// Build task prompt from automation info
	prompt := e.buildPrompt(req)
	history = append(history, proxy.Message{Role: "user", Content: prompt})

	// Make LLM call (no tools for dispatcher automations)
	chatReq := proxy.ChatRequest{
		Messages: history,
	}

	chatResp, err := client.Chat(ctx, chatReq)
	if err != nil {
		errStr := fmt.Sprintf("llm chat failed: %v", err)
		resp.State.LastError = errStr
		resp.State.IsRunning = false
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime))
		return resp, fmt.Errorf("llm chat failed: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		errStr := "no response choices"
		resp.State.LastError = errStr
		resp.State.IsRunning = false
		e.recordRun(req, resp.State, "", errStr, time.Since(startTime))
		return resp, fmt.Errorf("no response choices")
	}

	choice := chatResp.Choices[0]
	var runResult string
	var runError string

	if choice.Message.Content != "" {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		header := fmt.Sprintf("[%s] ▶ Executing automation `%s` in workspace `%s`...\nReading task file: `%s`\n\n",
			timestamp, req.AutomationName, req.WorkspaceID, req.TaskFile)

		output := strings.TrimSpace(choice.Message.Content)

		// If output already includes a code block, don't wrap it again
		formattedOutput := output
		if !strings.Contains(output, "```") {
			formattedOutput = fmt.Sprintf("```text\n%s\n```", output)
		}

		fullOutput := fmt.Sprintf("%s**Output:**\n\n%s",
			header, formattedOutput)

		resp.Output = fullOutput
		resp.State.LastOutput = fullOutput
		runResult = fullOutput
	}

	if len(choice.Message.ToolCalls) > 0 {
		resp.Output = fmt.Sprintf("Called %d tools (e.g., %s)", len(choice.Message.ToolCalls), choice.Message.ToolCalls[0].Function.Name)
		resp.State.LastOutput = resp.Output
		runResult = resp.Output
	}

	resp.State.IsRunning = false
	
	// Record to history
	e.recordRun(req, resp.State, runResult, runError, time.Since(startTime))

	return resp, nil
}

func (e *LLMTaskExecutor) recordRun(req ExecuteRequest, state *models.AgentState, output, errStr string, duration time.Duration) {
	if state == nil {
		return
	}

	run := models.AutomationRun{
		ID:             generateRunID(),
		WorkspaceID:    req.WorkspaceID,
		AutomationName: req.AutomationName,
		Timestamp:      time.Now(),
		Output:         output,
		Error:          errStr,
		DurationMs:     duration.Milliseconds(),
		Model:          req.Model,
	}

	// Add to full history (capped to last 50 for performance)
	state.History = append(state.History, run)
	if len(state.History) > 50 {
		state.History = state.History[len(state.History)-50:]
	}

	// Update per-automation latest run
	if state.LastRuns == nil {
		state.LastRuns = make(map[string]*models.AutomationRun)
	}
	state.LastRuns[req.AutomationName] = &run
}

func generateRunID() string {
	// Simple unique-ish ID without importing full UUID package if not strictly needed
	// but I checked go.mod and it's there. 
	// For now, use timestamp + nano for simplicity if I want to avoid adding the import
	// but let's just use UUID if I can.
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

// buildPrompt constructs the user prompt from automation details.
func (e *LLMTaskExecutor) buildPrompt(req ExecuteRequest) string {
	return fmt.Sprintf("TASK: Execute automation '%s' in workspace '%s'.\nFILE: %s\n\nCONTENT:\n%s\n\nINSTRUCTION: Finalize this task. Respond ONLY with the result. Do not repeat this header or mirror the task details.",
		req.AutomationName, req.WorkspaceID, req.TaskFile, req.TaskContent)
}

// ============================================================================
// Pulse Logic (Smart Skip)
// ============================================================================

const heartbeatOKMarker = "HEARTBEAT_OK"

func isHeartbeatOK(output string) bool {
	return strings.Contains(output, heartbeatOKMarker)
}

// ApplyPulseLogic applies the Smart Skip logic to suppress noisy heartbeat output.
func ApplyPulseLogic(resp *ExecuteResponse) {
	if isHeartbeatOK(resp.Output) {
		resp.Output = ""
		resp.State.LastPulse = time.Now()
	}
}
