---
status: proposed
date: 2026-10-11
last_reviewed: 2026-10-11
related_specs: [SPEC-005, SPEC-001]
constitution_references: [VI, II.5]
related_plans: [../ARCHIVE/cross-cutting/cloud-provider-token-budgets.md]
---

# Decouple Cloud Safety Thresholds From `max_tokens`

**Status:** proposed — Phase 0 (measure) not started. No code changed by this plan yet.

## Why this plan exists

On 2026-10-11, fixing the cloud history-budget tier-ceiling bug (DeepSeek-v4.1-flash's real
1M-token published context being clamped to the generic "openai" tier's 50,000-char row) led to
also removing the equivalent ceiling on the cloud **output cap** (`max_tokens`). That second
change was reverted before shipping: `docs/PLANS/ARCHIVE/cross-cutting/cloud-provider-token-budgets.md`
§1.8/§2.5 shows `max_tokens` is not just a wire parameter — it also sizes three agent-loop safety
mechanisms (`backend/internal/core/assistant/stream.go`):

- stuck detection: `stuckThreshold() = max_tokens * stuckThresholdMultiplier(2)`, floored at
  `MinReasoningStuckThreshold` (2000 chars) — `stream.go` (`stuckThreshold`, `checkStreamStuck`);
  `agent.go:41`.
- stream content cap: `content > max_tokens * streamCharCapMultiplier(4)` — `stream.go`
  (`exceedsContentCharCap`).
- native-tools content cap (same `max_tokens`-scaled shape per the archived plan §1.8).

The archived plan measured a real incident: a cloud model given the (wrong, local) `ctx/3`
formula got `max_tokens=42666`, which made stuck detection "effectively dormant" against actual
reasoning of 55–559 characters per step. The fix was the per-provider 8192 tier ceiling cloud
models carry today. Removing that ceiling for a model with a large real published cap (anything
from "a 128K-output model" to a bad catalog read) would silently reopen that exact failure mode.
**Full write-up of this session's investigation:** harness memory
`project-cloud-defaults-and-test-verbosity-2026-10-11.md`.

The user's ask (verbatim): "the right values for each cloud provider model and not restrict
them" — specifically, DeepSeek-v4.1-flash's `max_tokens` is clamped to 8192 (the generic openai
tier) when the real model may support much more. This plan is how to do that safely: decouple the
safety thresholds from `max_tokens` first, so raising the cap for a model that genuinely supports
it doesn't also loosen stuck detection past where it's been measured to work.

## Scope — cloud only. Local is explicitly out.

Three independent reasons, not one:

1. **Self-consistency.** Local uses `ModeThinkTokens`: the reasoning budget AND `max_tokens` are
   both derived from the same `ctx/3` serving-context number (`LocalBudgetPolicy.Derive`,
   `budget_policy.go`). When stuck detection scales with `max_tokens`, it scales with the same
   quantity that mechanically bounds how much the model is allowed to think — the threshold and
   the real ceiling move together, so a big local `max_tokens` (e.g. 87,381, seen in production on
   vertex for Qwen3.6 35B A3B) is not "loose", it's proportionate.
2. **Cloud has no such mechanical link.** Cloud providers use `ModeEffort` / `ModeObject` /
   `ModeEnableThinking` (`assistant/reasoning/reasoning_param.go`) — reasoning depth is a
   qualitative knob (`reasoning_effort: medium`), never a token count. Letting cloud `max_tokens`
   grow loosens the safety threshold with **no** corresponding mechanical limit on how much the
   model actually reasons. This is the arbitrary coupling; local's is not.
