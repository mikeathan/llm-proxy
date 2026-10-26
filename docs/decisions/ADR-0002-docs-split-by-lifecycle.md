---
id: ADR-0002
title: Docs split by lifecycle, linked by subsystem
status: accepted
date: 2026-10-26
subsystems: [agent-harness]
supersedes:
superseded_by:
---

# ADR-0002: Docs split by lifecycle, linked by subsystem

## Context
`docs/` is the repo's long-term memory for people and agents. By 2026-10-26 the plans tracker was 30 KB with 31
finished plans listed as active, finished plans of up to 128 KB sat beside live ones, `architecture.md` (64 KB)
mixed the package map, contracts, checklists and pitfalls, decisions lived only inside plans, and lessons were
spread across 25 date-named audits. Agents had to load large files to answer small questions, and nothing told
them what had already been tried.

## Decision
We will separate knowledge by how it changes, and link it by subsystem:

| Kind | Home | Lifecycle |
|---|---|---|
| How it works now | `docs/SPECS/` (contracts), `docs/architecture/` (map, contracts, checklists, pitfalls) | edited in place; always current |
| Why it is this way | `docs/decisions/` (ADRs) | immutable; superseded by a new ADR |
| What is next | `docs/PLANS/` (open plans only) | short-lived; harvested, then archived |
| What we tried and learned | `docs/learnings/<subsystem>.md` ledgers; raw evidence in `docs/audits/` | append-only |
| Finished plans | `docs/PLANS/ARCHIVE/` | frozen evidence, loaded on demand |

A hub page per subsystem in `docs/subsystems/` links all five. A plan is harvested as its phases merge
(behavior → SPEC/architecture, decisions → ADR, lessons → ledger) and archived when complete. The repo is the
shared memory; a harness's private memory is a per-user cache and holds nothing a fresh clone would need.

## Consequences
- An agent classifies the task, opens one hub, and loads only the pieces it needs.
- Archived plans keep their full text (evidence, grep-able) instead of being cut to stubs: they cost nothing
  until opened, and their current-truth content is reachable from SPECs, ADRs and ledgers.
- Every move needs its links rewritten; `scripts/check-agent-harness.sh` checks the catalogs, hubs, ADR index,
  and that no `complete` plan stays in the live tracker.

## Alternatives considered
- **One "memory" folder for everything** — mixes current truth with history; agents cannot tell which wins.
- **Cutting archived plans to stubs** — saves no context (archives are not loaded by default) and loses
  searchable evidence; git history is a poor search surface for agents.
- **A wiki or external tool** — not versioned with the code, not visible to agents working in a clone.

## Links
- Plan: [`docs-and-agent-harness-restructure.md`](../PLANS/ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md)
- Router: [`docs/INDEX.md`](../INDEX.md)
