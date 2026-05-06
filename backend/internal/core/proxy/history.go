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

	// 4. Dynamic Context Sieve: Token-Budgeted Middle-Pruning (approx 16,000 characters)
	// Open Claw v3: Protects against context size exceeded errors while preserving the goal and state.
	const charBudget = 16000
	totalChars := 0
	for _, msg := range merged {
		totalChars += len(msg.Content)
	}

	if totalChars > charBudget && len(merged) > 8 {
		systemMsg := Message{}
		hasSystem := false
		if merged[0].Role == SystemRole {
			systemMsg = merged[0]
			hasSystem = true
		}

		// Keep the first user message (the Task/Goal)
		firstUserIdx := -1
		for i, msg := range merged {
			if msg.Role == UserRole {
				firstUserIdx = i
				break
			}
		}

		// Keep the Priority Tail (last 3 turns / 6 messages)
		tailCount := 6
		if len(merged) < 8 {
			tailCount = len(merged) - 2 // Safety
		}
		startIndex := len(merged) - tailCount
		
		newMerged := make([]Message, 0)
		if hasSystem {
			newMerged = append(newMerged, systemMsg)
		}
		if firstUserIdx != -1 && firstUserIdx != 0 {
			newMerged = append(newMerged, merged[firstUserIdx])
		}

		// Add distillation marker
		newMerged = append(newMerged, Message{
			Role:    SystemRole,
			Content: "[System: (Earlier steps omitted for space preservation. Current project state is maintained.)]",
		})

		newMerged = append(newMerged, merged[startIndex:]...)
		merged = newMerged
	}

	return SanitizeHistory(merged)
}

// SanitizeHistory strips non-essential metadata from messages before sending to LLM.
// Open Claw v3 Phase 1: Overhead reduction.
func SanitizeHistory(history []Message) []Message {
	sanitized := make([]Message, len(history))
	for i, msg := range history {
		sanitized[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
			// Keep ToolCalls only if it's an assistant message and we are in a turn
			ToolCalls: msg.ToolCalls,
		}
	}
	return sanitized
}
