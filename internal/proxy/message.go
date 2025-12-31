package proxy

type ChatRole string

const (
	SystemRole ChatRole = "system"
	UserRole   ChatRole = "user"
)

// Message
type Message struct {
	Role    ChatRole `json:"role"`
	Content string   `json:"content"`
}

// Chat Request
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
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
	Message   Message    `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Chat Response
type ChatResponse struct {
	Choices []Choice `json:"choices"`
}
