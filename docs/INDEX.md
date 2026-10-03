# Documentation Index

This is the **router** for repository documentation. Each doc kind has one authoritative catalog;
follow the link — do not maintain a second copy here.

**Status legend (used by sub-catalogs):** `stable` | `draft` | `superseded` | `reference` | `complete` |
`partial` | `proposed` | `active` | `approved` | `reverted`.

---

## Start here

| Need | Go to |
|------|-------|
| The law (immutable invariants) | [`CONSTITUTION.md`](../CONSTITUTION.md) |
| How agents must operate | [`AGENTS.md`](../AGENTS.md) |
| Behavioral contract for a subsystem | [`docs/SPECS/README.md`](SPECS/README.md) — SPEC-ID → file map |
| Where things live, contracts, pitfalls, file checklists | [`docs/architecture.md`](architecture.md) |
| What work is left | [`docs/PLANS/README.md`](PLANS/README.md) — live tracker for non-complete plans |
| Post-mortems and audits | [`docs/audits/README.md`](audits/README.md) |
| Scratch a subsystem deeply | [`../.agents/skills/`](../.agents/skills/) — Agent Skills, loaded on demand |
| Mandatory language mechanics | [`../.agents/rules/`](../.agents/rules/) — Go / Vue rules for AI |

> **Agent Skills are self-describing.** Each `.agents/skills/<name>/SKILL.md` carries a `name` and a
> `description`; the description is the trigger and is always available to the agent. The table below
> exists only for human navigation — do not duplicate `description` text here.

## Skills (reference guides)

| Skill | Path |
|-------|------|
| agent-loop | `.agents/skills/agent-loop/SKILL.md` |
| assistant-ui-chat | `.agents/skills/assistant-ui-chat/SKILL.md` |
| assistant-ui-patterns | `.agents/skills/assistant-ui-patterns/SKILL.md` |
| automation | `.agents/skills/automation/SKILL.md` |
| clean-code | `.agents/skills/clean-code/SKILL.md` |
| connector-patterns | `.agents/skills/connector-patterns/SKILL.md` |
| debugging | `.agents/skills/debugging/SKILL.md` |
| documentation-stewardship | `.agents/skills/documentation-stewardship/SKILL.md` |
| engineering-practices | `.agents/skills/engineering-practices/SKILL.md` |
| event-streaming-patterns | `.agents/skills/event-streaming-patterns/SKILL.md` |
| lifecycle-events | `.agents/skills/lifecycle-events/SKILL.md` |
| llamacpp-setup | `.agents/skills/llamacpp-setup/SKILL.md` |
| memory-system | `.agents/skills/memory-system/SKILL.md` |
| task-planning | `.agents/skills/task-planning/SKILL.md` |
| tdd-guide | `.agents/skills/tdd-guide/SKILL.md` |
| testing-guide | `.agents/skills/testing-guide/SKILL.md` |

## Agent rules (mandatory mechanics)

| File | Title |
|------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI |

## Catalogs

| Kind | Authoritative catalog | Contents |
|------|-----------------------|----------|
| Specifications | [`docs/SPECS/README.md`](SPECS/README.md) | Behavioral contracts (SPEC-NNN) — read before modifying a subsystem |
| Implementation plans | [`docs/PLANS/README.md`](PLANS/README.md) | Active plans + the "Remaining Work" tracker; completed plans in `docs/PLANS/ARCHIVE/` |
| Audits | [`docs/audits/README.md`](audits/README.md) | Post-hoc analysis of system behavior against specs |

## Top-level guides

| File | Title | Audience |
|------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | End users |
| `CONTRIBUTING.md` | Contributing — commands, standards, git, releases | Developers |
| `AGENTS.md` | Operating contract for AI coding agents | AI assistants |
| `CONSTITUTION.md` | Architectural Invariants — The Law | Everyone |
| `docs/architecture.md` | Architecture Reference (mappings, contracts, checklists, pitfalls) | Developers |
| `docs/guides/loop-strategy.md` | Loop Strategy — operator guide (which archetype to pick) | Operators |
| `docs/guides/automation-digest.md` | Automation Digest — deliver a recurring research run to Telegram without repeats | Operators |
| `docs/guides/memory-testing.md` | Memory — verify it works (store then recall runs) and the optional live A/B comparison on a small local model | Operators |

## Other documents

| File | Title | Notes |
|------|-------|-------|
| `docs/services/llm-proxy.service` | systemd service unit (hardened: dedicated user, single root) | Operational |
| `docs/service_setup.md` | Service installation instructions | Setup |
| `docs/data-layout.md` | Data Layout — root resolution, meta/ vs runs/ split, cleanup surfaces | Reference |
| `docs/SPEC-change-management.md` | SPEC Lifecycle & Change Management | Reference |
| `docs/PLANS/ARCHIVE/` | Completed/superseded/not-implemented plans | Archive |

---

## Directory navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/guides/` | Operator-facing how-to guides (non-normative; SPECs stay authoritative). |
| `docs/PLANS/` | Active implementation strategies. Organized by subsystem. |
| `docs/PLANS/ARCHIVE/` | Completed and superseded plans — load on demand. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `.agents/skills/` | Repo Agent Skills — auto-discovered, loaded on demand for deep-dive topics. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
