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
npm install && npm run dev             # dev (proxies to :4001); /admin/design = token reference
npm test                               # unit + component (Vitest)
npm run lint                           # ESLint + palette ratchet + preset contrast gate (frontend/scripts/check-palette.mjs, check-contrast.cjs)
npm run test:visual                    # visual regression (Playwright, installed Chrome)
npm run build                          # production (runs lint + type check first)

# Full build (frontend assets + backend binary) — the single build script the
# installer calls; also runnable standalone. setup.sh runs it inside its TUI.
./scripts/build.sh

# One-time: install deps + enable secret-scanning git hook
./scripts/setup-gitleaks.sh            # installs gitleaks, registers .githooks
```

## Before You Write Code

1. Read `CONSTITUTION.md` — architectural invariants (6 sections).
2. Read the relevant SPEC (`docs/SPECS/README.md`).
3. Run `cd backend && go build ./... && go test ./...` for clean baseline.
4. Working with an AI agent? It follows `AGENTS.md`; docs/instruction changes must pass
   `./scripts/check-agent-harness.sh`.

## Code Standards

### Go
- Cyclomatic complexity ≤12 (`go run ./tools/check-complexity/`).
- Imports: stdlib → internal → external.
- Validate at boundaries; no secrets in logs.

### Vue / TypeScript
- Composables are singletons; `ref()` over `reactive()`.
- Services are stateless; types from `types/`.
- Colours come from design tokens (`src/styles/tokens.css`) via semantic classes
  (`bg-surface-raised`, `text-muted`, `border-hairline`, `text-state-error`) —
  never raw palette classes (`bg-gray-800`). The palette ratchet fails lint if a
  file gains one; after migrating a file run `node scripts/check-palette.mjs --update`.
- Product name: import `PRODUCT_NAME` / `APP_TITLE` from `src/config/brand.ts`.

Full Go/Vue rules: `.agents/rules/`. Architecture + directory map: `docs/architecture.md`.

## Git
- PRs only; no direct pushes to main.
- Conventional Commits format.
- AI agents: see `AGENTS.md` for git policy.

## Releases

Merging a PR to `main` automatically tags a release — no deployment, no artifacts:

- **Normal merge** → the `release.yml` workflow auto-bumps the latest tag's patch version
  (`v0.8.0 → v0.8.1`), pushes the tag, and creates a GitHub Release with auto-generated notes.
  It never pushes commits to `main`, so the `VERSION` file is not updated.
- **Want a specific version** (patch, minor `0.9.0`, major `1.0.0`)? Edit the root `VERSION`
  file in the PR — when it is higher than the latest tag, the merge is tagged with exactly that
  version (no extra bump).
- The **git tags are the release record**; the `VERSION` file is only a floor for the next release.
- `scripts/build.sh` stamps the binary with `git describe --tags` (nearest `v*` tag: exactly
  `v0.8.0` on a tagged commit, `v0.8.0-2-gabc1234` after it), fetching tags first because the
  release tag lands a few seconds after the merge. Binary artifacts attached to releases are not
  produced yet (see `docs/PLANS/cross-cutting/ci-github-actions-and-versioning.md` §3.4).

## Git Hooks (secret scanning + agent harness + CRAP scores)

A pre-commit hook blocks commits that contain secrets and keeps the agent-instruction docs consistent.

**Dependency:** [gitleaks](https://github.com/gitleaks/gitleaks) — `brew install gitleaks` (optional;
the secret scan is skipped when it is absent, but the harness check still runs).

**One-time enable:** from the repo root run `./scripts/setup-gitleaks.sh`. It installs the gitleaks
dependency and registers the hook automatically:

```bash
./scripts/setup-gitleaks.sh  # brew install gitleaks && git config core.hooksPath .githooks
```

Hooks are version-controlled under `.githooks/`, but Git does not auto-enable them —
that is what the script's `git config core.hooksPath .githooks` step does (do it once
per clone).

- Hook script: `.githooks/pre-commit` — runs `gitleaks git --staged`, then
  `scripts/check-agent-harness.sh` when instruction/doc files are staged.
- Allowlist / rules: `.gitleaks.toml` — add false-positive fixtures here.
- If gitleaks is not installed the secret scan warns and is skipped; the agent-harness
  check still runs and blocks on inconsistency. Bypass either with `git commit --no-verify`.
- Ignored secret files (`secrets.json`, `config.json`, `.env*`) are enforced via `.gitignore`.

The same checks run in CI on every PR (jobs `secrets` and `agent-harness`).

### Fast commit scan and full CI CRAP report

Install the pinned [TypeScript scanner](https://github.com/hbaldwin98/crap) and
[Go-native scanner](https://github.com/jadenmaciel/gauntlet) once (Go and a C compiler
are required), and make sure Go's binary directory is on `PATH`:

```bash
go install github.com/hbaldwin98/crap/cmd/crap@8eea294df1123778a77ecb826f0b16d95b65768d
go install github.com/jadenmaciel/gauntlet/cmd/crap4go@v0.2.0
export PATH="$(go env GOPATH)/bin:$PATH" # use GOBIN instead if explicitly configured
```

The existing pre-commit hook runs `bash scripts/crap-staged.sh`. It reads staged versions of
changed `.go`, `.ts`, and `.tsx` files, excluding tests, declarations, and test fixtures. It runs
no tests and performs no downloads. Without coverage it reports a conservative estimate assuming
0% coverage, rather than reusing stale coverage. Scores are report-only; analysis errors block
the commit. An absent scanner prints a setup reminder and skips the report. `.vue` files are
not analyzed. Run `bash scripts/test-crap-staged.sh` to verify the hook's selection and error handling.

CI's `crap` job scans all production Go/TS source using the fresh Go coverprofile and Vitest
Cobertura report produced by the existing test jobs. Full scores appear in the workflow summary
and the `crap-reports` artifact. Scores above the tool's default threshold of 30 are flagged but
do not fail CI; missing coverage is visibly treated as 0%. CI fails if analysis itself fails.
The scanner versions are pinned in CI and in the install commands above; update them together.
The separate Go scorer is necessary because the TypeScript scanner's bundled Go grammar rejects
Go 1.26 `new(value)` expressions. `crap4go` needs Go 1.26.4+ to build (Go can download that
toolchain during installation); the commit scan itself disables toolchain and dependency downloads.

## Documentation
After any change: follow `.agents/skills/documentation-stewardship/SKILL.md`. Verify the doc/agent
catalog still resolves with `./scripts/check-agent-harness.sh`.
