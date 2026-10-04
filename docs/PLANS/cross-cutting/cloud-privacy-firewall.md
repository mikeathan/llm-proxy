---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-006, SPEC-005, SPEC-001, SPEC-003]
constitution_references: [II.8, II.13, III.6, IV.1, IV.2, V.1, V.2, V.3]
related_plans: [../orchestrator/auto-model-router.md, ../orchestrator/cost-and-savings-ledger.md, admin-api-authentication.md]
---

# Cloud Privacy Firewall (redact → send → restore, or keep it local)

**Status:** proposed — needs D1–D4 before Phase 2. Phase 0–1 change no behaviour.

## Why this plan exists

The promise of a local-first proxy is "my private data stays on my machine". Today that holds only if
the user remembers, for every request, to pick a local model. The moment a chat, an automation or an
external `/v1` client uses a cloud model, everything in the prompt — pasted API keys, emails, phone
numbers, IBANs, home-directory paths, the contents of files the agent read — leaves the machine
unchanged.

Redaction exists today only on **tool output** (three API-key regexes applied to terminal results).
Nothing inspects what is sent **to the model provider**.

| User benefit | Service benefit |
|---|---|
| Use strong cloud models on private material: secrets and personal data are replaced with placeholders before leaving, and put back in the answer — the user sees the real values, the provider never does. | A policy point that makes "local-first" an enforced property, not a habit. |
| Or choose "sensitive → stay local": requests that contain protected data are routed to a local model instead (with `../orchestrator/auto-model-router.md`). | One redaction engine for all three egress paths (chat, automations, `/v1`) instead of ad-hoc regexes. |
| An audit view: "what was protected, when, from which workspace" — counts and types, never the values. | Few local proxies do *reversible* redaction across streaming; a real differentiator. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| `SecretPatterns` = 3 regexes (`sk-…`, `AKIA…`, `AIza…`); `RedactSecrets` replaces with `[REDACTED]` (not reversible) | `backend/internal/core/tools/security.go:10-27` |
| It is applied to terminal tool output only (plus blocked-path line stripping) | `backend/internal/core/tools/terminal.go:473-514` |
| `storage.MaskKey` only masks API keys for display in Settings — not a content redactor | `backend/internal/platform/storage/masking.go:5-16` |
| All agent LLM traffic (chat + automations) goes through the `proxy.Client` interface (`Chat`, `Stream`) — a single decorator seam | `backend/internal/core/proxy/client.go:26-35` |
| External `/v1` traffic is a `httputil.ReverseProxy` pass-through; `isCloud` is known per request (`mi.URL != ""`) | `backend/internal/transport/http/handlers/proxy_handlers.go:200-238` |
| Workload class (local vs cloud) is resolved at the runtime boundary (`ModelConfig.WorkloadClass`, `ClassifyModel`) | `backend/models/config.go:582-586`; `backend/internal/transport/http/handlers/services.go:47-50` |
| Messages sent upstream keep `role`, `content`, `tool_calls`, `tool_call_id` (Constitution II.8) — the fields redaction must cover | `CONSTITUTION.md` §II.8 |
| Guardrail decisions already have an approval flow (pause + user decision) | `docs/SPECS/guardrails.md` §II.3 |

## Design

### Engine (new package `backend/internal/core/privacy/`)
- **Detectors** (each a small type with tests): API keys (reuse and extend `SecretPatterns` — move the
  list into the new package and keep `tools.RedactSecrets` delegating, so there is one source of truth),
  private keys/PEM blocks, JWTs, emails, phone numbers, IBAN (with checksum), credit cards (Luhn),
  IPv4/IPv6 private addresses, absolute home paths (`/home/<user>`, `/Users/<user>`), and **user
  terms** (names, company, project codenames) from Settings.
- **Placeholder vault per request:** each distinct value → a stable token like `⟦EMAIL_1⟧`. Same value
  → same token within the request (and within one conversation, so the model can reason about
  "EMAIL_1 sent to EMAIL_2"). The vault lives in memory for the request/run only and is never logged or
  persisted.
- **Restore:** placeholders in the model's output (content and tool-call arguments) are replaced back
  before the result reaches the user or a tool. For streams, a small hold-back buffer (max token length)
  keeps a placeholder split across chunks from leaking half-restored.

### Three enforcement points, one engine
| Path | Seam | Notes |
|---|---|---|
| Chat + automations | decorator around `proxy.Client` installed by the client provider when the target is cloud | Tool calls are restored *before* execution, so a tool writes the real email, not `⟦EMAIL_1⟧` |
| External `/v1` | request-body rewrite + response restore in `EnsureModelProxyHandler` when `isCloud` | JSON and SSE; never applied to local targets |
| Router hand-off | `Decision` input flag `sensitive=true` | With the router: `auto` + sensitive → local candidates only |

