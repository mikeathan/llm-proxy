# Unify Automation + Assistant Event Consumption (single configurable path)

**Status:** `complete`
**Created:** 2026-07-18
**Subsystems:** assistant-ui (SPEC-003), automation (SPEC-007)
**Branches:** `task/universal_agent_completion`
**Supersedes:** the ad-hoc `automationEventsToMessages` bridge from
`docs/PLANS/assistant-ui/automation-unified-renderer-and-report-truncation.md`.

---

## 0. Goal & principle

One code path for **event → message consumption** and one **renderer**. Chat and
automation differ **only by configuration**, never by duplicated logic. Future
automation UI customization flows through the shared `mode` prop + documented
extension points — no fork in logic.

Decisions locked with the user:
- **Customization model = A**: mode-driven prop/slot surface on the shared
  renderer + configurable builder options. No separate theming/config object.
- **Finalize strategy = `lifecycle{completed}` + run-end fallback** (recommended,
  and the one closest to the Hermes Agent completion model the backend already
  follows: `session.go:268` — "no tool calls + substantive content = done",
  announced by `completeWith` → `lifecycle phase:"completed"` at `session.go:160`).

---

## 1. Root cause (verified)

The backend emits `reasoning` and `tool_stream` as **full cumulative snapshots**
every chunk — NOT deltas:
- `stream.go:661` → `a.notify(EventReasoning, fullMsg.ReasoningContent)`
- `stream.go:666` → `a.notify(EventToolStream, displayContent)` (full content)

The **chat** consumer (`useMessageBuilder.handleToolStream`,
`messageBuilder.ts:176-200`) dedups via prefix-replace:
```ts
if (clean.startsWith(lastClean)) { reasoningBuffer = clean; ... }  // REPLACE
```
The **automation** bridge (`automationEventsToMessages.ts`) used
`current.reasoning_content += ev.payload` (**APPEND**). So each full re-emit was
concatenated → the "The user wants me to…" cascade, and the single ever-growing
element broke auto-scroll (smooth scroll never caught up under constant
re-layout). Fix = delete the bridge, route automation through the shared
builder. Do **not** change the backend emission (chat depends on full re-emits).

---

## 2. Architecture (single path)

```
                         AgentEvent[]  (SSE live + history snapshot)
                                     │
                  ┌──────────────────┴───────────────────┐
            CHAT (useAssistant)                    AUTOMATION (useLiveConsole)
                  │                                        │
                  └──────────────┬─────────────────────────┘
                                 ▼
                    useMessageBuilder(messages, options)   ← SINGLE CONSUMER
                      • dedup cumulative re-emits          (prefix-replace)
                      • builds segments inline             (reasoning/tool_call)
                      • options.source / headerMessage /
                        finalizeOn drive variation
                                 │
                                 ▼  messages: AssistantMessage[]
                          groupTurns(messages) → turns: Turn[]
                                 │
                                 ▼
                    ChatMessages  (mode: 'chat' | 'automation')   ← SINGLE RENDERER
                      • automation hides welcome/retry/UserMessage
                      • #run-header slot = future customization point
```

---

## 3. Implementation steps (exact)

### Step 1 — `frontend/src/utils/message/messageBuilder.ts`
Make `useMessageBuilder` the single configurable consumer.

**1a. Add options interface** near the top (after `InsetPhase`):
```ts
export interface MessageBuilderOptions {
  source?: 'chat' | 'automation'
  // automation seeds a synthetic leading user message so groupTurns forms a
  // single clean turn (automation runs have no chat prompt of their own).
  headerMessage?: AssistantMessage
  // 'explicit'  → caller invokes finalize(reply) (chat: HTTP response).
  // 'lifecycle' → builder finalizes on lifecycle{phase:'completed'}
  //               (automation: Hermes-aligned, loop-announced completion).
  finalizeOn?: 'explicit' | 'lifecycle'
}
```

**1b. Change signature** (`messageBuilder.ts:30`):
```ts
export function useMessageBuilder(
  messages: Ref<AssistantMessage[]>,
  options: MessageBuilderOptions = {},
) {
  const opts = { source: 'chat' as const, finalizeOn: 'explicit' as const, ...options }
  let finalized = false
  let lastReply = ''
  // ... existing locals (assistantIdx, lastClean, reasoningBuffer, ...) unchanged
```

