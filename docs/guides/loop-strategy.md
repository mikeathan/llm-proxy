# Loop Strategy — Operator Guide

How to choose the agent's loop strategy per model. This is guidance, not the contract;
the behavioral contract is [SPEC-010](../SPECS/agent-loop-strategies.md).

## What changed

The agent loop is now a pluggable engine with three selectable archetypes. The default
(`""` / empty) is **react** — the original loop, unchanged. Nothing breaks if you leave
every model untouched; you only get different behavior when you explicitly pick a
non-default strategy.

## The three strategies

| Strategy | What it does | When to use | Cost / tradeoff |
|----------|--------------|-------------|-----------------|
| **react** (default) | The model decides each next step and stops when it produces a final report. | General purpose, open-ended tasks, anything with unpredictable steps. | Cheapest. May wander on large multi-step tasks. |
| **plan_execute** | Writes a step-by-step tool plan first, then executes each step in order. | Multi-step tool tasks where step order matters (e.g. "refactor X, then run tests"). | One extra planning call up front. Falls back to react if planning fails (no user message, no tools, or plan generation error). |
| **evaluator_optimizer** | React loop plus a bounded self-review nudge before finalizing (up to 2 rounds). | Code/analysis work where verifying before finishing matters. | Slower (extra turns), higher quality. Only fires on natural completion — never on forced-cap or error paths. |

## Where to set it

- **Per model** — Agent Tuning grid → "Loop Strategy" dropdown (empty = Provider default / ReAct). This is the primary lever.
- **Per automation run** — an automation's `loop_strategy` field overrides the model config for that run only.

Precedence (highest wins): per-run automation override → per-model config → react.

## Choosing a default

- Keep **react** for chat, Q&A, and open-ended work.
- Use **plan_execute** for tool-heavy automations and deterministic multi-step workflows.
- Use **evaluator_optimizer** for models that produce code, analysis, or anything you want double-checked before it reports "done".

## Not automatic (by design)

Loop selection is **manual**. There is no per-provider default table or workload
auto-detection yet (`providerDefaultStrategy` is intentionally static react — an
unmeasured loop-shape default changes the whole execution shape and is higher risk than
tuning temperature/max_tokens). If you want a strategy to apply everywhere without
per-model edits, set it explicitly on each model rather than relying on a default.
