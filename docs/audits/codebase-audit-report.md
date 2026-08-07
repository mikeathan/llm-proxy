---
status: active
last_reviewed: 2026-07-11
---

# Codebase Audit Report — 2026-07-03

Unified findings from 4 parallel audits (backend Go, frontend Vue/TS, agent files/docs, architecture/duplication).

> Prior backend-only audit: [`docs/audits/backend-audit-report.md`](../../audits/backend-audit-report.md).

---

## Executive Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Backend Bugs | 3 | 7 | 11 | 9 | 30 |
| Frontend Bugs | 0 | 3 | 6 | 0 | 9 |
| Architecture/Duplication | 0 | 10 | 13 | 6 | 29 |
| Docs/Agent Files | 0 | 7 | 10 | 3 | 20 |
| **Total** | **3** | **27** | **40** | **18** | **88** |

Top 3 highest-leverage fixes:
1. **C1 — Propagate HTTP context to agent execution** — prevents orphaned LLM calls on client disconnect
2. **H3/H4 — Extract `WorkspaceService` and `ConversationService`** — eliminates layer violations, 30+ duplicate lock stanzas, and 170-line god handler
3. **H6/H7 — Unify frontend HTTP layer** — delete `ApiService`, consolidate 4 fetch conventions into one typed `httpClient.ts`

---

## Part 1: Backend Bugs (Go)

### CRITICAL

#### C1 — HTTP request context never propagated to agent execution
- **File:** `internal/transport/http/assistant_handlers.go:776`
- **Issue:** `RunWithCancel` uses `context.Background()` instead of `r.Context()`. Client disconnects (tab close, proxy timeout) don't cancel in-flight LLM calls. Work runs until `AgentGlobalTimeout` (30 min) or explicit `/conversation/cancel`.
- **Impact:** Wasted LLM tokens, pegged worker pool, no upstream back-pressure.
- **Fix:** Pass `r.Context()` into `RunWithCancel`; chain `context.WithCancel(r.Context())` for the Stop button.

#### C2 — Webhook automation result extraction matches wrong event type
- **File:** `internal/transport/http/webhook_handlers.go:255`
- **Issue:** `extractRunResult` checks for `"run_complete"` (never emitted) and `"tool_result"` (fired on every tool call, not automation completion). Telegram gets the first tool result instead of the final report.
- **Impact:** Automation output never delivered to Telegram after `/run <name>`.
- **Fix:** Subscribe for `assistant.EventLifecycle` with `phase == "completed"`, or read `state.LastRuns[automationName].Output` after `Trigger` returns.

#### C3 — Data race in `processStream` between heartbeat goroutine and main loop
- **File:** `internal/core/assistant/stream.go:416-442` vs `454-573`
- **Issue:** Heartbeat goroutine reads `fullMsg.Content/ReasoningContent/ToolCalls` while main loop concurrently appends to them. No mutex. `go test -race` would flag; produces torn reads under load.
- **Fix:** Move heartbeat diagnostic reads behind a sync mutex, or push periodic snapshots via channel.

### HIGH

#### H1 — `RunWithCancel` race orphans concurrent in-flight agent
- **File:** `internal/transport/http/assistant_handlers.go:773-785`
- **Issue:** Two concurrent requests for same workspace: both call `cancelPriorForWorkspace`, both `Store` into `running` map — last writer wins. Loser's cancel/done overwritten, can't be canceled. Deferred `Delete` removes wrong entry.
- **Fix:** Use `LoadOrStore` semantics — reject with 409 or atomically cancel-and-await. Track stored entry identity in defer.

#### H2 — N+1 calls to `ListTools` inside `processToolCalls`
- **File:** `internal/core/assistant/tool_exec.go:136`
- **Issue:** `ListTools(ctx)` called once per tool call in batch. With `MultiToolProvider` this recurses into MCP servers (remote `tools/list` JSON-RPC per call). Error also discarded (`_`).
- **Fix:** Call `ListTools` once at top of `processToolCalls`, pass into loop.

#### H3 — Per-tool-call disk I/O for guardrail config
- **File:** `internal/core/assistant/registry.go:26-42` (called from network/terminal/filesystem tools)
- **Issue:** Every tool execution reads+decodes `config.yaml` from disk. 30 tool calls = 30 disk reads + 30 yaml decodes. fsnotify watches the file but no cache exists.
- **Fix:** Memoize per-workspace config with fsnotify invalidation (like `modelDiscoveryCache` in `admin_handlers.go:35`).

