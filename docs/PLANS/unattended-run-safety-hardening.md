---
status: approved
date: 2026-07-22
related_specs: [SPEC-001, SPEC-006, SPEC-007]
---

# Unattended Run Safety Hardening Plan

**Status:** approved — Steps 0–5 complete (2026-07-22)
**Date:** 2026-07-22
**Related:** SPEC-001 (Agent Loop), SPEC-007 (Automation Dispatcher), SPEC-006 (Guardrails)

---

## Overview

Agentic runs have multiple redundant guard layers (30-min global timeout, 50-step hard cap, repetition/spiral/starvation detection, stuck-stream checks, sieve retry caps). However, gaps create unbounded windows between guard firings or allow guard bypasses — especially for unattended automations.

This plan closes 13 safety gaps, fixes 7 memory leaks, and addresses 5 performance optimizations (O1, O3, O5, O6, O7). The refactoring work (Step 0) ships first to create a clean foundation, then safety fixes build on that foundation.

**Scope:** All changes flow through the existing config pipeline so users can tune every timeout. No hard-coded-only values.

---

## Config & UI Controllability

Every new timeout value must flow through the existing config pipeline so users can tune it. No hard-coded-only values.

| Timeout | Default | Config Backend Path | UI Location |
|---------|---------|-------------------|-------------|
| `DefaultToolTimeout` | 2 min | `AgentOptions.ToolTimeout` ← `ModelConfig.ToolTimeoutSeconds` (0=default) ← `adminTuningDefaults.tool_timeout_seconds` | `ProviderModelsCard.vue` (per-model), admin defaults |
| `FilesystemToolTimeout` | 30 sec | `AgentOptions.FilesystemToolTimeout` ← `ModelConfig.FilesystemToolTimeoutSeconds` (0=default) ← `adminTuningDefaults.filesystem_tool_timeout_seconds` | `ProviderModelsCard.vue` (per-model), admin defaults |
| `MaxPlanDuration` | 15 min | `AgentOptions.MaxPlanDuration` ← `ModelConfig.MaxPlanDurationMinutes` (0=default) ← `adminTuningDefaults.max_plan_duration_minutes` | `ProviderModelsCard.vue` (per-model), admin defaults |
| `MaxPlanSteps` | 50 | `AgentOptions.MaxPlanSteps` ← `ModelConfig.MaxPlanSteps` (0=default) ← `adminTuningDefaults.max_plan_steps` | `ProviderModelsCard.vue` (per-model), admin defaults |
| `GuardrailTimeout` | 5 sec | `AgentOptions.GuardrailTimeout` ← `ModelConfig.GuardrailTimeoutSeconds` (0=default) ← `adminTuningDefaults.guardrail_timeout_seconds` | `ProviderModelsCard.vue` (per-model), admin defaults |
| `GuardrailTimeoutBehavior` | `"fail-open"` | `AgentOptions.GuardrailTimeoutBehavior` ← `ModelConfig.GuardrailTimeoutBehavior` | `ProviderModelsCard.vue` (per-model) |
| `automationTimeout` (existing) | 10 min | `dispatcher.go:28` const → `ModelConfig.TimeoutMinutes` (already exists) | Already in `ProviderModelsCard.vue` |

**Config flow:**
```
adminTuningDefaults (global defaults)
        └→ ProviderModelsCard.vue (per-model override)
                └→ ModelConfig (/api/models)
                        └→ AgentOptions.ApplyModelConfig()
                                └→ agent.config (runtime)
```

All new `AgentOptions` fields must be added to `applyDefaults()`, `ApplyModelConfig()`, `adminTuningDefaults`, and the frontend types/components. See `docs/architecture.md#adding-a-frontend-settings-tab-checklist`.

---

## Rollback Strategy

All new timeouts are configurable. If a fix causes issues in production:
- Set timeout to `0` (disabled) or a very large value to effectively disable the guard
- Guardrail timeout: set `GuardrailTimeoutBehavior` to `"fail-open"` to allow all tool calls
- Alternating spiral detector: set threshold to `0` to disable
- AllowedTools: leave empty to allow all tools (backward compatible)

No code revert required for runtime adjustments.

---

## Step 0: Refactoring Foundation (No Behavior Change) — COMPLETE

**Purpose:** Create clean foundation. All safety changes in later steps are single-point. Zero behavior change — all existing tests must pass identically.

### Refactor-1: Extract `executeSingleToolStep` Helper

**Where:** `tool_exec.go`

**Problem:** `processToolCalls` (107-180) and `executePlan` (234-312) duplicate logic. Every safety fix must be applied twice.

**Target state:**
```
executeSingleToolStep(ctx, tc, history, mu, continueOnError) (result, error)
  - continueOnError: true for processToolCalls (continues batch on individual failure)
  - continueOnError: false for executePlan (halts entire plan on any step failure)
```

