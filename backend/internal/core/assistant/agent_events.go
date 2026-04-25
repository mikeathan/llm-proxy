package assistant

import (
	"time"

	"llm-proxy/internal/core/proxy"
)

type AgentEventType string

const (
	EventStepStart          AgentEventType = "step_start"
	EventMessage            AgentEventType = "message"
	EventToolCall           AgentEventType = "tool_call"
	EventToolResult         AgentEventType = "tool_result"
	EventGuardrailViolation AgentEventType = "guardrail_violation"
	EventError              AgentEventType = "error"
	EventToolStream         AgentEventType = "tool_stream"
)

type AgentEvent struct {
	Type      AgentEventType `json:"type"`
	Payload   any            `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

type Observer func(AgentEvent)

func (a *Agent) notify(t AgentEventType, payload any) {
	if a.observer != nil {
		a.observer(AgentEvent{
			Type:      t,
			Payload:   payload,
			Timestamp: time.Now(),
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
