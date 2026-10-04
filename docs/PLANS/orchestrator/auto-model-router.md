---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-005, SPEC-007, SPEC-003]
constitution_references: [II.2, IV.1, IV.2, IV.3, V.2, V.3, VI]
related_plans: [cost-and-savings-ledger.md, local-and-cloud-inference-performance.md, ../cross-cutting/cloud-privacy-firewall.md, ../cross-cutting/admin-api-authentication.md]
---

# `model: "auto"` — Runtime-Aware Model Router for `/v1`

**Status:** proposed — needs D1–D3 before Phase 2. Phase 0–1 change no behaviour.

## Why this plan exists

Every OpenAI-compatible client pointed at this proxy hard-codes one model name. The user must decide,
per tool and up front, "local or cloud?" — and that answer is wrong half the time: the local model is
cold, or busy with an automation, or the prompt does not fit its window; or a cloud model is used for a
one-line question the warm local model would answer in a second for free.

Hosted routers (OpenRouter auto, etc.) route on price and benchmark quality. **They cannot see the
user's machine.** This proxy can: it knows which local model is loaded, whether the single local slot
is taken, how big the local window is, and how long a cold start takes. That makes runtime-aware
routing a feature only a local control plane can offer.

| User benefit | Service benefit |
|---|---|
| Point every client at `model: "auto"` once; get the fastest suitable answer without picking. | Fewer cold starts and fewer evictions — routing *prefers* what is already resident (SPEC-007 §V.1 residency stays the law). |
| Long prompts that would overflow the local window go to cloud instead of failing with a 400. | Fewer `429 busy` answers to external callers when an automation holds the local slot. |
| Optional hints: `auto:fast`, `auto:cheap`, `auto:best`, `auto:local`. | A single routing decision point that the privacy firewall (`../cross-cutting/cloud-privacy-firewall.md`) and the cost ledger (`cost-and-savings-ledger.md`) can plug into. |
| Every response says which model answered (`X-LLM-Routed-Model`) — no magic. | Routing decisions are logged with their reason → measurable, tunable. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| `/v1/chat/completions` resolves the model from `X-Model-Name`, then body `model`, then the configured primary (`SelectModels`) | `backend/internal/transport/http/handlers/proxy_handlers.go:175-187` |
| The body is already read fully and restored to find the model (`peekModelName`) | `proxy_handlers.go:156-173` |
| External callers go through the inbound gate: a local-model request that would evict a model in use waits within its budget or gets `429` + `Retry-After` | `backend/internal/transport/http/handlers/inbound_gate.go:179-210`; SPEC-007 §V.1 |
| `EnsureModel` returns `ErrModelStarting` (202) or `ErrLocalModelBusy` (429) | `proxy_handlers.go:243-265` |
| The runtime exposes the active local model (`ActiveInfo`), model list, workload classification (`ClassifyModel`) and the primary/fallback pair | `backend/internal/transport/http/handlers/services.go:23-50` |
| Lane state is observable: per lane `Limit`, `Running`, `Waiting`, `Holders` | `backend/internal/core/runlane/types.go:94-107`; `GET /admin/api/active-runs` |
| Serving context window of local models is known (`Metadata.Nctx` or `-c` in args); cloud windows come from the provider catalog (`PublishedContextLength`) | `proxy_handlers.go:95-138`; `backend/models/config.go:588-590` |
| `/v1/models` lists configured models only; there is no virtual entry | `proxy_handlers.go:95-138` |
| Measured: local cold first prompt 6–7 s vs warm 0.3–0.6 s; NVIDIA free tier TTFT 17–40 s per call | `local-and-cloud-inference-performance.md` → Ground truth |

## Design

### Where it sits
A `Router` interface in a new package (`backend/internal/core/router/`), called from
`EnsureModelProxyHandler` **only** when the resolved model name is `auto` or `auto:<hint>`. Any other
name keeps today's path byte-for-byte. The router returns a concrete model name plus a reason; the
handler rewrites the body's `model` field to that name and continues through the existing inbound gate,
`EnsureModel` and reverse proxy. Residency, admission and the gate are **not** bypassed.

