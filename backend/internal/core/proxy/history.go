package proxy

import (
	"fmt"
	"strings"
)

// NormalizeHistory ensures the conversation history is compatible with strict LLM providers.
// It handles role mapping, merges consecutive messages of the same role, and
// applies optimizations to prevent context bloat and "prefill" errors.
func NormalizeHistory(history []Message, useNativeTools bool) []Message {
	if len(history) == 0 {
		return nil
	}

	// 1. Initial pass: Map roles and strip native-tool fields.
	// When native tools are disabled, tool-role messages are converted to user
	// role because many llama.cpp chat templates (Jinja) enforce a strict
	// assistant-with-tool_call → tool-result pattern and reject orphan tool
	// roles.  The tool_call_id is embedded in the content so the model can
	// still associate results with specific calls.
	prepared := make([]Message, 0, len(history))
	for _, msg := range history {
		newMsg := msg
		if !useNativeTools {
			if len(newMsg.ToolCalls) > 0 && newMsg.Content == "" {
				var buf strings.Builder
				for j, tc := range newMsg.ToolCalls {
					if j > 0 {
						buf.WriteString("\n")
					}
					args := tc.Function.Arguments
					if args == "" {
						args = "{}"
					}
					buf.WriteString("<tool_call>\n")
					buf.WriteString("{\"tool\": \"")
					buf.WriteString(tc.Function.Name)
					buf.WriteString("\", \"args\": ")
					buf.WriteString(args)
					buf.WriteString("}\n")
					buf.WriteString("</tool_call>")
				}
				newMsg.Content = buf.String()
			}
			newMsg.ToolCalls = nil
			if msg.Role == ToolRole {
				newMsg.Role = UserRole
				if msg.ToolCallID != "" {
					newMsg.Content = fmt.Sprintf("Tool result [%s]: %s", msg.ToolCallID, msg.Content)
				} else {
					newMsg.Content = fmt.Sprintf("Observation: %s", msg.Content)
				}
			}
		}
		prepared = append(prepared, newMsg)
	}

	if len(prepared) == 0 {
		return nil
	}

	// 2. Consolidation pass: Merge consecutive messages of the same role to ensure alternation.
	// Tool results (identified by non-empty ToolCallID) are never merged —
	// merging a tool result with an adjacent nag message corrupts both and
	// confuses the model into retrying already-successful tool calls.
	merged := make([]Message, 0, len(prepared))
	merged = append(merged, prepared[0])

	for i := 1; i < len(prepared); i++ {
		last := &merged[len(merged)-1]
		current := prepared[i]

		if last.Role == current.Role && last.ToolCallID == "" && current.ToolCallID == "" {
			if current.Content != "" {
				if last.Content != "" {
					last.Content += "\n\n"
				}
				last.Content += current.Content
			}
			last.ToolCalls = append(last.ToolCalls, current.ToolCalls...)
		} else {
			merged = append(merged, current)
		}
	}

	// 3. Final safety: Fill empty content with safe placeholders.
	for i := range merged {
		if strings.TrimSpace(merged[i].Content) == "" {
			if merged[i].Role == AssistantRole {
				merged[i].Content = "Thinking..."
			} else if merged[i].Role == UserRole || merged[i].Role == ToolRole {
				merged[i].Content = "Tool result: (action completed, no output)"
			}
		}
	}

	return SanitizeHistory(merged)
}

// SanitizeHistory strips non-essential fields to minimize context overhead.
// : Strict Token-Only Normalization.
func SanitizeHistory(history []Message) []Message {
	sanitized := make([]Message, len(history))
	for i, msg := range history {
		sanitized[i] = Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
	}
	return sanitized
}