Both callers use the same helper. The `continueOnError` parameter controls error-handling strategy.

**Acceptance criteria:**
- `processToolCalls` and `executePlan` both use `executeSingleToolStep`
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- `executeSingleToolStep` has ≥90% line coverage (covers guardrail-allow, guardrail-deny, execute-success, execute-failure paths)

### Refactor-2: Cache `BuildToolManual` / `BuildNativeToolReference` on Agent

**Where:** `prompts/templates.go`, `agent.go`

**Problem:** `BuildToolManual` rebuilt every turn (JSON marshal + string concat). High cost in hot path.

**Work:**
- Add `cachedToolManual string`, `cachedToolReference string`, and `toolsVersion uint64` fields to `Agent`
- Build once in `NewAgent` or on first use
- **Invalidation:** At the start of each `Execute()` call, call `ListTools()` and compute a hash of the tool set (tool names + arg schemas). If hash differs from stored `toolsVersion`, rebuild cached manuals. This catches MCP tools joining/leaving asynchronously.
- `injectToolInstructions` and `injectNativeToolReference` read from cache

**Acceptance criteria:**
- Tool manual built exactly once per `Agent.Execute()` call when tool set unchanged
- Cache rebuilt when tool set changes (MCP tool added/removed)
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- Cache hit path tested; cache invalidation path tested

### Refactor-3: Pass `toolsList` from `executeTurn` Through to `processToolCalls`

**Where:** `session.go`, `tool_exec.go`

**Problem:** `ListTools` called twice per turn (executeTurn + processToolCalls).

**Work:**
- `executeTurn` already returns `toolsList` — the caller `run()` discards it
- Change `handleToolTurn` signature to accept `toolsList []proxy.Tool` parameter
- `run()` passes `toolsList` from `executeTurn` to `handleToolTurn`
- `handleToolTurn` passes it to `processToolCalls`
- Remove redundant `ListTools` call at `tool_exec.go:110`

**Acceptance criteria:**
- `handleToolTurn` receives toolsList as parameter, passes to processToolCalls
- No `ListTools` call in `processToolCalls` (line 110 removed)
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- `handleToolTurn` still works when `toolsList` is `nil` (defensive check)

### Refactor-4: Name Magic Numbers

**Where:** `session.go`, `agent.go`

| Value | Constant name | Location |
|-------|---------------|----------|
| `20` | `MinAnswerContentLength` | `session.go:124, :278, :510` |
| `3` | `DuplicateStreakThreshold` | `agent.go:378` |
| `12` | `SpiralStreakThreshold` | `agent.go:390` |
| `0.7` | `MemoryFlushRatio` | `session.go:855` |

**Acceptance criteria:**
- All 4 magic numbers replaced with named constants
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- No new constants appear as magic numbers in the diff

### Refactor-5: Move `prefillDisabled` and `memoryInjected` Into `runSession`

**Where:** `agent.go` (line 122 TODO), `session.go`

**Problem:** Agent god object has mutable state fields that belong in session.

**Work:**
- Move `prefillDisabled bool` and `memoryInjected bool` from `Agent` to `runSession`
- Update all references in session.go and stream.go

**Acceptance criteria:**
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- Agent struct no longer has these two mutable state fields
- Comment `TODO(A8): move into runSession` removed

### Refactor-6: Fix Memory Leaks PL-1, PL-2

| Leak | Fix | Location |
|------|-----|----------|
| PL-1 | Bound persisted history with `TruncateHistory(updatedHistory, MaxPersistedHistoryChars)` — a **high last-resort ceiling (256KB)** — so pathological runs (hundreds of tool cycles) cannot write an unbounded session file, while normal runs persist full (untruncated) history and a reload shows the complete tool-call/reasoning trail (Bug 2). The LLM prompt is separately bounded by the sieve/`context_budget`. | `conversation_service.go` (`handleSuccessResult`/`handleErrorResult`) |
| PL-2 | Wrap webhook registration goroutine with `context.WithTimeout(context.Background(), 30*time.Second)` | `registry.go:221` |

**Acceptance criteria:**
- PL-1: After agent returns 500 messages, persisted history ≤ `MaxPersistedHistoryChars` (256KB); normal histories are persisted full.
- PL-2: Goroutine cannot outlive 30 seconds
- `go test ./internal/core/assistant/... ./internal/core/automation/... -count=1` — 100% pass

**Tests:**
- PL-1: Inject oversized history → verify persisted history is capped
- PL-2: Mock webhook registration that hangs → verify goroutine exits after 30s

### Refactor-7: Split `handleNoToolCalls` (89 lines → ≤50 lines)

**Where:** `session.go:640-728`

**Work:** Extract parse-error handling branch (lines 655-682, ~28 lines) into `handleParseErrorFeedback(err, toolsList)`.

**Acceptance criteria:**
- `handleNoToolCalls` ≤ 50 lines
- `handleParseErrorFeedback` is a separate method, unit-testable
- `go test ./internal/core/assistant/... -count=1` — 100% pass

