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

# One-time: install deps + enable secret-scanning git hook
./scripts/setup-gitleaks.sh            # installs gitleaks, registers .githooks
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

## Releases

Merging a PR to `main` automatically tags a release — no deployment, no artifacts:

- **Normal merge** → the `release.yml` workflow auto-bumps the patch version
  (`0.7.0 → 0.7.1`), commits the `VERSION` file back, pushes tag `v0.7.1`, and creates a
  GitHub Release with auto-generated notes.
- **Want a specific version** (patch `0.7.1`, minor `0.8.0`, major `1.0.0`)? Edit the root
  `VERSION` file in the PR — on merge that exact version is tagged verbatim (no extra bump).
- The `VERSION` file at the repo root is the single source of truth.
- `scripts/build.sh` still derives the build version from the latest git tag; binary
  artifacts attached to releases are not produced yet (see
  `docs/PLANS/cross-cutting/ci-github-actions-and-versioning.md` §3.4).

## Git Hooks (secret scanning)

A pre-commit hook blocks commits that contain secrets (API keys, tokens, private keys).

**Dependency:** [gitleaks](https://github.com/gitleaks/gitleaks) — `brew install gitleaks`.

**One-time enable:** from the repo root run `./scripts/setup-gitleaks.sh`. It installs the gitleaks
dependency and registers the hook automatically:

```bash
./scripts/setup-gitleaks.sh  # brew install gitleaks && git config core.hooksPath .githooks
```

Hooks are version-controlled under `.githooks/`, but Git does not auto-enable them —
that is what the script's `git config core.hooksPath .githooks` step does (do it once
per clone).

- Hook script: `.githooks/pre-commit` (runs `gitleaks git --staged`).
- Allowlist / rules: `.gitleaks.toml` — add false-positive fixtures here.
- If gitleaks is not installed the hook warns and skips (does not block commits).
- Ignored secret files (`secrets.json`, `config.json`, `.env*`) are enforced via `.gitignore`.

## Documentation
After any change: follow `docs/skills/documentation-stewardship.md`.
