---
name: documentation-stewardship
description: "Post-change documentation pass: map what you changed to the SPEC, plan, INDEX, architecture pitfalls, skill, and CONSTITUTION updates it requires, then verify the catalog still resolves. Use after finishing any code change."
when_to_use: "After finishing a feature, refactor, behavior fix, or revert — before declaring the task done."
status: reference
last_reviewed: 2026-09-26
---

# Documentation Stewardship — Post-Change Pass

**Trigger:** you finished a change. Docs are part of the deliverable, not an optional extra.
**Source of truth:** `docs/INDEX.md` is the catalog; `docs/PLANS/README.md` is the live "what's left" tracker.

Update by what actually changed — not all seven rows every time:

| You changed | Update |
|---|---|
| Contracted behavior (route, event, tool, phase, budget, config semantics) | The governing SPEC in `docs/SPECS/` (check `docs/INDEX.md`; follow `docs/SPEC-change-management.md`). |
| A multi-step feature / phased work | `docs/PLANS/<subsystem>/<kebab-name>.md` — status + **Remaining Work**; register it in `docs/PLANS/README.md` (Active Plans + Remaining Work) and the plans table in `docs/INDEX.md`. |
| A recurring trap / root cause ("if you do X it breaks because Y") | A numbered pitfall in `docs/architecture.md` → Common Pitfalls, or a new `docs/audits/` entry for a post-mortem. |
| A new file, directory, endpoint, or skill | Add it to `docs/INDEX.md` (and the directory map in `docs/architecture.md` if it's a new package). |
| An architectural invariant | `CONSTITUTION.md` — formalize it (security review first); don't leave the law stale. |
| A skill's content | Bump its frontmatter `last_reviewed:` and keep the one-line `description`/`when_to_use` trigger accurate. |
| User-facing setup / commands | `README.md`, `CONTRIBUTING.md`, `docs/service_setup.md`, or the operator guide that owns it. |

Rules:

- **Correct, don't append.** Update the existing doc/row; don't create a parallel one. Wrong docs are worse than none.
- **Don't duplicate.** If the fact lives in a SPEC, link the SPEC from elsewhere rather than restating it.
- **Keep the catalog navigable.** `docs/INDEX.md` and `docs/PLANS/README.md` must resolve — no dead links, no stale statuses.

## Verify

```bash
# every path referenced from the catalog still exists (skips <placeholder> paths)
rg -o --no-filename '`(docs|\.agents)/[^`<>]+`' docs/INDEX.md docs/PLANS/README.md | tr -d '`' | sort -u \
  | while read -r p; do [ -e "$p" ] || echo "MISSING: $p"; done
```

Then run the full gate for the code change (`AGENTS.md` → Pre-Completion Review) and report the documentation pass alongside it.

## Related

- `task-planning` — writes the plan this pass closes out.
- `clean-code` §3 — comment/doc smells (stale, redundant, commented-out code).
