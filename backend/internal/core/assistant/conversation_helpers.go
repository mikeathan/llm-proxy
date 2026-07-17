package assistant

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

var agentsFileCache core.ContentCache

const agentsCacheKey = "agents:"

const MaxHistoryChars = 12 * 1024

// LoadAgentsFile returns the workspace agent instructions from AGENTS.md,
// falling back to the built-in DefaultAgentsMD when the file does not exist.
func LoadAgentsFile(pm *persistence.WorkspaceManager, workspaceID string) string {
	if workspaceID == "" || pm == nil {
		return prompts.DefaultAgentsMD
	}
	content, err := agentsFileCache.Get(agentsCacheKey+workspaceID, func() (string, error) {
		return pm.ReadTaskFile(workspaceID, models.RulesFilename)
	})
	if err != nil || content == "" {
		return prompts.DefaultAgentsMD
	}
	return content
}

// buildInitialHistory constructs the initial session messages including system prompt and rules.
func BuildInitialHistory(persistence *persistence.WorkspaceManager, workspaceID, conversationID, message, contextVersion, timezone string, useNativeTools bool) ([]proxy.Message, error) {
	agentsFileContent := LoadAgentsFile(persistence, workspaceID)

	systemPrompt := prompts.AssembleSystemPrompt(agentsFileContent, useNativeTools)

	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: prompts.BuildSystemMessage(
				systemPrompt,
				useNativeTools,
				conversationID,
				contextVersion,
				timezone,
			),
		},
		{
			Role:    proxy.UserRole,
			Content: message,
		},
	}, nil
}

// TruncateHistory removes oldest non-system messages when total content exceeds the limit.
func TruncateHistory(history []proxy.Message) []proxy.Message {
	if len(history) <= 1 {
		return history
	}

	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}

	if totalChars <= MaxHistoryChars {
		return history
	}

	startIdx := 0
	if history[0].Role == proxy.SystemRole {
		startIdx = 1
	}

	for totalChars > MaxHistoryChars && startIdx < len(history)-1 {
		totalChars -= len(history[startIdx].Content)
		history = append(history[:startIdx], history[startIdx+1:]...)
	}

	return history
}

// GenerateRunID returns a short unique identifier for a recording session.
func GenerateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// FormatCollectedEvents converts agent events into the wire format for the API response.
func FormatCollectedEvents(events []AgentEvent) []map[string]any {
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
func PublishSessionLifecycle(events EventPublisher, workspaceID, conversationID, snippet, phase string) {
	if workspaceID == "" || conversationID == "" {
		return
	}
	events.Publish(workspaceID, AgentEvent{
		ID:             fmt.Sprintf("sse_%d", time.Now().UnixNano()),
		Type:           EventLifecycle,
		Channel:        ChannelAssistant,
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
// EventMessage with tool calls is skipped (already represented by the
// EventToolCall + EventToolResult chain).  The result matches the agent's
// actual updatedHistory structure from a completed execution.
func buildPartialHistory(base []proxy.Message, events []AgentEvent) []proxy.Message {
	history := make([]proxy.Message, len(base))
	copy(history, base)

	var (
		streamingContent   string          // visible text from EventToolStream
		streamingReasoning string          // thinking text from EventReasoning
		pending            []proxy.ToolCall
		turnContent        string          // snapshot of streamingContent at first call
		turnReasoning      string          // snapshot of streamingReasoning at first call
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
		case EventToolStream:
			if text, ok := ev.Payload.(string); ok {
				streamingContent = text
			}
		case EventReasoning:
			if text, ok := ev.Payload.(string); ok {
				streamingReasoning = text
			}
		case EventToolCall:
			tc, ok := ev.Payload.(proxy.ToolCall)
			if !ok {
				continue
			}
			if len(pending) == 0 {
				turnContent = streamingContent
				turnReasoning = streamingReasoning
			}
			pending = append(pending, tc)
		case EventToolResult:
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
		case EventMessage:
			flushPendingGroup()
			msg, ok := ev.Payload.(proxy.Message)
			if !ok {
				continue
			}
			// EventMessage with tool calls duplicates the assistant
			// message already built by flushPendingGroup from the
			// EventToolCall events — skip it.
			if len(msg.ToolCalls) > 0 {
				continue
			}
			history = append(history, msg)
		}
	}
	return history
}
