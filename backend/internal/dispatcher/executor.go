package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
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
// It avoids an import cycle by not depending on the api package.
type LLMServiceProvider interface {
	ClientProvider() proxy.LLMClientProvider
	NodeHerder() nodeherder.MCPService
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

	// Build messages
	var history []proxy.Message

	// Add system prompt
	systemPrompt, err := e.svc.NodeHerder().GetSystemPrompt()
	if err == nil && systemPrompt != "" {
		history = append(history, proxy.Message{Role: "system", Content: systemPrompt})
	}

	// Build task prompt from automation info
	prompt := e.buildPrompt(req)
	history = append(history, proxy.Message{Role: "user", Content: prompt})

	// Get tools
	tools, err := e.svc.NodeHerder().ListTools(ctx)
	if err != nil {
		e.svc.Logger().Warn("Failed to list tools for automation", "error", err)
		// Continue without tools
	}

	// Make LLM call
	chatReq := proxy.ChatRequest{
		Messages: history,
		Tools:    tools,
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
