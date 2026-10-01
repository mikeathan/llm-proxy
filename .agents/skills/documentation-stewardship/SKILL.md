---
name: documentation-stewardship
description: "Post-change documentation pass: map what you changed to the SPEC, plan, INDEX, architecture pitfalls, skill, and CONSTITUTION updates it requires, then verify the catalog still resolves. Use after finishing any code change."
last_reviewed: 2026-09-26
---

# Documentation Stewardship — Post-Change Pass

**Trigger:** you finished a change. Docs are part of the deliverable, not an optional extra.
**Source of truth:** `docs/INDEX.md` is the router; each doc kind has one authoritative catalog — SPECs (`docs/SPECS/README.md`), plans (`docs/PLANS/README.md`), audits (`docs/audits/README.md`). Never maintain a second copy of a catalog.

Update by what actually changed — not all seven rows every time:

| You changed | Update |
|---|---|
| Contracted behavior (route, event, tool, phase, budget, config semantics) | The governing SPEC in `docs/SPECS/` (check `docs/SPECS/README.md`; follow `docs/SPEC-change-management.md`). |
| A multi-step feature / phased work | `docs/PLANS/<subsystem>/<kebab-name>.md` — status + **Remaining Work**; register it in `docs/PLANS/README.md` (Active Plans + Remaining Work). |
| A recurring trap / root cause ("if you do X it breaks because Y") | A numbered pitfall in `docs/architecture.md` → Common Pitfalls, or a new `docs/audits/` entry for a post-mortem. |
| A new SPEC | A row in `docs/SPECS/README.md` (the canonical SPEC catalog). |
| A new package / directory | The directory map in `docs/architecture.md` (and a sub-catalog row if it owns docs). |
| A new skill | Nothing to register — skills are auto-discovered from `.agents/skills/<name>/SKILL.md`; ensure `name` matches the directory and `description` states the trigger. |
| Any other new top-level doc | A row in `docs/INDEX.md` (the router's guide/other tables). |
| An architectural invariant | `CONSTITUTION.md` — formalize it (security review first); don't leave the law stale. |
| A skill's content | Bump its frontmatter `last_reviewed:` and keep the one-line `description` trigger accurate. |
| User-facing setup / commands | `README.md`, `CONTRIBUTING.md`, `docs/service_setup.md`, or the operator guide that owns it. |

Rules:

- **Correct, don't append.** Update the existing doc/row; don't create a parallel one. Wrong docs are worse than none.
- **Don't duplicate.** If the fact lives in a SPEC, link the SPEC from elsewhere rather than restating it.
- **Keep the catalog navigable.** Every catalog and link must resolve — no dead links, no stale statuses.

## Verify

```bash
./scripts/check-agent-harness.sh   # skills, referenced paths, markdown links, plan links
```

Fix every `FAIL:` line before finishing. Then run the full gate for the code change (`AGENTS.md` →
Pre-Completion Review) and report the documentation pass alongside it.

## Related

- `task-planning` — writes the plan this pass closes out.
- `clean-code` §3 — comment/doc smells (stale, redundant, commented-out code).
