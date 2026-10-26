# Clean Code — Language-Agnostic Practices — Tests, Classes, Systems, Emergent Design and Concurrency

Reference for the [`clean-code`](../SKILL.md) skill, loaded on demand. You are writing tests, splitting classes, wiring construction vs. use, or touching concurrent code.

## 7. Tests

- **TDD/Red-Green-Refactor**: failing test → minimal pass → clean up. Test first
  forces testable design. (Repo flow: [`.agents/skills/tdd-guide`](../../tdd-guide/SKILL.md).)
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
