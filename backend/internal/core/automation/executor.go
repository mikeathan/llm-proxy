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
	Strategy       ExecutionStrategy
	State          *models.AgentState
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
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	// Mark as running
	resp.State.IsRunning = true

	// Get LLM client
	clientProvider := e.svc.ClientProvider()
	client, err := clientProvider.GetClient(ctx)
	if err != nil {
		resp.State.LastError = fmt.Sprintf("failed to get llm client: %v", err)
		resp.State.IsRunning = false
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
		resp.State.LastError = fmt.Sprintf("llm chat failed: %v", err)
		resp.State.IsRunning = false
		return resp, fmt.Errorf("llm chat failed: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		resp.State.LastError = "no response choices"
		resp.State.IsRunning = false
		return resp, fmt.Errorf("no response choices")
	}

	choice := chatResp.Choices[0]
	if choice.Message.Content != "" {
		resp.Output = choice.Message.Content
		resp.State.LastOutput = choice.Message.Content
	}

	if len(choice.Message.ToolCalls) > 0 {
		resp.Output = fmt.Sprintf("Called %d tools (e.g., %s)", len(choice.Message.ToolCalls), choice.Message.ToolCalls[0].Function.Name)
		resp.State.LastOutput = resp.Output
	}

	resp.State.IsRunning = false
	return resp, nil
}

// buildPrompt constructs the user prompt from automation details.
func (e *LLMTaskExecutor) buildPrompt(req ExecuteRequest) string {
	return fmt.Sprintf("Execute automation '%s' in workspace '%s'.\nTask: %s",
		req.AutomationName, req.WorkspaceID, req.TaskFile)
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
