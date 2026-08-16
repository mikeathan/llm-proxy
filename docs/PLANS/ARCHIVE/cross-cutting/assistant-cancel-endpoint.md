---
status: superseded
last_reviewed: 2026-07-11
---

# Add explicit /assistant/cancel endpoint + in-process agent registry

**Date**: 2026-06-26
**Subsystem**: cross-cutting (assistant + transport)
**Files**:
- `backend/internal/transport/http/assistant_handlers.go`
- `backend/internal/transport/http/assistant_handlers_test.go`
- `backend/internal/app/bootstrap.go`
- `frontend/src/services/assistantService.ts`
- `frontend/src/composables/useAssistant.ts`

## What

The assistant HTTP handler ran agent execution on `context.Background()` so the frontend's `AbortController` (which cancels the HTTP request on Stop) had no effect on the in-flight agent. The agent kept running, kept invoking tools, kept streaming — even writing to the persisted session after the frontend had moved on. The next user message would land on a session that had been mutated by the orphan agent.

Added a cancellation path:
1. Backend `AssistantMessageHandler` now stores the per-request `context.CancelFunc` in a `sync.Map` keyed by `conversation_id`. The agent runs on `context.WithCancel(context.Background())` (still decoupled from the HTTP request lifetime so proxies can't kill idle connections).
2. New `POST /admin/api/conversation/cancel` endpoint body `{workspace_id, conversation_id}`. Handler looks up the registry, fires the cancel func, returns `{status, canceled}`.
3. Frontend `cancel()` in `useAssistant` now calls `AssistantService.cancelAgent(ws, sid)` before aborting the HTTP request. The backend's existing `context.Canceled` branch (assistant_handlers.go:247) now actually fires and the partial session is persisted correctly.

## Why

The original `context.Background()` was a deliberate choice (see comment in `assistant_handlers.go:64-68`): it kept the agent alive across browser disconnects and proxy idle timeouts. The bug was that there was no other path to cancel. An explicit `/cancel` endpoint preserves the disconnect-resilience intent while giving the user a real stop button.

The user-facing symptom was the "stale turn bleed" — the cancelled turn's reasoning text appearing in the next turn's `Result` section. Frontend-only fixes (clearing `liveReasoning`, fixing `groupTurns` to use `last.segments`) cleaned up the *visible* bleed but did not stop the orphan agent from mutating the session DB. This change stops the orphan at the source.

## Decisions

- **In-process `sync.Map` registry** (not DB-backed): cancels must be fast and per-instance. A multi-instance deployment would need a different mechanism (e.g. shared cancel channel) but that is out of scope.
- **Cancel via context, not signal**: agent code already respects `ctx.Done()` (see `stream.go` LLM stream). No new signal handling needed — the existing cancellation path at `assistant_handlers.go:247-266` works.
- **Idempotent cancel**: handler returns 200 even if no agent is running. Avoids frontend timing races.
- **Frontend does best-effort cancel**: `cancelAgent` errors are logged but don't block the abort. Cancel is also fire-and-forget at the call site so the UI responds immediately.
- **Session still persisted on cancel**: existing logic at line 249-252 saves the partial history. User sees what was streamed; the cancelled bubble is a real artifact.
- **Did NOT add SSE keep-alive heartbeats**: the original "decoupled from r.Context" concern was about HTTP idle timeouts. The agent is a POST request returning JSON, not an SSE stream — heartbeats don't apply. The registry + endpoint solve cancel without changing the request lifecycle.

## Tradeoffs

- `sync.Map` of cancel funcs is unbounded if requests pile up. In practice the defer cleanup runs on every request completion, so it stays small.
- Multi-instance deploy: each instance has its own registry. If a request hits instance A and cancel hits instance B, the cancel is a no-op. The frontend must hit the same instance — could be fixed with sticky sessions or a shared store, but the current deployment is single-instance.
- Frontend `cancelAgent` is awaited inside `cancel()`. If the backend is slow to respond, the user sees a brief delay before the UI unfreezes. Could be made fire-and-forget; left as await for now because the cancel call is fast in practice.

## Verification

- `go test ./...` — all green; 4 new cancel tests added (TestAssistant_CancelAgent_NotRunning, _Running, _MissingFields, _NotRunningReturns200).
- `npm run build` — frontend builds clean.
- Manual: send msg → click stop mid-stream → confirm backend log shows "assistant execution canceled by user" and no further tool calls execute.
- Manual: send msg → stop → send new msg → new turn starts cleanly with no bleed.

## Out of scope

- SSE keep-alive (not applicable; this is a POST)
- Multi-instance cancel coordination
- Existing orphan agent runs from before this change (will finish on their own via `GlobalTimeout`/`AgentTurnTimeout`)

---

# Refinement: workspace-keyed registry + cancel prior on new request

**Date**: 2026-06-26
**Subsystem**: cross-cutting (assistant + transport)
**Files**:
- `backend/internal/transport/http/assistant_handlers.go`
- `backend/internal/transport/http/assistant_handlers_test.go`

## What

Changed the cancel registry from `conversation_id` keying to `workspace_id` keying. On every new assistant request, the handler now cancels any prior in-flight agent for the same workspace and waits up to 2 seconds for it to fully exit (drain pending events, persist partial session, close run-log). This guarantees the orphan cannot continue publishing events to the workspace event bus after the user has moved on.

## Why

First cut (above) only canceled the prior when a separate `/assistant/cancel` request arrived. But the user's typical flow is: send a message → agent starts running → user sends another message (or hits Stop then sends). The second message creates a new assistant request, but if the first agent is still alive, both run in parallel and the first one's events keep getting published to the shared workspace event bus, which the new SSE connection receives.

By keying the registry on `workspace_id` and canceling on every new request, the second message arrival triggers an explicit cancel of the first, eliminating the cross-request bleed at the source.

## Decisions

- **One agent per workspace**: the registry enforces single-flight per workspace. This is restrictive in principle (no parallel agents in the same workspace) but matches the current UI model and prevents the bleed. If parallel agents are needed later, key the registry on `workspace_id+conversation_id` and handle the cross-conv case separately.
- **Wait up to 2s for the prior to fully exit**: the `done` channel on `runningAgent` is closed in the `defer` of `ServeHTTP`, which runs after `handleAssistant` returns (which respects `ctx.Done()` cooperatively). 2s is a balance — long enough to let most agents drain (cancel a stream, persist session, close run-log), short enough to not noticeably block the new request.
- **Timeout fallback**: if the prior doesn't exit within 2s, log a warning and proceed anyway. The new request runs; the orphan will eventually time out via `GlobalTimeout` or `AgentTurnTimeout`.
- **No frontend wait**: the 2s wait is server-internal. The frontend's `cancel()` already awaits the cancel HTTP round-trip; the next `sendMessage` can fire immediately because the server's internal cancel of the prior is handled on the new request's side, not the cancel's side.

## Tradeoffs

- A 2s wait on every new request is a small but real latency cost. Acceptable for a chat UI.
- If the user's first message is in flight and they send a second, the second's response is delayed by up to 2s. This is the right tradeoff — the alternative is two interleaved responses.
- The prior request's session is persisted in its `defer` cleanup, which runs after `handleAssistant` returns. The wait ensures that persistence has completed before the new request reads or writes the same session.

## Verification

- `go test ./...` — all green; 3 new tests added: `TestAssistant_CancelPriorForWorkspace_WaitsForDone`, `_TimesOut`, `_NoOp`.
- Manual: send msg → send another immediately → backend log shows "canceled prior in-flight assistant request for workspace" + first request's `assistant execution canceled by user`.
- Manual: no orphan tool calls after the cancel.

## Future work

- Per-conversation cancel (instead of per-workspace) would enable parallel agents in the same workspace. Out of scope until the UI supports it.

---

# Fix: honor ShouldTerminate + add char cap safety net (no budget changes)

**Date**: 2026-06-26
**Subsystem**: cross-cutting (assistant)
**Files**:
- `backend/internal/core/assistant/stream.go`
- `backend/internal/core/assistant/agent_test.go`

## What

Two minimal changes to `processStream` to stop runaway LLM streams (where the model never emits an EOS and just keeps generating, e.g. the local Qwen3.5-9B model stuck in a joke-loop scenario):

1. **Honor `ShouldTerminate`**: when the orchestrator's stream interceptor signals that token/reasoning budgets are exceeded, the stream now actually returns nil (ends). Previously it only logged a warning ("letting server enforcement handle it") and ignored the signal — but the upstream server often doesn't enforce, so the stream ran forever.

2. **Char cap safety net**: added a hard character cap of `maxTokens * 4` on accumulated content. Only fires as a fallback if the token counter underestimates output (which would be a token-counter bug). For a 2730 maxTokens model, cap = 10920 chars.

No budget values were changed. `maxTokens`, `reasoningBudget`, `stuckThresholdMultiplier`, `stuckNonReasoningDivisor` are all unchanged.

## Why

User scenario: send "tell me a joke" to local Qwen3.5-9B. Stream produced 7467 chars of content over 90+ seconds, no EOS, no final answer, no premature termination. Per-turn checks didn't fire because the model never completed a turn. The budget-exceeded warning was logged but the stream kept consuming chunks.

## Decisions

- **Single change point**: `processStream` only. The interceptor's logic is unchanged. The fix is purely about respecting what the interceptor already says.
- **`return nil` on `ShouldTerminate`**: signals the agent loop that the turn ended (cooperatively). The agent's per-turn handling (`isPrematureTermination`, `handleEmptyStream`, normal turn processing) will then evaluate the partial response. If the partial is a valid final answer, it gets used. If not, the empty-stream handler triggers a nag.
- **Char cap = 4x maxTokens**: large enough to not interfere with normal long responses (most models stay well under this), small enough to catch runaway cases within seconds rather than minutes. Disabled when `maxTokens == 0` for safety.
- **No budget value changes**: explicitly excluded per user direction. The fix is in honoring existing signals + adding a safety net, not retuning the budgets.

## Tradeoffs

- A `ShouldTerminate` mid-stream now produces a partial turn that the agent must evaluate. For most models this is correct (a partial response is still informative). For some pathological models the partial may be unusable — but the empty-stream handler will nag, and the next iteration can try again.
- The 4x char cap could theoretically cut off a legitimate 10920+ char response. In practice, models that hit this are already past their reasonable generation length.
- The token counter's accuracy is now load-bearing for performance. If it underestimates, the char cap catches it. If it overestimates, the stream ends early. This is unchanged from before — the difference is that the signal is now actually used.

## Verification

- `go test ./...` — all green; 4 new subtests added for `TestExceedsContentCharCap` (under cap, at cap, over cap, zero maxTokens disabled).
- Manual: send "tell me a joke" to local Qwen3.5-9B → confirm stream terminates within seconds after budget exceeded, returns either the partial joke or a nag prompt.
- Manual: send a normal request that fits the budget → confirm unchanged behavior (no premature termination).

## Out of scope

- Budget value tuning
- Reasoning budget changes
- Token counter accuracy improvements
- New nag prompts
- Empty-stream handler changes

---

# Refinement: bail out of streaming fallback chains on context.Canceled

**Date**: 2026-06-26
**Subsystem**: cross-cutting (assistant)
**Files**:
- `backend/internal/core/assistant/stream.go`
- `backend/internal/core/assistant/agent_test.go`

## What

Two fallback chains in `stream.go` were falling back to non-streaming on ANY error, including `context.Canceled`. This meant: user clicks Stop → LLM HTTP returns `context.Canceled` → fallback fires → non-streaming call ALSO fails with the canceled context → outer loop continues → agent keeps running. Observed 1m23s of post-cancel activity.

Added an `isUserCanceled` helper. Both fallback sites now check the error first and bail out (return the error) if the user canceled, before any retry path runs.

## Why

The fallback chains are designed to recover from provider errors (no streaming support, prefill rejection, XML retry failure). User cancellation is not an error to recover from — it's a signal to stop. The outer loop's `s.ctx.Err()` check is the right place to handle termination; the fallback chain should get out of the way.

## Decisions

- **Single helper `isUserCanceled`** — `errors.Is(err, context.Canceled)`. Kept the name explicit ("user" not "context") to make intent obvious at the call site.
- **Bail-out is an early return** at the top of each fallback site, not a guard around the entire function — the existing prefill/XML-retry logic still runs for non-cancel errors.
- **No fallback inside the bail** — when the user cancels, we return the error immediately. Even if the prefill retry would succeed (unlikely with a canceled context), we don't try.

## Cyclomatic complexity

The two fallback sites were already heavy on nested `if` blocks. The bail-out is a flat early-return that *reduces* the nesting depth at the original call site:
- Site 1 (`computeNextResponse`): cancel check happens before the prefill block and again after the prefill retry. Each is a single-line early return, no additional nesting.
- Site 2 (`computeNextResponseStreamXML`): cancel check is a single-line early return before the existing `if err != nil` warn-and-fallback block.

Net complexity: ~flat. The change doesn't add depth; it short-circuits before the nested blocks.

## Verification

- `go test ./...` — all green; 2 new tests: `TestAgent_CancelDuringStream_NoFallbackToNonStreaming` (primary streaming path), `TestAgent_CancelDuringStreamXMLRetry_NoFallbackToNonStreaming` (XML retry path). Both verify the non-streaming `ChatFunc` is never called when the stream returns `context.Canceled`.
- Manual: send "list all files" → wait → click Stop → confirm agent exits within ~1s (only the in-flight LLM HTTP acknowledge time).

## Out of scope

- Forcing in-flight tools to abort mid-execution (current behavior: in-flight tool completes, no new tool calls).
- Adding `ctx.Err()` checks between tool execution and the next LLM call (already in place at top of `run()` loop).
- Multi-instance cancel coordination.

---

# Refinement: don't lose session_id on cancel

**Date**: 2026-06-26
**Subsystem**: cross-cutting (assistant)
**Files**:
- `backend/internal/transport/http/assistant_handlers.go`
- `backend/internal/transport/http/assistant_handlers_test.go`
- `frontend/src/services/assistantService.ts`
- `frontend/src/composables/useAssistant.ts`

## What

Cancelled turns were orphaned in their own session because the frontend only set `currentSessionId` in the `sendMessage` success path — and on cancel the success path never ran. The frontend's `abortController.abort()` discarded the cancel response that the backend DID send (containing `conversation_id`). Net effect: the cancelled turn became a separate conversation, the next send created yet another.

Two changes:

1. **`CancelAssistantHandler` now accepts an empty `conversation_id` (cancel by workspace)** and echoes `conversation_id` back in the response. The registry has always been workspace-keyed, so the cancel lookup never needed the conv_id — but the validation rejected the request when the frontend didn't have one. The echoed field lets the frontend learn the session id from the cancel response if it didn't have one at click time.

2. **`useAssistant.cancel()` no longer aborts the original HTTP request** and no longer requires `currentSessionId` to be set. It calls the cancel endpoint with whatever `currentSessionId` is available (possibly empty), stores the echoed `conversation_id` from the response, and lets the original `await AssistantService.sendMessage(...)` complete naturally. The cancel signal causes the backend's `handleAssistant` to return the cancel response (with the real `conversation_id`), which the existing success path stores as `currentSessionId`.

## Why

The user clicked Stop on the first send (no `currentSessionId` yet), then sent a second message. The two became separate sessions because:
- The frontend's `if (ws && sid)` guard rejected the cancel call (no sid yet).
- The abort killed the HTTP request that would have returned the session_id.
- The next send had no `currentSessionId`, so the backend minted a new one.

Now: the cancel endpoint can be called with just a workspace_id; the response carries back the session_id; the next send reuses it.

## Decisions

- **Cancel-by-workspace is allowed, not required**: existing callers sending both workspace_id and conversation_id still work. The handler logs both fields.
- **Echoed `conversation_id` is the same value the request sent** — even if empty. The frontend's in-flight `sendMessage` is the source of truth for the real session_id (the cancel endpoint doesn't know which session the agent is working on, only the workspace).
- **Abort removed from `cancel()`** but the `AbortController` itself is kept: the constructor is still used on each `sendMessage` and the signal is still passed to `fetch` (line 141). Removing the abort call from `cancel()` is the only change. If the request times out or the user navigates away, the abort signal is still useful.
- **`fetchSessions` and `newSession` behavior unchanged** — only the cancel path learned a new way to discover `conversation_id`.

