# Documentation Index

The **router** for repository documentation. Each doc kind has one authoritative catalog; follow the link —
do not keep a second copy here. How the pieces fit: [ADR-0002](decisions/ADR-0002-docs-split-by-lifecycle.md).

---

## Start here

| Need | Go to |
|------|-------|
| **Everything about one subsystem** (contract, code, decisions, plans, learnings, skills) | [`docs/subsystems/README.md`](subsystems/README.md) — one hub per subsystem |
| The law (immutable invariants) | [`CONSTITUTION.md`](../CONSTITUTION.md) |
| How agents must operate | [`AGENTS.md`](../AGENTS.md) |
| Behavioral contract for a subsystem (how it works now) | [`docs/SPECS/README.md`](SPECS/README.md) — SPEC-ID → file map |
| Where code lives, cross-cutting contracts, change checklists, pitfalls | [`docs/architecture.md`](architecture.md) → `docs/architecture/` |
| Why it is this way | [`docs/decisions/README.md`](decisions/README.md) — ADRs |
| What is next | [`docs/PLANS/README.md`](PLANS/README.md) — open plans and what is left |
| What was tried and learned | [`docs/learnings/README.md`](learnings/README.md) — one ledger per subsystem |
| Evidence: audits, evals, post-mortems | [`docs/audits/README.md`](audits/README.md) |
| Finished plans (history, load on demand) | [`docs/PLANS/ARCHIVE/README.md`](PLANS/ARCHIVE/README.md) |
| Deep procedural guides for agents | [`../.agents/skills/`](../.agents/skills/) — Agent Skills, loaded on demand |
| Mandatory language mechanics | [`../.agents/rules/`](../.agents/rules/) — Go / Vue rules |

> **Agent Skills are self-describing.** Each `.agents/skills/<name>/SKILL.md` carries a `name` and a
> `description`; the description is the trigger and is always available to the agent. Detail lives in each
> skill's `references/` folder. The table below exists only for human navigation — do not duplicate
> `description` text here.

## Skills (reference guides)

| Skill | Path |
|-------|------|
| agent-loop | `.agents/skills/agent-loop/SKILL.md` |
| assistant-ui-chat | `.agents/skills/assistant-ui-chat/SKILL.md` |
| assistant-ui-patterns | `.agents/skills/assistant-ui-patterns/SKILL.md` |
| automation | `.agents/skills/automation/SKILL.md` |
| clean-code | `.agents/skills/clean-code/SKILL.md` |
| config-persistence | `.agents/skills/config-persistence/SKILL.md` |
| connector-patterns | `.agents/skills/connector-patterns/SKILL.md` |
| debugging | `.agents/skills/debugging/SKILL.md` |
| documentation-stewardship | `.agents/skills/documentation-stewardship/SKILL.md` |
| engineering-practices | `.agents/skills/engineering-practices/SKILL.md` |
| event-streaming-patterns | `.agents/skills/event-streaming-patterns/SKILL.md` |
| lifecycle-events | `.agents/skills/lifecycle-events/SKILL.md` |
| llamacpp-setup | `.agents/skills/llamacpp-setup/SKILL.md` |
| memory-system | `.agents/skills/memory-system/SKILL.md` |
| pr-commit-message | `.agents/skills/pr-commit-message/SKILL.md` |
| provider-integration | `.agents/skills/provider-integration/SKILL.md` |
| task-planning | `.agents/skills/task-planning/SKILL.md` |
| tdd-guide | `.agents/skills/tdd-guide/SKILL.md` |
| testing-guide | `.agents/skills/testing-guide/SKILL.md` |

## Agent rules (mandatory mechanics)

| File | Title |
|------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI |

## Catalogs

| Kind | Authoritative catalog | Lifecycle |
|------|-----------------------|-----------|
| Subsystem hubs | [`docs/subsystems/README.md`](subsystems/README.md) | Index pages; updated when anything they link changes |
| Specifications | [`docs/SPECS/README.md`](SPECS/README.md) | Current contracts (SPEC-NNN); edited in place ([change management](SPEC-change-management.md)) |
| Architecture | [`docs/architecture.md`](architecture.md) | Current map, contracts, conventions, checklists, numbered pitfalls |
| Decisions | [`docs/decisions/README.md`](decisions/README.md) | ADRs; immutable once accepted, superseded by new ADRs |
| Plans | [`docs/PLANS/README.md`](PLANS/README.md) | Open work only; harvested as phases merge, then archived |
| Archived plans | [`docs/PLANS/ARCHIVE/README.md`](PLANS/ARCHIVE/README.md) | Frozen evidence |
| Learnings | [`docs/learnings/README.md`](learnings/README.md) | Append-only ledgers per subsystem |
| Audits | [`docs/audits/README.md`](audits/README.md) | Evidence documents (frontmatter: subsystem, tags, result) |

## Guides and other documents

| File | Title | Audience |
|------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | End users |
| `CONTRIBUTING.md` | Contributing — commands, standards, git, releases | Developers |
| `AGENTS.md` | Operating contract for AI coding agents (all harnesses) | AI agents |
| `CONSTITUTION.md` | Architectural Invariants — The Law | Everyone |
| `docs/api-reference.md` | HTTP API reference — all endpoints | Developers, integrators |
| `docs/data-layout.md` | Data Layout — root resolution, meta/ vs runs/ split, cleanup surfaces | Reference |
| `docs/SPEC-change-management.md` | SPEC Lifecycle & Change Management | Reference |
| `docs/service_setup.md` | Service installation instructions | Setup |
| `docs/services/llm-proxy.service` | systemd service unit (hardened: dedicated user, single root) | Operational |
| `docs/guides/loop-strategy.md` | Loop Strategy — operator guide (which archetype to pick) | Operators |
| `docs/guides/automation-digest.md` | Automation Digest — deliver a recurring research run to Telegram without repeats | Operators |
| `docs/guides/memory-testing.md` | Memory — verify it works (store then recall runs) and the optional live A/B comparison on a small local model | Operators |

Guides in `docs/guides/` are operator how-tos and non-normative; SPECs stay authoritative.
