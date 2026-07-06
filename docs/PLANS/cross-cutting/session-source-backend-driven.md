# Session `source` derived from backend (single source of truth)

**Status:** ✅ Completed
**Supersedes:** `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` §3–§4 (frontend source grouping)
**Deploy note:** the frontend (`frontend_dist/`) is embedded into the Go binary
via `embed.FS` at `go build` time. To deploy a frontend change, run in order:
```bash
cd frontend && npm run build && cd ../backend && go build ./... && go run main.go
```
A stale binary (missing frontend updates or `SessionSource`) causes the
"Webhook" header to not render and sessions to appear flat. Confirm the backend
is current via `GET /admin/api/conversation/sessions/{ws}` — expected:
`"source":"webhook-telegram"`. The "Webhook" header is now a collapsible folder;
click to expand/collapse webhook sessions. Manual sessions render flat in root.

## Problem

The frontend derived session origin by parsing the session ID prefix
(`wb_telegram_...` → telegram, `wb_...` → webhook, else manual) in two places:
`utils/assistant/source.ts` and a planned `webhookRunningSessions` filter. This
duplicates backend knowledge of the ID format (`wb_{platformType}_{chatID}_{timestamp}`).
Any ID-format change silently breaks both. It is also fragile because `session_started`
SSE events fabricate a `SessionBrief` without a source, so a running webhook session
had no grouping until a refetch.

## Solution

The backend is the authority on session source. It derives `source` from the ID in one
generic place and ships it on both the list endpoint and the realtime SSE event.

**No per-platform hardcoding.** Webhook session IDs embed the connector platform type
(`wb_{type}_...`), so the value is generated as `webhook-{type}` (`webhook-telegram`,
`webhook-slack`, ...) — generic, zero code change to add a platform. Manual sessions are
`manual`. The frontend only maps the `source` string to an icon/label and **never parses
the session ID** — the ID format lives in exactly one backend function (`SessionSource`).

**Grouping model.** Webhook sessions render under a single "Webhook" header (one grouped
section); manual sessions render flat with no header — they are not grouped. The grouping
logic lives in `utils/assistant/source.ts` (`groupSessionsBySource`), not in the view
component. Each grouped section has a "delete all in this group" action; single-row delete
and workspace-wide "delete all" are unchanged.

## Changes

### Backend

| File | Change |
|---|---|
| `models/assistant.go` | Add `Source string` (json `source`) to `SessionBrief`. Add `func SessionSource(id string) string` — the single source of truth. Generic: `wb_{type}_...` → `webhook-{type}`; anything else → `manual`. No platform if-chain. |
| `internal/platform/persistence/workspace.go` | `ListSessions`: set `Source: models.SessionSource(session.ID)` on each brief. |
| `internal/core/assistant/conversation_helpers.go` | `PublishSessionLifecycle`: add `"source": models.SessionSource(conversationID)` to the SSE payload. No signature change. |
| `models/assistant_test.go` | **new** — `TestSessionSource` covers telegram / slack / discord / manual / empty. |

### Frontend

| File | Change |
|---|---|
| `types/assistant.ts` | `SessionBrief.source?: string`. |
| `composables/assistant/useAssistantSSE.ts` | `SessionLifecyclePayload.source?: string`. |
| `utils/assistant/source.ts` | `groupSessionsBySource(sessions)` → `SourceSection[]` (webhook grouped, manual flat). `sourceIcon(source)` (any `webhook*` → `radio`). `sourceLabel(source)` ("Webhook" for coarse key, "Webhook — <Platform>" for full). No ID parsing, no `sessionSource(id)`. |
| `composables/assistant/useAssistant.ts` | `applySessionUpdate`: set `source: p.source`. Add `deleteSessionsByIds(ws, ids)` (batched delete + single resync); `deleteAllSessions` delegates to it. |
| `components/AgentIde/assistant/ChatSessionList.vue` | `groupedSessions = computed(() => groupSessionsBySource(props.sessions))`. Header `v-if="group.grouped"` with a trash button emitting `delete-group` (the section's session IDs). |
| `components/AgentIde/assistant/AssistantChat.vue` | `@delete-group="handleDeleteGroup"` on both `ChatSessionList` usages; `handleDeleteGroup(ids)` confirms then calls `deleteSessionsByIds`. |
| `components/AgentIde/assistant/AssistantActivity.vue` | Icon via `sourceIcon(s.source)`. |

## Why no `webhook-telegram` vs `webhook` split

The earlier `webhook` bucket was a hardcoded fallback for "non-telegram" platforms. That
special-casing is removed: every webhook is `webhook-{platform}`, labelled dynamically.
Adding Slack/Discord requires no code changes anywhere.

## Group delete

- The webhook section header shows a trash icon (manual section has none — it is flat).
- Clicking emits `delete-group` with that section's session IDs.
- `AssistantChat.handleDeleteGroup` confirms, then `useAssistant.deleteSessionsByIds`
  deletes each via the existing per-session endpoint and resyncs once. No backend change.

## Verification

```bash
cd backend && go test ./models/ -run TestSessionSource -v -count=1
cd backend && go build ./...
cd backend && go run ./tools/check-complexity/
cd frontend && npm run build
```

Manual: send 2 Telegram + 1 manual chat → webhook sessions under a "Webhook" header,
manual flat below; per-row delete + "Delete all" still work; webhook header trash deletes
only webhook sessions (manual untouched); running badge unaffected; reload persists.

## Impact

Adding a new webhook platform (e.g. Slack) requires **zero** code changes — the platform
type flows from the connector `cfg.Type` → session ID → `SessionSource` → frontend label.
The session ID format is the only backend concern.

## Docs updated

- `docs/PLANS/assistant-ui/running-indicator-webhook.md` — `s.id.startsWith('wb_')` note corrected to `s.source`.
- `docs/PLANS/cross-cutting/connector-auto-reply.md` — inline `session.id.startsWith('wb_...')` pattern noted as superseded.
- `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` — grouping section superseded by this plan.

## Checkpoint persistence during execution

The backend now checkpoints the assistant session history to disk at tool-cycle
boundaries during execution, rather than only after the agent completes. This
ensures that `getSession` from the frontend (e.g., clicking a running session
from the history sidebar) returns the latest tool calls and results, not just
the initial user message.

- **What is persisted:** completed tool cycles (tool call → tool result) and
  the final assistant message. Streaming text and reasoning is NOT persisted
  (it is ephemeral and would bloat the file). Every tool result and the final
  message trigger an unconditional checkpoint — no throttling or rate limiting.
  The overhead (~5ms per fsync on SSD) is negligible next to LLM API latency.
- **Mechanism:** `buildPartialHistory(baseHistory, collectedEvents)` in
  `conversation_helpers.go` rebuilds the history from the initial user message
  and the agent events collected so far. `WriteSession` persists the result
  atomically (temp-file + rename). The original `session.History` is never
  mutated during execution — a shallow copy is used for each checkpoint, so
  the final success/cancellation paths append to pristine history without
  duplication.
- **Concurrency:** throttle variables are closure-local per `Execute` call —
  independent across concurrent executions, with no shared locks or mutexes.
- **Tests:** `conversation_helpers_test.go` — `TestBuildPartialHistory` (5
  cases: empty, single cycle, multi-cycle, unknown types, nil payloads).