3. **Verification asymmetry (user's point, 2026-10-11).** Local's `Metadata.Nctx` comes from
   directly probing the running server (`/slots` or `/v1/props`) or reading the GGUF header —
   verified ground truth. Cloud's `Metadata`/`PublishedContextLength` is whatever the aggregator's
   catalog response claims, with **no equivalent probe** to check it against (the characterization
   golden test literally labels some of these cases "inflated"). A decoupling plan for cloud can't
   just "trust the published number" the way local trusts its probed `n_ctx` — it needs its own
   credibility handling (Phase 1, D2 below).

`LocalBudgetPolicy`, `ResolveLocalContext`, and the local stuck-detection path are **not touched**
by this plan. Any step that would change local behavior is out of scope — if local's byte-identical
golden-test rows change, the step is wrong.

## Current state (verified 2026-10-11)

| Mechanism | Formula | Scales with | File |
|---|---|---|---|
| Output cap (cloud) | `min(published, tier)` — tier is 8192 for openai/gemini/openrouter/nvidia | nothing (hard ceiling) | `output_cap.go` `publishedOutputCap` |
| Stuck detection | `max_tokens * 2`, floor 2000 chars | `max_tokens` | `stream.go` `stuckThreshold` |
| Stream content cap | `max_tokens * 4` | `max_tokens` | `stream.go` `exceedsContentCharCap` |
| History budget (cloud) | tier ceiling only when context is unverified (`PublishedContextKnown` false) — **fixed 2026-10-11** | real published context when known | `budget_policy.go` `CloudBudgetPolicy.Derive` |
| Reasoning depth (cloud) | `reasoning_effort` string, no numeric budget | nothing tied to `max_tokens` | `reasoning_param.go` (`ModeEffort`/`ModeObject`/`ModeEnableThinking`) |

The history-budget row is done (this session, not part of this plan). This plan is the output-cap
row only.

## Phases

### Phase 0 — Measure (no behavior change)

De-risk the unknowns before designing a threshold. Do not design against guesses (`AGENTS.md`
Working Agreements — "Verify before you act").

0.1. For every cloud credential currently configured (vertex + local dev), record via the admin
   API (`/admin/api/registry`, `/admin/api/state`) what `Metadata.MaxOutputTokens` /
   `PublishedOutputCap` actually looks like today — how many models have real data vs. none.
   Verify: a short table in this plan, sourced from a live read, not a guess.
0.2. For at least one cloud model with real reasoning traces (recordings or processlogs), measure
   actual `reasoning_content` length per step under today's 8192 cap. This is the archived plan's
   §1.8 methodology repeated for a second data point — confirm 8192 is still a reasonable "real
   reasoning" ceiling, not just inherited.
0.3. Check whether `go-staff-engineer.md` / the registrar enforces any upper sanity bound today on
   a catalog-reported `max_output_tokens` (e.g. "reject anything above N"). If none exists, Phase 1
   needs to add one — a catalog claiming 1,000,000 output tokens must not be taken at face value
   per the verification-asymmetry point above.
   Verify: `rg -n "MaxOutputTokens|PublishedOutputCap" backend/internal/core/llm/` read, reported
   plainly as "no sanity bound exists" or "bound exists at X, file:line".

### Phase 1 — Design (decision doc, no code)

Produce a short decision record (append to this plan) answering:

- **D1.** Should stuck detection / content cap scale off a *separate* cloud-specific ceiling
  (e.g. a new `tuning.go` field, `SafetyTokenCeiling`, independent of the output-cap row) rather
  than `max_tokens` directly? This lets `max_tokens` grow with real published data while the
  safety thresholds stay anchored to the measured-safe 8192 scale.
- **D2.** How is a catalog-reported output cap sanity-checked before being trusted (per the
  verification asymmetry)? Candidate: clamp to `min(published, a generous absolute ceiling e.g.
  128K)` rather than removing the ceiling outright — still data-driven, still per-model, but with
  a floor against a bad catalog read Phase 0.1 may have already surfaced.
- **D3.** Does `ModeObject`/`ModeEnableThinking` (openrouter/nvidia) need different handling than
  `ModeEffort` (openai/gemini), since OpenRouter's `reasoning.enabled` can still produce long
  reasoning traces? Check whether Phase 0.2's measurement differs by mode.

This phase blocks Phase 2 — do not start implementing before D1–D3 are answered and reviewed.

### Phase 2 — Implement (TDD)

- Add the Phase-1-decided safety ceiling, independent of the output-cap row, to `tuning.go` /
  `stream.go`. Red tests first per `tdd-guide`: one proving today's dormant-stuck-detection failure
  mode would recur without the new ceiling, one proving it doesn't with it.
- Local-safety regression: add/extend a test asserting `LocalBudgetPolicy` and local stuck
  detection are byte-identical before/after (mirrors `TestCharacterizationGolden`'s local-row
  invariant).
- Run the full backend gate (`go build`, `go vet`, `go test ./...`, `go run
  ./tools/check-complexity/`) after every step, not just at the end.

### Phase 3 — Lift the output-cap tier ceiling for cloud

Only after Phase 2 lands: remove (or loosen, per D2) the `min(published, tier)` clamp in
`output_cap.go`'s `publishedOutputCap`, now that it no longer silently drags the safety
thresholds along with it. Regenerate `testdata/characterization_golden.txt`; the diff must be
`max_tokens` changes on cloud `metadata-present`/`metadata-inflated` rows only, no local rows
touched (same audit shape as the 2026-10-11 history-budget fix).

### Phase 4 — Docs and close-out

- Update `docs/SPECS/orchestrator.md` §3 (the "deliberate ceiling" note added 2026-10-11 needs
  rewriting once it's no longer a flat ceiling).
- Update this plan's status to `complete`, record the final decision (D1–D3) and numbers.
- `documentation-stewardship` pass: `docs/PLANS/README.md`, `docs/INDEX.md`, `architecture.md`
  Common Pitfalls if a new pitfall was found.

## Remaining Work

Everything above — this plan has not started implementation. Phase 0 is next.
