---
status: reference
date: 2026-09-30
last_reviewed: 2026-09-30
---

# Platform Scan — Security, Memory, Performance, Assistant/Automation UI (2026-09-30)

A read-only scan of the whole repo to decide what to plan next. It produced six plans (listed at the
end). This document is the **evidence ledger**: every claim the plans rely on, labelled so a later
session does not have to re-derive it or trust a stale doc.

**Evidence labels**

- **VERIFIED** — opened in the code on 2026-09-30; `file:line` given.
- **FROM-DOC** — stated by a repo doc, not re-checked against code in this scan.
- **ASSUMPTION** — inferred; the plan that uses it has a step that confirms or kills it *first*.
- **STALE** — a doc says one thing, the code says another.

Nothing here was changed in code. Line numbers drift; re-`grep` the symbol before editing.

---

## 0. How to read the history (what we already tried, so it is not retried)

| Topic | Tried | Outcome | Source |
|---|---|---|---|
| Making a 4B model skip work memory already knows | system-prompt `<relevant_memories>`; `MemoryCheckGate` nag; assistant prefill; LLM task rewriter; hard-coded skip directives | only the rewriter worked, but it added **77–89 s** to every run and was removed; prefill/nag are read but lose to an explicit "run X" (instruction hierarchy); hard-coded directives rejected as unmaintainable | `docs/audits/memory-injection-investigation.md`, `docs/audits/remove-memory-rewriter.md` |
| Per-step memory annotation | FTS5 per task line, 46 annotations | ~4.6 kB of noise tripped the sieve on turn 1 and made the model repeat steps; capped at 5 + word-overlap filter | same, "Updates 2026-06-03" |
| Memory injection for automations | injected on every turn | disabled; completion tool had been silently dropped by a trailing comma in `manifests/system.json` (real cause of the `notify_user` mis-selection) | same, "Root Cause (June 7)" |
| Context budget for local models | `context_budget` from `ctx×2` chars/token | wrong ratio made the sieve fire at ~1/3 of the window; now `(ctx − max_tokens) × 4` | SPEC-005 §II.3 fn 1; `docs/audits/2026-08-30-llm-smoke-test-incomplete-run.md` |
| GPU/UI cost while streaming | backdrop blur, pulse animations, deep watchers, plain-text reasoning | the arc-orbit loader animation was the dominant cost (~11 points); removed. Re-measure before touching rendering again | `docs/PLANS/gpu-performance.md`, `docs/audits/gpu-performance-audit.md` |
| Sandboxing | Wazero/WASM (removed 2026-05), Docker (rejected: macOS bind-mount I/O 5–20× slower), sandbox-exec CLI (deprecated), per-use-case profiles (rejected) | shipped: network default-off + egress proxy + Landlock (Linux) + dedicated-user deployment | `docs/PLANS/cross-cutting/agent-os-sandboxing.md` §0, §2 |
| Model eviction by inbound `/v1` callers | swap freely | killed live runs; replaced by the residency gate (429 + `X-LLM-Status`) | SPEC-007 §V.1 |
| Design language | glass/shadows; dotted-relay mark; square-in-square glyphs; corner crosshairs | all rejected by the user; **no shadows** except the primary button offset; nothing may trace to reference products | `frontend-redesign-retro.md` "Design language", SPEC-003 §IV |

---

## 1. Security findings

