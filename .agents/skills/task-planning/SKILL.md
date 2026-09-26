---
name: task-planning
description: "Plan and execute non-trivial changes: classify, find the governing SPEC, trace the existing code path, scope, decompose into verifiable steps, record a plan, and resume safely. Use before multi-file/multi-subsystem or ambiguous work, and to understand a subsystem before changing it."
when_to_use: "A change spans multiple files/subsystems, is ambiguous, needs a SPEC, or will run over many steps; or you must understand how existing code works before changing it."
status: reference
last_reviewed: 2026-09-26
---

# Task Planning — Recon, Scope, Decompose, Verify

**Trigger:** multi-file / multi-subsystem / ambiguous / long-running change; or you need to understand a subsystem before changing it.
**Skip:** a one-line fix with a known path — go straight to the fix with `tdd-guide`.

The execution protocol (`AGENTS.md`) is **inspect → plan → implement → verify → review → report**. This skill makes each phase concrete and resumable.

## Phase 1 — Classify (do this first)

Label the work **backend / frontend / cross-cutting / docs / tests** and load *only* that layer's rule file + the affected SPEC(s):

- Rules: `.agents/rules/go-staff-engineer.md` (backend), `.agents/rules/frontend-vue-engineer.md` (frontend) — mandatory.
- SPEC: `docs/INDEX.md` → `docs/SPECS/SPEC-NNN-*.md` for the affected subsystem only.

Multi-subsystem work loads each affected subsystem's pieces, not all of them.

## Phase 2 — Recon: find and trace the existing path

Do not design against imagined code. Find the real path first.

1. **Read the governing SPEC** — if the behavior is contracted, the SPEC is the source of truth (e.g. agent loop = SPEC-001, memory = SPEC-004, budget = SPEC-005, guardrails = SPEC-006, run scheduler/residency = SPEC-007, connectors = SPEC-009).
2. **Map the subsystem** with `docs/architecture.md` (directory map, interfaces, Common Pitfalls, File Change Checklist).
3. **Trace with targeted grep** (name the seam, not the folder):

   | Looking for | Start here |
   |---|---|
   | HTTP route | `internal/app/routes.go` → `internal/transport/http/handlers/` |
   | Tool | `models/tools.go` → `core/tools/manifests/<tool>.json` → `core/tools/<category>.go` → `core/assistant/registry.go` |
   | Prompt text | `core/assistant/prompts/templates.go` (only home) |
   | Model field | `models/` → `transport/http/handlers/{registry_handlers,admin_handlers,admin_view}.go` → `core/llm/manager.go` → `internal/testing/mocks/manager.go` |
   | Event | `assistant/agent_events.go` producer → `core/eventbus/` → `/live` SSE → frontend handler |
   | Run state / lanes | `core/runlane/` (+ SPEC-007 §V) |
   | Frontend flow | `composables/` → `services/` → `types/` (load the affected UI skill) |

4. **Read the change checklist for the change type** in `docs/architecture.md` before editing:
   - new model field / new tool / new tool category / new connector / new endpoint / new settings tab — each has an ordered file list.
5. **Find the existing tests** that cover the area (`rg -n "TestX|describe\("`, `go test ./<pkg>/ -run TestX -count=1`). Improve the implementation, don't duplicate it.

## Phase 3 — Scope and decide whether to write a plan doc

Write a plan doc at `docs/PLANS/<subsystem>/<kebab-name>.md` when **any** holds:

- it touches more than one subsystem, or roughly >5 files;
- it changes a SPEC-level contract or an architectural invariant;
- it will span more than one session (you need durable state to resume);
- it has phases with real uncertainty.

Plan doc shape (match the existing plans and `docs/PLANS/README.md`):

- front matter/status line: `proposed` → `approved` → `active`/`partial` → `complete` (`superseded`/`reverted` stay archived);
- **Related Specs** + **Constitution refs**;
- phases, each with an explicit acceptance criterion;
- a **Remaining Work** line for anything not done.

Register the plan: add it to `docs/PLANS/README.md` (Active Plans + Remaining Work) and to the plans table in `docs/INDEX.md`.

**SPEC-first:** if the change alters contracted behavior, update the SPEC, not just the code — see `docs/SPEC-change-management.md` (propose/change/deprecate). A new invariant is formalized in `CONSTITUTION.md`.

## Phase 4 — Decompose into verifiable steps

Break the work into the smallest steps that each end with a proof:

```
step: <smallest change>
verify: <exact command>  → expected: <PASS / specific output>
```

- Put the uncertain/risky step **first** (de-risk before building on it).
- Each step should keep the tree building and the suite green.
- If a step cannot be verified by a command, the step is too big — split it.
- Check for an existing implementation before adding one (`AGENTS.md` Pre-Completion Review #6): reuse or extend.

## Phase 5 — Execute incrementally

- One step at a time; run that step's verify command before moving on.
- Behavior changes: TDD (`tdd-guide`). Structural gates: `go build ./...`, `go vet ./...`, `go run ./tools/check-complexity/`, `npm run build`.
- Keep edits scoped. Do not fix or reformat unrelated code you pass through.

## Phase 6 — Close out

- Run the full relevant gate (`AGENTS.md` → Workflow / Pre-Completion Review) and report it gate-by-gate.
- Update docs per `documentation-stewardship` (SPEC, plan + Remaining Work, `docs/INDEX.md`, `architecture.md` pitfalls, `CONSTITUTION.md` if an invariant changed).
- Mark the plan `complete`/`partial` and move it/archive as the conventions require.

## Resumability and failure handling

- **The repo is the resumption state.** A committed/updated plan doc + status + Remaining Work is how a later session picks up. Do not keep the plan only in your head.
- **If the plan is wrong:** stop, re-scope, and update the plan *before* continuing. Never silently widen scope (`AGENTS.md` Execution Protocol).
- **If a step fails:** switch to `debugging`; do not pile a workaround on top.
- **If you discover unrelated breakage:** note it (issue/plan), leave the tree consistent, keep to scope.

## Completion criteria

- Every step's verify command passed and was reported; the full gate is green.
- The governing SPEC and docs are updated; a plan (if written) reflects reality.
- No unrelated changes; `git diff HEAD` reviewed; Pre-Completion Review summarized to the user.

## Related

- `clean-code`, `tdd-guide`, `testing-guide` — design and test the steps.
- `debugging` — when a step fails.
- `documentation-stewardship` — the close-out doc pass.
