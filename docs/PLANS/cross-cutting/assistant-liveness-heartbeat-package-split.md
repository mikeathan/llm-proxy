# Plan: Assistant Liveness Heartbeat & Package Restructure

**Status:** `partial` — §2 (heartbeat), §3 (frontend empty-bubble fix), §4.1 (reasoning extract) **implemented & verified**; §4.2 (events extract) **rejected** (see §4.2 note); §4.3/§4.4 deferred.
**Created:** 2026-08-18
**Last Updated:** 2026-08-18
**SPECs affected:** SPEC-001 (agent loop), SPEC-010 (loop strategies), SPEC-003 (assistant UI / event streaming)
**Subsystems:** cross-cutting (assistant package, proxy/stream, assistant-ui)
**Rules:** `.agents/rules/go-staff-engineer.md`, `.agents/rules/frontend-vue-engineer.md`
**Constitution:** II.13 (prompt centralization), II.14 (goroutine lifecycle), IV.4 (no dead code)

---

## 0. Context & root cause

An assistant run against a slow cloud provider (`deepseek-v4-flash` via NVIDIA NIM,
`reasoning_mode = ModeEnableThinking`) produced the symptoms:

- an **empty assistant bubble** for ~2.5 min that "looked broken", and
- "eventually started outputting reasoning", then completed successfully.

Timeline from `runs/workspace-1/deepseek-v4-flash-0731/conv_20260818083057/events.jsonl`
(all times UTC):

| time | event | meaning |
|---|---|---|
| 07:30:57 | `NewAgent` + `step_start` + `agent_thinking` | agent ready; UI told "thinking" once |
| 07:31:23 | `stream request sent` | **26 s inside `Client.Stream`→`doRequest` waiting for response headers = provider TTFT** |
| 07:31:53 | heartbeat `reasoning_len 3` | model produced only `"Let"` in 30 s |
| 07:32:23 | `unexpected EOF` + `stream completed content_len 0` | **provider dropped the stream after 60 s of silence** |
| 07:32:23 | `empty response with native tools, returning stuck signal` + `re-arming nudge` | recovery ladder engaged |
| 07:32:30+ | tool calls → … → completed | recovered correctly |

### Root cause is two-fold

1. **Provider slowness (not our code).** Proof: `StreamChunkTimeout = 5 * time.Minute`
   (`proxy/client.go:56`) — we did **not** close the stream; the server did
   (`io.ErrUnexpectedEOF`). The 26 s TTFT is `doRequest` waiting for headers
   (`ResponseHeaderTimeout = 10 min`). Prior logs show `529 NVIDIA NIM "Service
   temporarily overloaded"` retries. **Recovery already works** — the run completed
   via the `[stuck]` → nag ladder.

2. **UX gap (our code).** The agent-side heartbeat exists but is inconsistent:
   - `stream.go:576` `streamHeartbeatInterval = 30 s` — **log-only** goroutine
     (`Logger.Info("stream still generating")`), never emits to the UI.
   - `stream.go:793` `nonStreamHeartbeatInterval = 15 s` — **emits**
     `notifyLifecycle("fallback_waiting", {elapsed})` to the UI.
   - The one-shot `agent_thinking` lifecycle fires once per LLM call.

   Two near-identical heartbeat goroutines, one of which is invisible to the
   frontend — the common (streaming) path is the one that only logs.

   Separately, `useAssistant.ts:289-296` clears `loading` in a `finally` block the
   instant the 202 POST returns (the comment above it even says "keep loading=true"),
   so the ChatBubble's render guard `loading && isLastTurn`
   (`ChatMessages.vue:130`) collapses the bubble during the whole slow phase.

---

## 1. Scope

- **In scope:** a single generic heartbeat component (agent-side liveness only),
  wired to emit `still_thinking`; the frontend `loading` lifecycle fix + `still_thinking`
  rendering; and two low/medium-risk package extractions to shrink the oversized
  `assistant` package (24 non-test files, ~17 kLOC).
- **Out of scope (deferred):** SSE transport keepalive; maintenance-ticker loops
  (EventBus reaper, EventSink sync, ledger cleaner, …); loop-strategy extraction;
  mega-file splitting (`stream.go` 993, `session.go` 950, `tool_exec.go` 778,
  `agent.go` 738).

---

## 2. Feature: generic `core.Heartbeat`

New file `backend/internal/core/heartbeat.go` (package `core`, beside
`ttl_cache.go` — precedent for a generic single-file reusable component). Pure:
depends only on `context` + `time`.

