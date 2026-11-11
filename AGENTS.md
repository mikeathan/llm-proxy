# AGENTS.md — For AI coding agents

Operating contract for this repo — a local-first LLM orchestrator and agent platform (Go backend in
`backend/`, Vue 3 admin UI in `frontend/`). Read before making changes. This file is the one source of
agent instructions for every harness; keep it tool- and vendor-neutral.

## Commands

Full reference: [CONTRIBUTING.md → Quick Start](CONTRIBUTING.md#quick-start). Go 1.26.2 · no Makefile; frontend
lint runs inside `npm run build` · gitleaks pre-commit hook (enable once with `./scripts/setup-gitleaks.sh`).

```bash
cd backend && go build ./... && go test ./...            # build + test
cd backend && go run ./tools/check-complexity/           # complexity ≤12
cd frontend && npm install && npm test && npm run build  # frontend test + build
cd frontend && npm run test:visual                        # visual regression (UI/theme changes)
cd backend && go run main.go                             # :4001 (omit --data for XDG/home layout)
./scripts/check-agent-harness.sh                         # verify the agent harness stays consistent
```

## Before coding

1. Read `CONSTITUTION.md` (6 sections — the law).
2. **Classify** the work: backend / frontend / cross-cutting / docs / tests, and name the subsystem.
3. Open its hub, `docs/subsystems/<slug>.md` (list: `docs/subsystems/README.md`). It links the SPEC, the code,
   the decisions, the open plans and the **learnings ledger** — read the ledger before choosing an approach so
   you do not repeat a dead end. Read the SPEC it names.
4. Load the matching rule file — `.agents/rules/go-staff-engineer.md` (backend),
   `.agents/rules/frontend-vue-engineer.md` (frontend). Mandatory.
5. Load the phase skill(s) below; for multi-file or ambiguous work start with `task-planning`, and use
   `debugging` when something fails.
6. Run the relevant baseline before changing behavior (see Workflow); docs/harness tasks skip the build.

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
- Load the matching rule file and the affected subsystem's hub, SPEC and skill only — never all of them.
- Finished plans in `docs/PLANS/ARCHIVE/` are evidence, not instructions: open one only when its topic is in
  play; the SPECs, `docs/architecture/` and the ADRs win if they disagree.

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
- **Hand over unfinished work.** When you stop and work remains (a slice not built, a decision or measurement waiting on the
  owner, a check you could not run), write or update one handoff plan before the final message:
  `docs/PLANS/<folder>/<topic>-handoff-<date>.md`, with the plan frontmatter (`status: partial`) and a row in
  `docs/PLANS/README.md`. It states: the branch and its commit state (pushed or not, merge order); what is built and which checks
  were run or **not** run; what is left, in order, and what each item needs from the owner; traps found; and the first command or
  file to open. Replace the topic's earlier handoff instead of adding a second one, and delete the file when its work is done
  (harvest first, `documentation-stewardship`). No work left means no handoff.
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
- Before proposing, planning or building a feature, search for it first: the subsystem hub's capability list (for the network audit,
  `docs/architecture/network-capabilities.md`), the ADRs that list what was rejected or already built, and the code's own check or action
  ids. A feature found after the work has started is a failure of this step. Add a row to the capability list in the same change that
  adds a capability.

**Proceed vs. ask.**
- Do without asking: read/grep/search; run builds, tests, linters; add tests; fix a clear bug in the
  touched code; choose between equivalent repo-consistent approaches.
- Ask first (or report and stop): ambiguous/conflicting requirements; changing a SPEC-level contract, an
  architectural invariant, or a public interface; adding/upgrading heavy dependencies or CI; any git
  mutation; deleting or rewriting unrelated work; security/network/secret changes `CONSTITUTION.md` does
  not already authorize.

## Knowledge and memory

**The repo is the shared, durable memory** — for people and for every agent on any harness
(`docs/decisions/ADR-0002-docs-split-by-lifecycle.md`). Do not let learnings die with the session. Trigger
after: a non-trivial bug / edge case solved · a subsystem understood beyond the source · an architectural
decision / convention / user preference set · a non-obvious build / test / setup quirk · an approach that
failed. Route it with `documentation-stewardship`:

- what was tried and what happened → a line in `docs/learnings/<subsystem>.md`;
- a recurring code trap → a numbered pitfall in `docs/architecture/pitfalls.md`;
- a choice between alternatives → an ADR in `docs/decisions/`;
- changed behavior → the SPEC; next steps → the plan's Remaining Work.

These homes win over any tool's or skill's default location: plans and designs go in `docs/PLANS/`,
contracts in `docs/SPECS/`, never in a tool-named folder.

Test for the repo: *would a fresh clone need this?* If yes, it goes in the repo. A harness's own private
memory (if it has one) is a per-user cache for personal preferences only, never the only copy of a finding.
Save quietly at task conclusion; never block the user; never claim anything was saved unless it was. Prefer a
short, searchable entry with `file:line` pointers, verbatim values and an evidence link.

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
- **Agent instructions stay neutral:** no tool, vendor or model names in this file, `.agents/`, or the docs
  routers; the only harness-specific adapter is the pointer file named in
  `docs/decisions/ADR-0001-harness-neutral-agent-instructions.md`. The harness check enforces it.

### Isolated worktrees (opt-in — explicit approval required)

When the user explicitly asks for a worktree or isolated branch, the approved workflow is:
1. `git worktree add -b <branch> ../<repo>-<topic> HEAD` — base on committed `HEAD`, not the working tree.
2. Do all work in the new worktree; never touch the primary branch or its uncommitted changes.
3. Report results and leave changes uncommitted unless the user approves commit/push.
Do not create a worktree, branch, commit, or PR on your own initiative.

## Skills and reference (load on demand — never all at once)

`.agents/skills/<name>/SKILL.md` are Agent Skills; each `description` is the trigger and the body loads when
relevant. Route by phase:
- **Plan / research** → `task-planning` (+ `clean-code`, `engineering-practices`, the subsystem skill its hub lists).
- **Implement** → `tdd-guide`. **Verify / debug** → `debugging` for failures, `testing-guide` for tests and runs.
- **Close out** → `documentation-stewardship`. **Commit / PR text** → `pr-commit-message`.
- **Ops / config** → `llamacpp-setup`; persisted config field, default or precedence → `config-persistence`;
  inference-provider work → `provider-integration`.

Everything else is routed from `docs/INDEX.md` (subsystems, SPECs, architecture, plans, learnings, skills catalog).
Adding a frontend settings tab? → `docs/architecture/checklists.md#adding-a-frontend-settings-tab-checklist`.

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
8. Persist durable findings to the repo (see Knowledge and memory).
Applies to all task sizes and user-requested change reviews.
