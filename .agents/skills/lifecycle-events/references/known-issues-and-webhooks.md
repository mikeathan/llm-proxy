# Session Lifecycle Events — Known Issues Fixed and the Webhook Flow

Reference for the [`lifecycle-events`](../SKILL.md) skill, loaded on demand. You are debugging a lifecycle regression or webhook-started sessions.

## Known Issues Fixed

### Duplicate user messages in sessions

**Root cause:** `handleAgentMessage` appended the user message to `session.History`, then `handleAssistant` appended it a second time (line 217-222). The session file ended up with `[user, user, assistant]` → two user bubbles in the chat.

**Fix:** Removed the append from `handleAgentMessage`. `handleAssistant` is the single source of truth for appending the user message to the session. Both HTTP chat and webhook flows use the same append path.

### Running flag lost on page refresh

**Root cause:** In `fetchSessions`, the `runningIds` set was captured at function entry (before the API call). SSE events (delivered during the call via `connectSSE`) updated `sessions.value` with `running: true`, but the stale `runningIds` didn't reflect this. When the API call returned, `sessions.value` was reassigned using the stale capture, overwriting the `running: true` flag.

**Fix:** Moved `runningIds` capture to after the API call, right before the reassignment. Any SSE events that arrived during the request are now reflected.

### No result visible after agent completes

**Root cause:** The `AssistantChat.vue` watch on `loading` collapses all work sections when `loading` goes from `true` to `false`. For webhook-originated sessions, `loading` never changes (no `sendMessage` call), so the work section stays in its default state. After the agent completes, the last turn's work remains collapsed, hiding the result text.

**Fix:** Added a watcher on the current session's `running` flag. When it transitions from `true` to `false`, the last turn's work section is expanded.

## Webhook Flow

When a Telegram (or other connector) webhook triggers the agent, the flow is:

1. Webhook receives message → `handleAgentMessage` persists it to disk, then returns immediately (200 OK to Telegram)
2. Agent runs in a background goroutine via `RunWithCancel` (same helper used by the HTTP chat handler)
3. Lifecycle events (`session_started`, `session_progress`, `session_completed`) are published to the workspace EventBus
4. Frontend SSE receives them → sidebar shows `running: true`, auto-selects the new session
5. When the agent finishes, `replyToChat` sends the reply via the connector's `Send()` method (the canonical reply path)
6. If cancelled via the cancel button (or a subsequent request), the context is cancelled and the agent stops

The `notify_user` tool is excluded from the agent in webhook context via the guardrail-derived tool schema (`DisabledToolNames` → `communication.enabled: false` manifest **default**, resolved at the agent's `resolveToolProvider` narrow waist), so with the default policy the model cannot call it. This prevents double-sends — the single reply path through `replyToChat` is the only mechanism. ⚠️ The exclusion is policy-conditional: if an operator enables Communication guardrails (or grants an override), `notify_user` is advertised to webhook sessions too, so the double-send guard no longer holds — the webhook channel has no structural `notify_user` exclusion anymore.

**Key files:**
- `chat.Service.Run` in `core/chat/service.go` — registers/unregisters the workspace's turn in its `running` map (the web handler and the webhook both call it)
- `handleAgentMessage` / `runAgentReply` in `webhook_handlers.go` — async agent execution
- `replyToChat` in `webhook_handlers.go` — sends via connector's `Send()` method
- `toolpolicy/tool_availability.go` — wraps `ToolProvider` to exclude tools per context / guardrail policy
