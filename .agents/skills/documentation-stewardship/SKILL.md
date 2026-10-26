---
name: documentation-stewardship
description: "Post-change documentation pass: route what you changed or learned to the right home (SPEC, architecture, ADR, plan, learnings ledger, subsystem hub, skill), harvest and archive finished plans, then verify with the harness check. Use after finishing any code change, merging a plan phase, or running an eval or investigation."
metadata:
  last_reviewed: "2026-10-26"
---

# Documentation Stewardship — Post-Change Pass

**Trigger:** you finished a change, merged a plan phase, or learned something from a run, eval or
investigation. Docs are the repo's shared memory and part of the deliverable.

**Where knowledge lives** (by lifecycle — `docs/decisions/ADR-0002-docs-split-by-lifecycle.md`):

| Kind | Home | Rule |
|------|------|------|
| How it works now | `docs/SPECS/` (contracts) · `docs/architecture/` (map, contracts, conventions, checklists, pitfalls) | Edit in place; always current. |
| Why it is this way | `docs/decisions/` (ADRs) | Immutable once accepted; supersede with a new ADR. |
| What is next | `docs/PLANS/` (open plans only) | Harvest as phases merge; archive when done. |
| What we tried | `docs/learnings/<subsystem>.md` · raw evidence in `docs/audits/` | Append a line; never delete. |
| One page per subsystem | `docs/subsystems/<slug>.md` | Index only — link, don't restate. |

`docs/INDEX.md` is the router; each kind has one catalog (`docs/SPECS/README.md`, `docs/PLANS/README.md`,
`docs/PLANS/ARCHIVE/README.md`, `docs/decisions/README.md`, `docs/learnings/README.md`, `docs/audits/README.md`,
`docs/subsystems/README.md`). Never keep a second copy of a catalog.

## Route by what happened (only the rows that apply)

| What happened | Update |
|---|---|
| Contracted behavior changed (route, event, tool, phase, budget, config semantics) | The governing SPEC (`docs/SPEC-change-management.md`). |
| A package or directory was added or moved | `docs/architecture/map.md` and the subsystem hub's **Code** row. |
| A recurring trap ("if you do X it breaks because Y") | Append a numbered pitfall to `docs/architecture/pitfalls.md` — numbers are stable IDs, never renumber. |
| A choice between real alternatives was settled | A new ADR from `docs/decisions/TEMPLATE.md`, a row in `docs/decisions/README.md`, and the hub's **Decisions** row. |
| A run, eval, audit or failed attempt taught something | One line in `docs/learnings/<subsystem>.md` (format in its README) with an evidence link; a long write-up goes to `docs/audits/` with the frontmatter in `docs/learnings/README.md`. |
| Phased work started | `docs/PLANS/<folder>/<kebab-name>.md` (front page ≤ ~15 KB: status, phases with acceptance, Remaining Work), a row in `docs/PLANS/README.md`, a link in the hub. |
| A plan phase merged | **Harvest** it: as-built → SPEC/architecture; decisions → ADR; lessons → ledger; update the plan's status and Remaining Work and its tracker row. |
| A plan is complete or superseded | Harvest, `git mv` it to `docs/PLANS/ARCHIVE/<folder>/`, move its row to `docs/PLANS/ARCHIVE/README.md`, drop it from the hub's open plans. Leftover follow-ups go under "Follow-ups left by archived plans". |
| An architectural invariant | `CONSTITUTION.md` (security review first). |
| A skill's content | Keep `SKILL.md` ≤ 150 lines — detail goes in `references/` (one level deep, linked from the skill's reference table); bump `metadata.last_reviewed`; keep `description` = what it does + when to use it. A new skill needs no registration beyond the `docs/INDEX.md` skills table. |
| Agent instructions | `AGENTS.md` only, in vendor-neutral words (`docs/decisions/ADR-0001-harness-neutral-agent-instructions.md`). |
| User-facing setup or commands | `README.md`, `CONTRIBUTING.md`, `docs/service_setup.md`, or the guide that owns it. |

Rules:

- **Correct, don't append** (ledgers excepted). Update the existing doc or row; wrong docs are worse than none.
- **Don't duplicate.** Link the SPEC, ADR or ledger instead of restating it.
- **Every link resolves, every status is current.**

## Verify

```bash
./scripts/check-agent-harness.sh   # frontmatter, paths, links, catalogs, hubs, ADRs, plan status, vendor-neutral wording
```

Fix every `FAIL:` line before finishing. Then run the full gate for the code change (`AGENTS.md` →
Pre-Completion Review) and report the documentation pass alongside it.

## Related

- `task-planning` — writes the plan this pass closes out.
- `clean-code` §3 (`references/comments-and-formatting.md`) — comment/doc smells (stale, redundant, commented-out code).