**1c. Dedup stays** exactly as-is in `handleToolStream` (`messageBuilder.ts:176`).
This is the single source of the cumulative-re-emit handling — do not duplicate
it anywhere else.

**1d. Track final reply.** In `handleEvent`'s `message` case
(`messageBuilder.ts:161`), after `handleMessage(msg)`, record the final text:
```ts
if (msg.role === 'assistant' && msg.content) {
  lastReply = msg.content
}
```

**1e. Lifecycle-triggered finalize.** In `handleEvent`, add a `lifecycle` case
(handles both chat-ignored and automation completion):
```ts
case 'lifecycle': {
  const p = ev.payload as Record<string, any>
  if (opts.finalizeOn === 'lifecycle' && p?.phase === 'completed' && !finalized) {
    finalized = true
    builderFinalize(typeof p.content === 'string' && p.content ? p.content : lastReply)
  }
  return
}
```
where `builderFinalize` is the existing `finalize` function body (rename the inner
`finalize` to `builderFinalize` or keep `finalize` and call it). Keep `finalize`
exported for the chat/explicit path.

**1f. Reset must clear `finalized`/`lastReply`.** In `reset()`
(`messageBuilder.ts:256`) add `finalized = false; lastReply = ''`.

**1g. Return** — unchanged shape: `{ handleEvent, finalize, reset, streaming,
thinking, liveReasoning, paused, phase, resetPauseTimer }`. (The `source`/
`headerMessage` are read from `opts`, not returned.)

> No behavioral change for chat: `finalizeOn` defaults to `'explicit'`, and chat
> never sends a `lifecycle{completed}` into the builder (chat's SSE filters to
> assistant channel; lifecycle completion is handled out-of-band). So chat still
> finalizes via the HTTP reply exactly as today.

### Step 2 — `frontend/src/composables/automation/useLiveConsole.ts` (rewrite core)
Current file: 100 lines, imports `automationEventsToMessages` (line 7), exposes
`displayMessages` as a computed mapping events→messages (lines 23-25). Replace
with builder-driven consumption.

**2a. Imports** — replace line 7:
```ts
import { useMessageBuilder } from '../../utils/message/messageBuilder'
```
Drop `automationEventsToMessages`. Keep `getMsgPayload`, `generateId`, `post`,
`AssistantMessage` types.

**2b. State + builder**:
```ts
const liveEvents = ref<AgentEvent[]>([])
const pendingDecision = ref<GuardrailBlockedPayload | null>(null)
const handledDecisionIds = new Set<string>()

const messages = ref<AssistantMessage[]>([])
const builder = useMessageBuilder(messages, {
  source: 'automation',
  finalizeOn: 'lifecycle',
  headerMessage: { role: 'user', content: runName ? `Automation run: ${runName}` : 'Automation run' },
})

const displayEvents = computed(() => liveEvents.value.length ? liveEvents.value : (historyEvents() || []))
// displayMessages IS the builder's live messages ref (single source of truth)
const displayMessages = messages
```

**2c. `handleAgentEvent`** — keep guardrail + booting logic; feed everything
else to the builder:
```ts
const handleAgentEvent = (ev: AgentEvent) => {
  if (ev.id && liveEvents.value.some(e => e.id === ev.id)) return
  if (!ev.id) (ev as any).id = generateId()

  if (ev.type === 'message' && getMsgPayload(ev).content?.includes('▶ Booting automation:')) {
    resetRun()
    return
  }
  if (ev.type === 'guardrail_blocked') {
    const payload = ev.payload as GuardrailBlockedPayload
    liveEvents.value.push(ev)
    if (!handledDecisionIds.has(payload.decision_id)) pendingDecision.value = payload
    return
  }
  if (ev.type === 'guardrail_invalidated') {
    const payload = ev.payload as { decision_id: string; reason: string }
    handledDecisionIds.add(payload.decision_id)
    if (pendingDecision.value?.decision_id === payload.decision_id) pendingDecision.value = null
    liveEvents.value.push(ev)
    return
  }
  // All other events flow through the shared builder (single consumption path).
  builder.handleEvent(ev)
}
```

