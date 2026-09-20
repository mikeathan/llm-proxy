---
name: clean-code
description: "Language-agnostic clean-code practices: naming, functions, comments, formatting, boundaries, error handling, tests, SOLID, emergent design, concurrency, and a smells checklist. Use when writing or reviewing code."
when_to_use: "Writing, refactoring, or reviewing code; naming/function/comment smells; design tradeoffs."
status: reference
last_reviewed: 2026-09-06
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

## 1. Names

A name is the cheapest documentation you will ever write.

- **Intention-revealing:** say *what it is/does*, not *how* or its type.
  `elapsedMs` > `d`; `activeSessions` > `list2`.
- **No disinformation:** avoid names that suggest the wrong type/meaning
  (`accountList` when it is a map; `isX` for non-booleans).
- **Meaningful distinctions:** `data`/`data2`, `get`/`fetch` used for identical
  concepts, or noise words (`Info`, `Manager`, `Data`, `Object`) that hide intent.
- **Pronounceable and searchable:** a reader should be able to say it and grep
  it. Single letters are only for tiny local scopes.
- **Avoid encodings:** no type prefixes (`strName`), no Hungarian notation, no
  member prefixes. Modern IDEs make them noise.
- **Pick one word per concept.** Don't mix `get`/`retrieve`/`fetch` for the same
  operation across the codebase.
- **Appropriate abstraction level:** name the domain concept, not the mechanism.
- **Scopes scale with length:** short names for short scopes; long descriptive
  names for long ones (and never for constants you may want to change).
- **Describe side effects.** If `parseAndCache()` mutates, the name must say so.
- **Context via a type/struct, not a prefix.** Prefer a `Workspace` type over
  `wsName`, `wsPath`, `wsRoot`. Don't add context the reader already has.

---

## 2. Functions

- **Small.** If a function needs a section header comment to be navigable, it's
  too big. Extract until each block is one idea at one level of abstraction.
- **Do one thing.** A function that name-and-does more than one thing can't be
  named without "and" (or a vague verb). Extract the "and".
- **One level of abstraction per function.** Don't mix orchestration
  (`processOrder`) with plumbing (`openSocket`). The **stepdown rule**: a
  function reads top-to-bottom, each line one abstraction level, callees named
  as the next level down.
- **Prefer polymorphism to switch/if-chains** that grow with new cases. A map of
  handlers/strategies is closed for modification and open for extension.
- **Arguments: fewer is better.** 0–3 ideal; 4 is the review ceiling; beyond that
  take a named options/deps struct. Booleans are the first candidate for a
  struct (see the flag-argument bullet below).
- **No output arguments.** Don't mutate a passed-in value to return a result;
  return the value (command–query separation).
- **Avoid flag arguments.** A boolean argument usually means the function does
  two things; split it or use an options struct.
- **No side effects.** A function promises one thing; hidden state mutation is a
  trap. If it must mutate, the name must say so and the caller must expect it.
- **Command–query separation.** A function either does something or answers
  something — not both.
- **Prefer exceptions/errors to sentinel return codes**, and extract the
  try/error-handling body into its own function. Error handling is *one thing*.
- **DRY, but 3-strikes.** Duplicate freely the first two times; extract on the
  third. Premature abstraction is as costly as duplication.
- **Structure**: use guard clauses / early returns; keep the happy path
  unindented; no deep nesting (≤3 levels). Keep the function within the repo's
  line/complexity limits (see AGENTS.md; `check-complexity` gate).

---

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

## 5. Objects, Data Structures & Boundaries

- **Objects hide data and expose behaviour; data structures expose data and
  have no behaviour.** Don't create "hybrids" that do both. Adding a new type
  should favor objects; adding a new behaviour should favor data structures.
- **Law of Demeter / no "train wrecks":** avoid `a.getB().getC().doThing()`. Talk
  to immediate collaborators; make the object do the walking.
- **Expose abstraction, not implementation.** A getter that returns raw internals
  leaks structure — provide the operation the caller actually wants.
- **DTOs are the deliberate exception**: pure data, public fields, no logic.
- **Encapsulate conditionals and boundary conditions**: name a small predicate
  (`isEligible()`) and put `+1`/`-1`/off-by-one math behind a named helper.
- **Boundaries (third-party code):** wrap external APIs behind your own interface
  so you can (a) swap them, (b) test them, (c) learn their quirks without
  scattering them. Write **learning tests** against the real dependency — they
  pin expected behaviour and are free to keep.
- **Keep configurable data high and passed down**; keep low-level modules free of
  policy and environment knowledge.
- **Avoid artificial coupling** (a function/class that uses an unrelated other)
  and **feature envy** (a method more interested in another object's data than
  its own — move it).

---

## 6. Error Handling

- **Exceptions/errors over return codes.** Error codes force callers to check
  immediately and bloat the happy path.
