# backend/AGENTS.md

This folder is part of the repository whose contract is [`../AGENTS.md`](../AGENTS.md) — read that first; it
outranks this file. This file only adds what applies to Go code under `backend/`:

- Load the mandatory Go rules: [`../.agents/rules/go-staff-engineer.md`](../.agents/rules/go-staff-engineer.md).
- Find your subsystem's hub in [`../docs/subsystems/`](../docs/subsystems/README.md) (SPEC, code, decisions,
  plans, learnings).
- After each meaningful edit: `go build ./...`. Before finishing: `go test ./...` and
  `go run ./tools/check-complexity/` (complexity ≤12).
- Prompt strings live only in `internal/core/assistant/prompts/templates.go` (CONSTITUTION II.13).
- `internal/core/assistant/prompts/AGENTS.template.md` is product data (the notes stub seeded into user
  workspaces), not instructions for you.