#### H4 — Concurrent `bytes.Buffer` writes in `executeShell`
- **File:** `internal/core/tools/terminal.go:495`
- **Issue:** Two reader goroutines (stdout + stderr) append to same `bytes.Buffer` without mutex. `bytes.Buffer.Write` can panic on concurrent growth.
- **Fix:** Protect with mutex, or use separate buffers + concatenate after `wg.Wait()`.

#### H5 — `validateToolArgs` sanitizes copy, history keeps malformed args
- **File:** `internal/core/assistant/tool_exec.go:448` + `session.go:208`
- **Issue:** `tc` is a per-iteration copy; sanitized args written to copy, not `msg.ToolCalls[i]`. History (already appended at line 208) retains malformed JSON. Model re-receives malformed args next turn.
- **Fix:** Take `msg *proxy.Message`, write back to `msg.ToolCalls[i].Function.Arguments`.

#### H6 — `stopCh` close not idempotent — `Dispatcher.Stop()` panics on double-call
- **File:** `internal/core/automation/dispatcher.go:170-180`
- **Fix:** Guard with `sync.Once`.

#### H7 — `EditFileBlock` non-atomic write can corrupt files
- **File:** `internal/core/tools/filesystem.go:268`
- **Issue:** Direct `os.WriteFile` overwrite, no temp+rename. Crash mid-write truncates workspace file. Every other writer uses temp+rename+sync.
- **Fix:** Mirror `persistence/workspace.go:274-302` atomic pattern.

### MEDIUM

| # | File | Issue |
|---|------|-------|
| M1 | `usagetracker.go` / `executor.go:155` | `UsageTracker` fields read without mutex (writes guarded, reads not) |
| M2 | `agent.go:77-106` | God object: 26 mutable fields, per-execution state on shared `Agent` |
| M3 | `dispatcher.go:678-688` | `StopAutomation` spawns 30s goroutine with no context — leaks |
| M4 | `broadcast.go:38-50` | `Unsubscribe` closes channel — fragile if caller holds reference |
| M5 | `workspace.go:305-336` | `ListWorkspaces` holds RLock across 3N file reads |
| M6 | `dispatcher_handlers.go:100-141` | `ListAutomations` per-entry disk reads (same workspace read multiple times) |
| M7 | `workspace.go:112,169` | `fmt.Printf` in production code instead of structured logger |
| M8 | `manifests.go:133-138` | 6 manifest-load errors silently swallowed |
| M9 | `storage/manager.go:81-87` | fsnotify reload errors silently swallowed |
| M10 | `terminal.go:416` | Subshell wrapping (`cd %q && %s`) — command injection window |
| M11 | `network.go:218-222` | `validateAddress` returns nil on DNS failure — bypass |

### LOW

| # | File | Issue |
|---|------|-------|
| L1 | `stream.go:506 vs 523` | `tryExtractToolCallFromReasoning` runs before current chunk appended |
| L2 | `stream.go:196-200` | Agent `maxTokens`/`reasoningBudget` mutated in-place by preflight |
| L3 | `terminal.go:467-469` | Retry uses cancelled context — masks original error |
| L4 | `terminal.go:534` | Only recognizes `DeadlineExceeded`, not `Canceled` |
| L5 | `filesystem.go:307-311` | Dead `else if` branch in `buildReplacement` |
| L6 | `terminal.go:366-375` | `extractAbsolutePaths` reports misleading paths for quoted args |
| L7 | `network.go:236` | `validateIP` missing `IsUnspecified()`, `IsMulticast()` checks |
| L8 | `dispatcher.go:537-563` | Double metrics record on nil response |
| L9 | `tool_exec.go:374-401` | `extractTruncatedJSONField` byte-indexes into UTF-8 — mojibake on non-ASCII |

---

## Part 2: Frontend Bugs (Vue/TS)

### HIGH