**Tests:** `handleParseErrorFeedback` tested for: XMLFound=false, XMLFound=true with tools, XMLFound=true without tools, modelCompat notification path.

### Step 0 Commit Boundary

```bash
go build ./... && go test ./... -count=1
```

All existing tests pass. No behavior change. Coverage unchanged or improved.

---

## Step 1: Config & UI Plumbing (No Behavior Change) — COMPLETE

**Purpose:** Add all new `AgentOptions` fields. Wire through config pipeline. Frontend shows controls but values not consumed yet.

### Work

1. **`agent.go`** — Add to `AgentOptions`:
   ```go
   ToolTimeout               time.Duration  // default 2 min, 0 = disabled
   FilesystemToolTimeout     time.Duration  // default 30 sec
   MaxPlanDuration           time.Duration  // default 15 min
   MaxPlanSteps              int            // default 50
   GuardrailTimeout          time.Duration  // default 5 sec
   GuardrailTimeoutBehavior  string         // "fail-open" | "fail-closed"
   ```

2. **`agent.go`** — Add to `applyDefaults()`: set each to its default when zero

3. **`agent.go`** — Add to `ApplyModelConfig()`: override from `cfg` when >0

4. **`models/config.go`** — Add to `ModelConfig`:
    ```go
    ToolTimeoutSeconds            int    `json:"tool_timeout_seconds,omitempty"`
    FilesystemToolTimeoutSeconds  int    `json:"filesystem_tool_timeout_seconds,omitempty"`
    MaxPlanDurationMinutes        int    `json:"max_plan_duration_minutes,omitempty"`
    MaxPlanSteps                  int    `json:"max_plan_steps,omitempty"`
    GuardrailTimeoutSeconds       int    `json:"guardrail_timeout_seconds,omitempty"`
    GuardrailTimeoutBehavior      string `json:"guardrail_timeout_behavior,omitempty"`
    ```

5. **`admin_handlers.go`** — Add to `adminTuningDefaults` struct + `adminConfigView` response

6. **Frontend** — Add to `admin.ts` types, `useConfig.ts` defaults, `modelUtils.ts` form, `ProviderModelsCard.vue` UI

### Acceptance Criteria

- `GET /admin/api/state` returns all new fields with correct defaults
- `go test ./internal/core/assistant/... -count=1` — 100% pass
- `cd frontend && npm run build` — succeeds
- No code path reads the new fields yet (verifiable: grep for new field names shows only config plumbing)

---

## Step 2: Per-Tool Timeouts + executePlan Timeouts + executeLocal Setpgid — COMPLETE

**Problem:** Tools can hang indefinitely. Filesystem ops on slow mounts are unbounded until GlobalTimeout (30 min). `executePlan` has no step limit or per-step timeout. `executeLocal` has no process group isolation — child processes survive context cancellation.

**Closes:** GAP-2 (no per-tool timeout), GAP-9 (executePlan step can hang), GAP-13 (executeLocal no Setpgid)

### Work

1. **`tool_exec.go`** — In `executeSingleToolStep` (from Step 0): wrap `Engine.ExecuteTool` with `context.WithTimeout(ctx, timeoutForTool(tc.Function.Name))`

2. **`tool_exec.go`** — `timeoutForTool(name)` helper:
   - terminal: `ToolTimeout` (default 2 min)
   - network: `ToolTimeout` (default 2 min)
   - filesystem: `FilesystemToolTimeout` (default 30 sec)
   - default: `ToolTimeout`

3. **`tool_exec.go`** — `executePlan`: wrap entire loop with `context.WithTimeout(ctx, a.config.MaxPlanDuration)` (default 15 min), each step uses same `executeSingleToolStep`

4. **`tools/terminal.go`** — `executeLocal`: add `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`, add kill goroutine (copy pattern from `shell/terminal.go:109-115`)

### Acceptance Criteria

- Tool timeout fires: mock engine that blocks → `executeSingleToolStep` returns timeout error within ToolTimeout + 100ms
- Plan timeout fires: 100 fast steps exceeding `MaxPlanDuration` → `executePlan` aborts
- `executeLocal` kills children: run `(sleep 300 &); echo done` → cancel context → child `sleep` process dead
- `go test ./internal/core/assistant/... ./internal/core/tools/... -count=1` — 100% pass

### Tests

```go
TestExecuteSingleToolStep_Timeout               // mock engine blocks > timeout → error
TestExecuteSingleToolStep_WithinTimeout         // mock engine returns fast → success
TestExecuteSingleToolStep_FilesystemTimeout     // filesystem tool uses shorter timeout
TestExecutePlan_PerStepTimeout                  // one step hangs → plan aborts
TestExecutePlan_PlanLevelTimeout                // many fast steps exceed duration → abort
TestExecuteLocal_KillsProcessGroup              // children killed on ctx cancel
TestExecuteLocal_DoesNotKillOtherProcesses      // other processes untouched
```

