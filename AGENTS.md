# AGENTS.md — For AI coding agents

Read before making changes.

## Navigation
- Start from `docs/INDEX.md` — do NOT recursively scan `docs/` with ls/find.
- Only load files the task requires (progressive disclosure). Every loaded file costs context tokens — prefer `file:line` pointers and small targeted reads over opening whole docs. When unsure whether a large doc is needed, ask first.
- **Classify first:** backend / frontend / cross-cutting / docs / tests. Load only the matching rule file and the SPEC/docs for the affected subsystem(s). For cross-cutting work, load each affected subsystem's rules. Do not pre-load all docs.

## Commands

Full command reference: [CONTRIBUTING.md → Quick Start](CONTRIBUTING.md#quick-start). Go 1.26.2 · no Makefile/lint · gitleaks pre-commit hook (enable once with `./setup.sh`, see [CONTRIBUTING.md → Git Hooks](CONTRIBUTING.md#git-hooks-secret-scanning)).

```bash
cd backend && go build ./... && go test ./...   # build + test
go run ./tools/check-complexity/                # complexity ≤12
cd frontend && npm install && npm test && npm run build   # frontend test + build
cd backend && go run main.go                    # :4001 (omit --data for XDG/home layout; --data sets the DATA root only)
```

## Workflow
- **Backend changes:** run `go build ./...` from `backend/` after each meaningful edit.
- **Frontend changes:** run `npm run build` from `frontend/` after each meaningful edit.
- **Before finishing:** backend changes require `go test ./...` + `go run ./tools/check-complexity/` from `backend/`; frontend changes require `npm test` + `npm run build` from `frontend/`. Then run Pre-Completion Review (end of file).
- **TDD required** for features/fixes — backend (`go test ./...`) and frontend (`npm test`) alike. Red→Green→Refactor. Details: `docs/skills/tdd-guide.md`.

## Execution Protocol
For non-trivial work follow: **inspect → plan → implement → verify → review → report**.
- Do not declare done while checks fail, warnings are unresolved, or scope changed without explanation.
- Never claim a check passed (build, test, lint, complexity) unless you actually ran it. Report the exact command and its result.
- Keep edits scoped to the task; do not revert, reformat, or touch unrelated existing changes.
- **Centralization:** any app-level initialization, lifecycle wiring, or long-lived background coordination (data refresh loops, global watchers/listeners, bootstrap) must be established in **one well-defined, discoverable place** (a dedicated entrypoint or init layer) rather than spread ad hoc across modules — so startup wiring is not half-forgotten or duplicated across changes.
- **Report the review, don't just pass it:** the Pre-Completion Review result must be summarized in the final message to the user (each gate: pass/fail/clean), not treated as an internal step.

## Boundaries
- **Git:** read-only inspection (`git diff`, `git status`, `git log`) is allowed; never commit, push, add, stash, reset, or checkout without explicit user approval.
- **Comments:** never remove comments unless factually incorrect — then correct the error, do not delete.
- **Secrets / telemetry / network:** governed by `CONSTITUTION.md` — comply.
- **Heavy deps / CI changes:** ask before adding or modifying.

## Before coding
1. Read `CONSTITUTION.md` (6 sections — the law).
2. Read the relevant SPEC (`docs/INDEX.md` → SPEC-001..009) for the affected subsystem only.
3. Load `.agents/rules/go-staff-engineer.md` for backend, `.agents/rules/frontend-vue-engineer.md` for frontend. Mandatory.
4. Run the relevant baseline: backend `cd backend && go build ./... && go test ./...` for backend work; frontend `cd frontend && npm test && npm run build` for frontend work (run `npm ci`/`npm install` only if `node_modules` is missing); docs/tests tasks skip build.

## Instruction Authority
On conflict, follow in this order: `CONSTITUTION.md` → this `AGENTS.md` → loaded rule file → relevant SPEC/architecture doc → existing code conventions. Ask only when the conflict cannot be resolved safely.

## Reference (load on demand)
- `docs/architecture.md` — directory map, contracts, checklists, pitfalls
- `docs/skills/README.md` — quick "when to load which skill" map
- `docs/INDEX.md` — full doc catalog
- After any change: `docs/skills/documentation-stewardship.md`
- Adding frontend settings tab? → `docs/architecture.md#adding-a-frontend-settings-tab-checklist`

## Pre-Completion Review (Mandatory Gate)
Before marking done, pass every check; fix or report failures.
1. Run relevant build, tests, and complexity checks (Workflow → Before finishing).
2. Review own diff (`git diff`) line by line.
3. Check `CONSTITUTION.md`, this file, and loaded rule file: security/input validation, network guardrails, output escaping, secrets, `ctx`, `%w`, prompts, and untrusted/LLM output.
4. Check leaks: goroutines/contexts, files/conns/rows, subscriptions/listeners, timers, queues, and unbounded growth.
5. Check bugs/perf: error paths, edge cases, nil/zero values, redundant work. Run `-race` only when concurrency/lifecycle code changed; inspect perf-sensitive paths without speculative caching or unmeasured optimization.
6. Search for existing implementations; reuse or extend them. Remove dead code, stubs, newly introduced TODOs, and incomplete work; preserve unrelated existing TODOs.
7. Confirm conventions, required documentation, tests, and production readiness; report checks run, failures, fixes, and remaining risk to user (or state "clean").
Applies to all task sizes and user-requested change reviews.
