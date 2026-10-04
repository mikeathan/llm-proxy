---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-005, SPEC-007, SPEC-003, SPEC-001]
constitution_references: [II.2, II.14, III.2, IV.1, IV.2, V.2, V.3]
related_plans: [../agent-loop/agent-improvements.md, local-and-cloud-inference-performance.md, auto-model-router.md, ../autonomy/attention-policy.md, ../autonomy/autonomy-roadmap.md]
---

# Usage, Cost and "Saved by Running Locally" Ledger

**Status:** proposed. Phase 1 (capture) changes no user-facing behaviour and has no open decision.

## Why this plan exists

The service runs work on paid cloud keys and on free local GPUs, but nobody can answer "what did this
cost me?" or "what did running locally save me?". Token counts are recorded for **chat turns only**;
automation runs and external `/v1` traffic — the parts that run unattended — record none.

| User benefit | Service benefit |
|---|---|
| **See spend** per provider, model, workspace and automation, per day/week/month. | One durable usage record feeds the budget orchestrator, the router (`auto-model-router.md`) and future quotas — instead of each feature counting on its own. |
| **Catch runaway automations** — a looping scheduled job on a paid key shows up as a spike, with an optional alert via `notify_user`. | Hard numbers for performance work (`local-and-cloud-inference-performance.md` Phase 0 needs per-call token data). |
| **Cap autonomous spend** — the autonomy program's attention policy stops agenda runs and goal ticks for the day at your limit (`../autonomy/attention-policy.md` Phase 3). | The ledger is the single spend source for that cap; Phase 2 (store + query API) is its prerequisite. |
| **"Saved by running locally: $X"** — local tokens priced at a reference cloud model the user picks. Makes the value of local-first concrete. | Closes the open UsageTracker item (`../agent-loop/agent-improvements.md` Phase 5) with a real consumer. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| `usage.Tracker` exists; `InputTokens/OutputTokens` are **local size estimates**; provider-reported counts are kept separately (`AddReportedUsage` / `ReportedUsage`) | `backend/internal/core/assistant/usage/usagetracker.go:15-70` |
| Reported usage is added once per stream from the provider's `usage` block | `backend/internal/core/assistant/stream.go:1236-1242` |
| **Chat** persists reported tokens on the user turn (`TurnRun.PromptTokens/CompletionTokens`) | `backend/internal/core/assistant/conversation_service.go:121-130`; `backend/models/llm_messages.go:59-68` |
| **Automations** install a tracker but only *log* the local estimates at run end; nothing is persisted | `backend/internal/core/automation/executor.go:188`, `:245-248` |
| `RunMeta` (`run-meta.json`) and `AutomationRun` have **no token fields** | `backend/internal/platform/rundir/rundir.go:31-47`; `backend/models/workspace.go:136-155` |
| External `/v1/chat/completions` is a plain `httputil.ReverseProxy` pass-through; the response body is never inspected, so no usage is recorded | `backend/internal/transport/http/handlers/proxy_handlers.go:175-238` |
| Provider catalogs already return dollar pricing (`ModelPricing{Prompt, Completion}` per token, as strings) — parsed for OpenAI-compatible providers such as OpenRouter | `backend/models/llm.go:74-77`; `backend/internal/core/llm/providers/provider_openai_compatible.go:176`, `:272-283` |
| That pricing is used **once** at model-add time to compute an ICU weight and then discarded; `ModelConfig` does not store it | `backend/internal/transport/http/handlers/model_handlers.go:226-227`; `backend/internal/core/orchestrator/budget_squeezer.go:96-112` |
| Nothing in the backend sends `stream_options.include_usage`; streamed usage arrives only when the provider sends it unasked | `grep -rn include_usage backend/internal` → no hits |

## Design

### One usage record, three producers
```go
type UsageRecord struct {
    At          time.Time
    Source      string // "chat" | "automation" | "inbound"
    Workspace   string // "" for inbound
    Automation  string // automation name, or "" 
    SessionID   string // chat session / run ID
    Model       string
    Provider    string
    Workload    string // "local" | "cloud"
    PromptTokens, CompletionTokens int
    Reported    bool   // false = estimate (provider sent no usage)
}
```
- **Chat:** emit from `finishTurnRun` (already has the numbers).
- **Automation:** emit at run end from the executor using `ReportedUsage()`; also persist the totals in
  `RunMeta` and `AutomationRun` so a run's cost shows next to its output.