**Coverage threshold:** >85% line coverage for new/modified functions.

**Integration tests:**
- Script `hang_10min.sh` → `terminal_execute` → agent loop aborts after `ToolTimeout` (not 30 min)
- Plan with step `sleep 300` → `executePlan` returns error after ToolTimeout (not 30 min)
- `executeLocal` path with `(sleep 300 &); echo done` → cancel → `ps aux | grep sleep` shows no orphan

---

## Step 3: StopAutomation Force-Kill — COMPLETE

**Problem:** `StopAutomation()` calls `cancel()` on the context and spawns a goroutine that checks after 30s if the run is still active. If it is, **it only logs a warning** — the hung run continues indefinitely. No SIGKILL, no process group termination, no force stop. The diagnostic goroutine has no context and can accumulate on rapid `StopAutomation` calls.

**Closes:** GAP-1 (StopAutomation observational), GAP-5 (no kill-switch after GlobalTimeout), GAP-8 (PGID tracking incomplete)

### Work

1. **`shell/terminal.go`** — Add `GetPGID() int` method on active session (returns `-cmd.Process.Pid`)

2. **`tools/terminal.go`** — Add `ShellPGID(ctx, workspaceID) (int, error)` (queries shell pool for session PID)

3. **`dispatcher.go`** — Add `runPGID int` field to `AutomationRun` struct

4. **`dispatcher.go`** — `executeAutomation`: call `ShellPGID` after agent creation, store on run struct

5. **`dispatcher.go`** — `StopAutomation`: after 30s goroutine fires, if still in activeRuns AND runPGID != 0 → `syscall.Kill(-runPGID, syscall.SIGKILL)`, log error-level message, remove from activeRuns

6. **`dispatcher.go`** — Tie diagnostic goroutine to cancellable context (fix goroutine leak PL-7)

### Acceptance Criteria

- `StopAutomation` with active shell: after 30s, SIGKILL fires, process group dead, run removed from `activeRuns`
- `StopAutomation` with no shell PGID (network-only run): after 30s, log warning only (graceful degradation)
- Double `StopAutomation` call: second call's goroutine is cancelled by context (no accumulation)
- `go test ./internal/core/automation/... ./internal/core/tools/... -count=1` — 100% pass

### Tests

```go
TestStopAutomation_ForceKillShell             // mock shell PGID → SIGKILL called with correct PID
TestStopAutomation_ForceKillNoShell            // no PGID → graceful (warning only)
TestStopAutomation_DoubleCall                  // two calls → second goroutine cancelled
TestStopAutomation_RunAlreadyFinished          // run already removed → no-op
TestStopAutomation_DiagnosticGoroutineLeak     // goroutine exits on context cancel
```

**Coverage threshold:** >85% line coverage for StopAutomation, >90% for new kill path.

**Integration tests:**
- Start shell running `while true; do sleep 1; done` → `StopAutomation` → 30s → `ps aux | grep bash` shows process dead, `activeRuns` empty
- Start automation → cancel context during `StopAutomation`'s 30s sleep → goroutine exits without mutex access

---

## Step 4: Uniform automationTimeout + executePlan Step Limit — COMPLETE

**Already done in Step 2:**
- `executePlan` pre-check: `if len(plan.Steps) > a.config.MaxPlanSteps → fail fast` (line 332 via `MaxPlanSteps` field)
- `executePlan` plan-level timeout: `context.WithTimeout(ctx, a.config.MaxPlanDuration)` (line 334)

**Remaining:**
1. ~~**`dispatcher.go`** — `Trigger()`: wrap with `context.WithTimeout(ctx, automationTimeout)` before passing to `executeAutomation`~~ ✓
2. ~~**`tool_exec.go`** — `executePlan`: add in-loop check `if i >= a.config.MaxPlanSteps → abort with error`~~ ✓

**Closes:** GAP-3 (executePlan bypasses all loop guards), GAP-4 (automationTimeout only in cron path)

### Work

1. **`dispatcher.go`** — `Trigger()`: wrap with `context.WithTimeout(ctx, automationTimeout)` before passing to `executeAutomation`

2. **`tool_exec.go`** — `executePlan`: ~~add pre-check `if len(plan.Steps) > a.config.MaxPlanSteps → fail fast`~~ ✓ (done in Step 2); in-loop check `if i >= a.config.MaxPlanSteps → abort with error`

### Acceptance Criteria

- Webhook-triggered run: context deadline matches `automationTimeout` (10 min)
- ~~Plan with 60 steps: aborts at step 50 (MaxPlanSteps default)~~ ✓ (pre-check catches this)
- ~~Plan with 100 steps: fails fast before any step executes~~ ✓ (pre-check catches this)
- `go test ./internal/core/automation/... ./internal/core/assistant/... -count=1` — 100% pass

