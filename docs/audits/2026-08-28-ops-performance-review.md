---
status: reference
date: 2026-08-28
---

# Ops & Backend Performance Review — Findings and Fixes (2026-08-28)

**Scope:** wider-scope performance review outside the frontend-rendering/GPU domain
(the GPU work lives in `docs/audits/gpu-performance-audit.md`). Static analysis of the
backend hot paths, event bus, persistence, storage, logging, SQLite, and cross-cutting
polling. Findings first, fixes second, deferred items last.

## Findings

### P1

1. **No log rotation anywhere.** `logging/file_logger.go` opened files with
   `O_CREATE|O_APPEND|O_WRONLY` only — workspace process logs and the app log grow
   without bound.
2. **Full-file log reads on polls.** The workspace process-log endpoint
   (`AdminWorkspaceProcessLogsHandler`) did `os.ReadFile` of the entire file; the
   frontend Logs panel polls every 10s. The app-log `/tail` endpoint already bounded
   itself to 64KB, but the workspace one did not.
3. **Host-metric reads per /metrics request** (`MetricsService.readHostMetrics`):
   `cpu.Percent(0)` is a "since the last call" delta, unsafe under concurrent callers,
   and ran on every snapshot (multiple frontend components poll /metrics on a 10s
   cadence).
4. **EventBus `recent` replay buffer capped by count only** (1000 events): coalesced
   reasoning/content snapshots are 5–20KB each, so a count-only cap can retain tens of
   MB per workspace/channel after a heavy run.
5. **Slow-subscriber warnings logged per dropped event** (`EventBus.Publish`) — a
   stalled subscriber produced a log line per event.

### P2

6. **Per-chunk O(n) scans in `processStream`** (`stream.go`): `FilterStreamingMarkup`
   (14× `strings.Index`), `countEmptyClosedToolCalls` (`ToLower` of full reasoning),
   `isRepetitionDominated` (sliding-window map when content ≥400 chars),
   `tryExtractToolCallFromReasoning` (regex), and `fullMsg.Content +=` concat — all
   O(n)/chunk → O(n²)/stream, bounded by the char caps (~10–11KB) and 90s stream cap,
   so absolute cost is a few MB of scanning per stream.
7. **Flaky test** `TestBroadcastCriticalEventDeliveredWhenFull` — pre-existing timing
   flake (failed once under full-suite load; passes 3/3 isolated and on clean HEAD).

### P3 / notes

8. `Store.Get()` deep-copies every config/registry read via JSON round-trip (intentional
   C1 safety; fine at current scale).
9. `FormatCollectedEvents` builds the full event array even though the detached
   assistant flow replies 202 and discards it.
10. Heartbeat INFO log every 30s per active stream.

## Fixes shipped (2026-08-28)

- **Log rotation** (`logging/file_logger.go`): `Options.MaxSizeBytes` (default 10MiB);
  when a file exceeds it, it rotates to `<path>.1` (previous backup replaced) and a
  fresh file starts — disk bounded at ~2×maxSize per path. Rotation is tracked by a
  bytes-written counter (no per-line stat). Tests: `TestFileLogger_RotatesAtMaxSize`,
  `TestFileLogger_NoRotationWhenUnderMaxSize`.
- **Workspace process-log tail** (`process_handlers.go`): shared `readTail` helper
  (last 64KB) used by the app-log `/tail` handler and the workspace process-log
  endpoint — polls never transfer a whole growing file.
- **Host-metrics cache** (`metrics_service.go`): `readHostMetrics` now caches for 2s
  under a mutex, serializing `cpu.Percent(0)` and cutting per-request cost.
- **EventBus byte budget + rate-limited warnings** (`automation/broadcast.go`): the
  `recent` replay buffer is now capped by both count (1000) and approximate bytes
  (4MiB/channel); slow-subscriber warnings are throttled to one per 10s per
  workspace/channel (throttle entry cleaned on Unsubscribe). Tests:
  `TestRecentCappedByBytes`, `TestDropWarnRateLimited`.
- **Compact session marshal** (`persistence/workspace.go`): `WriteSession` uses
  `json.Marshal` instead of `MarshalIndent` — sessions can grow to MBs over long runs
  and are written on every tool cycle.