### Modes (per workspace, plus a global default and a `/v1` setting)
- `off` — today's behaviour.
- `redact` — redact + restore (default proposal for cloud).
- `local_only` — if anything is detected, refuse cloud: route local (router) or return a clear 451/422
  with the detected *types* (never values).
- `ask` — for interactive chat only: pause with the guardrail approval UI ("this message contains 2 emails
  and 1 API key; send redacted / send as is / cancel"). Unattended runs treat `ask` as `redact`.

### Audit
Per request: workspace, path (chat/automation/inbound), model, counts by detector type, mode applied.
Stored with the usage records of `../orchestrator/cost-and-savings-ledger.md` when that lands, otherwise
in the app log at INFO. Values are **never** stored.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Default mode for cloud traffic | `redact` for chat/automations; `off` for `/v1` until the user opts in (external clients may rely on exact text) |
| D2 | Detector set enabled by default | keys, PEM, JWT, emails, IBAN, cards, home paths. Phones and private IPs opt-in (false positives) |
| D3 | Placeholder format | `⟦TYPE_N⟧` — unusual brackets, rarely produced by models by accident, survives JSON |
| D4 | Hold-back buffer size for stream restore | longest possible placeholder (≈ 24 chars) — adds no visible latency |

## Phases

### Phase 0 — Characterise + corpus (no behaviour change)
- Build a test corpus (new `backend/internal/core/privacy/testdata/`) of positive and negative samples per
  detector, including tricky ones (UUIDs vs keys, version strings vs IPs, code with emails in comments).
- **Acceptance:** corpus checked in; a benchmark harness that reports precision/recall per detector.

### Phase 1 — Engine (pure)
- Detectors, vault, redact/restore including stream hold-back; move `SecretPatterns` here with
  `tools.RedactSecrets` delegating (behaviour unchanged — existing tool tests stay green).
- **Acceptance:** `go test ./internal/core/privacy/ ./internal/core/tools/ -count=1`; round-trip property
  test: `restore(redact(x)) == x` for every corpus sample; streaming test with placeholders split at every
  byte offset; precision ≥ 0.95 on the corpus for default-on detectors.

### Phase 2 — Agent path (needs D1–D3)
- `proxy.Client` decorator for cloud targets; restore tool-call arguments before execution; mode setting
  per workspace.
- **Acceptance:** agent-loop test with a mock cloud LLM that echoes its input — recorded request contains
  only placeholders; the tool receives real values; the user-visible answer contains real values.
  `go test -race ./internal/core/assistant/ ./internal/core/proxy/` (new stream wrapper).

### Phase 3 — `/v1` path
- Body rewrite + JSON/SSE restore in the proxy handler for cloud targets when enabled.
- **Acceptance:** `httptest` upstream that records bodies — no raw secret reaches it; client gets restored
  text; local targets byte-identical to today (golden test).

### Phase 4 — `ask` mode + UI + audit
- Reuse the guardrail approval flow for `ask`; Settings → Privacy tab (follow
  `docs/architecture.md#adding-a-frontend-settings-tab-checklist`); audit list view.
- **Acceptance:** `npm test && npm run build && npm run test:visual`; handler tests for settings.

### Phase 5 — Router integration (after `../orchestrator/auto-model-router.md` Phase 2)
- `local_only` + `auto` → local candidates only.

### Phase 6 — Docs
- SPEC-006 (new section: outbound content protection), SPEC-001 (decorator + tool-arg restore),
  SPEC-003 (Privacy tab), `docs/api-reference.md`. Consider a Constitution amendment (V.1 — security
  review) making "cloud egress passes the privacy engine" an invariant once proven.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **False negatives** — regex detectors miss things (a name in free text). The UI must say "reduces
  exposure", never "guarantees privacy". User terms cover the most important names.
- **False positives degrade answers** — e.g. an example email in code. Mitigated by per-detector toggles
  and by restore (the user still sees the original).
- **Models mangling placeholders** (`⟦EMAIL 1⟧`) — restore matches tolerant variants; unrestorable
  placeholders are counted in the audit so the problem is visible.
- **Non-goal:** ML/NER-based detection, file attachments/images, encrypting data at rest
  (already covered for secrets by Constitution III.6).

## Remaining Work
All phases (0–6). Phases 2–3 blocked on D1–D3; Phase 5 on the router plan.