#### FB1 — Memory leak: `handledDecisionIds` Set never cleared
- **File:** `src/composables/assistant/useAssistantSSE.ts:21,36-44,116-121`
- **Issue:** `reset()` clears `receivedEventIds` but never `handledDecisionIds`. Long sessions leak memory; reused IDs suppress legitimate decisions.
- **Same pattern in:** `src/composables/automation/useLiveConsole.ts:12,47,137`
- **Fix:** Clear `handledDecisionIds = new Set()` in `reset()` and `disconnect()`.

#### FB2 — Race condition: concurrent `loadModels` overwrites stale list
- **File:** `src/components/settings/ProviderModelsCard.vue:140-157`
- **Issue:** Rapid API key changes fire multiple in-flight `fetchProviderModels()`. Last-resolved (not latest-requested) wins — catalog shows wrong key's models.
- **Fix:** Track request token (`let reqId = 0; const mine = ++reqId; ... if (mine !== reqId) return`) or use `AbortController`.

#### FB3 — `applySessionUpdate` may run on stale `sessions.value` after navigation
- **File:** `src/composables/assistant/useAssistant.ts:293-326`
- **Issue:** Late `session_completed` for a deleted session re-inserts ghost entries via `sessions.value.unshift` when `idx === -1`. SSE persists across navigation.
- **Fix:** Ignore `session_*` events where `activeWorkspaceId.value` mismatches event's `workspace_id`.

### MEDIUM

| # | File | Issue |
|---|------|-------|
| FB4 | `useMetrics.ts`, `useProcesses.ts`, `TerminalMonitor.vue` | Polling overlaps — `setInterval` with no in-flight guard (older response clobbers newer) |
| FB5 | `useConfirm.ts:22` | Two overlapping `confirm()` calls — first Promise hangs forever |
| FB6 | `useAssistantSSE.ts:102`, `useLiveConsole.ts:160` | SSE `onerror` has no backoff/death detection — hammers server on 404/401 |
| FB7 | `GlobalSettings.vue:106,138,152,162,175,218,233,245,256` | Direct prop mutation via `v-model="editConfig.*"` — breaks one-way flow, silent data loss on parent clone |
| FB8 | `GlobalSettings.vue:47-53` | `localProvider` computed setter bypassed by nested `v-model="localProvider.model_dir"` |
| FB9 | `useAssistant.ts:91-104` | `loadSession` non-running branch doesn't call `sse.reset()` — stale `pendingDecision`/`liveEvents` linger |

---

## Part 3: Architecture & Duplication

### HIGH

#### A1 — Duplicated handler boilerplate (no shared base helper)
- **Files:** All `*_handlers.go` in `internal/transport/http/`
- **Issue:** 125 `writeJSONError()` call sites, 81 `if x == ""` validation blocks. Two parallel error helpers (`writeJSONError` vs `respondError`) with divergent behavior.
- **Fix:** Single `writeError(w, status, msg)`, `parsePathVar(w, r, name) (string, bool)`, `handleServiceError(w, err, mappings...)`. Effort: M.

#### A2 — Dispatcher/UI handlers reach directly into `persistence.WorkspaceManager` (layer violation)
- **File:** `internal/transport/http/dispatcher_handlers.go` — 30+ direct `Persistence().ReadConfig/WriteConfig/AcquireLock` calls
- **Issue:** Transport layer mutates disk state directly. Business logic (lock acquiring, config merging, file seeding) in handlers. No validation/service boundary.
- **Fix:** Introduce `WorkspaceService` interface with `GetConfig/SaveConfig/MutateConfig(fn)/CreateWorkspace/...`. Effort: L. **Unblocks M3, M5, L2.**

#### A3 — Handler-side orchestration in `handleAssistant` (170-line god method)
- **File:** `internal/transport/http/assistant_handlers.go:156-328`
- **Issue:** Does: client acquisition, session load/create, model config, history build+truncation, persistence write, lifecycle events, run-ID, run-infra setup, event collection, agent builder config (12 calls), cancel-turn filtering, post-exec history diffing, response shaping. Two near-duplicate history-append branches.
- **Fix:** Extract `ConversationService.Execute(ctx, msg) → Result`. Handler decodes/validates/calls/encodes. Effort: L.

#### A4 — Cancelled-turn filtering logic in transport layer (not domain)
- **Files:** `assistant_handlers.go:623` (`computeCancelIndices`), `:658` (`filterCancelledTurns`)
- **Issue:** Domain logic (proxy.Message inspection, cancellation bookkeeping) lives in transport handler, not `core/proxy` which owns `NormalizeHistory`.
- **Fix:** Move to `core/proxy` or `core/assistant`. Effort: S.

