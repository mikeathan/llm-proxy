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

// DefaultTaskExecutor is a placeholder that marks execution as running.
// In Phase 3, this will be replaced with a proper LLM-backed executor.
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