**2d. Run lifecycle helpers**:
```ts
function resetRun() {
  builder.reset()
  pendingDecision.value = null
  handledDecisionIds.clear()
  messages.value = [{
    role: 'user',
    content: runName ? `Automation run: ${runName}` : 'Automation run',
  }]
}

// Run-end fallback: if the live run ends without a lifecycle{completed}
// (interrupted), finalize with the last streamed answer so it is not dropped.
let wasExecuting = false
watch(
  () => _isExecuting(),
  (executing) => {
    if (wasExecuting && !executing) {
      const last = [...messages.value].reverse().find(m => m.role === 'assistant' && m.content)
      if (last) builder.finalize(last.content)
    }
    wasExecuting = !!executing
  },
  { immediate: true },
)
```
> Note: `_isExecuting` is the existing param (currently unused, prefixed `_`).
> It is `() => boolean | undefined`. Wire it here.

**2e. `connect` / `clearEvents`** call `resetRun()` then replay history:
```ts
const connectWithReset = () => {
  resetRun()
  for (const ev of (historyEvents() || [])) builder.handleEvent(ev)
  sse.connect()
}
```
Expose `connect: connectWithReset`. Keep `disconnect`, `submitDecision`.
`clearEvents` → `resetRun()` + `sse` clear.

**2f. Return** — replace `displayMessages` computed with the `messages` ref;
expose builder reactive state:
```ts
return {
  liveEvents,
  displayEvents,
  displayMessages,           // Ref<AssistantMessage[]> = builder.messages
  thinking: builder.thinking,
  liveReasoning: builder.liveReasoning,
  paused: builder.paused,
  phase: builder.phase,
  isConnected: sse.isConnected,
  pendingDecision,
  connect: connectWithReset,
  disconnect,
  clearEvents,
  submitDecision,
}
```

### Step 3 — `frontend/src/components/AgentIde/automation/AutomationDetails.vue`
Current file already imports `useLiveConsole`, `groupTurns`, `ChatMessages`
(`mode="automation"`), `GuardrailBanner`; builds `automationTurns` from
`displayMessages` and passes static `loading//thinking/liveReasoning/paused/phase`.
Update to use builder state.

**3a. Destructure builder state** (currently lines 50-58):
```ts
const {
  displayMessages, thinking, liveReasoning, paused, phase,
  isConnected, pendingDecision, connect, disconnect, clearEvents, submitDecision,
} = useLiveConsole(...)
```
Remove `automationTurns` computed and the static `automationPhase`/`insetCollapsed`/
`isInsetCollapsed`/`isSegExpanded` placeholders — keep `insetCollapsed`/
`expandedSegments`/`isInsetCollapsed`/`isSegExpanded` as they were (passed to
`ChatMessages`), but now `turns` is computed from `displayMessages`:
```ts
const automationTurns = computed(() => groupTurns(displayMessages.value))
```

**3b. `ChatMessages` binding** — replace static values:
```html
<ChatMessages
  mode="automation"
  :messages="displayMessages"
  :turns="automationTurns"
  :loading="showLiveUI"
  :thinking="thinking"
  :live-reasoning="liveReasoning"
  :paused="paused"
  :last-message-is-user="false"
  :workspace-id="automation.workspace"
  :turns-collapsed="insetCollapsed"
  :expanded-segments="expandedSegments"
  :is-inset-collapsed="isInsetCollapsed"
  :is-seg-expanded="isSegExpanded"
  :phase="phase"
/>
```
(Keep `GuardrailBanner`, status dot, connect/disconnect `onMounted`/`onUnmounted`/
`watch(workspace)` exactly as already present.)

### Step 4 — `frontend/src/components/AgentIde/assistant/ChatMessages.vue`
Add the `#run-header` extension slot (documented customization point). No logic
change to the `mode` gating already present (lines 29-31, 81, 98).

**4a. Template**: inside `.message-container`, before the welcome card, add:
```html
<slot name="run-header" :phase="phase" :is-automation="isAutomation" />
```
**4b. Keep** `mode?: 'chat' | 'automation'` prop (line 26) and `isAutomation`
gating. This is THE customization surface: future automation-specific chrome
(badges, timers, step tracker) slots in via `#run-header` without forking logic
or touching chat.

### Step 5 — DELETE `frontend/src/utils/message/automationToMessages.ts`
Verified only `useLiveConsole` imported it. After Step 2a it has zero importers.
Delete the file.

---

## 4. Data flow (end-to-end, automation)

1. `AutomationDetails.onMounted` → `connect()` → `resetRun()` (seeds synthetic
   user msg) → replays `activeRun.events` through `builder.handleEvent` →
   `sse.connect()`.
