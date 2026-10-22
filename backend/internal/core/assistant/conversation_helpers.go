package assistant

import (
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

const (
	agentsCacheKey            = "agents:"
	agentsFileCacheMaxEntries = 100              // per-workspace cache bound (PL-6)
	agentsFileCacheTTL        = 30 * time.Minute // stale entries evicted lazily on Get
)

// agentsFileCache memoizes AGENTS.md reads per workspace, bounded at
// agentsFileCacheMaxEntries to prevent unbounded growth for long-lived servers.
// Entries expire after agentsFileCacheTTL (lazy eviction on Get) so edits to
// AGENTS.md are picked up without a restart.
var agentsFileCache = core.NewTTLCache[string, string](agentsFileCacheMaxEntries, agentsFileCacheTTL, nil)

// MaxHistoryChars bounds the initial system + first-user base history (a small
// fixed seed). It is NOT used for the persisted session file.
const MaxHistoryChars = 12 * 1024

// MaxPersistedHistoryChars is the last-resort ceiling for persisted session
// history. Normal runs persist full (untruncated) history so a reload shows the
// complete tool-call/reasoning/output trail (the 12KB whole-message cap used to
// drop earlier tool calls — Bug 2). This bound only fires when a pathological
// run (e.g. hundreds of tool cycles) would otherwise write an unbounded file,
// keeping the PL-1 disk-usage hardening intent without losing normal-run data.
const MaxPersistedHistoryChars = 256 * 1024

// historyChars sums the content length of every message — the request-size
// proxy shared by history truncation, the ICU pre-flight check, token
// accounting and the memory-flush budget. It counts raw Content only (not
// reasoning or tool-call arguments), matching every caller's existing basis.
func historyChars(history []proxy.Message) int {
	total := 0
	for _, m := range history {
		total += len(m.Content)
	}
	return total
}

// LoadAgentsFile returns the workspace's own notes from AGENTS.md, as the model should see them: comments removed.
// It is empty when the file is missing, holds only comments (the seeded stub) or is one of the old defaults this app
// used to seed — a persona that now lives in the profiles, which a workspace must not keep layering on top of them.
func LoadAgentsFile(pm *persistence.WorkspaceManager, workspaceID string) string {
	if workspaceID == "" || pm == nil {
		return ""
	}
	notes, err := agentsFileCache.Get(agentsCacheKey+workspaceID, func() (string, error) {
		content, err := pm.ReadTaskFile(workspaceID, models.RulesFilename)
		if err != nil || prompts.IsLegacyDefaultNotes(content) {
			return "", nil
		}
		return models.WithoutComments(content), nil
	})
	if err != nil {
		return ""
	}
	return notes
}

// BuildSystemPrompt layers a run's system prompt — the profile's behaviour, the core rules, the workspace's notes —
// for the tool-call mode in use. Chat and automation both build it here.
func BuildSystemPrompt(pm *persistence.WorkspaceManager, workspaceID string, p profile.Profile, useNativeTools bool) string {
	return prompts.AssembleSystemPrompt(prompts.PromptParts{Native: useNativeTools, Profile: p.Instructions, Notes: LoadAgentsFile(pm, workspaceID)})
}

// conversationSystemMessage is a conversation's system message: the layered prompt with the conversation's own facts
// (its id and context version, and the zone it reads local times in).
// It is rebuilt every turn from the current profile and notes (so an edit or a profile switch applies at once) and is
// the same text turn after turn, which keeps the model server's cache.
func conversationSystemMessage(pm *persistence.WorkspaceManager, session *models.AssistantSession, zone string, p profile.Profile, useNativeTools bool) string {
	return prompts.BuildSystemMessage(BuildSystemPrompt(pm, session.WorkspaceID, p, useNativeTools), useNativeTools, session.ID, session.ContextVersion, zone)
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
// maxChars. Callers pass the appropriate bound: the small MaxHistoryChars for
// the initial base history, or the high MaxPersistedHistoryChars for the
// persisted session file (so normal runs persist full and only pathological runs
// are bounded).
func TruncateHistory(history []proxy.Message, maxChars int) []proxy.Message {
	if len(history) <= 1 {
		return history
	}

	totalChars := historyChars(history)

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

// NeutralizeStaleToolFailures rewrites replayed terminal "tool unavailable"
// results once the tool is available again, and reports how many it changed.
//
// A missing/rejected credential is a run-scoped condition, but its tool result
// is persisted and replayed into every later run in the conversation. That
// result carries the literal instruction "Do NOT retry it", so after an operator
// reconfigures the credential the model still refuses to call a tool that now
// works — it reports the capability as broken instead. Rewriting those stale
// results at replay time removes the obsolete instruction without touching
// still-broken tools or the run that originally hit the failure.
func NeutralizeStaleToolFailures(history []proxy.Message, gr *guardrails.GuardrailEngine, workspaceID string) int {
	if gr == nil || len(history) == 0 {
		return 0
	}
	available := currentToolAvailability(gr, workspaceID, history)
	if len(available) == 0 {
		return 0
	}

	neutralized := 0
	for i := range history {
		if history[i].Role != proxy.ToolRole {
			continue
		}
		tool, ok := staleToolUnavailable(history[i].Content)
		if !ok || !available[tool] {
			continue
		}
		history[i].Content = fmt.Sprintf(prompts.ToolUnavailableRetryPrompt, tool, tool)
		neutralized++
	}
	return neutralized
}

// currentToolAvailability maps each tool named in a replayed tool call to
// whether it is runnable now. It reuses the guardrail engine's denied-tool set —
// the same rule that decides schema visibility — so availability can never drift
// from what the model is actually offered.
func currentToolAvailability(gr *guardrails.GuardrailEngine, workspaceID string, history []proxy.Message) map[string]bool {
	denied := make(map[string]bool)
	for _, name := range gr.DisabledToolNames(workspaceID) {
		denied[name] = true
	}
	available := make(map[string]bool)
	for _, m := range history {
		for _, tc := range m.ToolCalls {
			available[tc.Function.Name] = !denied[tc.Function.Name]
		}
	}
	return available
}

// staleToolUnavailable reports whether a replayed tool result is the terminal
// "tool unavailable" notice, and which tool it names.
func staleToolUnavailable(content string) (string, bool) {
	if !strings.HasPrefix(content, prompts.ToolUnavailablePrefix) {
		return "", false
	}
	rest := content[len(prompts.ToolUnavailablePrefix):]
	tool, _, found := strings.Cut(rest, " — ")
	if !found || tool == "" {
		return "", false
	}
	return tool, true
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
		streamingContent   string // visible text from EventToolStream
		streamingReasoning string // thinking text from EventReasoning
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