> **Placement note:** `heartbeat.go` goes in the **parent** `package core`
> (`backend/internal/core/`), NOT a subpackage. This differs from the `reasoning`/
> `events` extractions (leaf subpackages, §4). It is cycle-safe: `core` does not
> import `assistant`, and `assistant` already imports `core`
> (`conversation_helpers.go`, `registry.go`). The `proxy → assistant/prompts`
> import is a leaf, not a back-edge.

```go
type Heartbeat struct {
    C         <-chan time.Time // one tick per interval (dropped if owner busy)
    c         chan time.Time
    stop      chan struct{}
    startOnce sync.Once
    stopOnce  sync.Once
}

func NewHeartbeat() *Heartbeat                      // allocate only
func (h *Heartbeat) Start(ctx context.Context, interval time.Duration) // idempotent; <=0 disables
func (h *Heartbeat) Stop()                          // idempotent
```

- Owns a ticker goroutine; forwards ticks to a **buffered size-1 channel** consumed
  by the owner's `select` (channel-back-to-owner-loop). Non-blocking drop when the
  owner is busy.
- Goroutine exits on `ctx.Done()` or `Stop()` (Constitution II.14).
- **Mechanism only** — the phase/payload is supplied at each call site (extensible).

### 2.1 Stream path (`processStream`, stream.go)

- Delete the log-only heartbeat goroutine (`stream.go:575-590`) and its
  `atomic.Int64` counters.
- `hb := core.NewHeartbeat(); hb.Start(ctx, streamHeartbeatInterval); defer hb.Stop()`.
- Add `case <-hb.C:` to the read-loop `select` (`stream.go:601`). On tick:
  1. keep the existing `"stream still generating"` log;
  2. emit `still_thinking{elapsed}` **only when no content/reasoning advanced since
     the last tick** (silent-stall gate — no noise during active streaming).
     Reuse the existing `streamContentLen`/`streamReasoningLen` atomics
     (stored at `stream.go:700-702` in the read loop, already loaded by the
     heartbeat goroutine at `581-582`) — compare values at the previous tick vs
     the current tick; do **not** add new counters.

### 2.2 Non-stream path (`computeNextResponseNonStreaming`, stream.go)

- Replace the `fallback_waiting` goroutine (`stream.go:789-808`) with the same
  `core.Heartbeat`, consumed via a small owner `select` that wraps the blocking
  `Client.Chat` in a result channel (`tick` / `result` / `ctx.Done`). All retries
  (prefill / tool-support / unsupported-param) reuse the wrapper.
- **Keep the `fallback_waiting` phase** (safe default — `icons.ts:26,60` already
  consumes it; no contract break).

### 2.3 Lifecycle phase

- Add `PhaseStillThinking = "still_thinking"` to `agent_events.go` beside
  `PhaseAgentThinking`. Content-free; payload `{elapsed}` only. `agent_thinking`
  remains one-shot (contract enforced by `reasoning_working_test.go:147`).

### 2.4 Merge duplicated code

The two hand-rolled ticker goroutines (`stream.go:575-590`, `stream.go:789-808`)
are the duplication being collapsed into `core.Heartbeat`. The
`streamHeartbeatInterval` / `nonStreamHeartbeatInterval` constants stay (different
cadences for different phases).

---

## 3. Frontend: empty-bubble fix

1. **`useAssistant.ts` — `loading` lifecycle** (`289-296`): stop clearing `loading`
   on 202 return. Keep `loading=true` on success (cleared later by
   `session_completed` in `applySessionUpdate:387-395`); clear only on non-abort
   error.
   - **Exact edit:** in the `finally` block, delete the `loading.value = false`
     assignment at `useAssistant.ts:293` (the non-abort branch). The abort branch
     (`289-292`) is unchanged — a navigation abort leaves the run alive. The
     "keep loading=true" comments at `277` and `290-291` already describe the
     intent; this just makes the code match them.
   - The `session_started` re-arm (`351-366`) then behaves correctly (no race)
     because `loading` is already `true` for normal sends — the guard
     `if (p.snippet && !loading.value)` at `359` correctly skips re-pushing the
     user message that `sendMessage()` already added.
2. **`messageBuilder.ts` — handle `still_thinking`** (`268-280`): treat like
   `agent_thinking` — from `idle` set `phase='thinking'`, `thinking=true`; idempotent.
   Unknown phases already fall through safely, so this is additive.