### Tests

```go
TestTrigger_HasAutomationTimeout           // Trigger path context has deadline
TestTrigger_ContextDeadlineMatchesConst    // deadline = automationTimeout from dispatcher.go:28
TestExecutePlan_MaxStepsExceeded           // 51 steps → abort at 50
TestExecutePlan_PreCheckMaxSteps           // 100 steps → fail before any execution  ✓
TestExecutePlan_UnderMaxSteps              // 25 steps → completes normally
```

**Coverage threshold:** >90% line coverage for Trigger timeout path, >90% for executePlan step limit paths.

---

## Step 5: Guardrail Timeout & Defense-in-Depth — COMPLETE

**Status:** All sub-items complete (2026-07-22): Fix-11 guardrail timeout, Fix-5 global watchdog, Fix-6 flag split (`nagSent`/`hardCapTriggered`), PL-3 config-cache bound, PL-4 EventBus orphan reaper, PL-5 guardrail override-cache TTL, PL-6 `agentsFileCache` TTL, O5 single-marshal.

**Problem:** `resolveGuardrail()` calls `ValidateToolCall()` synchronously in the hot path. If a guardrail implementation calls an external policy service (HTTP), hits a DB, or does expensive computation, it blocks the agent loop. No timeout wraps this call. The `forcedCompletionSent` flag is shared between nag and hard cap, disabling the hard cap after a nag. No watchdog monitors the global timeout.

**Implementation note (Fix-11):** The guardrail timeout is applied at the `ValidateToolCall` level inside `resolveGuardrail` (via `guardrailCtxWithTimeout`), not by wrapping the whole `resolveGuardrail` call in `executeSingleToolStep`. This is deliberate: a context deadline must be detected *before* the generic error path runs, otherwise a timeout would be misread as a policy violation and could wrongly trigger the allow/deny approval dialog. On `context.DeadlineExceeded` the configured `GuardrailTimeoutBehavior` (`fail-open` | `fail-closed`) is applied directly. `GuardrailTimeout == 0` leaves evaluation unbounded (legacy behavior).

**Closes:** GAP-5 (no kill-switch after GlobalTimeout), GAP-6 (forcedCompletionSent flag shared), GAP-11 (guardrail evaluation can block loop)

### Work

1. **`agent.go`** — Fix-5: spawn watchdog goroutine in `Execute()`:
   ```go
   go func() {
       select {
       case <-time.After(config.GlobalTimeout + 5*time.Minute):
           log.Critical("watchdog: context still alive past deadline, forcing shutdown")
           // Force-cancel the agent's context to unblock stuck goroutines
           cancel()
       case <-execCtx.Done():
           return
       }
   }()
   ```
   The watchdog force-cancels the context if the global timeout fires but the agent is still running. This unblocks stuck goroutines.

2. **`session.go`** — Fix-6: rename fields and split logic:
    ```
    forcedCompletionSent → nagSent          (handleNoToolCalls only)
    forcedCompletionSent → hardCapTriggered  (checkForcedCompletion only)
    ```
    **Critical:** These flags must be completely independent. `nagSent` is only set in `handleNoToolCalls` (line 714). `hardCapTriggered` is only set in `checkForcedCompletion` (line 440). Neither flag affects the other's logic. Update all references. Update `session_test.go` expectations.

3. **`tool_exec.go`** — Fix-11: in `executeSingleToolStep` (from Step 0), wrap `resolveGuardrail` call:
   ```go
   guardrailCtx, cancel := context.WithTimeout(ctx, a.config.GuardrailTimeout)
   defer cancel()
   approved, stopBatch := a.resolveGuardrail(guardrailCtx, tc, history, mu)
   if guardrailCtx.Err() == context.DeadlineExceeded {
       switch a.config.GuardrailTimeoutBehavior {
       case "fail-closed": return nil, errGuardrailTimeout
       case "fail-open":   log.Warn("guardrail timeout, allowing")
       }
   }
   ```

4. **Memory leak fixes:**
    - PL-3: `registry.go:35` — add `maxEntries=100` + `entryTTL=5*time.Minute` to workspaceConfigCache
    - PL-4: `broadcast.go` — add orphan reaper goroutine. **Problem:** If a subscriber never calls `Unsubscribe` (e.g., HTTP connection drops without cleanup), the channel stays in `subscribers` map forever. **Fix:** Every 30s, scan all subscriber channels. For each channel where `len(ch) == cap(ch)` (buffer full), track when it first became full in a `map[chan]time.Time`. If full for >60s, remove from subscribers map. **Do NOT close the channel** — the subscriber goroutine should exit on `ctx.Done()`. This avoids races with concurrent sends (the code already uses non-blocking sends via `select { case ch <- event: default: }`).
    - PL-5: `guardrails.go:34` — add `createdAt` to cache entries, reaper goroutine removes entries >30min old
    - PL-6: `conversation_helpers.go:16` — convert agentsFileCache to TTL-based (30 min)

