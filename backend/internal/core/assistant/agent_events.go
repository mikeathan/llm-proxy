// agent_events.go — Agent event types, Observer, event notification methods,
// GuardrailDecisionCallback, and lifecycle events for UI progress reporting.
package assistant

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/proxy"
)

// eventIDCounter is a process-global monotonic counter for AgentEvent IDs.
// Using a single shared counter (instead of a per-Agent field) guarantees IDs
// stay unique across concurrently running agents; a per-Agent counter would let
// two runs produce colliding IDs while still avoiding per-event UUID allocation.
var eventIDCounter atomic.Uint64

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
	EventUpstream             AgentEventType = "upstream"
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
	// PhaseAgentThinking is emitted at the start of every LLM call to signal the
	// UI that the agent is working (reasoning compute or pre-token wait) before
	// any response content arrives. It carries NO content — the frontend shows a
	// neutral "thinking…" status, never fabricated reasoning text.
	PhaseAgentThinking = "agent_thinking"
	// PhaseStillThinking is emitted periodically while a call is still running
	// but has not advanced (silent-stall liveness). It carries NO content — a
	// payload of {elapsed} only. agent_thinking remains one-shot; this phase
	// repeats on the heartbeat cadence while the stall persists.
	PhaseStillThinking = "still_thinking"
)

// User-facing status message constants. Emitted as EventMessage payloads (Role
// "system") so the assistant panel shows progress during otherwise-silent
// phases. Centralized here — the single home for this UI status/emoji copy.
const (
	// MsgGeneratingPlan is shown while the plan-and-execute strategy runs its
	// synchronous pre-loop plan-generation LLM call.
	MsgGeneratingPlan = "🧠 Generating execution plan…"
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
	DecisionID  string `json:"decision_id"`
	Tool        string `json:"tool"`
	Args        string `json:"args"`
	Reason      string `json:"reason"`
	Category    string `json:"category"` // "terminal", "filesystem", "network", "search", "communication"
	WorkspaceID string `json:"workspace_id"`
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

// UpstreamEventPayload describes a transient upstream LLM failure that is being
// retried. It is observational: the retry/backoff policy is unchanged, and the
// UI uses this to surface a live "retrying…" notice instead of silent stalls.
type UpstreamEventPayload struct {
	Event       string `json:"event"`                // "retry"
	Reason      string `json:"reason"`               // "transport" | "status"
	Attempt     int    `json:"attempt"`              // 1-based attempt being retried
	MaxAttempts int    `json:"max_attempts"`         // total attempts (incl. first)
	Error       string `json:"error,omitempty"`      // transport error text
	ErrClass    string `json:"err_class,omitempty"`  // transport error bucket ("connection-closed", "timeout", "tls", ...)
	Status      int    `json:"status,omitempty"`     // upstream HTTP status
	ElapsedMs   int64  `json:"elapsed_ms,omitempty"` // time since this LLM call started
}

// GuardrailDecisionCallback is called by the agent when a guardrail blocks a
// tool call. The agent blocks until the callback returns. The callback should
// respect ctx cancellation to avoid hanging during shutdown.
type GuardrailDecisionCallback func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error)

type Observer func(AgentEvent)

func (a *Agent) notify(t AgentEventType, payload any) {
	if a.deps.Observer != nil {
		a.deps.Observer(AgentEvent{
			ID:             strconv.FormatUint(eventIDCounter.Add(1), 10),
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

// notifyUpstream surfaces a transient upstream retry to the UI. It maps a
// proxy.RetryInfo to the UpstreamEventPayload wire shape and is invoked from
// the retry observer wired in Agent.Execute. It never blocks the retry loop.
func (a *Agent) notifyUpstream(info proxy.RetryInfo) {
	payload := UpstreamEventPayload{
		Event:       "retry",
		Attempt:     info.Attempt,
		MaxAttempts: info.MaxAttempts,
		ElapsedMs:   info.ElapsedMs,
	}
	if info.Reason == proxy.RetryReasonStatus {
		payload.Reason = "status"
		payload.Status = info.Status
	} else {
		payload.Reason = "transport"
		payload.Error = info.Error
		payload.ErrClass = info.ErrClass
	}
	a.notify(EventUpstream, payload)
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

// notifyAgentThinking signals the UI that the agent has begun an LLM call and is
// working (reasoning compute / pre-token wait) before any response content
// arrives. It carries NO content — the frontend shows a neutral "thinking…"
// status, never fabricated reasoning text. Emitted once per call so even opaque
// providers (no readable reasoning stream) get a working indicator.
func (a *Agent) notifyAgentThinking() {
	a.notifyLifecycle(PhaseAgentThinking, map[string]any{})
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
