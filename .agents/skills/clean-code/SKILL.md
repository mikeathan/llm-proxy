---
name: clean-code
description: "Language-agnostic clean-code practices: naming, functions, comments, formatting, boundaries, error handling, tests, SOLID, emergent design, concurrency, and a smells checklist. Use when writing or reviewing code."
metadata:
  last_reviewed: "2026-10-26"
---

# Clean Code — Language-Agnostic Practices

**Source:** distilled from *Clean Code* (Robert C. Martin) plus this repo's existing
rules. Principles are paraphrased and generalized; language-specific mechanics live
in [`.agents/rules/`](../../rules/) and [`.agents/skills/engineering-practices`](../engineering-practices/SKILL.md).

**When to load:** writing or reviewing any code — new features, refactors, bug
fixes, PR review. Pair with [`.agents/skills/tdd-guide`](../tdd-guide/SKILL.md) for the test cycle and
[`.agents/skills/engineering-practices`](../engineering-practices/SKILL.md) for repo/Go/Vue specifics.

**How to use:** prefer the smallest change that satisfies a rule. Rules conflict —
when they do, optimize for *the next reader*, then for testability, then for
locality. If you cannot justify a rule in one sentence, you are over-applying it.

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/names-and-functions.md`](references/names-and-functions.md) | You are naming things or shaping a function (size, arguments, side effects). |
| [`references/comments-and-formatting.md`](references/comments-and-formatting.md) | You are writing or reviewing comments or layout. |
| [`references/objects-errors-boundaries.md`](references/objects-errors-boundaries.md) | You are designing data structures, wrapping a third-party API, or handling errors. |
| [`references/tests-and-design.md`](references/tests-and-design.md) | You are writing tests, splitting classes, wiring construction vs. use, or touching concurrent code. |
| [`references/smells-checklist.md`](references/smells-checklist.md) | You are reviewing a diff (yours or someone else's) and want the full smells list. |

## 0. Mindset

- **Code is read far more than written.** The cost of code is the cost of
  maintaining it for years, not typing it once. Optimize for comprehension.
- **Leave it cleaner than you found it** (Boy Scout rule): a rename, a removed
  dead branch, an extracted function. Every touch is an opportunity; no cleanup
  is too small.
- **You are an author.** A function, module, or commit message is prose for the
  next engineer. Write for them, not the compiler.
- **Mess compounds.** A small shortcut becomes a "broken window" that invites
  more shortcuts. Fix or flag it immediately.
- **Clarity beats cleverness.** If it needs a comment to be understood, rewrite
  it. If a reviewer has to trace three files to trust it, restructure it.
- **Honesty over appearance.** Do not claim a check passed, an edge is handled,
  or a mechanism is enforced when it is not. Report uncertainty plainly.

---

## 13. Applying This in This Repo

- **Language rules are mandatory and take precedence on mechanics:**
  [`.agents/rules/go-staff-engineer.md`](../../rules/go-staff-engineer.md)
  (backend), [`.agents/rules/frontend-vue-engineer.md`](../../rules/frontend-vue-engineer.md)
  (frontend).
- **Repo-specific patterns & limits** (complexity ≤12, function length, comments,
  constants, strategy maps): [`.agents/skills/engineering-practices`](../engineering-practices/SKILL.md).
- **Test flow & speed**: [`.agents/skills/tdd-guide`](../tdd-guide/SKILL.md).
- **Post-change doc updates**: [`.agents/skills/documentation-stewardship`](../documentation-stewardship/SKILL.md).
- **Law over everything**: [`CONSTITUTION.md`](../../../CONSTITUTION.md) and
  [`AGENTS.md`](../../../AGENTS.md). When this skill conflicts with either, they win.
