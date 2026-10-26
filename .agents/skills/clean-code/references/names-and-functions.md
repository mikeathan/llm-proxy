# Clean Code — Language-Agnostic Practices — Names and Functions

Reference for the [`clean-code`](../SKILL.md) skill, loaded on demand. You are naming things or shaping a function (size, arguments, side effects).

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
  line/complexity limits (see `engineering-practices`; `check-complexity` gate).

---
