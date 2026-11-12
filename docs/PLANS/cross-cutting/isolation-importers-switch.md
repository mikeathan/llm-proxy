---
status: built
date: 2026-11-12
last_reviewed: 2026-11-12
related_specs: [SPEC-006]
constitution_references: [IV.6]
evidence: Read on 2026-11-12 at branch audit-handoff-2026-11-12 (HEAD 4476ac07, v0.82.0) - `grep -rn 'platform/sandbox"\|platform/progexec"' backend --include='*.go'` (importers outside platform/sandbox, platform/progexec and platform/isolation); backend/internal/platform/isolation/isolation.go (the re-exports); backend/internal/platform/privhelper/client/client.go:17,48,69; backend/internal/archtest/boundary_test.go:294,694,774; plan stacks-as-packages-and-network-stack.md "Remaining Work" (says 22 importers); docs/PLANS/README.md row for that plan (says 17). Built 2026-11-12 on branch agents-stack-watch-and-isolation-close-out (uncommitted): `go test ./internal/archtest/ ./internal/platform/... ./internal/app/` pass; the new test failed on `privhelper/client` before the switch.
---

# Switch the remaining direct `sandbox`/`progexec` importers to `isolation`

Parent: [`stacks-as-packages-and-network-stack.md`](stacks-as-packages-and-network-stack.md) (Remaining Work, "switching the 17 remaining
direct importers to `isolation`"). Part of the Network stack close-out; no behaviour change.

## 1. The count is stale

The parent plan said 22 importers (2026-11-06), the plans README says 17. **Measured 2026-11-12, nothing outside the isolation
library imports `progexec`, and two files outside it import `sandbox`:**

| File | Uses | Why it still imports `sandbox` |
|---|---|---|
| `internal/platform/privhelper/client/client.go:17` | `sandbox.EffectiveState` (line 48), `sandbox.EnforcementNone` (line 69) | the interlock reads the sandbox's effective state |
| `internal/platform/privhelper/client/client_test.go:17` | the same types in test doubles | same |

The rest of the 22 were switched by the Network moves (#209 onward: `app`, `core/tools/terminal.go`, `shell`, `toolregistry`, `netaudit/*`,
the helper and `main.go` all import `isolation` today). `platform/progexec` imports `sandbox` and `isolation` imports both: that is the
library itself and stays.

So the job is **two files and a test that keeps it so**, not seventeen.

## 2. Scope

In: switch the two files to `isolation`'s re-exports (`isolation.EffectiveState`, `isolation.EnforcementNone`); add the fitness test that
stops a new direct importer; correct the two stale counts.

Out: any change to `sandbox`, `progexec` or `isolation` behaviour; moving the privhelper client into a stack (ADR-0034 keeps the
helper core); the 12 Network sensors (done, ADR-0038).

## 3. Steps (TDD: the test first, watch it fail)

1. **Red.** In `backend/internal/archtest/boundary_test.go` add `TestOnlyTheIsolationLibraryImportsTheSandboxAndTheRunner`: over the
   `go list` data the file already loads, fail when any package other than `platform/isolation`, `platform/progexec` (it imports
   `sandbox`), `platform/sandbox` and `platform/progexec` themselves imports `llm-proxy/internal/platform/sandbox` or `.../progexec`
   in production code. Add a negative case in the style of the existing `helperImportViolations` tests (a fake package that imports
   `sandbox` is reported). **Expected first result: it fails on `privhelper/client` only**; if it names anything else, the count above
   is wrong and this plan is updated before going on.
2. **Green.** `client.go`: import `isolation`, change `Sandbox.Effective()` to return `isolation.EffectiveState` and the check to
   `isolation.EnforcementNone`. `client_test.go`: the same. These are aliases (`isolation.go:20-33`), so the types are identical and no
   caller in `internal/app/privhelper.go` changes; confirm with `go build ./...`.
3. **Allowlist check.** `helperAllowed` (`boundary_test.go:693`) and the privhelper package rules (`:680`) must still pass: `client`
   is not the helper package, but read the privhelper import rules once more after the edit and run `go test ./internal/archtest/`.
4. **Docs.** Fix the README row (17 to "none left outside the library, fitness test added") and the parent plan's Remaining Work line;
   one learnings line in `docs/learnings/platform.md` (the count drifts, so the plan's number was wrong; the test is the guard now).
   Then `./scripts/check-agent-harness.sh`.

## 4. Verify

```bash
cd backend && go build ./... && go test ./internal/archtest/ ./internal/platform/... ./internal/app/ -count=1
cd backend && go run ./tools/check-complexity/
./scripts/check-agent-harness.sh
```

Also run the new test once with the `client.go` edit reverted to see it fail (the repo's convention for fitness tests). Linux-only
code is untouched, so the colima run is not needed (the alias change compiles on both).

## 5. Risks and decision for the owner

- **Risk:** none at runtime; the types are aliases of the same types. The test could be too strict if a future package legitimately
  needs `sandbox` directly: the answer is to extend `isolation`, not to loosen the test (ADR-0033).
- **Decision needed:** none that is SPEC-level. This is a mechanical change; it needs a branch, which per AGENTS.md waits for
  your approval. Suggested version bump: patch (0.82.1).

## Remaining Work

None. Section 3 steps 1 to 3 are built (`TestOnlyTheIsolationLibraryImportsTheSandboxAndTheRunner` in `internal/archtest/boundary_test.go`, `privhelper/client` and its test now use `isolation`); step 4 is this note, the README rows and the learnings line. Version: bundled with the Agents stack change, no separate bump.
