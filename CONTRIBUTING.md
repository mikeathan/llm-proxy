# Contributing

## Quick Start

_(Canonical command reference — AGENTS.md links here.)_

```bash
# Backend
cd backend
go build ./... && go test ./...         # build + test
go run ./tools/check-complexity/        # complexity ≤12
go run main.go                          # :4001

# Frontend
cd frontend
npm install && npm run dev             # dev (proxies to :4001)
npm run build                          # production
```

## Before You Write Code

1. Read `CONSTITUTION.md` — architectural invariants (6 sections).
2. Read the relevant SPEC (`docs/INDEX.md` → SPEC-001..009).
3. Run `cd backend && go build ./... && go test ./...` for clean baseline.

## Code Standards

### Go
- Cyclomatic complexity ≤12 (`go run ./tools/check-complexity/`).
- Imports: stdlib → internal → external.
- Validate at boundaries; no secrets in logs.

### Vue / TypeScript
- Composables are singletons; `ref()` over `reactive()`.
- Services are stateless; types from `types/`.

Full Go/Vue rules: `.agents/rules/`. Architecture + directory map: `docs/architecture.md`.

## Git
- PRs only; no direct pushes to main.
- Conventional Commits format.
- AI agents: see `AGENTS.md` for git policy.

## Documentation
After any change: follow `docs/skills/documentation-stewardship.md`.