| ID | Finding | Evidence | Label |
|---|---|---|---|
| S1 | **No authentication or authorization anywhere.** The router has no middleware beyond panic recovery; the ~100 routes are registered bare; the server binds `0.0.0.0:<port>` unless `Server.Bind` is set. | `internal/transport/http/router.go` (only `recoverHandler`); `internal/app/routes.go` (routes); `internal/app/app.go:92-98` (`ResolveBindAddr` → `AddrAllInterfaces`); `docs/api-reference.md:5`, `:219` | VERIFIED |
| S2 | **What an unauthenticated LAN client can do.** Lead case: `POST /admin/api/conversation/message` starts an agent run, and the agent has a shell tool (subject to guardrails and the OS jail). Also: kill any PID the service user may signal; turn off the host sandbox switches; read/write/delete workspace files; change the registry and provider base URLs; factory-reset / wipe / restart. A further chain, read but not executed: `PUT /admin/api/config` persists `llama_server_binary` (`app_context_system.go:171`, the UI form edits it) and starting a model `exec`s it (`providers/local_provider.go:176`). | `routes.go:157-196` (route list); `handlers/process_handlers.go:291-314` + `platform/process/process.go:51` (`Kill(pid)` has no ownership check); `local_provider.go:176` | VERIFIED that the routes are open and the kill path has no ownership check; the binary chain is VERIFIED BY READING (not executed); "agent shell tool is available" is FROM-DOC (SPEC-006) |
| S3 | **Browser-origin exposure — includes destructive one-click CSRF.** No `Host`/`Origin` validation → DNS rebinding defeats same-origin. `DecodeJSON` requires `Content-Type: application/json`, so cross-site *JSON* POSTs need a preflight and fail. But these handlers read **no body**, so a plain cross-site `<form method=POST>` (a CORS "simple request") triggers them: `POST /admin/api/system/wipeout` (removes the data root, workspaces and secrets, then stops the process), `/system/factory-reset`, `/system/clear-runtime-data`, `/system/restart`, `/stop`, `/runtime/processes/{pid}/stop`. `POST /admin/api/start` decodes JSON only when the content type is exactly `application/json` and otherwise takes `?name=` from the query, so it is also reachable. Cross-site SSE is readable because the workspace stream answers `Access-Control-Allow-Origin: *`. | `transport/http/helpers.go:DecodeJSON`; `handlers/system_handlers.go:132-137, 180-222` (no body read; method check only); `handlers/process_handlers.go:48-100`; `handlers/dispatcher_handlers.go:354` (`ACAO: *`); no `Origin`/`Host` handling anywhere (`grep` over `internal/transport internal/app main.go`) | VERIFIED |
| S4 | **Webhook token is optional.** With no `webhook_token` in the connector settings the endpoint is unauthenticated (only a per-connector rate limit); inbound messages can start agent runs. | `handlers/webhook_handlers.go:66-76` | VERIFIED |
| S5 | **Key exfiltration via base URL.** A credential can carry a `base_url` override; the proxy sends the real key as `Authorization: Bearer` to that host. With S1 an attacker could point a credential at their server and call "test connection". | architecture.md ("per-credential `base_url` overrides"); `providers/provider_openai_compatible.go:341-361` (header built from the hydrated key) | ASSUMPTION — SEC plan step 1 writes the test that proves or kills it |
| S6 | **Agent network guard lets `0.0.0.0` / `::` through.** The socket-level guard (`NewNetworkTools` `DialContext`, `network.go:49-82`) resolves the host, runs `validateIP` on every address, and **fails closed on DNS errors** — so the `nil` that `validateAddress` returns on DNS failure (`network.go:235-239`) is *not* a bypass for the HTTP path. But `validateIP` (`network.go:250-272`) rejects only loopback and link-local; an unspecified address (`0.0.0.0`, `::`) is neither loopback nor private, so it is classed as "internet" and dialled — on Linux/macOS that connects to the local host. With internet access on, `fetch_url http://0.0.0.0:<port>/admin/api/...` reaches the unauthenticated admin API (chains with S1/S2). Multicast is also unchecked. | `core/tools/network.go:49-82, 225-272` | VERIFIED by reading; **not executed** — SANDBOX plan step 1 writes the failing test |
| S6b | **A network-on shell reaches loopback.** Landlock TCP-deny applies only to network-off children; a `networkOn=true` shell (and any `curl`) can call `127.0.0.1:<port>`, i.e. the admin API. So "loopback is trusted" is false for this product. | SPEC-006 §II.7.5, `agent-os-sandboxing.md` §6 risk 2 | FROM-DOC |
| S7 | **`freePort` kills whatever listens on the model port**, including processes we do not own (`fuser -k` / `lsof -ti :P \| xargs kill -9`). | `providers/local_provider.go:140-152` | VERIFIED; same class as the repo memory "verify process ownership before killing" |
| S8 | `master.key` is co-located with `secrets.json` (accepted in Constitution §III.6). Matters only because of S1/S2. | `CONSTITUTION.md` §III.6 | FROM-DOC |
| S9 | **Doc mismatch:** SPEC-007 §V.1 says the operator cancel is "behind admin auth" and SPEC-003 §I says the UI has "no account model" — the first describes something that does not exist. | `docs/SPECS/automation-dispatcher.md` §V.1 (cancel bullet), `docs/SPECS/discovery-panel.md` §I | STALE |
| S10 | Sandbox residuals carried over: macOS Seatbelt not built; per-run proxy scope (only a per-process token); a `networkOn` shell reaches LAN and internet; macOS cannot cap memory; MCP servers out of scope; complexity-debt baseline of 69 functions. | `agent-os-sandboxing.md` §6, §10 "Residuals" | FROM-DOC |
| S12 | **STALE:** the sandbox plan says the egress audit AST test was added at `internal/platform/network/egress_audit_test.go`; that directory holds only `address`, `ip` and `transport` files. The "zero raw clients in the binary is grep-enforced" claim needs its test located or re-created. | `ls backend/internal/platform/network` (2026-09-30); `agent-os-sandboxing.md` §10 slice 1a | VERIFIED (file absent at that path) |
| S11 | Asked-for topic "sandbox": the agent sandbox is substantially built (SPEC-006 §II.7, Landlock, egress proxy, env allowlist, `.sandbox` invisibility). The *gap* is the control plane in front of it (S1–S4), not the jail. | `backend/internal/platform/sandbox/` (Landlock provider, runner, profile), `backend/internal/platform/sizewatch/`, `sandbox.New(...)` wired at `internal/core/assistant/registry.go:403`; SPEC-006, sandboxing plan §10 | VERIFIED that the packages and wiring exist (2026-09-30); enforcement itself FROM-DOC + S1/S2 |

