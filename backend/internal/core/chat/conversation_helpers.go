package chat

import (
	"fmt"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// MaxPersistedHistoryChars is the last-resort ceiling for persisted session
// history. Normal runs persist full (untruncated) history so a reload shows the
// complete tool-call/reasoning/output trail (the 12KB whole-message cap used to
// drop earlier tool calls — Bug 2). This bound only fires when a pathological
// run (e.g. hundreds of tool cycles) would otherwise write an unbounded file,
// keeping the PL-1 disk-usage hardening intent without losing normal-run data.
const MaxPersistedHistoryChars = 256 * 1024

// conversationSystemMessage is a conversation's system message: the layered prompt with the conversation's own facts
// (its id and context version, and the zone it reads local times in).
// It is rebuilt every turn from the current profile and notes (so an edit or a profile switch applies at once) and is
// the same text turn after turn, which keeps the model server's cache.
func conversationSystemMessage(pm *persistence.WorkspaceManager, session *models.AssistantSession, zone string, p profile.Profile, useNativeTools bool) string {
	return prompts.BuildSystemMessage(assistant.BuildSystemPrompt(pm, session.WorkspaceID, p, useNativeTools), useNativeTools, session.ID, session.ContextVersion, zone)
}

// withSystemMessage returns history led by the given system message: an existing system head is replaced, none is added.
func withSystemMessage(history []proxy.Message, content string) []proxy.Message {
	head := proxy.Message{Role: proxy.SystemRole, Content: content}
	if len(history) > 0 && history[0].Role == proxy.SystemRole {
		out := append([]proxy.Message(nil), history...)
		out[0] = head
		return out
	}
	return append([]proxy.Message{head}, history...)
}

// TruncateHistory removes oldest non-system messages when total content exceeds
// maxChars. The conversation service passes MaxPersistedHistoryChars, the stored
// session's last-resort ceiling, so normal chats keep everything and only
// pathological runs are bounded; what the model is sent is the sieve's business.
func TruncateHistory(history []proxy.Message, maxChars int) []proxy.Message {
	if len(history) <= 1 {
		return history
	}

	totalChars := assistant.HistoryChars(history)

	if totalChars <= maxChars {
		return history
	}

	startIdx := 0
	if history[0].Role == proxy.SystemRole {
		startIdx = 1
	}

	// Anchor the first user message (the original task) so truncation never
	// drops it. A persisted/replayed session must always retain the user's
	// prompt; dropping it renders the conversation blank on reopen.
	userAnchor := -1
	for i := startIdx; i < len(history); i++ {
		if history[i].Role == proxy.UserRole {
			userAnchor = i
			break
		}
	}

	for totalChars > maxChars && startIdx < len(history)-1 {
		// Protect the original user task from being dropped: prefer trimming
		// later messages, but if we reach the user anchor and still over
		// budget, cap its content (dropping the whole task would blank the
		// conversation on reopen).
		if startIdx == userAnchor {
			// Trim any later messages first.
			for startIdx+1 < len(history) && totalChars > maxChars {
				totalChars -= len(history[startIdx+1].Content)
				history = append(history[:startIdx+1], history[startIdx+2:]...)
			}
			if totalChars <= maxChars {
				break
			}
			// Still over budget: cap the user message content itself.
			remaining := maxChars - (totalChars - len(history[startIdx].Content))
			if remaining > 0 && remaining < len(history[startIdx].Content) {
				capped := []byte(history[startIdx].Content)
				capped = capped[:remaining]
				history[startIdx].Content = string(capped) + "\n…[truncated]"
				totalChars = maxChars
			}
			break
		}
		totalChars -= len(history[startIdx].Content)
		history = append(history[:startIdx], history[startIdx+1:]...)
		// Indices after the removed one shift down by one.
		if userAnchor > startIdx {
			userAnchor--
		}
	}

	return history
}

// FormatCollectedEvents converts agent events into the wire format for the API response.
func FormatCollectedEvents(events []assistant.AgentEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		out = append(out, map[string]any{
			"type":    ev.Type,
			"payload": ev.Payload,
		})
	}
	return out
}

// ComputeCancelIndices determines the cancel marker scope for a session history at cancel time.
func ComputeCancelIndices(history []proxy.Message) (canceledIdx, canceledUserIdx int) {
	lastUserIdx, lastAssistantIdx := -1, -1
	for i := len(history) - 1; i >= 0; i-- {
		switch history[i].Role {
		case proxy.AssistantRole:
			if lastAssistantIdx < 0 {
				lastAssistantIdx = i
			}
		case proxy.UserRole:
			if lastUserIdx < 0 {
				lastUserIdx = i
			}
		}
		if lastUserIdx >= 0 && lastAssistantIdx >= 0 {
			break
		}
	}

	if lastUserIdx < 0 {
		return -1, -1
	}
	if lastAssistantIdx > lastUserIdx {
		return lastAssistantIdx, lastUserIdx
	}
	return -1, lastUserIdx
}

