package app

import (
	"context"
	"fmt"

	"llm-proxy/internal/api"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/workspace"
	"llm-proxy/models"
)

type defaultWorkspaceExecutor struct {
	svc api.AssistantService
}

func NewWorkspaceExecutor(svc api.AssistantService) workspace.AgentExecutor {
	return &defaultWorkspaceExecutor{svc: svc}
}

func (e *defaultWorkspaceExecutor) Execute(ctx context.Context, prompt string, state *models.AgentState) (string, error) {
	clientProvider := e.svc.ClientProvider()
	
	client, err := clientProvider.GetClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get llm client: %w", err)
	}

	// Prepare history
	var history []proxy.Message
	
	// Add system prompt from NodeHerder if desired?
	systemPrompt, err := e.svc.NodeHerder().GetSystemPrompt()
	if err == nil && systemPrompt != "" {
		history = append(history, proxy.Message{Role: "system", Content: systemPrompt})
	}

	history = append(history, proxy.Message{Role: "user", Content: prompt})

	// Get tools
	tools, err := e.svc.NodeHerder().ListTools(ctx)
	if err != nil {
		e.svc.Logger().Warn("Failed to list tools for heartbeat", "error", err)
	}

	// Simple non-loop execution for now (or could implement tool loop)
	req := proxy.ChatRequest{
		Messages: history,
		Tools:    tools,
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("llm chat failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	// If the model invoked tools, we would ideally execute them. For this phase, just return the raw or tool call output.
	choice := resp.Choices[0]
	if choice.Message.Content != "" {
		return choice.Message.Content, nil
	}
	
	if len(choice.Message.ToolCalls) > 0 {
		return fmt.Sprintf("Called %d tools (e.g., %s)", len(choice.Message.ToolCalls), choice.Message.ToolCalls[0].Function.Name), nil
	}

	return "", fmt.Errorf("empty response")
}