## Tradeoffs

- UI is still idle-immediately on cancel (`loading.value = false` set synchronously after the await). The original HTTP request may still be in flight for ~1s after the UI unfreezes. The next send is gated by `if (loading.value) return` in `sendMessage` (line 80), so the user can't start a new send until the original completes. This is a small latency cost: ~1s between cancel and being able to type the next message. Acceptable — the alternative is sending a second message into a request whose `currentSessionId` we just learned.
- **No new state machines**, no new event types, no new ID generation. Pure re-plumbing of the existing cancel signal.

## Verification

- `go test ./...` — all green; 1 new test (`TestAssistant_CancelHandler_EmptyConvIDAllowed`), 1 test updated to verify echoed `conversation_id`.
- `npm run build` clean.
- Manual: send "list files" → click Stop → send "tell me a joke" → confirm both turns in the same session; backend log shows `assistant cancel requested | canceled=true`.

## Out of scope

- The fact that `builder.finalize("")` may add an empty assistant message on cancel (partial segments render + empty `Result` section). Separate concern; the visible artefact is the cancelled bubble with its partial reasoning and an empty result. Fixable later.
- `lastUserMessage` tracking — currently the cancel path derives the snippet by walking `messages.value` for the last user role. This is good enough; a dedicated ref would add state for negligible UX benefit.
- Frontend test for the new cancel flow. The behavior is hard to assert without a real fetch/HTTP mock; manual verification covers it.