#### A5 — Frontend: 3 incompatible fetch/error conventions + duplicate `ApiService`
- **Files:** `frontend/src/services/{api,adminService,memoryService,assistantService,dispatcherService,mcpService}.ts`
- **Issue:** 4 distinct error idioms. `ApiService` (untyped, `any` payloads) and `AdminApiService` (typed) overlap — same endpoints, two sources of truth.
- **Fix:** One `httpClient.ts` with `get<T>/post<T>/put<T>/del<T>` + `handleResponse<T>`. Delete `ApiService`. Effort: M.

#### A6 — `ProviderModelsCard.vue` is 1352 lines (god component)
- **File:** `frontend/src/components/settings/ProviderModelsCard.vue`
- **Issue:** Aggregates model list, add/edit form, metadata enrichment, ICU weight preview, override editor, per-card actions. Next largest is 555 LOC.
- **Fix:** Split into `ProviderModelList.vue` + `ProviderModelForm.vue` + `ProviderModelMetadata.vue` + `useProviderModels()` composable. Effort: L.

#### A7 — `AdminHandlers` god handler (19+ methods across 6 files)
- **Files:** `admin_handlers.go` (478 LOC), `registry_handlers.go` (683 LOC), `process_handlers.go` (299 LOC), `system_handlers.go`, `secrets_handlers.go`, `admin_template_handlers.go`
- **Issue:** One receiver type owns: admin state, frontend assets, model discovery, log level, provider/model CRUD, MCP CRUD, metadata scanning, app logs, process listing/kill, terminal reset, host settings, system config, restart.
- **Fix:** Split into `ModelHandlers`, `ProcessHandlers`, `SystemHandlers`, `SecretsHandlers`, `MCPHandlers`, `WebhookAdminHandlers`. Effort: M.

#### A8 — `Agent` struct god object (26 mutable fields)
- **File:** `internal/core/assistant/agent.go:77-106`
- **Issue:** Mixes LLM knobs, tool format flags, memory state, guardrails, orchestration, strategy. Per-execution mutable state (`prefillDisabled`, `memoryInjected`, `maxTokens`) on shared struct — leaks across `Execute` calls.
- **Fix:** Split into `AgentConfig` (data), `AgentRuntimeDeps` (client/provider/engine), keep `Agent` as executor. Move per-exec state into `runSession`. Effort: M.

#### A9 — `app_context.go` (756 LOC) + `bootstrap.go` (590 LOC) — god files
- **Issue:** Classic composition root becoming god module. Every new subsystem adds 50+ lines.
- **Fix:** Per-subsystem `Wire()` functions colocated with each package. `bootstrap.go` becomes thin sequence of `Wire()` calls. Effort: L.

#### A10 — Unvalidated path params (`file`, `connector_name`)
- **Files:** `dispatcher_handlers.go:440,455,612` — raw `r.PathValue("file")` flows into file I/O
- **Issue:** Only `workspaceID` and `automation` are `validateID`-gated. Filename path-traversal guard missing at handler layer.
- **Fix:** Expand `validateID` or add `validateFileName`. Effort: S.

### MEDIUM (architecture)

| # | Issue | Effort |
|---|-------|--------|
| A11 | `runSession` 13 fields, `run()` 144 LOC with 4 nested error branches — near complexity threshold | M |
| A12 | `executePlan` bypasses guardrails entirely (no `resolveGuardrail`, no `notifyToolCall`, no observer events) — security regression for `EnableExecutionPlan` | M |
| A13 | Switch-on-type in `executor.go:549-579` and `dispatcher.handleConfigChange` — 11 event types, doesn't scale | M |
| A14 | Frontend: duplicated `testLoading`/`testSuccess`/`testError` state machine + secret-value-with-test pattern in 6 settings components | M |
| A15 | `AssistantMessageHandler` stores 8 deps + entire `AssistantService` — duplicate state, `svc` wider than handler needs | M |
| A16 | `process_handlers.go` reads files with `os.ReadFile`/`os.Open` directly in handlers | S |
| A17 | Domain logic in transport DTO: `enrichMetadataFromProviders` in `registry_handlers.go:206-275` does ICU weight, context heuristics, metadata construction | M |

