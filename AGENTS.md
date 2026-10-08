# AGENTS.md — For AI coding agents

Operating contract for this repo. Read before making changes.

## Instruction hierarchy (higher wins on conflict)

`CONSTITUTION.md` → this file → `.agents/rules/*` (mandatory language mechanics) → the relevant SPEC
(`docs/SPECS/README.md`) → `docs/architecture.md` → existing code conventions. Skills are procedural
guides loaded on demand; they inherit everything above. An explicit instruction in the current request
outranks repo conventions unless it violates `CONSTITUTION.md` — if you deviate, say so and why. Ask
only when the conflict cannot be resolved safely.

## Navigation (progressive disclosure)

- Start from `docs/INDEX.md` — a router to each doc catalog. Do NOT recursively scan `docs/` with ls/find.
- Load only what the task needs. Every loaded file costs context — prefer `file:line` pointers and
  targeted reads over opening whole docs. When unsure whether a large doc is needed, ask first.
- **Classify first:** backend / frontend / cross-cutting / docs / tests. Load the matching rule file and
  the affected subsystem's SPEC/skill only — never all of them.

## Commands

Full reference: [CONTRIBUTING.md → Quick Start](CONTRIBUTING.md#quick-start). Go 1.26.2 · no Makefile/lint
· gitleaks pre-commit hook (enable once with `./scripts/setup-gitleaks.sh`).

```bash
cd backend && go build ./... && go test ./...            # build + test
cd backend && go run ./tools/check-complexity/           # complexity ≤12
cd frontend && npm install && npm test && npm run build  # frontend test + build
cd frontend && npm run test:visual                        # visual regression (UI/theme changes)
cd backend && go run main.go                             # :4001 (omit --data for XDG/home layout)
./scripts/check-agent-harness.sh                         # verify the agent harness stays consistent
```

## Workflow

- **Backend edit:** run `go build ./...` from `backend/` after each meaningful edit.
- **Frontend edit:** run `npm run build` from `frontend/` after each meaningful edit.
- **Before finishing:** backend → `go test ./...` + `go run ./tools/check-complexity/`; frontend →
  `npm test` + `npm run build` (+ `npm run test:visual` when UI, theme or tokens change);
  docs/harness → `./scripts/check-agent-harness.sh`. Then run the
  Pre-Completion Review (end of file).
- **TDD required** for features/fixes — Red→Green→Refactor (`.agents/skills/tdd-guide/SKILL.md`).

## Execution Protocol

Non-trivial work follows **inspect → plan → implement → verify → review → report**.
- Do not declare done while checks fail, warnings are unresolved, or scope changed unexplained.
- Never claim a check passed (build, test, lint, complexity, harness) unless you actually ran it. Report
  the exact command and its result.
- Keep edits scoped; do not revert, reformat, or touch unrelated existing changes.
- **Centralization:** app-level initialization, lifecycle wiring, and long-lived background coordination
  (refresh loops, global watchers/listeners, bootstrap) live in **one discoverable place** (a dedicated
  entrypoint/init layer), never spread ad hoc.
- **Report the review, don't just pass it:** summarize each Pre-Completion Review gate (pass/fail/clean)
  in the final message to the user.

## Working agreements

**Verify before you act.**
- Read, don't guess: confirm a symbol, path, config key, or convention exists in the repo *before* using
  or citing it. Never mention a file/function you have not opened.
- Label your evidence: separate *repository convention* (verified), *user requirement* (quoted), and
  *your assumption* (flagged). Never present an assumption as a convention.
- If a fact is in neither the repo nor the request, say so and ask or take the smallest reversible step.
- Investigate first: find the existing implementation and tests for the area; reuse or extend them.

**Proceed vs. ask.**
- Do without asking: read/grep/search; run builds, tests, linters; add tests; fix a clear bug in the
  touched code; choose between equivalent repo-consistent approaches.
- Ask first (or report and stop): ambiguous/conflicting requirements; changing a SPEC-level contract, an
  architectural invariant, or a public interface; adding/upgrading heavy dependencies or CI; any git
  mutation; deleting or rewriting unrelated work; security/network/secret changes `CONSTITUTION.md` does
  not already authorize.

## Memory / Knowledge Persistence

Persist durable findings to the harness's long-term memory if it exposes one — do not let learnings die
with the session. Trigger after: a non-trivial bug / edge case solved · a subsystem understood beyond
the source · an architectural decision / convention / user preference set · a non-obvious build / test /
setup quirk.

Save quietly at task conclusion; never block the user; never claim memory was written unless it was.
Prefer a short, searchable summary with `file:line` pointers and verbatim values.

## Boundaries

- **Git: read-only by default.** Run only `status`, `diff`, `log`, `show`, `blame`, `rev-parse`,
  `ls-files`. Never `add`, `commit`, `push`, `reset`, `restore`, `checkout`/`switch`, `stash`, `rm`,
  `mv`, `clean`, `apply`, `rebase`, `merge`, `tag`, or `config` without explicit approval.
  - The index and history are the user's. Never stage/unstage/stash/reset to *tidy*, *repair*, or
    *diagnose* — including for a baseline or bisect. A wrong repair is worse than an untouched tree:
    **report state, never fix it.**
  - Staged files are expected (the harness stages on file writes). Never claim a git state you did not
    just verify with `git status` / `git diff --cached`.
- **Comments:** never remove comments unless factually incorrect — then correct the error, do not delete.
- **Secrets / telemetry / network:** governed by `CONSTITUTION.md` — comply.
- **Heavy deps / CI changes:** ask before adding or modifying.

### Isolated worktrees (opt-in — explicit approval required)

When the user explicitly asks for a worktree or isolated branch, the approved workflow is:
1. `git worktree add -b <branch> ../<repo>-<topic> HEAD` — base on committed `HEAD`, not the working tree.
2. Do all work in the new worktree; never touch the primary branch or its uncommitted changes.
3. Report results and leave changes uncommitted unless the user approves commit/push.
Do not create a worktree, branch, commit, or PR on your own initiative.

## Skills (load on demand — never all at once)

`.agents/skills/<name>/SKILL.md` are Agent Skills the harness surfaces automatically: only each `name` +
`description` are always in context; the body loads when relevant. **The `description` is the trigger —
there is no separate trigger field.** Route by phase:
- **Plan / research** → `task-planning`; add `clean-code` + `engineering-practices` and the affected
  subsystem skill (catalog: `docs/INDEX.md` → Skills).
- **Implement** → `tdd-guide` (+ `clean-code`, `engineering-practices`).
- **Verify / debug** → `debugging` for anything failing; `testing-guide` for authoring/running tests,
  smoke runs, record-replay, and run analysis.
- **Close out** → `documentation-stewardship`.
- **Ops / config** → `llamacpp-setup`; changing a persisted config field, default or precedence →
  `config-persistence`; inference-provider work (not search or connectors) → `provider-integration`.
- **Commit / PR text** → `pr-commit-message`.

## Before coding

1. Read `CONSTITUTION.md` (6 sections — the law).
2. Read the affected subsystem's SPEC (`docs/SPECS/README.md`).
3. Load the matching rule file — `.agents/rules/go-staff-engineer.md` (backend),
   `.agents/rules/frontend-vue-engineer.md` (frontend). Mandatory.
4. Load the phase skill(s) above; for multi-file or ambiguous work start with `task-planning`, and use
   `debugging` when something fails.
5. Run the relevant baseline before changing behavior (see Workflow); docs/harness tasks skip the build.

## Reference (load on demand)

- `docs/INDEX.md` — router to every doc catalog (SPECs, plans, audits, skills, rules).
- `docs/architecture.md` — directory map, critical contracts, file-change checklists, Common Pitfalls.
- `docs/PLANS/README.md` — live "what's left" tracker for non-complete plans.
- Adding a frontend settings tab? → `docs/architecture.md#adding-a-frontend-settings-tab-checklist`.

## Pre-Completion Review (Mandatory Gate)

Before marking done, pass every check; fix or report failures and summarize the result gate-by-gate.
1. Run the relevant build, tests, complexity, and harness checks (Workflow → Before finishing).
2. Review your own diff line by line with `git diff HEAD` — plain `git diff` hides harness-staged
   changes and silently skips most of it.
3. Check `CONSTITUTION.md`, this file, and the loaded rule file: security/input validation, network
   guardrails, output escaping, secrets, `ctx`, `%w`, prompts, and untrusted/LLM output.
4. Check leaks: goroutines/contexts, files/conns/rows, subscriptions/listeners, timers, queues, and
   unbounded growth.
5. Check bugs/perf: error paths, edge cases, nil/zero values, redundant work. Run `-race` only when
   concurrency/lifecycle code changed; don't add speculative caching or unmeasured optimization.
6. Search for existing implementations; reuse or extend them. Remove dead code, stubs, newly introduced
   TODOs, and incomplete work; preserve unrelated existing TODOs.
7. Confirm conventions, required documentation, tests, and production readiness; report checks run,
   failures, fixes, and remaining risk to the user (or state "clean").
8. Persist durable findings to harness memory if one is exposed (see Memory / Knowledge Persistence).
Applies to all task sizes and user-requested change reviews.