## 2. Memory findings

| ID | Finding | Evidence | Label |
|---|---|---|---|
| M1 | **Hot memory is probably visible to the model on the first request only.** `injectActiveMemory` inserts a `<memory>` message into the transient `prepared` slice, then sets a one-shot flag. `prepared` is rebuilt from `history` each turn and `history` is not modified, so later turns do not carry it. SPEC-004 §II.4 and the skill describe "once per session" without saying the block then disappears. | `core/assistant/stream.go:153-223`; flag `session.go:133`, `agent.go:277-288` | VERIFIED by reading; **confirm with a test** (MEM plan step 1) |
| M2 | Injection cap is a fixed **2000 chars** regardless of context window, ordered by `updated_at DESC`, truncated on entry boundaries with no signal that entries were dropped. | `stream.go:39`, `:225-237`; skill query ordering | VERIFIED |
| M3 | Automations get **no** hot memory and no pre-sieve flush nudge (both gated on `EnableHotMemory`, set only by the assistant path). The audit's three fixes (step-aware query, early placement, relevance filter) were never built. | `conversation_service.go:232` (only `WithHotMemory(true)`); `session.go:1373`; `memory-injection-investigation.md` "What would fix" | VERIFIED + FROM-DOC |
| M4 | **The sieve forgets by deleting.** Over budget it truncates mid-history messages to 4000/2000 chars, then drops everything between the 3-message head and the 10-message tail and inserts a fixed note. Nothing records *what* was dropped (completed steps, files written). Audit: this is what made the smoke test repeat steps. | `assistant/sieve.go:40-83`, constants `:130-137` | VERIFIED |
| M5 | **STALE plan:** `memory-improvements-implementation-plan.md` marks Task 1.2 (usage meter) DONE, but `WorkspaceCharCount` has no non-test caller and no `[memory store:` string exists; `MemoryProactiveNudge` was removed on purpose. `SoftMemoryCharLimit` (4000) is also unused and disagrees with the 2000 injection cap. | `grep` of `internal/`: only `templates.go:413`, `store.go:285` and its test | VERIFIED |
| M6 | `session_search` (FTS5 over past conversations) and the procedural **skills** store are designed but never started. | `docs/PLANS/README.md` "Remaining Work"; no `internal/platform/sessionsearch` | FROM-DOC + absence |
| M7 | `injectActiveMemory` calls `SearchHot(context.Background(), …)` — a DB read with a detached context (Constitution IV.1 wants the caller's ctx). Low severity. | `stream.go:197` | VERIFIED |
| M8 | Hard constraints: no synchronous LLM call on the run path (rewriter: 77–89 s cold); small models obey the most specific imperative, so memory must change what the model *sees*, not ask it to behave. | §0 above | FROM-DOC |
| M9 | The memory UI lists, filters by type, full-text searches, opens, deletes and clears entries. It does not show what is injected, what it costs in tokens, or let the operator mark an entry hot. | `frontend/.../memory/MemoryPanel.vue` (read); `MemoryDetail.vue` not reviewed | VERIFIED (panel) / ASSUMPTION (detail) |

## 3. Performance and leak findings

| ID | Finding | Evidence | Label |
|---|---|---|---|
| P1 | `ChatRequest` has no `cache_prompt` / `id_slot` field, so KV-cache reuse depends on the llama.cpp server default and on the prompt prefix staying byte-stable. The physical sieve mutates mid-history messages in place (`sieve.go:54-56`) and inserts the memory block before the last user message — each is a prefix change that forces re-processing. Local prefill is the dominant per-turn cost (audit: 500–650 tok/s cold, 200–350 warm; generation ~21 tok/s). | `models/llm_messages.go:57-74`; `sieve.go`; `llamacpp-setup` skill | VERIFIED (absence of fields) / ASSUMPTION (server default) |
| P2 | **Session checkpoint is O(n²) per run.** The observer rewrites the whole session on every `tool_result`/`message`, and each rewrite re-walks *all* collected events via `buildPartialHistory`; `collectedEvents` is unbounded for the run. The undecided product call (keep / debounce / debounce+flush) is recorded in the ops review. | `core/assistant/conversation_service.go:189-221`; `docs/audits/2026-08-28-ops-performance-review.md` "Open product decision" | VERIFIED |
| P3 | `time.AfterFunc` allocated per SSE line. | `core/proxy/client.go:725` | VERIFIED (still open from backend-audit §2.3) |
| P4 | `NormalizeHistory` copies the whole history (then merge and `SanitizeHistory` copy again) once per turn. | `core/proxy/history.go:22-60` | VERIFIED (first copy); rest FROM-DOC |
| P5 | `ListSessions` opens and decodes **every** session file to build snippets; the assistant list and search use it. | `platform/persistence/workspace.go:662-694` | VERIFIED |
| P6 | Workspace state is re-read from disk per entry in several dispatcher paths (N+1 pattern from the July audit, moved from the handler into the automation package). | `core/automation/execution.go:133,145`, `history.go:58`, `scheduling.go:198` | VERIFIED (sites); cost UNMEASURED |
| P7 | Per-chunk O(n) scans in `processStream` (markup filter, repetition window, regex) — deferred on purpose, bounded by the 10–11 kB char cap and 90 s stream cap. | ops review finding 6 | FROM-DOC |
| P8 | **Fixed since the July audits** (do not re-plan): SSE one-byte reader; `Chat()` body-close goroutine (no detached `<-ctx.Done()` goroutine remains in `client.go`); guardrail decision `retained` map is bounded (100); `StopAutomation` diagnostic goroutine is cancellable; log rotation; event-bus byte budget; host-metrics cache; `transport`-layer god handlers split. | `client.go` (only select-based waits), `agent.go:351-386`, `automation/execution.go:258-266` | VERIFIED |
| P9 | Remote llama.cpp serving a `.gguf` is classified local (fixed 2026-08-29); cloud chat uses HTTP/1.1 and a 45 s header timeout (NVIDIA HTTP/2 incident). Slow cloud runs were provider TTFT, not local logic. | architecture.md pitfalls 27/28/33; `known-performance-findings.md` | FROM-DOC |
| P10 | **No leak/soak tooling.** Leak work so far is by reading code (PL-1…PL-7, Phase 6 audit). `/admin/api/metrics` does not expose goroutine count or heap. | unattended-run-safety Step 0; frontend-redesign Phase 6 | FROM-DOC + absence (metrics fields not re-read) |
| P11 | Open, already planned elsewhere: unattended-run-safety Steps 6–9; GPU plan P1–P5 (clean re-measure first); Tool-Call Grammar re-enable (proposed). | `docs/PLANS/README.md` Remaining Work | FROM-DOC |
| P12 | Constitution §VI and SPEC-005's intro say "slot persistence" is defined in SPEC-005; the SPEC text has no such section. | `CONSTITUTION.md` §VI vs `docs/SPECS/orchestrator.md` | STALE |

## 4. Assistant / automation UI findings

| ID | Finding | Evidence | Label |
|---|---|---|---|
| U1 | The assistant is a fixed-height box (`h-[calc(100vh-10rem)]`) that **replaces** the files view; you cannot watch the editor or tree while the agent works on those files. | `views/WorkspacesView.vue:349-394` | VERIFIED |
| U2 | Automation detail stacks Configuration → Last result → Console (capped at 60 vh) → Runs in one column; a past run opens in a drawer; the live console and the run history are never side by side; no run-to-run comparison. | `components/AgentIde/automation/AutomationDetails.vue:131-220`, `views/AutomationsView.vue:346-354` | VERIFIED |
| U3 | Good foundation to keep: assistant and automation already share `ChatMessages` (`mode="automation"`) and `useMessageBuilder`; the status strip states real run state; session list has search/pins/time groups; mobile drawer works at 390 px. | SPEC-003, pitfall 25, overhaul plan Phase 4 | VERIFIED/FROM-DOC |
| U4 | The Monitor drawer is wired separately in both views. | `WorkspacesView.vue:418-429`, `AutomationsView.vue:356-364` | VERIFIED |
| U5 | Still open: backend SSE-bleed on cancel (overhaul Phase 5); follow-ups not taken by the redesign (usage ledger, composer selectors, terminal-status source, push channel, DOMPurify, theme export-all). | overhaul plan; redesign "Follow-ups not taken" | FROM-DOC |
| U6 | The UI contract is SPEC-003 v2.2. Layout changes that add a destination, route or polling source are SPEC-level changes. | SPEC-003 §II–III | FROM-DOC |
| U7 | **A finished automation run is drawn by a different renderer than a live one.** Live: `ChatMessages mode="automation"`. Finished: `ExecutionAuditTrail`, a "Terminal Log" box with emoji glyphs, used by `HistoricalRunDetails`. | `frontend/src/components/AgentIde/automation/HistoricalRunDetails.vue:86`, `ExecutionAuditTrail.vue` (`tool-icon` emoji) | VERIFIED |

---

## 5. Resulting plans

| Plan | Covers |
|---|---|
| `docs/PLANS/cross-cutting/admin-api-authentication.md` | S1–S5, S9 |
| `docs/PLANS/cross-cutting/sandbox-and-egress-residuals.md` | S6, S6b, S7, S10, S11, S12 |
| `docs/PLANS/memory/small-context-memory.md` | M1–M9 |
| `docs/PLANS/orchestrator/local-and-cloud-inference-performance.md` | P1, P9, P12 (local + cloud inference) |
| `docs/PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md` | P2–P8, P10, P11 |
| `docs/PLANS/assistant-ui/assistant-automation-workbench-layout.md` | U1–U7 |

Recommended order: **authentication first** (it gates everything that could be exposed), then the
**measurement phases** of both performance plans (cheap, and they decide whether later steps are
needed), then memory, then UI.