- **Write the error path first.** Define what failure looks like before the
  success path; it shapes the interface.
- **Carry context.** Wrap with enough info to diagnose (operation + input) —
  and favour wrapping/`errors.Is/As` chains over bare strings.
- **Define error types by caller need.** Catch/convert at the boundary where the
  caller can act, not per low-level source.
- **Never return `null`/`nil`.** Return an empty collection, an option/zero
  object, or raise. Never pass `null` into a function that doesn't expect it.
- **Don't log and re-return** the same error at every layer — either handle it or
  propagate it; log once at the boundary that has context.
- **Keep errors on the happy path's terms:** the normal flow should not depend on
  exceptional control flow.
- **Clean up resources deterministically** (defer/`finally`/RAII); no leaks on
  any exit path.

---

## 7. Tests

- **TDD/Red-Green-Refactor**: failing test → minimal pass → clean up. Test first
  forces testable design. (Repo flow: [`.agents/skills/tdd-guide`](../tdd-guide/SKILL.md).)
- **Tests are first-class code** — clean, readable, as maintained as production.
  Dirty tests decay into "test debt" and eventually get deleted.
- **One concept per test.** Multiple asserts are fine if they assert one
  behavior; avoid "one assert" dogma at the expense of clarity.
- **F.I.R.S.T.:** Fast, Independent, Repeatable, Self-validating, Timely.
- **Build and test must be a single command each.** If setup takes multiple
  steps, that is a defect; automate it.
- **Test near bugs**: when a defect appears, exhaustively test that area — bugs
  cluster. Use coverage to find *untested*, not to prove correctness.
- **Boundary conditions** get explicit tests (empty, one, many, max, off-by-one,
  negative, null, malformed).
- **Don't skip/ignore tests** — an ignored test is an unanswered question; resolve
  or delete it. Test only what can break; don't test the language/stdlib.
- **Never keep production code alive only to satisfy a test.** If a symbol's sole
  caller is a test, it is dead code (Constitution IV.4) — delete the symbol *and*
  its test. A test proves the behavior of production code; it is never a reason for
  that code to exist. Seams, hooks, and registration APIs are added when a real
  consumer needs to extend them, never speculatively "so it can be tested".
- **Deterministic tests.** No sleeps-as-synchronization, no order dependence, no
  shared mutable state.

---

## 8. Classes & Design

- **Small, one responsibility** (SRP). A class has one reason to change; "and"
  in its description is a smell. Extract until each type fits in your head.
- **High cohesion:** methods should share the fields they use. When a subset of
  methods uses a subset of fields, split the class.
- **Organize for change:** isolate what changes (interfaces/plugins) so the rest
  of the system is closed to it.
- **Depend on abstractions, not concretions** (DIP); keep interfaces small and
  role-specific (ISP); subtypes must be substitutable (LSP).
- **Open/Closed:** extend by adding new code, not by editing a switch that grows
  with each new case (see §2 strategy maps and §10 rule 4).

---

## 9. Systems: Construction vs. Use

- **Separate wiring from logic.** Construction/bootstrap belongs at one
  composition root; business logic never constructs its own dependencies.
- **Dependency injection over global/singleton lookup.** Pass collaborators in;
  make required vs optional explicit.
- **Centralize cross-cutting concerns** — logging, auth, transactions, tracing —
  in one place (decorator/middleware/aspect), not copy-pasted per site.
- **Centralize app lifecycle & long-lived background coordination** in one
  discoverable init/entrypoint so startup wiring is not half-forgotten (repo rule).
- **Decide late:** keep policy (rates, paths, feature choices) out of low-level
  code; inject it. Use standards/frameworks only when they add demonstrable value.

---

## 10. Emergent Design (4 rules, in priority order)

1. **Runs all the tests.** A design that fails its tests is not "clean".
2. **No duplication.** Duplication is the primary enemy of maintainability;
   remove it (after the 3rd occurrence).
3. **Expresses intent.** Names, structure, and tests make the author's intent
   obvious without a manual.
4. **Minimal classes and methods.** Don't add speculative structure, indirection,
   or generality. Lowest count that satisfies 1–3 wins.

These are a *refactoring* discipline, not a checklist to design up front.

---

## 11. Concurrency

- **Concurrency is a decoupling of *what* from *when*; it is hard and optional.**
  Keep concurrent code separate from the domain (SRP) so the domain stays simple
  and testable.
- **Limit the scope of shared data.** Prefer copies, thread-local, or
  confinement. Threads should be as independent as possible.
- **Know your primitives and execution model.** Use the platform's thread-safe
  collections and documented patterns (producer–consumer, readers–writers) rather
  than hand-rolled locking.
- **Avoid dependencies between synchronized methods**; keep critical sections
  small; never call out to unknown code while holding a lock.
- **Shutdown is hard:** design termination paths explicitly and test them. Every
  goroutine/thread/timer/listener needs an owner and a stop path — no orphans.