5. **Optimization O5:** `tool_exec.go` — Change `appendToolResult` to return `[]byte` (the marshaled result). In `processToolCalls`, capture the returned bytes and reuse for logging at line 165. Remove the second `json.Marshal` call. **Note:** This only applies to the success path where `finalResult == result`. In the error path, `finalResult` is a different structure (error map), so marshal separately.

### Acceptance Criteria

- Watchdog: set GlobalTimeout=1s via config override, run agent that sleeps 7s → critical error logged, context force-cancelled
- Flags: `grep forcedCompletionSent` returns 0 results across assistant package
- Guardrail timeout: slow mock guardrail (10s) + 5s timeout → tool proceeds (fail-open) or rejected (fail-closed)
- PL-3: inject 101 unique workspace configs → oldest entry evicted
- PL-4: 200 leaked subscriber channels → reaper removes them within 60s, no panic on concurrent send, channel not closed by reaper
- PL-5-PL-6: entries older than TTL → gone from cache
- O5: per-tool-execution: exactly 1 `json.Marshal` call for result (verified via coverage or mock)
- `go test ./... -count=1` — 100% pass

### Tests

```go
TestWatchdog_Fires                      // short timeout → critical log + context cancelled
TestWatchdog_NoFalsePositive            // normal completion → no log
TestFlagSplit_NagDoesNotDisableHardCap  // nagged run → hard cap still fires at MaxSteps*2
TestFlagSplit_HardCapIndependent        // hard cap triggered after nag + enough steps
TestGuardrailTimeout_FailOpen           // slow guardrail + fail-open → tool proceeds
TestGuardrailTimeout_FailClosed         // slow guardrail + fail-closed → tool rejected, error returned
TestGuardrailTimeout_WithinLimit        // fast guardrail → normal operation
TestConfigCache_EvictsOnMaxSize         // 101 entries → 100 remain, oldest gone
TestConfigCache_EvictsOnTTL             // stale entry → TTL eviction
TestBroadcastOrphanReaper               // leaked channels → removed from subscribers within 60s, no panic, channel not closed
TestGuardrailCache_TTL                  // entries expire after 30 min
TestJsonMarshal_OncePerToolResult       // result marshaled once (check mock call count)
```

**Coverage threshold:** >80% line coverage for all new functions. >90% for guardrail timeout paths. Existing tests MUST still pass identically.

**Integration tests:**
- Mock guardrail that sleeps 10s → `executeSingleToolStep` → guardrail timeout fires at 5s, agent continues (not hung)
- Configure `GuardrailTimeoutBehavior: "fail-closed"` → same scenario → tool call rejected
- 100 rapid config cache lookups for different workspace IDs → eviction happens, no unbounded growth

---

## Step 6: Context-Aware I/O Hardening

**Problem:** Go's stdlib I/O does **not** reliably respect `context.Context` for DNS lookups (cgo resolver default) and file syscalls on network filesystems. A `network_fetch` to a domain with hung DNS, or `file_read` on a stuck NFS mount, blocks the goroutine past all timeouts.

**Closes:** GAP-10 (Go stdlib operations ignore context)

### Work

1. **`network.go`** — Replace `net.DefaultResolver.LookupIP()` (line 51) with `(&net.Resolver{PreferGo: true}).LookupIP()`. This scopes the pure-Go DNS resolver to the network tool only, avoiding the global side effects of `GODEBUG=netdns=go` (which breaks `/etc/nsswitch.conf`, `NDOTS`, `search` domains, and mDNS). **Tradeoff:** Pure-Go resolver doesn't honor nsswitch but is context-aware and faster for most cases.

2. **`filesystem.go`** — Add `ReadFileWithTimeout(ctx, path, timeout)` helper using goroutine + channel pattern:
    ```go
    func ReadFileWithTimeout(ctx context.Context, path string, timeout time.Duration) ([]byte, error) {
        type result struct {
            data []byte
            err  error
        }
        ch := make(chan result, 1)
        go func() {
            data, err := os.ReadFile(path)
            ch <- result{data, err}
        }()
        select {
        case <-ctx.Done():
            // Goroutine leaks but bounded by filesystem timeout
            leakedFileReads.Add(1)
            return nil, ctx.Err()
        case <-time.After(timeout):
            leakedFileReads.Add(1)
            return nil, fmt.Errorf("file read timeout after %v", timeout)
        case r := <-ch:
            return r.data, r.err
        }
    }
    ```
    Use in `ReadFile`. Add `leakedFileReads atomic.Int64` metric to track goroutine leaks. Add bounded worker pool (e.g., `semaphore.Weighted` with limit 10) to prevent unbounded concurrent file reads on stuck mounts. Document goroutine-leak-on-timeout tradeoff (acceptable for unattended safety — bounded by timeout duration, tracked by metric).

