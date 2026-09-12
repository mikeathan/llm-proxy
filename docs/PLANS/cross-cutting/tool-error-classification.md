# Tool Error Classification & Run-Fatality Policy

**Status:** active — implemented; automated gates green (build/vet/race/complexity); manual end-to-end pending
**Date:** 2026-09-12
**Related Specs:** SPEC-001 (Agent Loop), SPEC-010 (Agent Loop Strategies), SPEC-006 (Guardrail Engine)

## Goal

Stop two opposite failure modes seen in practice:
1. **Flailing:** a tool fails for an operator-actionable reason (invalid API key) and the agent keeps trying alternative tools until `max_steps` (observed: `internet_search` 401 → three `fetch_url` attempts → more turns). Nothing bounds it.
2. **Misleading success:** a run reports "completed" when an essential capability was down.

…without breaking the legitimate cases: recoverable/input errors must still record-and-continue (a `fetch_url` 404 is the model adapting), and a **delivery** tool failing (Telegram down) must **not** fail a run whose report was still produced offline.

## Core design: two orthogonal axes

| Axis | Set by | Meaning |
|---|---|---|
| **Terminal** (don't retry this tool) | the tool, via a code marker | retrying can't help — credential rejected, integration unavailable |
| **Fatal to the run** | the loop's policy, keyed on tool **role** + channel | the task's result can no longer be produced/trusted |

Conflating these is the bug. `notify_user` is terminal but **not** fatal (delivery side-effect). `internet_search` in a "find me X" task is terminal **and** fatal (acquisition is the goal).

## 1. Error taxonomy (code, never the manifest)

The manifest (`manifests/*.json`) is static schema + default guardrails (`parameters`, `guardrails`, `runtime`). Error classification is runtime, provider-specific behaviour — so it lives in code as a sentinel, mapped where the HTTP status is known. **No `terminal_errors` list in JSON; no manifest change.**

- **`models.ErrToolUnavailable`** — NEW sentinel, operator-actionable (missing/rejected credential, disabled integration). Declared in the leaf `models` package (`models/tools.go`, alongside the tool constants).
  - *Why `models`, not `tools`:* `models` is the leaf both `tools`/`searchproviders` and `assistant` import, so no cycle — and `tools/` is already at the 15-file package soft cap (`engineering-practices`), so a new `tools/errors.go` would exceed it. Precedent for a leaf sentinel: `models.ErrSecretNotFound`.
- **Transient** (timeouts, 429, 5xx) — stays a plain error; retryable; **no behaviour change**.
- **Input/content** (404, bad URL, blocked filename) — stays a plain error; model-actionable; **no behaviour change**.
- **Security/guardrail** — existing `guardrails.ErrNetworkDisabled` + `isGuardrailSecurityBoundary` (`assistant/tool_exec.go:499`); unchanged, already synchronous and non-approvable.
- **Unclassified** = continue (fail-open). Only a *marked* error changes behaviour — this is the "if they don't exist it continues" default.

Unify the search sentinel so the loop has one check: redefine
`tools.ErrSearchNotConfigured = fmt.Errorf("internet search is not configured: %w", models.ErrToolUnavailable)`
(preserves the specific message, and `errors.Is` now matches the shared marker through the existing `%w` wrap in `InternetTools.Search`).

## 2. Producers — per-tool opt-in (audit result)

Only tools with an operator-actionable failure mode emit the marker.

| Tool | Emit marker? | Change |
|---|---|---|
| `internet_search` | **Yes** | `searchproviders/search_tavily.go` / `search_brave.go` / `search_serpapi.go`: map **401/403** → wrap `models.ErrToolUnavailable`. Other statuses stay plain. |
| `notify_user` | **Yes** (token/no-connector); **No** for "wrong connector type" (model-actionable — it already lists available types) | `notifiers/telegram.go:53`: 401/403 → wrap marker. `tools/communication.go:107`: replace `fmt.Errorf("some notifications failed: %v", errs)` with `errors.Join(errs...)` so the chain survives (`%v` currently flattens it). |
| MCP tools | **Deferred** | Errors are opaque ("client not initialized", "tool call failed"), can't distinguish config vs transient. Leave continue; revisit only if MCP surfaces an auth signal. |
| `fetch_url` / `scan_local_network` / `get_network_info` | No | input/transient; policy handled synchronously by guardrails. |
| filesystem / terminal | No | input/permission; guardrails synchronous. |
| `memory_search` / `memory_update` | No | disabled ⇒ tools not registered; no operator credential. |
| `system_error` / `execute_plan` | No | internal. |

Optional consistency: the registry nil-handler errors (`registry.go` "communication tools not configured" / "network tools not configured") may wrap the marker; mostly unreachable (availability/registration gates).

Shared helper in `searchproviders/search_registry.go`: `isAuthStatus(code int) bool` (401/403) so the three providers map identically.

## 3. Tool role (code, deterministic)

A tiny classification — a property of the tool, not of the task, so it is deterministic and needs no config:
- **delivery/side-effect:** `models.ToolNotifyUser` — non-fatal by nature.
- **essential:** everything else (default).

`assistant/toolpolicy/` helper: `PolicyFor(name) Policy` (explicit map + `Fatal` default), keyed by tool name. That package now exists (delivered by the assistant package restructure). No manifest, no per-automation config, no "required_for_completion" flag.

## 4. Loop policy

New per-run state on `runSession` (`assistant/session.go:57-114`, cleared in `run()` at `:739`):
- `disabledTools map[string]struct{}` — tools disabled for the remainder of the run (terminal errors). **Per run only** — a fixed key must re-probe next run.
- `toolFailureStreak int` — consecutive errored tool calls, any tool. Reset on any successful tool call (mirror the existing reset-on-success in `resetParseErrorState`, `session.go:483`).
- `toolWarnings []string` — accumulated non-fatal (delivery) failures for the run meta/report.

New consts (alongside `tool_exec.go:44-55` / guards near `guardrailBlockStreakLimit` at `:1039`): `toolFailureStreakLimit = 3`.

Behaviour in `executeSingleToolStep` / `processToolCalls` (`tool_exec.go:302-401`):
1. **Pre-exec check:** if `tc.Function.Name ∈ disabledTools` → short-circuit with the directive tool-result (no execution, no network).
2. **On execution error:**
   - `if errors.Is(err, models.ErrToolUnavailable)`:
     - add to `disabledTools`;
     - build a directive tool-result (new prompt const);
     - **delivery tool** → append to `toolWarnings`; do **not** fail (chat or automation).
     - **essential tool** → `ChannelAssistant`: continue with the directive; `ChannelAutomation`: **fail the run** (return the typed error so it propagates `handleToolTurn` → strategy → `executor.handleAgentError`).
   - else → `toolFailureStreak++`; if `≥ toolFailureStreakLimit`: chat → force finalization with an honest "couldn't complete: …" (`finalizeReport`/`bestAvailableAnswer` path); automation → fail the run.
3. **On success:** `toolFailureStreak = 0`.

Directive wording (new consts in `assistant/prompts/templates.go`, the only prompt home per Constitution II.13):
- essential: `"TOOL UNAVAILABLE: <tool> — <reason>. Do NOT retry it or route around it. If the task depends on this capability, report the failure; otherwise continue with the remaining work and note the gap."`
- delivery: `"DELIVERY FAILED: <tool> — <reason>. The result is still valid; do NOT retry. State the delivery failure in your final answer."`

Note: `prompts.ToolErrorNagPrompt` (`templates.go:278-280`) exists but is **not injected anywhere** — either wire it or replace it with the above.

**Plan-execute path** (`executePlan`, `tool_exec.go:590-610`, which currently tolerates every step error): a **terminal essential** error must **abort the plan** and follow the same channel policy (chat: directive + continue/finalize; automation: fail). Input/transient step errors keep the existing "recorded, not run-killers" behaviour (SPEC-010 §II.2). Update SPEC-010 accordingly.

## 5. Surfacing (delivery warnings + UI/ops)

- **Chat:** emit a system-role warning message (reuse the `notifyFallbackWarning`/`notifyPrefillDisabled` shape, `agent_events.go:165/223`) and `log.Warn`. No run failure.
- **Automation:** expose accumulated warnings from the agent to the executor (add `Agent.ToolWarnings() []string`, read after `Execute`) and persist:
  - `RunMeta.Warnings []string` — `automation/rundir.go:26-38`, written in `handleAgentSuccess` (`executor.go:501-513`).
  - `models.AutomationRun.Warnings []string` — `models/workspace.go:108-123`, written by `recordRun` (`executor.go:531-566`).
  - A run with warnings but empty `Error` still counts **success** in `dispatcher.LoadHistory` (`dispatcher.go:435-441`); optionally add a "succeeded with warnings" metric later (out of scope).
  - Optionally append a delivery-failure note to `final-report.md` (`rundir.go:77-79`).

## Files to change (backend)

| File | Change |
|---|---|
| `backend/models/tools.go` | `ErrToolUnavailable` sentinel (+ wrap note) |
| `backend/models/workspace.go` | `AutomationRun.Warnings []string` |
| `backend/internal/core/tools/searchproviders/search_registry.go` | `isAuthStatus(int) bool` helper |
| `backend/internal/core/tools/searchproviders/search_{tavily,brave,serpapi}.go` | 401/403 → wrap `models.ErrToolUnavailable` |
| `backend/internal/core/tools/search.go` | `ErrSearchNotConfigured` wraps the marker |
| `backend/internal/core/tools/notifiers/telegram.go` | 401/403 → wrap marker |
| `backend/internal/core/tools/communication.go` | `errors.Join(errs...)` (preserve chain) |
| `backend/internal/core/assistant/prompts/templates.go` | tool-unavailable / delivery-failed prompt consts |
| `backend/internal/core/assistant/session.go` | `runSession` fields + reset-on-success |
| `backend/internal/core/assistant/tool_exec.go` | classifier, disabled set, directives, failure streak, automation fail, plan-path handling |
| `backend/internal/core/assistant/toolpolicy/` | per-tool failure policy table (`Fatal` default) |
| `backend/internal/core/assistant/agent.go` / `agent_events.go` | `ToolWarnings()` + warning notification |
| `backend/internal/core/automation/rundir.go` | `RunMeta.Warnings` |
| `backend/internal/core/automation/executor.go` | capture warnings → meta + `recordRun` |

## Tests (mirror source, reuse existing tables/MockClient)

- `searchproviders/search_tavily_test.go` (+ brave/serpapi): 401/403 → `errors.Is(err, models.ErrToolUnavailable)`; 5xx stays unmarked.
- `tools/communication_test.go`: `NotifyAll` preserves the chain (`errors.Is` through `errors.Join`).
- `notifiers/telegram_test.go`: 401 → marker.
- `assistant/tool_exec_test.go`: (a) essential terminal + `ChannelAutomation` → run fails; (b) essential terminal + chat → directive recorded, tool added to `disabledTools`, a second call short-circuits (no engine call), run continues; (c) delivery tool failure → no fail + `toolWarnings` populated; (d) unclassified error → unchanged record-and-continue; (e) 3 consecutive failures → chat finalizes / automation fails; (f) success resets the streak.
- `assistant/react_strategy_test.go` + `plan_execute_strategy_test.go`: integration (extend `TestReactStrategy_ToolErrorContinues` style; add terminal-abort in plan).
- `automation`: `RunMeta.Warnings` written on a delivery failure (executor test).

## Docs

- `docs/SPECS/agent-loop.md` §II.6 (Error Recovery, `:68-83`): add the classification contract + per-channel policy.
- `docs/SPECS/agent-loop-strategies.md` §II.4 (`:52`) / §VI: cross-strategy invariant; plan-execute terminal-abort exception.
- `docs/SPECS/guardrails.md` §II.3 (`:93-102`): cross-reference that tool-terminal errors are a separate axis from guardrail denials.
- `docs/architecture.md`: one-line pointer (optional).
- `.agents/skills/engineering-practices/SKILL.md` "When adding a search provider": note the 401/403 → marker mapping.

## Implementation order

1. Sentinel + provider/connector mapping + `NotifyAll` chain fix (+ tests).
2. Loop: classifier, `disabledTools`, directives, failure streak, plan-path handling (+ tests).
3. Automation: essential-terminal fail policy + delivery warnings in `RunMeta`/`AutomationRun` (+ tests).
4. Docs (SPEC-001/010/006, engineering-practices).

## Verification

- Backend: `go build ./...` · `go vet ./...` · `go test ./... -race -coverprofile=coverage.out -covermode=atomic` · `go run ./tools/check-complexity/`.
- Manual E2E:
  1. Bad search key, chat "find X" → model is told the tool is unavailable, does **not** retry, final answer states the failure.
  2. Bad search key, automation → run **fails** with a clear error (no misleading success).
  3. Telegram down, automation "generate report + notify" → run **completes**, `final-report.md` present, `run-meta.json` has `warnings`, no `error`.
  4. Unclassified error (bad URL 404) → unchanged record-and-continue.
- Frontend `npm test && npm run build` if any `RunMeta`/run-view type changes are surfaced (currently none planned).

## Out of scope

- MCP error classification (opaque) — deferred with rationale.
- Per-automation "optional step"/"required tool" markers.
- Retry/backoff for transient errors (existing behaviour unchanged).
- New UI for warnings beyond the existing system-message/run-meta surfaces.
- Changing the binary success/failure inference in `dispatcher.LoadHistory` beyond adding `Warnings`.

## Open decisions

1. Sentinel home: `models` (recommended, cap + leaf) vs `tools` (cohesive, exceeds the 15-file cap).
2. `toolFailureStreakLimit` default: 3 (proposed) — tune after observing runs.
3. Delivery warnings: also append a note to `final-report.md`, or run-meta/event only?
