# Audit: Ephemeral Turn Context — Failed Run Analysis

## Summary

The initial implementation of the ephemeral turn context (UserRole + "Proceed directly.")
caused a ~50% regression in smoke test duration: from ~370s to ~555s (9.25 min).

## Symptom

- 25 LLM calls, 555s total
- No stuck-detection or fallback events — the flow was clean but slow
- Reasoning chars grew from 389 (turn 1) to 3,012 (turn 22) — far above expected ~570
- Turns 17-23 showed the model redoing dev-test steps (write → compile → run → repeat),
  suggesting lost task coherence

## Root Cause

Two compounded issues:

### 1. UserRole + Imperative Language

The injected message used `proxy.UserRole` (the model treats this as actual human
input) with trailing `| Proceed directly.` (looks like a command). The model spent
extra processing cycles trying to interpret this "instruction," and broke the
natural ToolResult → Assistant alternation that smaller models rely on for
coherence.

### 2. Recap Is Invisible to Rule 8

The model's recap happens in `reasoning_content` (hidden from the visible output).
Rule 8 only suppresses recap in the visible `Content`. The model was doing both:
recapping in reasoning (~500 chars/turn) PLUS reading the injected message
(~30 chars/turn). Net effect: +30 tokens of overhead, not -470.

## Fix

1. Changed `proxy.UserRole` → `proxy.SystemRole` — the model reads the turn
   context as a system annotation, not user input.
2. Removed `| Proceed directly.` — the imperative was redundant with rule 8 in
   the system prompt. Message now just states facts: `[Turn 22 | Tools used: ...]`.

## Outcome: Reverted Entirely

Neither the UserRole nor SystemRole variant improved runtime. The root assumption
was wrong: **recap is functional working memory, not waste.** The model uses
internal reasoning tokens to maintain task coherence across turns. Rule 8 cannot
suppress this because it happens in `reasoning_content` (hidden thinking), not
visible `Content`. The injected message is pure overhead — ~30 tokens/turn with
zero reduction in internal recap.

The entire ephemeral turn context feature was reverted. The code is back to
baseline. See `docs/PLANS/agent-loop/ephemeral-turn-context.md` (status: reverted).

## Resolution: Per-Model Temperature Override

The Gemma 4 repetition was identified as a sampling-parameter issue, not a
prompt engineering problem. The solution was to add per-model `Temperature` and
`TimeoutMinutes` overrides to the agent tuning system:

- `temperature` (float, default 0.1) — settable per-model via the UI or
  settings.yml. For Gemma 4, raising this from 0.1 to 0.4–0.7 reduces the
  deterministic loop behavior (see AGENTS.md common pitfall #8: automation uses
  0.1 which is too low for Gemma 4).
- `timeout_minutes` (int, default 30) — settable per-model. Allows slower models
  more time per execution.
- Server-side llama.cpp flags recommended: `--repeat-penalty 1.12 --repeat-last-n
  256 --frequency-penalty 0.5 --presence-penalty 0.5`.

### Files Changed for the Override System

| Layer | File | Change |
|-------|------|--------|
| Model | `models/config.go` | Added `Temperature float64` to `ModelConfig` |
| Model | `models/infrastructure.go` | Added `Temperature float64` to `ModelOverride` |
| Agent | `agent.go` | Added `DefaultAutomationTemperature = 0.1` const, `temperature` field on `Agent` + `AgentOptions` |
| Stream | `stream.go` | `buildChatRequest()` uses `a.temperature` if set, fallback `DefaultAutomationTemperature` |
| Transport | `registry_handlers.go` | Added `Temperature` to `modelFormRequest`, `writeModelOverrides`, `hasModelOverrides` |
| Transport | `admin_handlers.go` | Added `Temperature`, `TimeoutMinutes` to `adminTuningDefaults` + `adminModelView` |
| Transport | `admin_view.go` | Mapped `mc.Temperature` + `mc.TimeoutMinutes` |
| Manager | `manager.go` | `ApplyModelOverrides` applies Temperature override |
| Executor | `executor.go` | Wires `cfg.Temperature` → `opts.Temperature` |
| Frontend | `types/admin.ts`, `types/model.ts` | Added `temperature`, `timeout_minutes` to TypeScript types |
| Frontend | `utils/modelUtils.ts` | Added to `ModelForm`, defaults, form creation |
| Frontend | `composables/useConfig.ts`, `useModels.ts` | Added to fallback defaults |
| Frontend | `ProviderModelsCard.vue` | Added form inputs with tooltips + validation (min/max/step) |
