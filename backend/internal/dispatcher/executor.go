package dispatcher

import (
	"context"
	"strings"
	"time"

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

// DefaultTaskExecutor is the default implementation backed by LLM calls.
type DefaultTaskExecutor struct {
	llmService LLMService
}

type LLMService interface {
	Complete(ctx context.Context, model, prompt string, temperature float64) (string, error)
}

func NewDefaultTaskExecutor(llm LLMService) *DefaultTaskExecutor {
	return &DefaultTaskExecutor{llmService: llm}
}

func (e *DefaultTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	resp := &ExecuteResponse{
		State: req.State,
	}

	if resp.State == nil {
		resp.State = &models.AgentState{}
	}

	resp.State.IsRunning = true

	return resp, nil
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
