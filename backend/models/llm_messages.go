package models

import "strings"

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

type ResponseFormat struct {
	Type   string      `json:"type,omitempty"`
	Name   string      `json:"name,omitempty"`
	Schema interface{} `json:"schema,omitempty"`
}

// Message

type Message struct {
	Role             ChatRole          `json:"role"`
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // llama.cpp / Qwen / NVIDIA: structured reasoning field
	Reasoning        string            `json:"reasoning,omitempty"`         // openrouter / some gateways: opaque reasoning string
	ReasoningDetails []ReasoningDetail `json:"reasoning_details,omitempty"` // openrouter: structured reasoning parts
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
	// Error carries a terminal run failure (e.g. upstream LLM error) as a
	// persisted, assistant-role message so a reloaded session can render the
	// failure as an error segment instead of a blank turn. Never sent to the
	// model as prompt context — it is a UI/observability marker only.
	Error string `json:"error,omitempty"`
	// FinishReason is the upstream finish_reason of the generation that
	// produced this message ("stop", "length", "tool_calls", ...). It is an
	// in-memory stream signal only — never serialized to the wire, because
	// strict OpenAI-compatible providers (Mistral, Moonshot, ...) reject
	// unknown message keys when history is replayed (mirrors Hermes's
	// finish_reason stripping in chat_completion_helpers.py).
	FinishReason string `json:"-"`
}

// ReasoningDetail models openrouter-style structured reasoning parts
// (summary / thinking / content / text). Single unmarshal, zero cost when absent.
type ReasoningDetail struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// Chat Request
type ChatRequest struct {
	Model                string              `json:"model"`
	Messages             []Message           `json:"messages"`
	MaxTokens            int                 `json:"max_tokens,omitempty"`
	Temperature          float64             `json:"temperature,omitempty"`
	ReasoningBudget      int                 `json:"reasoning_budget,omitempty"`
	ThinkingBudgetTokens int                 `json:"thinking_budget_tokens,omitempty"`
	ReasoningEffort      string              `json:"reasoning_effort,omitempty"`     // openai / gemini
	Reasoning            *ReasoningObject    `json:"reasoning,omitempty"`            // openrouter
	ChatTemplateKwargs   *ChatTemplateKwargs `json:"chat_template_kwargs,omitempty"` // nvidia
	Tools                []Tool              `json:"tools,omitempty"`
	ToolChoice           ToolChoice          `json:"tool_choice,omitempty"`
	Stream               bool                `json:"stream,omitempty"`
	ResponseFormat       *ResponseFormat     `json:"response_format,omitempty"`
	Grammar              *string             `json:"grammar,omitempty"`        // llama.cpp / TGI: GBNF grammar string
	GuidedJSON           *string             `json:"guided_json,omitempty"`    // vLLM: JSON schema for guided decoding
	GuidedGrammar        *string             `json:"guided_grammar,omitempty"` // vLLM: GBNF grammar alternative
}

// ReasoningObject is the openrouter reasoning-enable payload.
type ReasoningObject struct {
	Effort    string `json:"effort,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// ChatTemplateKwargs carries provider-specific chat-template overrides.
// NVIDIA NIM / Poolside use it to enable thinking via
// chat_template_kwargs.enable_thinking. The bool has no omitempty so an
// explicit false is serialized as "enable_thinking": false — omitting it would
// drop the disabled override and leave the provider's native default in force.
// The parent pointer is nil'd by resolvers when the kwargs must not appear.
type ChatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

// HasSeparateReasoning reports whether the message carries reasoning in a
// dedicated field (not inline). OpenAI o-series/GPT-5 reasoning is opaque and
// never populates these — callers use this to decide between showing real
// reasoning vs. a neutral "working" status.
func (m Message) HasSeparateReasoning() bool {
	return m.ReasoningContent != "" || m.Reasoning != "" || len(m.ReasoningDetails) > 0
}

// extractReasoning returns the reasoning text for m following a fixed
// precedence: ReasoningContent → Reasoning → joined ReasoningDetails → inline
// <think>/<thinking>/<reasoning>/<REASONING_SCRATCHPAD> tags (only when present
// in Content). Returns "" when none. It never mutates m.Content.
func (m Message) ExtractReasoning() string {
	if m.ReasoningContent != "" {
		return m.ReasoningContent
	}
	if m.Reasoning != "" {
		return m.Reasoning
	}
	if len(m.ReasoningDetails) > 0 {
		var b strings.Builder
		for _, d := range m.ReasoningDetails {
			if d.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(d.Text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	if strings.Contains(m.Content, "<think") ||
		strings.Contains(m.Content, "<thinking") ||
		strings.Contains(m.Content, "<reasoning") ||
		strings.Contains(m.Content, "<REASONING_SCRATCHPAD") {
		return extractInlineReasoning(m.Content)
	}
	return ""
}

// extractInlineReasoning pulls the first inline reasoning block out of content.
// Supported tags: <think>, <thinking>, <reasoning>, <REASONING_SCRATCHPAD>.
func extractInlineReasoning(content string) string {
	pairs := []struct{ open, close string }{
		{"<think>", "</think>"},
		{"<thinking>", "</thinking>"},
		{"<reasoning>", "</reasoning>"},
		{"<REASONING_SCRATCHPAD>", "</REASONING_SCRATCHPAD>"},
	}
	best := -1
	bestPair := -1
	for i, p := range pairs {
		idx := strings.Index(content, p.open)
		if idx < 0 {
			continue
		}
		if best == -1 || idx < best {
			best = idx
			bestPair = i
		}
	}
	if bestPair < 0 {
		return ""
	}
	p := pairs[bestPair]
	tagEnd := best + len(p.open)
	closeIdx := strings.Index(content[tagEnd:], p.close)
	if closeIdx < 0 {
		// Unterminated — return remainder.
		return strings.TrimSpace(content[tagEnd:])
	}
	return strings.TrimSpace(content[tagEnd : tagEnd+closeIdx])
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
	// FinishReason carries the chunk-level finish_reason ("stop", "length",
	// "tool_calls", ...) surfaced by the upstream on the final stream chunk
	// or a non-streaming response.
	FinishReason string `json:"finish_reason,omitempty"`
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
