# Clean Code — Language-Agnostic Practices — Objects, Errors and Boundaries

Reference for the [`clean-code`](../SKILL.md) skill, loaded on demand. You are designing data structures, wrapping a third-party API, or handling errors.

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