### Inputs (all already available, no new probes)
- request: estimated prompt tokens (chars / 4, the repo's ratio), `max_tokens`, presence of `tools`,
  presence of images, the hint;
- runtime: active local model and whether it is loaded/starting, local lane running/waiting,
  candidate list = user-configured **route set** (D1);
- per model: serving window, workload class, tool-calling support, price (from the ledger plan when
  available).

### Policy (deterministic, ordered, explainable)
1. **Filter** candidates that cannot serve the request: window < prompt + `max_tokens` + margin;
   no tool support when `tools` present; no vision when images present.
2. **Hint overrides:** `auto:local` → local only (fail with a clear 503 if none fits);
   `auto:best` → the user's ranked "best" list; `auto:cheap` → lowest price.
3. **Default (`auto`/`auto:fast`)**, in order:
   a. local model **already loaded and lane free** → use it;
   b. local loaded but lane busy → cloud candidate if one exists, else queue on local (today's behaviour);
   c. no local loaded → cloud if the user allows cloud for `auto` (D2), else cold-start the preferred
      local model.
4. **Fallback on terminal failure** of the chosen upstream (Constitution IV.3 — not on "starting"):
   retry once with the next candidate, only for non-streaming requests or before the first byte is sent.

Every decision logs `{chosen, reason, candidates_rejected[{model, why}]}` and sets
`X-LLM-Routed-Model` / `X-LLM-Route-Reason` response headers.

### `/v1/models`
Adds virtual entries `auto`, `auto:fast`, `auto:cheap`, `auto:best`, `auto:local` with the *smallest*
context window among their candidates, so clients that pre-check windows stay safe.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Candidate set: all configured models, or an explicit "route set" in Settings? | Explicit route set (opt-in per model). Avoids surprise spend on a model added for testing |
| D2 | May plain `auto` go to a **paid** cloud model without an explicit hint? | No by default — `auto` uses cloud only for models marked free or when the user enables "allow paid cloud for auto" |
| D3 | Expose `X-LLM-Route-Reason` to all callers or only admin? | All callers — it is the user's own proxy; revisit when `../cross-cutting/admin-api-authentication.md` lands |
| D4 | Mid-stream failover? | No. Only before the first byte (a half-answer from two models is worse than an error) |

## Phases

### Phase 0 — Characterise (no behaviour change)
- Tests pinning the current model-resolution order (header → body → primary) and the gate's 429 path.
- Confirm whether any configured entry's `Name` differs from the upstream model ID sent to the provider
  (decides whether the body rewrite uses `Name` or a separate upstream ID).
- **Acceptance:** `go test ./internal/transport/http/handlers/ -run Proxy -count=1` green; finding noted here.

### Phase 1 — Router core (pure, no wiring)
- `Router.Route(ctx, Request, RuntimeView) (Decision, error)` with the filter and policy above;
  `RuntimeView` is an interface so tests use fakes.
- **Acceptance:** table tests — window overflow → cloud; tools on a no-tools model → filtered;
  local loaded + lane free → local; local busy + cloud allowed → cloud; `auto:local` with nothing fitting
  → typed error. `go run ./tools/check-complexity/` (policy must stay ≤12 per function).

### Phase 2 — Wire into `/v1` (needs D1, D2)
- Hook in `EnsureModelProxyHandler`, body rewrite, response headers, `/v1/models` virtual entries,
  route-set setting in the model config (follow `docs/architecture.md` → File Change Checklist, "new model-level field").
- **Acceptance:** handler tests with `httptest` upstreams — `model: "gpt-x"` path unchanged (golden);
  `model: "auto"` routes and sets headers; gate still returns 429 when the only candidate is busy local.

### Phase 3 — Pre-first-byte failover (D4)
- **Acceptance:** upstream returning 500 before headers → second candidate answers; streaming request
  after first byte → error passed through unchanged.

### Phase 4 — UI + docs
- Settings: route set and "allow paid cloud for auto"; dashboard shows recent routing decisions.
- SPEC-005 (routing policy), SPEC-007 §V.1 note (router runs *before* the residency gate and never
  bypasses it), `docs/api-reference.md` (`auto` models, headers).
- **Acceptance:** `npm test && npm run build && npm run test:visual`; `./scripts/check-agent-harness.sh`.

### Phase 5 — Use the ledger (after `cost-and-savings-ledger.md` Phase 4)
- `auto:cheap` uses real prices; "saved by routing" appears in the ledger.

## Risks and non-goals
- **Surprise spend** — D1/D2 default to opt-in; every response names its model.
- **Inconsistent answers** across turns of one conversation (different models) — clients can pin with a
  normal model name; optionally add sticky routing per `user`/conversation header later.
- **Non-goal:** learned/ML routing or quality scoring. Deterministic rules first; shadow evaluation is a
  separate future idea.
- **Non-goal:** routing for the internal agent loop — it keeps its explicit primary/fallback selection.

## Remaining Work
All phases (0–5). Phase 2 blocked on D1, D2; Phase 5 on the ledger plan.