3. **`ChatBubble.vue` — elapsed while thinking:** `useElapsedTimer` already starts on
   `loading && isLastTurn` (`33-36`); extend the header to show `· {seconds}s`
   during `thinking`/`working`, not only `done` (`117`).
4. **`icons.ts`:** add `still_thinking: "💭"` to `LIFECYCLE_ICONS` + a
   `getPhaseMessage` case (`Thinking… elapsed Xs`).

---

## 4. Refactor: shrink the `assistant` package (low/medium risk)

The package is 24 non-test files / ~17,289 lines, plus 27 test files. Structural
smells: god-struct `Agent` (56 methods across 6 files), unexported `runSession`
referenced by the whole strategy cluster, and 4 mega-files.

### 4.1 (Low) Extract reasoning wire params — `reasoning_param.go`

`reasoning_param.go` (293 lines) is self-contained — no `func (a *Agent)` methods;
it only operates on `models.ChatRequest`, `ReasoningSpec`, `ReasoningCapability`.
Move to `backend/internal/core/assistant/reasoning` (package `reasoning`), a leaf
like the existing `prompts/` and `guardrails/` subpackages. Export the
`ReasoningSpec`, `ReasoningMode`, `ReasoningEffort`, resolvers, and
`ReasoningCapabilityFor` capabilities map; the `assistant` package re-imports and
re-exposes the few symbols used by `handlers` (`ReasoningCapabilityFor`,
`ProviderTuningDefaults`).

### 4.2 (Low logic / high churn) Extract event vocabulary — `backend/internal/core/events`

