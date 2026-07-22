# TDD Guide — Intelligent Test-Driven Development

Trigger: new feature, fix, or non-trivial change.
Skip: getters, setters, wiring, pass-through boilerplate. Don't test what can't
break.

## Golden Rule: Keep the suite FAST

Test time compounds. Every test must earn its place.

### Do
- **Add cases to existing table-driven tests** before creating new test files
- **One `func Test(t *testing.T)` with N `t.Run("case", ...)`** — not N test functions
- **Share setup in helpers** (`newTestStore`, `newTestAgent`) — reuse, don't recreate
- **Pure unit tests preferred** — mock I/O, no DB, clean in-memory state
- **Target <3s** for the package you're working in during Red/Green cycles.

### Don't
- Don't create new `_test.go` when a `t.Run` to an existing table suffices
- Don't test stdlib behavior (`TestSprintf`)
- Don't test trivial wiring (DI constructors, field assigns)
- Don't spin DB/resources when a mock works
- Don't add edge-case tests upfront — add after happy path passes, only if the
  edge can actually fail

## Test Level Decision

| Complexity | Test type | Example in codebase |
|---|---|---|
| Isolated algorithm / pure function | Unit (mock deps) | `TestBuildHotInjection` |
| Feature spanning packages | Integration/behavior | Agent loop with MockClient |
| New subsystem | Integration + smoke | `testing-guide.md` patterns |

Prefer integration tests over unit tests when the agent handles the feature —
one acceptance criteria = one test cycle. Let the agent write unit tests for
internal helpers during the refactor step.

## TDD Flow

### 1. Red — Write failing test first

- Table-driven: `func TestFoo(t *testing.T)` with `t.Run` cases for each scenario
- Check existing test files — add a `t.Run` case before creating a new file
- **Run and see it fail**: `go test ./pkg/... -run TestFoo -count=1`
  - This prevents the agent from over-implementing before confirming a real gap
- The test _is_ the spec — make the test name describe the behavior clearly

### 2. Green — Minimal code to pass

- Only code the test needs. No "future-proofing." No dead code (Constitution IV.4).
- Run: `go test ./pkg/...` — must pass
- **Hardest step for agents** — coach it if it adds more than the test covers

### 3. Refactor — Clean up while green

- Extract helpers, simplify, improve names
- Keep tests passing: `go test ./pkg/...` after each meaningful change
- Ask the agent: "Clean up the logic but keep all tests green"
- Add unit tests for internal helpers if needed

### 4. Verify — Full suite once (not per-edit)

Run once at the end:
```bash
cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/
```

## Edge Cases

- Add after happy path is green
- Only if the edge case can actually trigger a failure in real usage
- Add as a new `t.Run` case to an existing table, not a separate test function

## References

- Project patterns: `testing-guide.md`, `engineering-practices.md`
- Constitution IV.5: Mock the interface, not the implementation
- go-staff-engineer.md §5: Table-driven tests, sub-test pattern