- **`liveEvents` bounded** (frontend): chat's retained event log is capped at 1000
  (only the newest element is ever read); the automation `useLiveConsole` `liveEvents`
  array was dead (no consumer) and removed.
- **settings.yml GPU keys canonicalized with a migration guard** (`models/`):
  `MetricsConfig`/`GPUConfig` now carry `yaml` tags with the documented snake_case keys
  (`gpu_sample_interval_seconds`, `gpu_smoothing_alpha`, `sysfs_path`); custom
  `UnmarshalYAML` still accepts the legacy field-name-derived keys
  (`gpusampleintervalsec`, `gpusmoothingalpha`, `sysfspath`) so existing files load
  after upgrade, and the next store write migrates them. Canonical keys win when both
  are present.
- **Dead code removed**: `useAdminState` composable (zero consumers).

## Deferred (deliberate)

- `isRepetitionDominated` / `FilterStreamingMarkup` incrementalization — correctness
  risk across chunk boundaries; bounded cost.
- `Store.Get` deepCopy, `FormatCollectedEvents`, heartbeat log level — minor, unchanged.
- Flaky `TestBroadcastCriticalEventDeliveredWhenFull` — unchanged (test-only).

## Open product decision — session checkpoint throttling (DECISION NEEDED)

**Context.** The conversation-service observer (`conversation_service.go` `buildObserver`)
writes the FULL session file on every `tool_result` and `message` event: `buildPartialHistory`
(re-walks all collected events) + `WriteSession` (marshal + atomic disk write). Long runs
with many tool cycles do many full serializations of a session that can reach MBs (tool
results are stored in full). The compact-marshal half is fixed; the write FREQUENCY is not.

**Why it's a product call, not a bug fix.** The reload contract is documented as
"unconditional, no throttling" (`session-source-backend-driven.md`) and is asserted by
`TestBuildObserver_CheckpointsEveryToolResult`. Coalescing checkpoints means a page refresh
mid-run can show state up to one tool cycle (≤ ~1s) stale.

**Options.**
- **A — keep unconditional (status quo).** Correctness-neutral, maximum reload fidelity;
  pays N full writes per run. Fine if measured session-write cost is small.
- **B — debounce checkpoints (e.g. ≤1 per 500ms), final save stays unconditional.**
  Cuts writes ~N-fold on long runs; mid-run refresh may miss the very latest tool cycle
  (the final `handleSuccessResult`/`handleCancelResult`/`handleErrorResult` write always
  captures everything). Requires updating the documented contract + the test.
- **C — debounce + always persist the latest within the window** (pending-save flag).
  Same staleness bound as B with a guaranteed flush of the newest state on completion.

**Suggested criteria:** measure `WriteSession` cost on a representative long automation run
(per-cycle marshal+write time, session file size). If per-cycle cost is small (< ~1ms), A is
fine; if it shows up in profiling, pick B (simplest) and update contract + test together.

## Tool-call dialect salvage (2026-08-28, post-review follow-up)

**Observed.** Qwen3.6-35B-A3B-UD-Q4_K_M.gguf (4-bit quant, XML tool mode) repeatedly emitted
`{"list_directory", "path": "."}` inside `<tool_call>` tags — an object whose first member is
a **keyless bare string** (the tool name), which is never valid JSON. Two attempts were
rejected + nudged before the model produced valid XML; the recovery ladder worked but burned
2 extra LLM calls.

**Verdict.** Prompt and parser are correct (`UnifiedToolManual` shows `{"tool": ..., "args": {...}}`;
the model got a translated, specific error hint both times) — this is model non-compliance,
not a code bug. Hermes Agent's `_repair_tool_call_arguments`
(`hermes-agent/agent/message_sanitization.py`) follows the same philosophy: a **lenient repair
cascade + WARNING log + proceed**, rather than reject-and-retry (Hermes targets the ARGUMENTS
payload; it uses native structured tool_calls so the envelope dialect never occurs there).

**Shipped fix** (`internal/core/proxy/tool_call_parser.go`): `salvageKeylessToolCall` repairs
the unambiguous `{"NAME", <rest>}` shape (rest wrapped in braces when not already an object)
**only after strict decoding fails**, and only when the remainder is itself valid JSON — the
tool name is still validated against the available tools downstream (`ValidateToolCall`), so a
wrong-name synthesis falls back to the existing recovery. Each repair is WARN-logged
(Hermes-style). Tests: both rest variants, multiple args, garbage-rest rejection, and
valid-JSON non-trigger.

