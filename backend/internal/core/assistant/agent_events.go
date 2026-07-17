// agent_events.go — Agent event types, Observer, event notification methods,
// GuardrailDecisionCallback, and lifecycle events for UI progress reporting.
package assistant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"llm-proxy/internal/core/proxy"
)

type AgentEventType string

const (
	EventStepStart          AgentEventType = "step_start"
	EventMessage            AgentEventType = "message"
	EventToolCall           AgentEventType = "tool_call"
	EventToolResult         AgentEventType = "tool_result"
	EventGuardrailViolation   AgentEventType = "guardrail_violation"
	EventGuardrailBlocked     AgentEventType = "guardrail_blocked"
	EventGuardrailInvalidated AgentEventType = "guardrail_invalidated"
	EventError                AgentEventType = "error"
	EventReasoning            AgentEventType = "reasoning"
	EventToolStream           AgentEventType = "tool_stream"
	EventLifecycle            AgentEventType = "lifecycle"
	EventMemoryRecall         AgentEventType = "memory_recall"
	EventMemoryFlush          AgentEventType = "memory_flush"
)

// EventChannel isolates event streams by producer so a frontend subscriber
// only receives events for the mode it is viewing. Assistant chat and
// automation runs share one per-workspace SSE topic; the channel discriminator
// lets the EventBus route each event to the correct subscriber set and lets the
// SSE handler serve a single channel per connection.
type EventChannel string

const (
	// ChannelAssistant is the assistant chat event stream.
	ChannelAssistant EventChannel = "assistant"
	// ChannelAutomation is the automation run event stream.
	ChannelAutomation EventChannel = "automation"
)

// Lifecycle phase constants for the AgentEvent lifecycle payload.
// Used to communicate session state changes to the frontend via SSE.
const (
	PhaseSessionStarted   = "session_started"
	PhaseSessionProgress  = "session_progress"
	// PhaseSessionCompleted fires when a task completes — the model responds to
	// a tool result with a final assistant message and stops calling tools.
	PhaseSessionCompleted = "session_completed"
)

type AgentEvent struct {
	ID             string         `json:"id"`
	Type           AgentEventType `json:"type"`
	Channel        EventChannel   `json:"channel"`         // producer stream: assistant | automation
	ConversationID string         `json:"conversation_id"` // assistant session id (empty for automation)
	Payload        any            `json:"payload"`
	Timestamp      time.Time      `json:"timestamp"`
}

// GuardrailBlockedPayload is sent with EventGuardrailBlocked when the agent
// pauses and waits for user approval.
type GuardrailBlockedPayload struct {
	DecisionID string `json:"decision_id"`
	Tool       string `json:"tool"`
	Args       string `json:"args"`
	Reason     string `json:"reason"`
	Category   string `json:"category"` // "terminal", "filesystem", "network", "search", "communication"
}

// GuardrailDecision is the user's response to a guardrail block.
type GuardrailDecision struct {
	Allow   bool `json:"allow"`   // false = cancel this tool call
	Persist bool `json:"persist"` // update workspace override so future calls pass
}

// GuardrailInvalidatedPayload is sent when a pending guardrail decision is
// auto-resolved (context cancelled, e.g. automation stopped).  The frontend
// uses decision_id to clear the matching approval prompt.
type GuardrailInvalidatedPayload struct {
	DecisionID string `json:"decision_id"`
	Reason     string `json:"reason"` // "context_cancelled"
}

// GuardrailDecisionCallback is called by the agent when a guardrail blocks a
// tool call. The agent blocks until the callback returns. The callback should
// respect ctx cancellation to avoid hanging during shutdown.
type GuardrailDecisionCallback func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error)

type Observer func(AgentEvent)

func (a *Agent) notify(t AgentEventType, payload any) {
	if a.deps.Observer != nil {
		a.deps.Observer(AgentEvent{
			ID:             uuid.NewString(),
			Type:           t,
			Channel:        a.config.Channel,
			ConversationID: a.config.ConversationID,
			Payload:        payload,
			Timestamp:      time.Now(),
		})
	}
}

// Named Notification Wrappers

func (a *Agent) notifyThinking() {
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "🤖 Agent is thinking...",
	})
}

func (a *Agent) notifyStepStart(step int) {
	a.notify(EventStepStart, map[string]int{"step": step})
}

func (a *Agent) notifyFallbackWarning(err error) {
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "⚠️ WARNING: The selected model does not support tool calling. Fallback mode engaged (tools disabled). " + err.Error(),
	})
}

func (a *Agent) notifyPrematureTerminationNag(history *[]proxy.Message) {
	nagMsg := proxy.Message{
		Role:    "user",
		Content: "You returned an incomplete response. You MUST continue using tools or reply with the final comprehensive Markdown report as requested.",
	}
	*history = append(*history, nagMsg)
	a.notify(EventMessage, nagMsg)
}

func (a *Agent) notifyToolCall(tc proxy.ToolCall) {
	a.notify(EventToolCall, tc)
}

func (a *Agent) notifyToolResult(id, name string, result any) {
	a.notify(EventToolResult, map[string]any{"id": id, "name": name, "result": result})
}

func (a *Agent) notifyGuardrailViolation(tool string, err error) {
	a.notify(EventGuardrailViolation, map[string]string{
		"tool":  tool,
		"error": err.Error(),
	})
}

func (a *Agent) notifyPrefillDisabled() {
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "⚙️ Response prefill was disabled — the model rejected the prefill (thinking mode active on the server). For faster execution, set `prefill: false` on this model.",
	})
}

// notifyLifecycle emits a structured lifecycle event to the UI so the user
// sees what phase the agent is in: thinking, stuck_detected, fallback_started,
// fallback_waiting, fallback_completed, etc.
func (a *Agent) notifyLifecycle(phase string, extra map[string]any) {
	payload := map[string]any{"phase": phase}
	for k, v := range extra {
		payload[k] = v
	}
	a.notify(EventLifecycle, payload)
}

func (a *Agent) notifyMemoryRecall(query string, count int) {
	a.notify(EventMemoryRecall, map[string]any{"query": query, "count": count})
}

func (a *Agent) notifyMemoryFlush(count int) {
	a.notify(EventMemoryFlush, map[string]any{"saved_count": count})
}

func (a *Agent) notifyModelCompatWarning(useNativeTools bool) {
	suggest := "xml"
	if !useNativeTools {
		suggest = "native"
	}
	a.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: "⚠️ The model is not generating valid tool calls after multiple attempts. Try setting `tool_call_format: \"" + suggest + "\"` for this model.",
	})
}