---

## Part 4: Agent Files & Documentation

### HIGH

#### D1 — Conflicting `ref()` vs `reactive()` guidance
- **Files:** `AGENTS.md:66` says "`ref()` over `reactive()`" | `.agents/rules/frontend-vue-engineer.md:16` says "Use `ref()` for primitives and `reactive()` for objects"
- **Fix:** Pick one canonical rule, write once, point from other. Update rules file to drop `reactive()` advice.

#### D2 — Duplicated checklists in AGENTS.md and architecture.md
- **Issue:** "Adding a Frontend Settings Tab" 8-step checklist appears verbatim in `AGENTS.md:43-54` AND `docs/architecture.md:204-213`. "Coding Rules" in `AGENTS.md:62-69` is partial copy of rules file. Two copies will drift.
- **Fix:** AGENTS.md = thin "start here" pointer. Keep content in one canonical place. Replace duplicates with one-line references.

#### D3 — `docs/SPECS/README.md` missing SPEC-009
- **Issue:** `docs/INDEX.md:46` catalogs SPEC-009 (communication) but `docs/SPECS/README.md` only lists SPEC-001→SPEC-008. Two catalogs disagree.
- **Fix:** Add SPEC-009 row OR delete SPECS/README.md (single source via INDEX.md).

#### D4 — No SPEC change-management process
- **Issue:** SPECs have `version: "1.0"` and `supersedes:` fields but no documented process for bumping, deprecating, or amending.
- **Fix:** Add "SPEC Change Management" section to `docs/SPECS/README.md` or CONSTITUTION.md defining: when to bump version, when to fill `supersedes`, PLAN = change-instance, SPEC = steady-state contract.

#### D5 — `orchestrator/` and `nodeherder/` undocumented in architecture.md
- **Issue:** `internal/core/orchestrator/` (12 files) not listed in architecture.md "Core Systems". SPEC-005 exists but implementation not mapped.
- **Fix:** Add bullet points for `orchestrator/` (budget, slots, reasoning normalizer, squeezer) and `nodeherder/` (MCP multiplexer).

#### D6 — No API endpoint reference document
- **Issue:** ~70 HTTP endpoints registered in `bootstrap.go:476-586`. No `docs/api-reference.md`, OpenAPI, or Swagger. Frontend/CLI must reverse-engineer routes.
- **Fix:** Create `docs/api-reference.md` grouped by subsystem: Admin, Conversation, Memory, Dispatcher, Recordings, MCP, Secrets, Connectors, Webhooks.

#### D7 — No CONTRIBUTING.md
- **Issue:** Repo has 9 SPECs, Constitution, 13 skill files — but no contribution guide. `AGENTS.md` is "instructions for coding agents" not "how to contribute".
- **Fix:** Create `CONTRIBUTING.md` at repo root with: project layout pointer, spec-driven flow, build & test chain, git policy, doc discipline, code review, playbook for adding (tool/tab/connector/model/endpoint).

### MEDIUM

| # | Issue | Priority |
|---|-------|----------|
| D8 | No "start here" breadcrumb or read-order in docs | Medium |
| D9 | 3 global config files (`AGENTS.md`, `context-mode.md`, `AGENTS.caveman.md`) overlap on tool-routing policy | Medium |
| D10 | `docs/skills/` (13 files, 67 KB) has no load-strategy — no trigger rules | Medium |
| D11 | SPECs lack explicit Acceptance Criteria sections | Medium |
| D12 | Subsystems without SPECs: process lifecycle, recordings, enricher, agent-memory injection | Medium |
| D13 | SPECs lack "Reference Files" sections linking spec → source | Medium |
| D14 | Webhook security (dual-header convention) under-specified | Medium |
| D15 | Recordings subsystem undocumented (record-replay, build tags) | Medium |
| D16 | EventBus / eventsink contract only in skills, not SPEC | Medium |
| D17 | `backend_audit.md` at repo root is orphan noise | Low |

---

## Part 5: Testing Gaps