**Refactor in the same area:** the two parallel parse loops (`ParseContentToolCalls` /
`ParseNativeToolCalls`) were unified into a shared `parseToolCallBlocks` runner (parse fn +
first-error retention + block stripping), and `parseSingleToolCall` now composes a strict
`decodeToolCall` with the salvage — no behavior change for valid or previously-rejected input
(all 17 existing parser tests unchanged + 5 new).

**CONSTITUTION note.** II.4 prefers "rejected formats get specific feedback" over silent
acceptance; the salvage is a deliberate, documented exception for one unambiguous dialect
where feedback already failed — matching Hermes' repair-and-proceed precedent.

## UI follow-ups from the 2026-08-28 runs (frontend)

1. **Raw JSON flash in the bubble = premature finalize bug (fixed).** Malformed
   `<tool_call>` attempts arrive as `message` events with the full markup in `content`
   (the backend cuts it from the live *stream* via `FilterStreamingMarkup` but not from
   the payload). The frontend's content-based finalize heuristic
   (`messageBuilder.ts` `message` case — "content + no tool_calls = final answer")
   treated the failed attempt as the answer: it rendered the raw JSON in the result
   area, set `phase='done'` (terminal), and suppressed the real final answer. **Fixed:**
   a content-only message whose content contains tool-call markup (`<tool_call`,
   `[TOOL_CALLS]`) is never finalized on. Tests added.
2. **Collapse/expand flicker during streaming (fixed).** The `.bubble-inset` was
   `v-if`'d — collapsing unmounted it, and expanding mid-stream re-mounted +
   re-parsed the live markdown (flash). Now `v-show` (kept mounted, `display:none`
   when hidden): no re-mount on expand, empty-panel suppression preserved (no paint
   while hidden). Trade-off: a hidden inset still re-renders live content per flush
   (bounded CPU; zero paint). Tests updated to assert hidden-not-absent.

## Measurement A/B flags (frontend, temporary — REMOVED after the pass)

To attribute the residual during-run GPU (Firefox GPU helper ≈15%, WindowServer ≈8–10%,
gauge ≈21% — confirmed all UI; llama.cpp is remote), a dev-only flag kit was added in
`frontend/src/utils/perf/perfFlags.ts`, wired at `main.ts` and gated in the hot paths.
**Deliberately no `requestAnimationFrame` instrumentation** — a page-side rAF loop
sustains phantom GPU (the removed `?perf=1` confound). Flags were read once at module
load with zero behavior change without a query param, and were **removed after the pass
concluded** (the loader was identified as the dominant cost; Activity Monitor alone
suffices for any re-check). The flag set was:

| Flag | Isolates | Caveat |
|------|----------|--------|
| `?noscroll=1` | both programmatic scroll paths (outer `maybeScroll` + inset watcher) | pane won't follow content while streaming |
| `?noloader=1` | arc-orbit loader animation (bubble + input) | loader stays inert |
| `?noliveflush=1` | per-flush live text re-raster | also stops the liveReasoning-driven scroll watchers — pair with `?noscroll=1` to separate |
| `?longtask=1` | main-thread side (passive `PerformanceObserver('longtask')`, zero GPU) | logs `>50ms` tasks to console; idle main thread + high GPU = compositor/raster |

Interpretation: drop with `noscroll` → scrolling is the residual; drop with `noliveflush`
(and `noscroll` held) → text re-raster; drop with `noloader` → the loader repaint.
Removable once the pass is done.

**Results (2026-08-28, peaks):** baseline ~21%; `?noloader=1` **9.9%** (≈ idle 8) →
the arc-orbit animation was the DOMINANT residual (~11 points) — the loader animation was
removed (static ring). `?noliveflush=1` 22% and `?longtask=1` 22.2% ≈ baseline → text
re-raster and main-thread are negligible. `?noscroll=1` 31% was inconsistent (loader
confounded that run; likely variance) — re-run `?noscroll=1&?noloader=1` to isolate the
scroll-only contribution if needed, but it is clearly a minor term.

## Upstream-outage run failure — root cause + fixes (2026-08-28, post-review)