> **DECISION (2026-08-18): REJECTED — not worth it.** The stated rationale —
> "removes the `automation → assistant` import" — is **not achievable**: `automation`
> still imports `assistant` for agent construction regardless of the vocabulary move
> (`assistant.NewAgent`, `AgentOptions`, `Engine`, `ParseLoopStrategy`, `LoadAgentsFile`,
> `NewAllowedToolsProvider`, `ToolProvider`, `WithUsageTracker`, `GetUsageTracker`,
> `DefaultMaxSteps`, `LoopStrategyName`). The cycle this would "fix" is already solved at
> the interface level (`conversation.go:11` — `EventPublisher` exists "to avoid a cyclic
> dependency with the automation package"). Cost: a new leaf package + type aliases in
> `assistant` (16 files reference the moved symbols → two names for the same type) + re-point
> ~10–15 cross-package files (`automation/*`, `handlers/*`, `app/*`, `mocks/*`), for **zero
> runtime/behavior change**. No consumer is blocked by the assistant dependency for
> vocabulary alone. **Left unimplemented.** (Revisit only if `automation`'s agent-construction
> coupling is ever extracted to a shared package.)

Move the **event-type vocabulary only** out of `agent_events.go` into a new leaf
package `backend/internal/core/events` (both `assistant` and `automation` import it
— this removes the `automation → assistant` import and is the highest-value
extraction):

- `AgentEventType` + `EventStepStart` … `EventLifecycle`
- `EventChannel` + `ChannelAssistant` / `ChannelAutomation`
- `AgentEvent` struct
- lifecycle phase constants (`PhaseSessionStarted`, `PhaseSessionProgress`,
  `PhaseSessionCompleted`, `PhaseAgentThinking`, `PhaseStillThinking`)
- payload structs `GuardrailBlockedPayload`, `GuardrailDecision`,
  `GuardrailInvalidatedPayload`
- `Observer` type

**Stays in `assistant`** (service/domain, referenced outside): `EventPublisher`,
`EventRecorder` (`conversation.go`), `GuardrailDecisionStore` (`agent.go`), the
`notify*` methods on `*Agent`, and all agent config/tuning symbols
(`AgentOptions`, `DefaultMaxSteps`, `LoopStrategyName`, `ProviderTiers`, …).

`EventPublisher.Publish(workspaceID, events.AgentEvent)` and
`EventRecorder.Write(events.AgentEvent)` re-point to the new type; `automation`'s
`EventBus` implements the updated signature mechanically.

> **Sequencing (recommended):** land the heartbeat feature (§2-§3) in its own
> commit, and land the §4 refactors (especially 4.2, which touches ~10 files:
> `conversation.go`, `automation/*`, `handlers/*`, `mocks/*`) as a separate
> commit. Mechanical but wide-blast-radius — isolating it lets a `git bisect`
> cleanly attribute any regression to either the feature or the refactor.

### 4.3 (Medium) Loop-strategy cluster — **deferred**

`loop_strategy.go` + `react_strategy.go` + `plan_execute_strategy.go` +
`evaluator_optimizer_strategy.go` + `evaluator_guard.go` + `stop_guard.go` +
`loop_resolver.go` are hard-coupled to the **unexported** `runSession`
(`LoopStrategy.Run(ctx, *runSession)`). Extracting them requires exporting a session
abstraction (rename `runSession`→`Session` + export ~30 methods/fields, or define a
narrow `LoopSession` interface) — that leaks loop internals and is closer to
high-risk. **Defer** until the mega-file split (4.4) gives `session.go` a smaller,
exportable core.

### 4.4 (High) Mega-file split + de-god `Agent` — **deferred**

`stream.go`/`session.go`/`tool_exec.go`/`agent.go` (3,459 lines across 4 files)
need a responsibility split. Depends on 4.3; out of scope for this plan.

---

## 5. Files affected

**New**
- `backend/internal/core/heartbeat.go` (+ `heartbeat_test.go`)
- `backend/internal/core/events/*.go` (event vocabulary; 4.2)
- `backend/internal/core/assistant/reasoning/*.go` (4.1)
- `docs/PLANS/cross-cutting/assistant-liveness-heartbeat-package-split.md` (this doc)

**Modified (backend)**
- `backend/internal/core/assistant/stream.go` (heartbeat wiring, 2.1/2.2)
- `backend/internal/core/assistant/agent_events.go` (`PhaseStillThinking`; later split in 4.2)
- `backend/internal/core/assistant/conversation.go` + `conversation_service.go` (event type re-point, 4.2)
- `backend/internal/core/assistant/agent.go`, `agent_builder.go` (import `events`, `reasoning`)
- `backend/internal/core/automation/{broadcast,eventsink,executor,dispatcher}.go` (event type re-point, 4.2)
- `backend/internal/transport/http/handlers/*.go`, `backend/internal/app/*.go`, `backend/internal/testing/mocks/*.go` (same)

**Modified (frontend)**
- `frontend/src/composables/assistant/useAssistant.ts` (loading fix)
- `frontend/src/utils/message/messageBuilder.ts` (still_thinking)
- `frontend/src/components/AgentIde/assistant/ChatBubble.vue` (elapsed while thinking)
- `frontend/src/constants/icons.ts` (still_thinking icon + message)

---

## 6. Tests (TDD: red → green → refactor)

**Backend**
- `backend/internal/core/heartbeat_test.go`: `t.Run` table — ticks fire; stops on
  `ctx.Done()`; stops on `Stop()`; `Stop()` idempotent; `interval <= 0` disables.
- Stream: `t.Run` — slow stream (mock delayed chunks) emits `still_thinking`; verify
  `agent_thinking` still precedes content and stays content-free (reuse
  `reasoning_working_test.go` patterns).
- Non-stream: `fallback_waiting` still emitted via `Heartbeat`.

**Frontend**
- `src/__TESTS__/utils/message/messageBuilder.test.ts`: `still_thinking` flips
  `idle→thinking`; no-op after `done`.

---

## 7. Verification

```bash
cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/
cd frontend && npm test && npm run build
```

Run `go test -race ./internal/core/assistant/...` after the heartbeat wiring
(goroutine lifecycle changed). Pre-Completion Review per `AGENTS.md`.

---

## 8. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Goroutine leak | `Heartbeat.Stop()` + `ctx.Done()`; `defer hb.Stop()` at both call sites |
| Event-bus spam / replay | silent-stall gate + 15/30 s cadence; `recent` buffer already capped (1000) |
| Phase drift | `PhaseStillThinking` constant + matching `icons.ts` entry |
| `agent_thinking` contract | separate phase; content-free; ordering preserved |
| `fallback_waiting` consumer break | keep phase unchanged (2.2); only the mechanism is replaced |
| Refactor churn (4.2) | mechanical re-point; full `go build ./... && go test ./...` after; no behavior change |
| `runSession` over-export (4.3) | deferred — not attempted in this plan |

---

## 9. Deferred (tracked, not lost)

- SSE transport keepalive (Kind 2) — reuse `core.Heartbeat` in
  `dispatcher_handlers.StreamWorkspaceEvents` when proxies drop idle streams.
- Loop-strategy extraction (4.3) + mega-file split (4.4) — separate plan after 4.2.