| Area | Issue | Effort |
|------|-------|--------|
| `platform/{env,db,process}` | Zero test files — process/ has platform-specific branches | M |
| `dispatcher_handlers_test.go` | Uses real `*persistence.WorkspaceManager` not mocked — integration tests masquerading as unit tests | M |
| `assistant_handlers_test.go` | 1069 LOC, defines own `noopLogger`, only checks `res.Code == 200` — shallow assertions | M |
| Frontend | No component tests, no composable tests, no e2e — entire frontend is untested | L |

---

## Recommended Implementation Order

### Phase 1 — Critical bug fixes (1-2 days) ✅ complete
1. `[x]` C1 — Propagate `r.Context()` to agent execution (`assistant_handlers.go` path; webhook path intentionally uses `context.Background()` because the agent outlives the webhook request)
2. `[x]` C2 — Fix `extractRunResult` event matching (now reads `state.LastRuns[name].Output`)
3. `[x]` C3 — Add mutex to stream heartbeat (replaced direct field reads with `atomic.Int64` counters)
4. `[x]` H6 — `sync.Once` on `Dispatcher.Stop()`
5. `[x]` H7 — Atomic write in `EditFileBlock` (temp file + `os.Rename`)

### Phase 2 — High-impact refactors (1 week) ✅ complete

6. `[x]` A2 — Extract `WorkspaceService` interface (unblocks A3, M3, M5, L2; 30 `Persistence()` calls replaced with `h.workspace`; 5 lock stanzas consolidated into `MutateConfig`; unit tests added)
7. `[x]` A5 — Unified frontend `httpClient.ts`, delete `ApiService` (all services now use shared `get<T>/post<T>/put<T>/del<T>` from `httpClient.ts`; `api.ts` removed; services restructured into subdirs: `admin/`, `monitoring/`, `mcp/`, `automation/`, `assistant/`, `memory/`, `template/`)
8. `[x]` A7 — Decompose `AdminHandlers` into focused handlers (split into 5 focused types: `SystemHandlers`, `SecretsHandlers`, `ProcessHandlers`, `MCPHandlers`, `ModelHandlers`; `AdminHandlers` reduced to 4 methods)
9. `[x]` H2 — Cache `ListTools` result per turn (`toolsList` now fetched once per `processToolCalls` invocation; error surfaced instead of discarded)
10. `[x]` H3 — Cache guardrail config with mtime-based invalidation (`workspaceConfigCache`, like `modelDiscoveryCache` pattern)

### Phase 3 — Architecture cleanup (1-2 weeks) ⏸️ pending
11. `[x]` A3 — Extract `ConversationService` from `handleAssistant` (172-line god handler → 5-line delegation; interface + implementation + helpers moved to `core/assistant/`; `AgentBuilder` + `ServiceProvider` moved to `core/assistant/`; `EventPublisher` interface breaks cyclic dep with `automation`; 8 helper functions exported from `core/assistant/`)
12. `[x]` A6 — Split `ProviderModelsCard.vue` (1352-line script extracted into `useProviderModels()` composable; component now thin — delegates all state + actions to composable; template splitting deferred)
13. `[x]` A8 — Split `Agent` into `AgentConfig` (data) + `AgentRuntimeDeps` (services) + `Agent` (executor); per-exec mutable state (`prefillDisabled`, `memoryInjected`) kept on Agent for now, marked for future `runSession` migration
14. `[x]` A9 — Per-subsystem `Wire()` functions (extracted `wireHandlers()` → `HandlerSet`; `buildRouter()` takes single `*HandlerSet` instead of 12 params; handler construction + route registration decoupled; remaining backend Wire extraction in progress)
15. `[x]` A12 — Route `executePlan` through guardrails (added `resolveGuardrail`, `notifyToolCall`, guardrail-approved context, `notifyToolResult`; moved `ListTools` outside loop)

### Phase 4 — Documentation (2-3 days) ⏸️ pending
16. `[x]` D7 — Create `CONTRIBUTING.md`
17. `[x]` D6 — Create `docs/api-reference.md`
18. `[x]` D1-D2 — Reconcile AGENTS.md vs rules files, deduplicate (AGENTS.md stays concise, references `.agents/rules/`; rules files remain as authoritative detailed source)
19. `[x]` D4 — Add SPEC change management process (`docs/SPEC-change-management.md`)
20. `[x]` D5 — Document orchestrator/nodeherder in architecture.md (added detailed package breakdowns with file-level descriptions)

