package workspace

import (
"context"

"llm-proxy/models"
)

// AgentExecutor is the decoupled interface for executing heartbeats.
// This allows swapping local engines, remote services, or mock implementations.
type AgentExecutor interface {
	Execute(ctx context.Context, prompt string, state *models.AgentState) (string, error)
}
