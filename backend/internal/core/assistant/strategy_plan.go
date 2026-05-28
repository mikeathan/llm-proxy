package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

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
		MaxTokens: 4096,
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
