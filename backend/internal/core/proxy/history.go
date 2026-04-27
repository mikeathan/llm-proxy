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

	// 1. Initial pass: Map roles and apply protocol mappings
	prepared := make([]Message, 0, len(history))
	for _, msg := range history {
		newMsg := msg
		// Compatibility: Map 'tool' role to 'user' for models that don't support native tool calls.
		if !useNativeTools && msg.Role == ToolRole {
			newMsg.Role = UserRole
			newMsg.Content = fmt.Sprintf("### Tool Output (%s):\n%s", msg.ToolCallID, msg.Content)
		}
		prepared = append(prepared, newMsg)
	}

	if len(prepared) == 0 {
		return nil
	}

	// 2. Consolidation pass: Merge consecutive messages of the same role to ensure alternation.
	// This prevents the "Jinja Exception: roles must alternate" error.
	merged := make([]Message, 0, len(prepared))
	merged = append(merged, prepared[0])

	for i := 1; i < len(prepared); i++ {
		last := &merged[len(merged)-1]
		current := prepared[i]

		if last.Role == current.Role {
			// Merge content and tool calls into the previous message
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

	// 3. Final safety: Ensure no message has empty content (required by many strict templates)
	// and ensure history doesn't end with an Assistant message (prevents prefill issues).
	for i := range merged {
		if strings.TrimSpace(merged[i].Content) == "" {
			if merged[i].Role == AssistantRole {
				merged[i].Content = "..." // Placeholder for tool-only messages or empty generations
			} else if merged[i].Role == UserRole {
				merged[i].Content = "Continue."
			} else {
				merged[i].Content = "..."
			}
		}
	}

	if !useNativeTools && len(merged) > 0 && merged[len(merged)-1].Role == AssistantRole {
		merged = append(merged, Message{
			Role:    UserRole,
			Content: "Continue.",
		})
	}

	return merged
}
