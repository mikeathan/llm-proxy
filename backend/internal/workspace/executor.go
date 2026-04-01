package workspace

import (
	"context"

	"llm-proxy/models"
)

type AgentExecutor interface {
	Execute(ctx context.Context, prompt string, state *models.AgentState) (string, error)
}
