# Clean Code — Language-Agnostic Practices — Comments and Formatting

Reference for the [`clean-code`](../SKILL.md) skill, loaded on demand. You are writing or reviewing comments or layout.

## 3. Comments

Comments compensate for our failure to express intent in code. Use them only when
the code cannot say it.

**Good comments** — legal/licence headers, warnings of consequences, TODO with a
clear removal condition, clarification of an external/quirky dependency, legal
copy. Prefer making the *why* discoverable over prose.

**Bad comments — delete or fix:**
- **Redundant**: restates the code. Well-named code already says this.
- **Misleading / stale**: worse than none — it lies. Update or delete.
- **Mandated**: a comment box that must be filled even when empty of value.
- **Noise / journal**: changelog- or author-blame comments; version control does
  this better.
- **Commented-out code**: delete it. History has it; dead text rots and is
  misread. (Rare exception: a one-line, clearly-dated toggle with rationale.)
- **Position/closing-brace markers, bylines, HTML markup** in source.
- **Nonlocal information**: comments about something that lives elsewhere.
- **A comment where a function or variable name would do.** Extract and name it.

Rule of thumb: if a comment explains *what*, replace it with better code. If it
explains *why* (non-obvious constraint, workaround, decision), keep it — and keep
it one line where possible.

---

## 4. Formatting

- **Vertical**: concepts separated by blank lines; related lines kept dense.
  Declare things near where they are used. Callees appear below callers
  (newspaper: headline → detail). Order fields/methods consistently.
- **Horizontal**: open with short lines, allow longer ones, don't align
  columns across unrelated assignments. Keep indentation meaningful. Avoid
  dummy scopes.
- **Team consistency beats personal preference.** Run the formatter
  (`gofmt`, `prettier`) and follow the language rule file. Consistent style lets
  readers trust the code's shape.

---
