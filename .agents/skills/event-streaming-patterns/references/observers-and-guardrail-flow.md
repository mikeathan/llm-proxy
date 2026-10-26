# Event Streaming Patterns — SSE, Observers & Lifecycle Events — Observer Pattern and Guardrail Decision Flow

Reference for the [`event-streaming-patterns`](../SKILL.md) skill, loaded on demand. You are adding an observer or changing how guardrail approvals flow through events.

## Observer Pattern

### Observer is a raw function type

```go
type Observer func(AgentEvent)
```

There can **only be one observer per agent** — setting a new one silently replaces
the prior one. If both test capture and production publish are needed, chain them:

```go
var captured []AgentEvent
agent := NewAgent(..., AgentOptions{
    Observer: func(ev AgentEvent) {
        captured = append(captured, ev)
        productionObserver(ev)  // forward to SSE
    },
})
```

### The chat observer collects as it publishes

A chat turn's events reach the client over SSE only: `POST /admin/api/conversation/message` answers `202 Accepted`
at once and the turn runs detached (`chat.Service.Run`), so nothing comes back in the HTTP body. The chat observer
(`buildObserver` in `core/chat/conversation_service.go`) still keeps the events it publishes: they rebuild the
session's partial history for the disk checkpoint on every `tool_result` / `message`, and the cancel path reads the
last failure from them (`lastRunFailureText`). It keeps only the newest snapshot of a stream (`assistant.SupersedesSnapshot`), because
`buildPartialHistory` rescans the log on every tool result.

```go
// In core/chat/conversation_service.go (buildObserver), simplified:
obs := func(ev assistant.AgentEvent) {
    collected = append(collected, ev)
    events.Publish(workspaceID, ev)            // SSE
    // + recorder.Write(ev), the checkpoint on tool_result/message, the step counter on tool_call
}
```

## Guardrail Decision Flow

```
Model calls tool → GuardrailEngine blocks → GuardrailDecisionCallback invoked
  ├── Decision stored in GuardrailDecisionStore with unique `decision_id`
  ├── `guardrail_blocked` event published (SSE)
  ├── Frontend shows GuardrailBanner → user clicks Allow/Deny
  │     └── POST /admin/api/conversation/guardrail-decision {decision_id, allow, persist}
  │           └── DecisionStore.Resolve() → wakes callback
  ├── Callback returns {Allow, Persist} to resolveGuardrail (consumed via
  │     executeSingleToolStep, shared by processToolCalls and executePlan)
  └── Callback blocks up to GuardrailApprovalTimeout (default 5m, per-model
      configurable, Constitution II.10) for a decision; on expiry → guardrail_invalidated
      (reason "timeout") → treated as denied (violation recorded, run continues). The bound is
      applied at the call site (resolveGuardrail), not inside the callback —
      it is distinct from DefaultGuardrailTimeout (5s, the validation bound).
```

> **Delivery guarantee:** `guardrail_blocked` / `guardrail_invalidated` / `error` are
> `criticalEvents` in the EventBus (`internal/core/eventbus/bus.go`) — they are never dropped
> when a subscriber's buffer is full (bounded blocking send, 3s), unlike cosmetic events
> (reasoning/tool_stream). Dropping a `guardrail_blocked` would leave the agent waiting on
> an approval the UI never shows; the `recent` replay buffer is only the reconnect net, not
> the primary delivery path.

### Frontend guardrail state management

```typescript
// useLiveConsole.ts / useAssistantSSE.ts:
const pendingDecision = ref<GuardrailBlockedPayload | null>(null)
const handledDecisionIds = new Set<string>()

// On guardrail_blocked event:
if (!handledDecisionIds.has(payload.decision_id)) {
    pendingDecision.value = payload
}

// On guardrail_invalidated event:
handledDecisionIds.add(payload.decision_id)
if (pendingDecision.value?.decision_id === payload.decision_id) {
    pendingDecision.value = null
}

// After submitting decision:
handledDecisionIds.add(id)
pendingDecision.value = null
```