### Phase 5 — Frontend bugs (2-3 days) ⏸️ pending
21. `[x]` FB1 — Clear `handledDecisionIds` in `reset()` and `disconnect()` (both `useAssistantSSE` + `useLiveConsole`)
22. `[x]` FB3 — Guard `applySessionUpdate` against stale workspace (skip events where `workspace_id` doesn't match `activeWorkspaceId`)
23. `[x]` FB2 — Request token for `loadModels` (`let loadModelsReqId = 0; const mine = ++loadModelsReqId; if (mine !== loadModelsReqId) return`)
24. `[x]` FB4-FB9 — Polling guards, confirm queue, SSE backoff, prop mutations, stale pendingDecision

---

## Complexity Check Warnings

The following functions likely exceed cyclomatic complexity threshold of 12 (run `go run ./tools/check-complexity/` to confirm):
- `stream.go:processStream` (~13 branches)
- `terminal.go:splitCommandSegments` (~14)
- `terminal.go:checkPathSecurity` (~13)
- `session.go:run` (~13)

---

## Appendix: Backend Duplication & Dead Code — RESOLVED (2026-07-29)

Follow-up focused audit (was `DUPLICATION_AUDIT.md` at repo root, merged here
after completion). All items fixed via PR1 + PR2. Grep-verified against live tree;
five corrections (C1–C5) documented below.

### HIGH — Substantial duplication (all resolved)
1. **HTTP error helpers — 3 defs of same fn** (`writeJSONError` in `helpers.go:11`
   + `admin_handlers.go:229`, `respondError` in `dispatcher_handlers.go:327`).
   → Single exported `api.WriteJSONError`; `helpers.go` version promoted because
   `admin_handlers.go` had a header-ordering bug (set `Content-Type` after
   `WriteHeader` → ~80 sites missing the header). Fixed as side effect (C2).
2. **`decodeJSON`/`ErrUnsupportedContentType`/`maxBodySize` duplicated in
   `http/helpers.go` and `handlers/http_internal.go`.** → One copy in `api`,
   bridge aliases in `handlers`.
3. **Atomic file write (temp→write→sync→rename) — 7 copies** (workspace.go ×5,
   store.go `saveLocked`, tools/filesystem.go). → `storage.WriteAtomic(dest,
   pattern, data)`; `saveLocked` gained a `Sync()` it previously skipped (C5 —
   safer but slower, flagged in PR).

### MEDIUM — Same pattern repeated (all resolved)
4. **Lock acquisition** (`AcquireLock`/`TryAcquireLock` share setup lines) → `WorkspaceManager.openLockFile(workspaceID)` extracted.
5. **`os.IsNotExist`→zero-value 8×** → extraction **SKIPPED** by decision (marginal benefit, awkward `nil` callers).
6. **Workspace param validation 13+× across 6 handlers** → `api.RequirePathParams` (variadic, no map alloc) + `api.RequireQueryParam`; `RequirePathParamsMsg` variant preserves combined 400 text.
7. **Head+tail truncation — 2 impls** (`proxy.TruncateResult` vs `sieve.truncateLongContent`) → unified `proxy.TruncateResult(content, limit, marker)`; sieve keeps terse `\n...[Truncated]...\n`, tool-results keep verbose `SYSTEM NOTE` (via `TruncateResultDefault`). LLM-behavior-tested.
8. **URL construction — `network.FormatURL()` exists but ignored 6×** → adopted (slot_manager, provider, admin/process/proxy handlers, admin_view).
9. **JSON tool-arg parsing — `decodeArgs()` underused 12+×** → `proxy.DecodeToolArgs(raw, target)`; `decodeArgs` moved from `assistant` to `proxy` to avoid import cycle (C3: guardrails imports assistant, not vice-versa). Empty-args semantics audited per site; `validateToolArgs` reverted to inline `json.Unmarshal` to preserve empty-input error.

### LOW / minor (all resolved)
10. **Slot-manager HTTP blocks ×3** → `SlotManager.doSlotRequest(ctx, method, url)` extracted.
11. **`fmt.Printf` bypasses logger in `storage/`** (manager/keys/store/secrets_store) → **deferred** to separate PR (Phase 7b; needs constructor logger injection).

### Dead code (removed)
- `ToJson` (`utils/encoding.go`) — zero callers (C1 confirmed). File deleted.
- `ExecutionHistory` type alias — dead in BOTH `proxy/message.go:19` and `models/llm_messages.go:19` (C4). Both deleted.

### Key engineering notes
- Two PRs: PR1 = HIGH (#1–3) + dead code; PR2 = MEDIUM/LOW (#4–10). Logger injection = PR3.
- Each phase: `go build ./...` + `go test ./...` + `check-complexity/` ≤12; TDD for every new exported helper.
- Layering respected: `persistence` imports `storage` one-way → new helpers live in `storage`, never reverse.
- All verification clean: build, vet, test, check-complexity; no per-request map alloc in path-param helpers; no import cycles.

### Post-verification fixes (2026-07-29, second round)
Second pass confirmed all phases compile/vet/pass tests with `check-complexity` ≤12 and
surfaced message-text regressions + cheap cleanups. Applied (no API-contract breaking changes):
- **Message-text restorations:** `RequirePathParamsMsg` added to `api` (combined single 400
  message when ANY key is empty, bridge alias in `http_internal.go`); `dispatcher.parse`
  reverted to per-key early-return loop (`<k> is required` / `invalid <k>`); session/memory
  handlers use `requirePathParamsMsg` with ORIGINAL combined strings (`"workspace and session
  are required"`, `"workspace and id are required"`); `tool_exec.go:647` `validateToolArgs`
  reverted to inline `json.Unmarshal` (preserves empty-args error `"failed to parse arguments
  as JSON: unexpected end of JSON input"`) while keeping the sanitize-retry block.
- **Missed Phase-4 sites routed** (exact message match): `recordings_handlers.go`
  Get/Delete (`"id is required"`) and `webhook_handlers.go` ServeHTTP
  (`"connector_name is required"`) through `requirePathParams`. `mcp_handlers.go:31`
  (`"name and url are required"`) is a body-payload check, intentionally left inline.
- **Cleanups:** `proxy/tool_args.go` (`DecodeToolArgs`) merged into `proxy/utils.go` with doc
  comment; doc comments on `storage.WriteAtomic`, `proxy.MaxReturnChars`,
  `proxy.DecodeToolArgs`; added `proxy/utils_test.go` (DecodeToolArgs empty/valid/malformed;
  TruncateResult variants) + extended `helpers_test.go` (Content-Type assertion,
  `TestRequirePathParamsMsg`).

### Run-log investigation & salvage/truncation fixes (2026-07-30)
Slow/blank sample run (`workspace-test`, `laguna-xs-2.1`, `run.log` + `events.jsonl` under
`backend/data/runs/.../20260730T173015Z_...`, persisted session
`~/.config/llm-proxy/workspace-test/sessions/conv_20260730183015.json`).
- **BUG A (salvage path → blank UI + lost report).** `session.go` `handleToolTurn` appended
  `turnMsg` by value before `processToolCalls` salvaged the truncated `write_file` payload into
  `turnMsg.Content`; the persisted history copy kept `Content=""` with `ToolCalls` set, so the
  frontend skipped the message during reconstruction → blank conversation + never-saved report.
  **Fix (in tree):** on salvage set `turnMsg.ToolCalls = nil` and write the updated `turnMsg`
  back into `s.history[n-1]` so the report persists as pure assistant text. Covered by
  `tool_exec_salvage_test.go`.
- **BUG B (missing `role:user` on multi-tool runs).** Reproduced attempt showed the current tree
  DOES retain the user role (`newRunSession` copies `llmHistory`; `handleSuccessResult` appends
  only `updatedHistory[len(llmHistory):]`); loss came from a prior code revision — no change needed.
- **Slowness (~131s).** Model streaming stalls (3× 30–46s "stream still generating"); tool
  execution sub-second. Not a backend defect.
- **TruncateHistory regression** (caught by `TestHandleAssistant_HistoryTruncation`): user-anchor
  logic kept an oversized user message whole, breaking the shrink expectation. **Fix:** when
  trimming later messages can't reach budget, cap the user message's *content* to the remaining
  budget (append `\n…[truncated]`) instead of dropping it — preserves both
  `TestTruncateHistory_PreservesFirstUserMessage` and the shrink behavior.