- **Inbound `/v1`:** a `ModifyResponse` hook on the reverse proxy that parses the `usage` object of a
  JSON response, or tees an SSE stream and reads the final chunk's `usage`. Never buffers the whole
  stream; never alters bytes sent to the client.

### Storage
Append to a SQLite table in the existing platform DB layer (`backend/internal/platform/db/`), one row
per completed call/turn/run, with daily roll-ups computed on read. Bounded by a retention setting
(default 400 days), cleaned by the existing ledger cleaner pattern
(`backend/internal/platform/ledger/cleaner.go`). Writes are asynchronous through a bounded channel
drained by one goroutine tethered to the root context (II.2, II.14); on overflow, drop and count (never
block a request).

### Prices
- Persist `pricing` on the model entry when the provider catalog publishes it (currently discarded).
- Allow a manual per-model price override in Settings (for providers without a published price).
- Local models price at $0; the **"saved"** figure prices local tokens at a user-chosen *reference
  model*'s price. If none is chosen the card is hidden — no invented numbers.
- Prices are stored as decimal strings and computed with integer micro-dollars to avoid float drift.

### API + UI
- `GET /admin/api/usage?from=&to=&group_by=model|provider|workspace|automation|source|day`
- `GET /admin/api/usage/summary` — this month's spend, saved-by-local, top 5 automations by spend.
- Dashboard card + a Usage view (charts follow the repo's theme tokens).
- Optional alert: "automation X spent more than $N today" → `notify_user` via the configured connector.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Request usage from providers (`stream_options.include_usage: true`) for agent calls? | Yes for providers known to accept it; it is how OpenAI reports streamed usage. Behind the provider manifest so strict providers are not broken |
| D2 | For inbound `/v1` streams without usage: store an estimate (flagged `Reported=false`) or nothing? | Store the estimate, flagged; the UI shows estimates with a "~" prefix |
| D3 | Default retention | 400 days |
| D4 | Spend alerts in v1 or later? | Later (Phase 5) — capture and show first |

## Phases

### Phase 0 — Measure what providers actually report (no behaviour change)
- With recording on (`go run main.go --record`), run one chat and one automation per configured provider
  type (llama.cpp, OpenAI-compatible cloud, Gemini) and note whether `usage` arrives in stream and
  non-stream modes. Record the results in this plan.
- **Acceptance:** a table here: provider × mode → usage present yes/no.

### Phase 1 — Capture for automations (no UI)
- Persist reported tokens to `RunMeta` and `AutomationRun`; emit `UsageRecord` for chat and automation.
- **Acceptance:** executor test with a mock LLM reporting usage → `run-meta.json` contains the token
  totals; `go test ./internal/core/automation/ ./internal/core/assistant/ -count=1`.

### Phase 2 — Store + query API
- SQLite table, async writer, retention cleaner, `/admin/api/usage*` endpoints.
- **Acceptance:** store tests (insert, group-by, retention), handler tests; `go test -race` on the writer
  package (new goroutine).

### Phase 3 — Inbound `/v1` capture
- Reverse-proxy response hook for JSON and SSE; estimate fallback per D2.
- **Acceptance:** proxy handler tests with `httptest` upstreams — JSON with usage, SSE with usage in the
  final chunk, SSE without usage → estimate; client receives byte-identical bodies (compare checksums).

### Phase 4 — Prices + UI
- Persist catalog pricing on the model; manual override field (follow
  `docs/architecture.md` → File Change Checklist, "new model-level field"); reference model setting; dashboard card and
  Usage view.
- **Acceptance:** `npm test && npm run build && npm run test:visual`; Go handler tests for the new field.

### Phase 5 — Alerts (needs D4)
- Daily per-automation spend threshold → a `normal` notice through the attention policy (`../autonomy/attention-policy.md`), or `notify_user` directly if that plan has not landed yet.
- **Acceptance:** test with a fake clock and a fake connector.

### Phase 6 — Docs
- SPEC-005 (usage record + pricing), SPEC-007 (run meta fields), SPEC-003 (Usage view),
  `docs/api-reference.md`, `docs/data-layout.md` (new table).
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Prices change** — provider prices are fetched at model-add/refresh time; the record stores tokens,
  not dollars, so history is re-priced on read and a price change never corrupts data. (A "price at the
  time" snapshot can be added later if needed.)
- **Streaming overhead on `/v1`** — tee + last-chunk parse only; Phase 3 includes a benchmark to show
  no measurable added latency.
- **Non-goal:** billing, invoices, multi-user quotas.

## Remaining Work
All phases (0–6). Phase 0 first; Phase 5 blocked on D4.