### Acceptance Criteria

- Network DialContext uses scoped `PreferGo: true` resolver (verifiable: existing test passes, audit confirms `network.go:51` changed)
- ReadFileWithTimeout returns within timeout for slow filesystem
- `leakedFileReads` metric increments on timeout (verifiable in test)
- Bounded worker pool limits concurrent file reads (verifiable: 20 concurrent reads → only 10 proceed)
- `go test ./... -count=1` — 100% pass

### Tests

```go
TestReadFileWithTimeout_WithinLimit      // file read completed before timeout
TestReadFileWithTimeout_Exceeded         // timeout fires, metric increments
TestReadFileWithTimeout_BoundedPool      // 20 concurrent reads → only 10 proceed
```

**Coverage threshold:** >80% line coverage for ReadFileWithTimeout. No regression in existing coverage.

---

## Step 7: Unattended Tool Restriction & Spiral Detection

**Problem:** There is no mechanism to restrict tool access for unattended runs beyond the per-tool guardrails. All automations get the same tool set as the assistant. The alternating-tool spiral bypasses spiral detection — model alternates `file_read` → `grep` → `file_read` → `grep`... Spiral counter resets every turn. Starvation counter doesn't fire (tools ARE called). Duplicate detection doesn't fire (different tools).

**Closes:** GAP-7 (no safe mode for unattended runs), GAP-12 (alternating-tool spiral bypasses detection)

### Work

1. **Fix-7:** `dispatcher.go` — read `AllowedTools` from `AutomationEntry` config. When set, wrap `ToolProvider` with `NewFilteredToolProvider` configured as allowlist (not excludelist). **Note:** `FilteredToolProvider` type is unexported (`filtered_provider.go:9`) — export it or add a new constructor `NewAllowedToolsProvider`. Empty = pass existing exclude mechanism through (backward compatible).

2. **Fix-12:** `agent.go` — add `alternatingToolWindow []toolCallRecord` to `repetitionDetector`. New method `checkAlternating() -> (bool, string, error)`:
    - Window of last 20 tool calls
    - Compute `uniqueRatio = uniqueToolNames / windowSize`
    - If `uniqueRatio <= 0.3` over 15+ turns → abort (tool oscillation: e.g., alternating 2 tools repeatedly)
    - Call from `handleToolTurn` after existing `detectSpiral()` check
    - **Note:** Path staleness is NOT used as a signal — legitimate agent tasks often read/grep/write the same files with different tools. Tool oscillation alone is sufficient to detect stuck loops.

### Acceptance Criteria

- Automation with `allowed_tools: [read_file, directory_list]` → `terminal_execute` blocked by FilteredToolProvider
- Automation with empty `allowed_tools` → all tools available
- Alternating `file_read`/`grep` 15 times → "alternating tool spiral detected" abort (uniqueRatio ≤ 0.3)
- Reading 5 different files with `file_read` → no false positive (uniqueRatio = 1.0)
- `go test ./internal/core/automation/... ./internal/core/assistant/... -count=1` — 100% pass

### Tests

```go
TestAllowedTools_RestrictsAccess          // allowlist blocks terminal
TestAllowedTools_EmptyAllowsAll           // empty = all tools pass through
TestAllowedTools_ReadOnlyAutomation       // read_file, directory_list only; terminal blocked
TestAlternatingSpiral_Detected            // 15 turns alternating 2 tools → abort (uniqueRatio ≤ 0.3)
TestAlternatingSpiral_NoFalsePositive     // 15 turns with 10 unique tools → no abort (uniqueRatio = 1.0)
TestAlternatingSpiral_DoesNotConflict     // legacy detectSpiral still works
```

**Coverage threshold:** >90% line coverage for `checkAlternating`. >85% for FilteredToolProvider allowlist path.

**Integration tests:**
- Configure automation with `allowed_tools: [read_file, directory_list]`. Run task requesting terminal. Verify guardrail blocks
- Agent bouncing between `file_read` and `grep` 15+ times. Verify "alternating tool spiral detected"

---

## Step 8: Performance Optimizations

**Problem:** Hot-path inefficiencies: `uuid.NewString()` (crypto RNG syscall) per stream chunk, `persistence.ReadConfig` file I/O on every tool call, lock held through `json.Marshal` + `TruncateResult`, two `strings.ReplaceAll` in `cleanReasoningContent` per chunk.

**Optimizations:** O1, O3, O6, O7

### Work

1. **O1:** `agent_events.go` — replace `uuid.NewString()` with `atomic.AddUint64(&a.eventCounter, 1)` for stream events. Add `eventCounter uint64` to Agent. (Task-scoped, not security-sensitive)

2. **O3:** `guardrails.go` — replace `persistence.ReadConfig(workspaceID)` with `workspaceConfigCache` lookup (from registry). Cache already exists at `registry.go:33-76`