**Symptom.** An assistant run "stopped, not finished": the UI stuck, no final answer. The
remote llama-server was DOWN (`connection refused` ×6 in the app log) because the remote
llm-proxy's model idle reaper had stopped it (`Idle timeout on model … → stopping`). The run
burned its retries and the backend salvaged the truncated `<tool_call>` stream content as the
"completed" answer; the frontend refused to show it (markup guard) and had NO other way to
finalize → stuck UI.

**Operational root cause (remote):** the remote llm-proxy manages llama-server's lifecycle
(`internal/core/llm/lifecycle.go` reaper, `idle_timeout_seconds`, default 1800) but the Mac
connects to llama-server DIRECTLY (`:8081`), so its traffic never keeps the model warm and
doesn't reliably trigger the remote's auto-restart. **Fix on the remote:**
`settings.yml → server: idle_timeout_seconds: -1` (-1 = never reap; `lifecycle.go:115-116` treats
any <= 0 as never-reap. NOTE: 0 does NOT survive a reload — the defaults-merge treats 0 as
"unset" and rewrites it to the default 1800; -1 does survive), or
run llama-server independently of the remote llm-proxy.

**Code fixes shipped:**
1. **Backend — `bestAvailableAnswer` now skips tool-call markup** (`session.go`): the salvage
   could complete a run with a truncated `<tool_call>` attempt as the "best available answer"
   (exactly what the outage produced). Markup content is skipped; if nothing else exists the
   finalization now falls through to the error path (visible failure) instead of garbage.
   `checkTaskCompletion` already guarded markup; `lastNonEmptyAssistantContent` (dead, no
   callers) removed.
2. **Frontend — lifecycle-completed finalize was silently broken** (`messageBuilder.ts`): the
   lifecycle case pre-set `finalized = true` before calling `finalize()`, but `finalize()`
   early-returns `if (finalized)` — so the lifecycle completion path NEVER ran. The message
   path was masking it; once the markup guard blocked the garbage message, the UI had no
   finalize at all (stuck run). Pre-set removed; `finalize` sets the flag itself.
3. **Frontend — markup guard extended to `lifecycle.completed`**: a completed lifecycle whose
   content is tool-call markup falls back to the last clean reply instead of rendering raw
   JSON (defense-in-depth alongside the backend fix).

## Proxy-as-gateway for remote llama.cpp (option 2 — implemented 2026-08-28)

**Goal.** Let the Mac's "openai" provider point at the REMOTE llm-proxy (`:4001`) instead of
llama-server directly (`:8081`), so the remote owns metadata + routing + model lifecycle, and
the idle reaper measures real use (`RecordActivity` fires per proxied request →
`LastUsed` updates → 30-min idle works as designed; the "model died mid-use" failure mode
disappears).

**What was added (shared codebase; deploy to the remote):**
- `GET /v1/models` on the proxy (`ProxyHandlers.ModelsListHandler` + route) serving an
  OpenAI-compatible listing shaped like llama.cpp's, so the Mac's openai provider parses it
  unchanged:
  - `owned_by: "llamacpp"` + `meta.n_ctx_train` → keeps the client's local-workload
    fingerprint (`listingServesLocalWorkload`).
  - `meta.n_ctx` / `context_length` = the **real serving window** (from the model's launch
    `--ctx-size`, or `Metadata.Nctx` when a `/slots` probe recorded it) — the client's budget
    keys on serving context, not `n_ctx_train`. The client's `/slots` probe on the proxy 404s
    harmlessly (probe failure → listing values stand).
  - `max_tokens` / `max_completion_tokens` = the model's output cap.
  - `id` = registry name — the same name the chat proxy routes on.
- Chat routing already existed (`EnsureModelProxyHandler`: name → `EnsureModel` starts the
  local model → reverse-proxy → llama-server; `RecordActivity` keeps the reaper honest).

**Deployment/config (the user's steps):**
1. Rebuild + restart the REMOTE llm-proxy with this change (it now serves `/v1/models`).
2. On the Mac, for each openai-provider model that should use the remote:
   - `base_url` → `http://192.168.50.60:4001/v1` (the proxy; `:4001` not `:8081`).
   - `name` → must equal the remote's registry name (e.g. `Qwen3.6-35B-A3B`, NOT
     `Qwen3.6-35B-A3B-UD-Q4_K_M.gguf`) so `/v1/chat/completions` routes correctly.