- **Test threaded code deliberately:** make it pluggable/tunable, run under a race
  detector, and treat any spurious failure as a real concurrency bug until proven
  otherwise.

---

## 12. Review Checklist — Smells & Heuristics

Scan this during self-review. Each item is a *question to answer*, not an
automatic violation.

### Names (N)
| # | Smell | Fix |
|---|-------|-----|
| N1 | Name doesn't reveal intent | Rename to what it is/does |
| N2 | Wrong abstraction level | Name the domain concept, not the mechanism |
| N3 | Non-standard nomenclature | Use the ecosystem's established term |
| N4 | Ambiguous name | Disambiguate (`getActiveUsers` vs `getUsers`) |
| N5 | Short name for a long scope | Lengthen it |
| N6 | Encoded/type-prefixed name | Drop the encoding |
| N7 | Side effect not in the name | Rename to expose it |

### Functions (F)
| # | Smell | Fix |
|---|-------|-----|
| F1 | Too many arguments | Group into an options/deps struct |
| F2 | Output argument | Return the value instead |
| F3 | Flag argument | Split or use options |
| F4 | Dead/unused function | Delete it |

### Comments (C)
| # | Smell | Fix |
|---|-------|-----|
| C1 | Inappropriate/irrelevant info | Delete |
| C2 | Obsolete comment | Update or delete |
| C3 | Redundant comment | Delete; let code speak |
| C4 | Poorly written comment | Rewrite or delete |
| C5 | Commented-out code | Delete (VCS remembers) |

### Tests (T)
| # | Smell | Fix |
|---|-------|-----|
| T1 | Insufficient tests | Cover boundaries + error paths |
| T2 | No coverage visibility | Use a coverage tool |
| T3 | Skipping trivial tests | Don't; trivial tests still catch regressions |
| T4 | Ignored/disabled test | Resolve the ambiguity or delete |
| T5 | Untested boundaries | Test empty/one/many/edge |
| T6 | Untested area around a past bug | Exhaustively test there |
| T7 | Failure patterns ignored | Cluster failures reveal root causes |
| T8 | Coverage gaps ignored | Gaps reveal untested logic |
| T9 | Slow tests | Keep the suite fast; split/parallelize |
| T10 | Production code whose only caller is a test | Delete the code and its test; don't ship a symbol to justify a test |

### Environment (E)
| # | Smell | Fix |
|---|-------|-----|
| E1 | Build needs >1 step | One command to build |
| E2 | Tests need >1 step | One command to test |

### General (G)
| # | Smell | Fix |
|---|-------|-----|
| G1 | Multiple languages in one file | Separate by concern |
| G2 | Obvious behavior unimplemented | Implement or remove the promise |
| G3 | Incorrect boundary behavior | Test + fix edges |
| G4 | Safety/validation overridden | Don't disable guards; fix the cause |
| G5 | Duplication | Extract (3-strikes) |
| G6 | Code at wrong abstraction level | Move to the right layer |
| G7 | Base depends on derived | Invert: derived depends on base |
| G8 | Too much information / wide interface | Expose only what's needed |
| G9 | Dead code | Delete |
| G10 | Concepts not vertically separated | Group related code, blank-line separate |
| G11 | Inconsistency | Pick one convention everywhere |
| G12 | Clutter (unused vars/params/imports) | Remove |
| G13 | Artificial coupling | Decouple unrelated things |
| G14 | Feature envy | Move the method to the data it envies |
| G15 | Selector/flag argument | Split the function |
| G16 | Obscured intent | Rename / restructure / add explanatory names |
| G17 | Misplaced responsibility | Put it where it belongs |
| G18 | Inappropriate static/global | Make it injectable/instance-scoped |
| G19 | Unexplained expression | Extract an explanatory variable |
| G20 | Name doesn't say what it does | Rename |
| G21 | Algorithm not understood | Understand and simplify it |
| G22 | Logical dependency left implicit | Make it physical (explicit) |
| G23 | Growing switch/if-chain | Polymorphism/strategy map |
| G24 | Local convention not followed | Follow the standard |
| G25 | Magic number/string | Named constant |
| G26 | Imprecision (loose types/assumptions) | Be exact; validate |
| G27 | Convention where structure is clearer | Encode the structure |
| G28 | Complex conditional repeated | Encapsulate in a named predicate |
| G29 | Negative conditionals | Prefer positive conditions |
| G30 | Does more than one thing | Split |
| G31 | Hidden temporal coupling | Make ordering explicit / enforce it |
| G32 | Arbitrary choices | Justify or normalize |
| G33 | Boundary math inline | Encapsulate in a helper |
| G34 | Mixed abstraction levels in one function | One level per function |
| G35 | Configurable data buried low | Hoist it up |
| G36 | Transitive navigation (`a.b().c().d()`) | Hide the structure behind a method |

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
