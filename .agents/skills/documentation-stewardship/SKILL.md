---
name: documentation-stewardship
description: "Post-completion checklist of the docs to update after any code change. Use after finishing a change."
when_to_use: "After finishing a change — deciding which SPEC/plan/skill/INDEX docs to update."
status: reference
last_reviewed: 2026-07-11
---

# Documentation Stewardship — Post-Completion Checklist

After every new feature, refactor, behavior fix, or revert:

1. **SPEC files** (`docs/SPECS/`) — update behavioral contracts if the change alters system behaviour
2. **Plan files** (`docs/PLANS/`) — add a new entry documenting what changed and why, organized by subsystem
3. **Skill files** (`.agents/skills/<name>/SKILL.md`) — add new gotchas, patterns, or architecture decisions discovered during the work
4. **INDEX** (`docs/INDEX.md`) — add entries for any new files created; update statuses for changed plans
5. **Audits** (`docs/audits/`) — create a new audit for any regression or post-mortem analysis
6. **`docs/architecture.md`** — update Common Pitfalls if a new pattern emerges that future agents should know
7. **`CONSTITUTION.md`** — if the change introduces or alters an architectural invariant, formalize it in the Constitution
