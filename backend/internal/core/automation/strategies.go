package automation

import (
	"context"
	"fmt"

	"llm-proxy/models"
)

// ExecutionStrategy determines how previous state is included in the execution context.
type ExecutionStrategy interface {
	Prepare(ctx context.Context, workspaceID string, automationName string, state *models.AgentState) (context.Context, error)
	Name() string
}

// IsolatedStrategy: Fresh context, no memory from previous runs.
type IsolatedStrategy struct{}

func (s *IsolatedStrategy) Prepare(ctx context.Context, workspaceID, automationName string, state *models.AgentState) (context.Context, error) {
	return ctx, nil
}

func (s *IsolatedStrategy) Name() string {
	return "isolated"
}

// PersistentStrategy: Injects previous state.json into the execution context.
type PersistentStrategy struct{}

func (s *PersistentStrategy) Prepare(ctx context.Context, workspaceID, automationName string, state *models.AgentState) (context.Context, error) {
	if state == nil {
		return ctx, nil
	}
	ctx = context.WithValue(ctx, contextKeyState{}, state)
	return ctx, nil
}

func (s *PersistentStrategy) Name() string {
	return "persistent"
}

type contextKeyState struct{}

func NewStrategy(name string) (ExecutionStrategy, error) {
	switch name {
	case "isolated":
		return &IsolatedStrategy{}, nil
	case "persistent":
		return &PersistentStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q (expected 'isolated' or 'persistent')", name)
	}
}

func StrategyFromAutomation(auto *models.Automation) ExecutionStrategy {
	if auto.Strategy == "" || auto.Strategy == "isolated" {
		return &IsolatedStrategy{}
	}
	if auto.Strategy == "persistent" {
		return &PersistentStrategy{}
	}
	return &IsolatedStrategy{}
}
