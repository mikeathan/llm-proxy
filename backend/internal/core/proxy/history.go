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
		// Compatibility: Strip native tool call fields if the model doesn't support them.
		// Many local LLM servers (Ollama, vLLM) crash with a 500 error if they see
		// tool_calls in the history when native tools are disabled or malformed.
		if !useNativeTools {
			newMsg.ToolCalls = nil
			if msg.Role == ToolRole {
				newMsg.Role = UserRole
				newMsg.Content = fmt.Sprintf("### Tool Output (%s):\n%s", msg.ToolCallID, msg.Content)
			}
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
	
	return NormalizeContextSieve(merged, 15000)
}

// NormalizeContextSieve applies the token-budgeted middle-pruning logic.
// : Structural History Pruning ("The Sieve")
func NormalizeContextSieve(history []Message, budget int) []Message {
	totalChars := 0
	for _, msg := range history {
		totalChars += len(msg.Content)
	}

	if totalChars <= budget || len(history) < 8 {
		return SanitizeHistory(history)
	}

	// 1. Keep the System Message
	systemMsg := Message{}
	hasSystem := false
	if history[0].Role == SystemRole {
		systemMsg = history[0]
		hasSystem = true
	}

	// 2. Keep the first user message (the Task/Goal)
	firstUserMsg := Message{}
	firstUserIdx := -1
	for i, msg := range history {
		if msg.Role == UserRole {
			firstUserMsg = msg
			firstUserIdx = i
			break
		}
	}

	// 3. Keep the Priority Tail (The last 3 turns = 6 messages)
	tailCount := 6
	if len(history) < tailCount {
		tailCount = len(history)
	}
	
	markerMsg := Message{
		Role:    SystemRole,
		Content: "Memory Sieve: Older troubleshooting steps omitted. Progress so far: (Tasks completed successfully)",
	}

	newMerged := make([]Message, 0)
	if hasSystem {
		newMerged = append(newMerged, systemMsg)
	}
	if firstUserIdx != -1 && firstUserIdx != 0 {
		newMerged = append(newMerged, firstUserMsg)
	}

	newMerged = append(newMerged, markerMsg)
	
	startIndex := len(history) - tailCount
	if startIndex < 0 {
		startIndex = 0
	}
	// Avoid duplicating messages if they are already in the head
	for i := startIndex; i < len(history); i++ {
		if (hasSystem && i == 0) || (firstUserIdx != -1 && i == firstUserIdx) {
			continue
		}
		newMerged = append(newMerged, history[i])
	}
	
	return SanitizeHistory(newMerged)
}

// SanitizeHistory strips non-essential fields to minimize context overhead.
// : Strict Token-Only Normalization.
func SanitizeHistory(history []Message) []Message {
	sanitized := make([]Message, len(history))
	for i, msg := range history {
		sanitized[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
			// ONLY include ToolCalls for the very last assistant message if relevant, 
			// otherwise strip them to save space.
			ToolCalls: nil,
		}
		if msg.Role == AssistantRole && i == len(history)-1 {
			sanitized[i].ToolCalls = msg.ToolCalls
		}
		// For tool messages, we MUST keep ToolCallID for internal validation if the model needs it,
		// but since we are doing text-only, we usually don't need it in the prompt.
		// However, keeping Role and Content is the core requirement.
	}
	return sanitized
}