3. **Idle timeout is now editable in the UI**: Settings → Local Engine Configuration →
   "Model Lifecycle" → Idle Timeout (seconds). With traffic flowing through the proxy it now
   means "N seconds after last use" (not "after load"); `-1` keeps the model loaded
   indefinitely (the only "never" value that survives a reload — `0` is clobbered by the
   defaults-merge). Backend: `SystemUpdatePayload.IdleTimeoutSecs` is now `*int` so an
   explicit `0`/`-1` can be sent (`ApplySystemUpdate` applies any explicit value).

**Why not the alternatives:** systemd-managed llama-server (option 3) works but keeps two
entry points; nginx keepalive hacks don't fix lifecycle; `idle_timeout_seconds: 0` (option 1)
keeps the model pinned in VRAM forever. The gateway is the single-source-of-truth shape.

**Tests:** `TestModelsListHandler_ServesMetadata` (serving ctx from args, fallback to
training ctx, llama.cpp fingerprint), `TestServingCtxFromArgs`.

## HTTP errors on remote llama.cpp — non-streaming fallback timeout (2026-08-29, post-review)

**Symptom.** An assistant run with Ornith-1.5-35B against the remote llama-server
(`192.168.50.60:8084`) failed with three consecutive
`Post ".../v1/chat/completions": net/http: timeout awaiting response headers`
(45s / 90s / 135s elapsed, `err_class timeout`), then hung in `fallback_waiting` until the
backend was stopped.

**Trigger chain (from run events + remote llama-server log):**
1. Step 2 of the run entered a **repetition loop** (`stuck_detected`, reason
   `content_repetition`, 410 chars) — the repetition guard fired as designed.
2. Stuck recovery → `handleEmptyStream` → `computeNextResponseNonStreaming` → a
   **non-streaming** `Chat` request. llama-server only sends response headers for a
   non-streaming request after the FULL generation completes.
3. The remote log shows why it exceeded the client timeout: task 78 generated the full
   `max_tokens` budget (**2730 tokens @ 26.5 t/s = 103 s** of eval) — the model was still
   looping and ran to the cap.

**Root cause (code):** the bootstrap client factory (`internal/app/bootstrap.go`)
selected the transport by endpoint host only (`WorkloadClassifier.ClassifyEndpoint`).
`192.168.50.60` is not the Mac's interface IP, not loopback, and does not match
`model_host: 0.0.0.0` → classified **cloud** → `CloudLLMChatTransport` with a **45s
response-header timeout**. The manager already classifies the same model **local** (the
registry name ends in `.gguf`). The two classifiers disagreed; the client got the 45s cloud
timeout and every non-streaming request longer than ~45s failed. (Streaming never noticed:
headers arrive immediately.)

**Fix shipped (2026-08-29):** added `WorkloadClassifier.ClassifyClient(baseURL, modelID)`
(`backend/models/workload.go`) — local when the endpoint is local **or** the model id names
a `.gguf` artifact — and the bootstrap factory now uses it. A remote llama.cpp serving a
`.gguf` model gets `NewLLMClientForLocal` (10-minute header timeout + `thinking_budget_tokens`),
matching SPEC-005's rule that a remote llama.cpp serving GGUF is a local workload
(`docs/SPECS/orchestrator.md`). The `/slots` discovery probe (`isEffectiveLocal`) intentionally
keeps the host-only check — it gates a 5s metadata probe, not the inference transport.

**Tests:** `backend/models/workload_test.go` → `TestClassifyClient` (loopback, local
interface IP, remote non-gguf → cloud, remote `.gguf` → local, case-insensitive suffix,
empty inputs).

**Operational note (user config):** the repetition loop itself is likely aggravated by the
llama-server flag change `--cache-type-k f16 → q8_0` (quantized K-cache can nudge long-context
coherence into loops). The HTTP errors are NOT caused by the flags — any non-streaming
fallback >45s hits the old classification bug regardless. If Ornith keeps looping, revert the
K-cache to `f16`; the timeout fix is required either way.

## Verification

- Backend: `go build ./...`, `go vet ./...`, `go test ./...`, `go run ./tools/check-complexity/` — all green (one pre-existing unrelated flake noted above).
- Frontend: `npm test` (67), `npm run lint`, `npm run build` — all green.
