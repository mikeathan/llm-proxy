# AGENTS.md — For AI coding agents

Read before making changes.

## Navigation
- Start from `docs/INDEX.md` — do NOT recursively scan `docs/` with ls/find.
- Only load files the task requires (progressive disclosure). Every loaded file costs context tokens — prefer `file:line` pointers and small targeted reads over opening whole docs. When unsure whether a large doc is needed, ask first.

## Commands

Full command reference: [CONTRIBUTING.md → Quick Start](CONTRIBUTING.md#quick-start). Go 1.26.2 · no Makefile/lint · gitleaks pre-commit hook (enable once with `./setup.sh`, see [CONTRIBUTING.md → Git Hooks](CONTRIBUTING.md#git-hooks-secret-scanning)).

```bash
cd backend && go build ./... && go test ./...   # build + test
go run ./tools/check-complexity/                # complexity ≤12
cd frontend && npm install && npm run build     # frontend
cd backend && go run main.go                    # :4001 (--data <dir> overrides)
```

## Workflow
- **Backend changes:** run `go build ./...` from `backend/` after each meaningful edit.
- **Frontend changes:** run `npm run build` from `frontend/` after each meaningful edit.
- **Before finishing:** run `go test ./...` + `go run ./tools/check-complexity/` from `backend/` — must pass.

## Boundaries
- **Git:** never run git (commit, push, add, stash, etc.) without explicit user approval.
- **Comments:** never remove comments unless factually incorrect — then correct the error, do not delete.
- **Secrets / telemetry / network:** governed by `CONSTITUTION.md` — comply.
- **Heavy deps / CI changes:** ask before adding or modifying.

## Before coding
1. Read `CONSTITUTION.md` (6 sections — the law).
2. Read the relevant SPEC (`docs/INDEX.md` → SPEC-001..009).
3. For Go/Vue patterns: `.agents/rules/`.
4. Baseline: `cd backend && go build ./... && go test ./...`.

## Reference (load on demand)
- `docs/architecture.md` — directory map, contracts, checklists, pitfalls
- `docs/skills/README.md` — quick "when to load which skill" map
- `docs/INDEX.md` — full doc catalog
- After any change: `docs/skills/documentation-stewardship.md`
- Adding frontend settings tab? → `docs/architecture.md#adding-a-frontend-settings-tab-checklist`
