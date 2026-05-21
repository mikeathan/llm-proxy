package models

type ChatRole string
type ToolChoice string

const (
	SystemRole    ChatRole = "system"
	UserRole      ChatRole = "user"
	AssistantRole ChatRole = "assistant"
	ToolRole      ChatRole = "tool"

	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceRequired ToolChoice = "required"
	ToolChoiceNone     ToolChoice = "none"
)

type ExecutionHistory = []Message

// Message

type Message struct {
	Role             ChatRole   `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// Chat Request
type ChatRequest struct {
	Model           string     `json:"model"`
	Messages        []Message  `json:"messages"`
	MaxTokens       int        `json:"max_tokens,omitempty"`
	Temperature     float64    `json:"temperature,omitempty"`
	ReasoningBudget int        `json:"reasoning_budget,omitempty"`
	Tools           []Tool     `json:"tools,omitempty"`
	ToolChoice      ToolChoice `json:"tool_choice,omitempty"`
	Stream          bool       `json:"stream,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Choice struct {
	Message Message `json:"message"`
	Delta   Message `json:"delta,omitempty"`
}

// Chat Response
type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

type Tool struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}