// FilterCancelledTurns strips messages from every cancelled turn so they
// don't leak into the LLM context on the next turn.
func FilterCancelledTurns(history []proxy.Message, cancelledIndices []int) []proxy.Message {
	if len(cancelledIndices) == 0 {
		return history
	}

	skipSet := make(map[int]bool, len(cancelledIndices))
	for _, idx := range cancelledIndices {
		if idx >= 0 && idx < len(history) {
			skipSet[idx] = true
		}
	}
	if len(skipSet) == 0 {
		return history
	}

	result := make([]proxy.Message, 0, len(history))
	skipping := false
	for i, m := range history {
		if skipSet[i] {
			skipping = true
			continue
		}
		if skipping && m.Role == proxy.UserRole {
			skipping = false
		}
		if skipping {
			continue
		}
		result = append(result, m)
	}
	return result
}

// PublishSessionLifecycle publishes a lifecycle event to the workspace event bus.
func PublishSessionLifecycle(events assistant.EventPublisher, workspaceID, conversationID, snippet, phase string) {
	if workspaceID == "" || conversationID == "" {
		return
	}
	events.Publish(workspaceID, assistant.AgentEvent{
		ID:             fmt.Sprintf("sse_%d", time.Now().UnixNano()),
		Type:           assistant.EventLifecycle,
		Channel:        assistant.ChannelAssistant,
		ConversationID: conversationID,
		Payload: map[string]any{
			"phase":           phase,
			"conversation_id": conversationID,
			"workspace_id":    workspaceID,
			"snippet":         snippet,
			"source":          models.SessionSource(conversationID),
		},
		Timestamp: time.Now(),
	})
}

// buildPartialHistory reconstructs the full conversation history from the base
// (user message + system prompt) and the collected agent events.  Streaming
// events supply both Content and ReasoningContent for tool-call assistant
// messages, parallel tool calls are grouped into a single message, and
// assistant.EventMessage with tool calls is skipped (already represented by the
// assistant.EventToolCall + assistant.EventToolResult chain).  The result matches the agent's
// actual updatedHistory structure from a completed execution.
func buildPartialHistory(base []proxy.Message, events []assistant.AgentEvent) []proxy.Message {
	history := make([]proxy.Message, len(base))
	copy(history, base)

	var (
		streamingContent   string // visible text from assistant.EventToolStream
		streamingReasoning string // thinking text from assistant.EventReasoning
		pending            []proxy.ToolCall
		turnContent        string // snapshot of streamingContent at first call
		turnReasoning      string // snapshot of streamingReasoning at first call
	)

	flushPendingGroup := func() {
		if len(pending) == 0 {
			return
		}
		history = append(history, proxy.Message{
			Role:             proxy.AssistantRole,
			Content:          turnContent,
			ReasoningContent: turnReasoning,
			ToolCalls:        pending,
		})
		pending = nil
		turnContent = ""
		turnReasoning = ""
	}

	for _, ev := range events {
		switch ev.Type {
		case assistant.EventToolStream:
			if text, ok := ev.Payload.(string); ok {
				streamingContent = text
			}
		case assistant.EventReasoning:
			if text, ok := ev.Payload.(string); ok {
				streamingReasoning = text
			}
		case assistant.EventToolCall:
			tc, ok := ev.Payload.(proxy.ToolCall)
			if !ok {
				continue
			}
			if len(pending) == 0 {
				turnContent = streamingContent
				turnReasoning = streamingReasoning
			}
			pending = append(pending, tc)
		case assistant.EventToolResult:
			flushPendingGroup()
			payload, ok := ev.Payload.(map[string]any)
			if !ok {
				continue
			}
			id, _ := payload["id"].(string)
			content := ""
			if r, ok := payload["result"]; ok && r != nil {
				content = fmt.Sprint(r)
			}
			history = append(history, proxy.Message{
				Role:       proxy.ToolRole,
				Content:    content,
				ToolCallID: id,
			})
		case assistant.EventMessage:
			flushPendingGroup()
			msg, ok := ev.Payload.(proxy.Message)
			if !ok {
				continue
			}
			// assistant.EventMessage with tool calls duplicates the assistant
			// message already built by flushPendingGroup from the
			// assistant.EventToolCall events — skip it.
			if len(msg.ToolCalls) > 0 {
				continue
			}
			history = append(history, msg)
		}
	}
	return history
}

// withHelpersNote returns history with the helpers note appended to the last user message, for the model only, like the
// local time: what is stored and shown stays what the user typed.
func withHelpersNote(history []proxy.Message) []proxy.Message {
	last := len(history) - 1
	if last < 0 || history[last].Role != proxy.UserRole {
		return history
	}
	out := make([]proxy.Message, len(history))
	copy(out, history)
	out[last].Content += "\n\n" + prompts.HelpersNote
	return out
}

// withLocalTime returns history with the user's local time appended to the last
// user message, for the model only: the stored session is not touched, so the
// note is current on every turn however long the conversation has been open.
func withLocalTime(history []proxy.Message, now time.Time, zone string) []proxy.Message {
	last := len(history) - 1
	if last < 0 || history[last].Role != proxy.UserRole {
		return history
	}
	out := make([]proxy.Message, len(history))
	copy(out, history)
	out[last].Content += "\n\n" + prompts.TurnTimeNote(now, zone)
	return out
}