2. SSE `reasoning`/`tool_stream` chunks arrive → `builder.handleEvent` →
   `handleToolStream` dedups (prefix-replace) → single growing reasoning inset.
3. `tool_call`/`tool_result` → `builder` appends `tool_call` segment, then
   resolves it to `success`/`error` via `handleToolResult`.
4. `message` (assistant, content) → `handleMessage` commits reasoning; `lastReply`
   updated. (Intermediate texts between tool turns are NOT pushed as separate
   bubbles — identical to chat behaviour.)
5. `lifecycle{phase:'completed', content}` → builder `finalize(content)` →
   segments message closed, final answer pushed, `phase='done'`.
6. Run ends without `completed` (interrupted) → `_isExecuting` watch →
   `builder.finalize(lastReply)` fallback.
7. `displayMessages` (builder `messages`) → `groupTurns` → `turns` →
   `ChatMessages mode="automation"` renders. Auto-scroll watch
   (`ChatMessages.vue:67`) fires on `turns` change → scrolls to bottom (now works
   because no cascade → stable layout).

---

## 5. Edge cases / risks (verified non-regressions)

- **Intermediate plain-text messages between tool turns**: builder only pushes
  the final answer (via `finalize`). Same as chat — not a regression. Reasoning/
  tool segments still render.
- **`lifecycle`/`step_start` text lines**: shown via the inset phase label
  (chat-consistent), not standalone bubbles. The old terminal rendered them as
  text; unification drops that, which is intentional and consistent.
- **`guardrail_blocked`/`guardrail_violation`**: kept in `useLiveConsole` →
  `pendingDecision` → `GuardrailBanner` in `AutomationDetails`. Not fed to the
  builder (it ignores them). Behavior preserved.
- **Chat path unchanged**: `finalizeOn` defaults to `'explicit'`; chat never
  sends `lifecycle{completed}` into the builder. Chat finalizes on HTTP reply as
  today. No diff in chat behaviour or output.
- **History replay ordering**: `resetRun()` replays `activeRun.events` in array
  order before live SSE; builder is order-stable (same as chat's live order).

---

## 6. Verification

- `cd frontend && npm run build` → eslint + vue-tsc + vite must be clean.
- `cd backend && go build ./... && go test ./...` → green baseline (no backend
  change, but confirm).
- `go run ./tools/check-complexity/` → ≤ 12 (frontend change doesn't affect Go,
  but run if touching backend-adjacent logic).
- Manual: run `network-recon` automation →
  - single clean reasoning inset (NO repeated "The user wants me to…" cascade),
  - final report delivered once,
  - view auto-scrolls to bottom during the run; pauses when user scrolls up,
  - tool calls/results render as collapsible `ToolCallSegment`s,
  - `GuardrailBanner` appears if a guardrail blocks,
  - completed-run history (reopen details) renders identically.

---

## 7. Docs to update (after implementation)

- `docs/PLANS/assistant-ui/automation-unified-renderer-and-report-truncation.md`
  §2.2 → note the bridge was replaced by shared `useMessageBuilder` consumption.
- `docs/architecture.md` pitfall #25 → "automation events are consumed by the
  shared `useMessageBuilder` (same path as chat); do not add a bespoke automation
  mapping. Customize automation UI via `ChatMessages` `mode` prop + `#run-header`
  slot, not a fork."
- `docs/skills/assistant-ui-patterns.md` → note chat + automation share
  `useMessageBuilder` as the single event→message consumer; `automationEventsToMessages`
  is deleted; `mode`/`#run-header` are the customization surface.

---

## 8. Files touched (summary)

| File | Change |
|------|--------|
| `frontend/src/utils/message/messageBuilder.ts` | Add `MessageBuilderOptions` (`source`, `headerMessage`, `finalizeOn`); lifecycle finalize; reset clears flags. |
| `frontend/src/composables/automation/useLiveConsole.ts` | Rewrite to feed `useMessageBuilder`; expose `displayMessages`/`thinking`/`liveReasoning`/`paused`/`phase`; run-end fallback. |
| `frontend/src/components/AgentIde/automation/AutomationDetails.vue` | Wire builder state to `ChatMessages`; drop static phase. |
| `frontend/src/components/AgentIde/assistant/ChatMessages.vue` | Add `#run-header` slot (extension point). Keep `mode` gating. |
| `frontend/src/utils/message/automationToMessages.ts` | **DELETE** (logic now in shared builder). |
