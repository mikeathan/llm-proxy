# Clean Code — Language-Agnostic Practices — Review Checklist — Smells and Heuristics

Reference for the [`clean-code`](../SKILL.md) skill, loaded on demand. You are reviewing a diff (yours or someone else's) and want the full smells list.

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
