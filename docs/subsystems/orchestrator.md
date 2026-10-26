---
subsystem: orchestrator
specs: [SPEC-005]
---

# Inference, budgets, providers and performance

Choosing and running models: token budgets, providers and failover, local llama.cpp, run lanes and model residency, inference performance.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-005](../SPECS/orchestrator.md) |
| **Code** | `backend/internal/core/orchestrator/`, `backend/internal/core/llm/`, `backend/internal/core/runlane/`, `backend/internal/core/nodeherder/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: budget, max_tokens, provider, lane, residency |
| **Decisions (why)** | [ADR-0004](../decisions/ADR-0004-explicit-primary-and-fallback-model.md) No model auto-bootstrap; explicit primary and fallback |
| **Open plans (next)** | [`orchestrator/local-and-cloud-inference-performance.md`](../PLANS/orchestrator/local-and-cloud-inference-performance.md)<br>[`orchestrator/auto-model-router.md`](../PLANS/orchestrator/auto-model-router.md)<br>[`orchestrator/cost-and-savings-ledger.md`](../PLANS/orchestrator/cost-and-savings-ledger.md)<br>[`orchestrator/cloud-safety-threshold-decoupling.md`](../PLANS/orchestrator/cloud-safety-threshold-decoupling.md) |
| **Learnings (tried before)** | [`learnings/orchestrator.md`](../learnings/orchestrator.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-10-09-local-prompt-cache.md`](../audits/2026-10-09-local-prompt-cache.md)<br>[`known-performance-findings.md`](../audits/known-performance-findings.md) |
| **Skills** | [`provider-integration`](../../.agents/skills/provider-integration/SKILL.md), [`llamacpp-setup`](../../.agents/skills/llamacpp-setup/SKILL.md) |
| **Guides** | — |
| **Archived plans** | [`ARCHIVE/orchestrator/`](../PLANS/ARCHIVE/orchestrator/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