3. **O6:** `tool_exec.go` (appendToolResult) — marshal + truncate before acquiring `mu.Lock()`, only hold lock for `*history = append(...)`

4. **O7:** `stream.go` — precompile `cleanReasoningContent` replacements as `regexp.Regexp` or single pass

### Acceptance Criteria

- Event stream uses counter IDs, no UUID allocation per chunk
- Guardrail validation uses cached config, no file I/O (verify via mock on persistence)
- `appendToolResult` lock held only for slice append (verify lock hold duration in benchmark)
- `cleanReasoningContent` uses precompiled regex
- `go test ./... -count=1` — 100% pass

### Tests

```go
TestEventStream_UsesCounterID           // event IDs are sequential integers, no UUID
TestGuardrailValidation_UsesCachedConfig // no ReadConfig call on cached workspace
TestAppendToolResult_LockScope          // lock held only for append (≤1 microsecond)
TestCleanReasoningContent_Precompiled   // regex compiled once, reused
```

**Coverage threshold:** No regression in existing coverage.

---

## Step 9: Documentation Sync

**Problem:** Docs out of sync with code. Key constants missing, execution flow incomplete, `executePlan` bypass path undocumented, `automationTimeout` scope unclear.

### Work

1. `docs/skills/automation.md:42` — "5-minute" → "10-minute"

2. `docs/skills/automation.md` — Add `StopAutomation` behavior section (best-effort cancel + 30s diagnostic goroutine + force-kill after 30s)

3. `docs/skills/automation.md` — Document `automationTimeout` scope: all trigger paths (cron, webhook, manual) after Step 4

4. `docs/skills/agent-loop.md:134-147` — Add `DefaultStarvationLimit=15`, `AgentRetryTimeout=5min` to Key Constants table

5. `docs/skills/agent-loop.md` — Add `executePlan` bypass path to Execution Flow diagram

6. `docs/skills/agent-loop.md` — Add section on `executePlan` execution: step limits, per-step timeouts, plan-level timeout, guardrail checks

7. `docs/SPECS/agent-loop.md` — Add `executePlan` section under II. Functional Requirements

8. `docs/SPECS/agent-loop.md:175-199` — Clarify shell process group kill is tool-specific (not general StopAutomation force-kill)

9. `docs/SPECS/automation-dispatcher.md` — Document `automationTimeout` scope (all trigger paths)

### Acceptance Criteria

- All 9 doc audit items resolved
- No broken links (run `grep -r "\.md#" docs/` to verify)

---

## Files Modified (Complete List)

| Step | Files |
|------|-------|
| Step 0 | `tool_exec.go`, `prompts/templates.go`, `agent.go`, `session.go`, `session_test.go`, `stream.go`, `conversation_service.go`, `registry.go` |
| Step 1 | `agent.go`, `models/config.go`, `admin_handlers.go`, frontend: `admin.ts`, `model.ts`, `useConfig.ts`, `modelUtils.ts`, `ProviderModelsCard.vue` |
| Step 2 | `tool_exec.go`, `agent.go`, `tools/terminal.go` |
| Step 3 | `dispatcher.go`, `shell/terminal.go`, `tools/terminal.go` |
| Step 4 | `dispatcher.go`, `tool_exec.go`, `agent.go` |
| Step 5 | `agent.go`, `session.go`, `session_test.go`, `tool_exec.go`, `registry.go`, `broadcast.go`, `guardrails/guardrails.go`, `conversation_helpers.go` |
| Step 6 | `network.go`, `filesystem.go` |
| Step 7 | `dispatcher.go`, `agent.go`, `filtered_provider.go` |
| Step 8 | `agent_events.go`, `guardrails/guardrails.go`, `tool_exec.go`, `stream.go` |
| Step 9 | `docs/skills/automation.md`, `docs/skills/agent-loop.md`, `docs/SPECS/agent-loop.md`, `docs/SPECS/automation-dispatcher.md` |

---

## Verification at Each Step

Every step ends with:

```bash
cd backend && go build ./... && go test ./... -count=1
go run ./tools/check-complexity/
```

Step 1 additionally:
```bash
cd frontend && npm install && npm run build
```

No step is complete until all pass clean.

---

## Step Dependencies

Steps depend on prior steps in ways not always obvious from the step descriptions:

```
Step 0 (refactor) → Step 1 (config) → Step 2 (timeouts use new config + executeSingleToolStep)
                                      → Step 5 (guardrail timeout uses executeSingleToolStep)
                                      → Step 7 (allowed tools uses FilteredToolProvider; alternating uses executeSingleToolStep)
Step 2 (Setpgid, PGID) → Step 3 (StopAutomation force-kill needs PGID)
Step 6 (standalone — no dependency on prior steps)
Step 8 (standalone — can be done anytime after Step 0)
Step 9 (last — documents completed changes)
```

Steps 3, 4, 5, 7 could be parallelized after Step 2 completes (assuming no file conflicts).
