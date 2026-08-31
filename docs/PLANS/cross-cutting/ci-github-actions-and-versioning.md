---
id: PLAN-CI-001
status: proposed
created: 2026-08-30
owner: mikeathan
related: CONTRIBUTING.md, CONSTITUTION.md, .gitleaks.toml, backend/main.go (buildinfo)
---

# CI, Code Hygiene & (Deferred) Release Flow (Cross-Cutting)

> **Status: P1 (CI) implemented** in `.github/workflows/ci.yml` (PRs + pushes to `main`,
> manual dispatch; jobs: `secrets`, `backend`, `frontend`). P2/P3 hygiene work pending;
> see §4. **P4 (tag job) implemented** in `.github/workflows/release.yml` + root `VERSION`
> file (go-ahead given 2026): merge to `main` → auto patch bump (or tag the `VERSION`
> file verbatim if it was raised in the PR), annotated tag, GitHub Release with
> auto-generated notes. **No build/deploy artifacts** — the `build-release` job (P5) is
> deliberately not implemented; `scripts/tag_version.sh` is still in place until the
> manual-bump path is verified on a real merge (P6).
> All tooling must be **free and self-hosted in GitHub Actions** — no external paid services,
> no SaaS scanners (no Codecov/SonarCloud), no third-party hosted runners.

## 1. Goal

Establish a production-ready, zero-cost **CI** flow on GitHub Actions:

1. **CI** on every PR and push to `main` — build, tests, **code hygiene** (static analysis,
   vetting, complexity), secret scanning, and **coverage/reporting** so issues are caught
   before merge.
2. **Deferred (CD):** versioning & tagging on merge to `main` + release artifacts — design
   kept below (§3.4) for future execution.

## 2. Current State (investigated 2026-08-30)

| Aspect | State |
|---|---|
| Workflows | **None** — no `.github/` directory exists |
| Secret scanning | `.gitleaks.toml` present (default ruleset + test-fixture allowlists); pre-commit hook in `.githooks/` (manual opt-in via `./setup.sh`) |
| Code analysis | `go vet` available; custom complexity gate `backend/tools/check-complexity/` (≤12); ESLint + `vue-tsc` run inside `npm run build`. **No** staticcheck/gosec/govulncheck; **no** coverage anywhere |
| Versioning | `backend/main.go` declares `Version/Commit/BuildDate` vars, injectable via `-ldflags "-X main.Version=..."`; `--version` flag prints them. **Manual flow today:** `scripts/tag_version.sh` (hardcoded `VERSION="v0.7.0"`, edited by hand) creates + pushes an annotated tag; `scripts/build.sh` derives the version from the latest `git tag --sort=-v:refname` and injects it via ldflags. No CHANGELOG |
| Frontend version | `frontend/package.json` → `"version": "0.0.0"` (unused) |
| Go | `go 1.26.2` (`backend/go.mod`); module root is `backend/`; CGO via `mattn/go-sqlite3` |
| Tests | Backend: `cd backend && go build ./... && go test ./...`; Frontend: `cd frontend && npm test` (vitest) + `npm run build` |

## 3. Design

### 3.1 CI workflow — `.github/workflows/ci.yml`

Triggers: `pull_request` (all branches) + `push` to `main`.
Add `concurrency` group per ref with `cancel-in-progress: true` to save free minutes.

Jobs (run in parallel):

1. **`secrets`** — gitleaks over full history (`fetch-depth: 0`).
   Run the **pinned gitleaks binary** downloaded from its GitHub release (or
   `go install github.com/gitleaks/gitleaks/v8@<pinned-version>`) — NOT
   `gitleaks/gitleaks-action@v2`, which requires a paid license for private repos.
   Command: `gitleaks detect --source . --config .gitleaks.toml --redact`.
2. **`backend`** — `actions/setup-go@v5` with `go-version-file: backend/go.mod`
   (has built-in module cache), `working-directory: backend`:
   1. `go build ./...`
   2. `go vet ./...`
   3. `go test ./... -race -coverprofile=coverage.out -covermode=atomic`
   4. `go run ./tools/check-complexity/` (existing ≤12 gate)
   5. Coverage report (see §3.2)
3. **`backend-hygiene`** — static analysis, see §3.2 (can reuse the setup-go cache;
   separate job so hygiene findings never slow/block the build signal).
4. **`frontend`** — `actions/setup-node@v4` (node 24, `cache: npm`,
   `cache-dependency-path: frontend/package-lock.json`), `working-directory: frontend`:
   1. `npm ci`
   2. `npm test -- --coverage` (vitest, see §3.2)
   3. `npm run build` (already includes `eslint` + `vue-tsc` type-check)

### 3.2 Code hygiene & reports (the "catch issues before merge" layer)

All free, all in-runner. Reports are published as **job artifacts** and inline
**`$GITHUB_STEP_SUMMARY`** tables (visible on the PR run page) — no Codecov or any
external report service.

**Backend:**

| Check | Tool (free) | Gate? | Notes |
|---|---|---|---|
| Static analysis | `staticcheck` (pinned, via `go install honnef.co/go/tools/cmd/staticcheck@<ver>`) | **advisory first** → gate later | Best-of-breed Go linter; expect an initial findings backlog — run report-only, append findings to the step summary, triage, then flip to `--fail` |
| Go vet | built-in | **gating** | Cheap, zero false positives |
| Vulnerabilities | `govulncheck ./...` (`golang.org/x/vuln`, official Go team tool, queries the public Go vuln DB) | **advisory first** → gate on high | Dependency+call-graph level CVE detection; runs in-runner, no SaaS |
| Security lint | `gosec` (pinned) — optional, P2b | advisory | Noisy on first run; include only after staticcheck backlog is triaged |
| Complexity | existing `tools/check-complexity` | **gating** | Already the project standard (≤12) |
| Race detector | `go test -race` | **gating** | Catches concurrency bugs pre-merge; slower run, acceptable |
| Coverage | `go test -coverprofile` → total % + per-package table | **report only** (no threshold initially) | Write `go tool cover -func=coverage.out` summary to `$GITHUB_STEP_SUMMARY` (total % bolded, packages below 50% flagged); upload `coverage.out` + `coverage.html` (`go tool cover -html`) as artifact, retain 14 days. Add an ratchet/threshold gate (e.g. fail if total drops >2% vs `main`) only after a baseline run shows real numbers |

**Frontend:**

| Check | Tool (free) | Gate? | Notes |
|---|---|---|---|
| Lint | ESLint (already in `npm run build`) | **gating** | Already enforced |
| Type check | `vue-tsc` (already in `npm run build`) | **gating** | Already enforced |
| Coverage | vitest built-in coverage: `npm test -- --coverage` (V8 provider, no extra dep) | **report only** initially | Requires `vitest` coverage config (add `coverage.include: ['src/**']`); write total + per-file table to `$GITHUB_STEP_SUMMARY` and upload `coverage/` artifact, retain 14 days. Same later-ratchet approach as backend |
| Dependency audit | `npm audit --omit=dev` | **advisory** | Report-only in summary; gating would fail on transitive noise |

**Design rules:**
- Gating checks = the ones already trusted locally (`build`, `vet`, `test`, complexity,
  gitleaks, eslint, vue-tsc, race). Advisory checks = anything with a possible initial
  backlog (staticcheck, govulncheck, npm audit) — they report but don't block until
  triaged, then get promoted to gating by flipping a flag.
- Every report lands in the PR run page (step summary) or artifacts — nothing leaves
  GitHub.

### 3.3 Branch protection (manual, one-time, GitHub UI)

After first green run: require CI checks (`secrets`, `backend`, `frontend` —
`backend-hygiene` joins when P2 lands) to pass before merge to `main`. Documented in
CONTRIBUTING so it isn't forgotten.

### 3.4 Release/versioning workflow (partially implemented — tag job only)

> The tag job below is **implemented** in `.github/workflows/release.yml` (no-deploy
> scope, go-ahead given 2026). The `build-release` job (step list at the end of this
> section) remains **not implemented** — release artifacts/deployment are parked.

**Chosen flow: single `VERSION` file + auto-patch, manual minor/major.** No
Conventional Commits, no release-please — versioning is decoupled from commit messages.

**Version source of truth:** a `VERSION` file at the repo root (single line, e.g. `0.7.0`).
This replaces the hardcoded `VERSION="v0.7.0"` inside `scripts/tag_version.sh` (the script
itself is retired once the workflow is live).

**Trigger:** `push` to `main` only.

**DECIDED:** the `VERSION` file always holds the **full `X.Y.Z`** (not just `X.Y`). It is
the floor for manual bumps; the workflow **never pushes commits to `main`** (branch
rulesets require PRs and the `GITHUB_TOKEN` cannot bypass them — `github-actions[bot]`
is not a bypass-eligible installed app).

> **Revised (post-implementation):** the original design had the workflow commit the
> auto-bumped version back to `main`. That push was rejected by the repository
> ruleset on `main`, so the decision logic was changed: auto-bump now derives from
> the **latest tag** instead of writing back to the file. See below.

**Job `tag` (release job, all shell — no third-party actions):**
1. Read `VERSION` file → `FILE_VER` (e.g. `0.7.1`).
2. Read latest tag: `git tag --sort=-v:refname | head -n1` → `v0.7.0`.
3. Decision (revised: no commits to `main`):
   - **`FILE_VER` > `LATEST_VER`** (dev edited the file — a manual **patch, minor, or
     major** bump via PR) → tag **`v$FILE_VER` verbatim**.
   - **`FILE_VER` <= `LATEST_VER`** (equal or stale) → **auto-bump the latest tag's
     patch**: tag `v0.7.0` → tag `v0.7.1`. The `VERSION` file on `main` is NOT raised;
     it is only consulted as a manual-bump floor.
4. `git tag -a vX.Y.Z -m "Release vX.Y.Z"` + push tag.
5. `gh release create "$TAG" --generate-notes` (auto-generated notes from merged PRs —
   free, no commit-message convention needed).

**DECIDED:** docs-only and every other merge release a patch too — no path filters; the
release job runs unconditionally on push to `main`.

**DECIDED:** rapid-merge race is acceptable — the job re-fetches the latest tag right
before tagging and fails loudly if it moved (manual re-run). Fine for a
single-maintainer repo.

**Job `build-release` (`needs: tag`, always runs on tag creation):**
1. `setup-go` from `backend/go.mod`.
2. `go build -trimpath -ldflags "-s -w -X main.Version=$TAG -X main.Commit=$GITHUB_SHA -X main.BuildDate=<utc>" -o ../dist/llm-proxy-linux-amd64 .` (from `backend/`)
   — same ldflags contract as `scripts/build.sh`, so `--version` output stays consistent.
3. Attach the binary to the release via `gh release upload "$TAG" dist/* --clobber` with
   `GH_TOKEN: ${{ github.token }}` — no third-party upload action needed.

Workflow `permissions`: `contents: write` only.

**Operator cheat-sheet:**
- Normal merge to `main` → patch bump off the latest tag, tag, release. Zero action
  required; no release commit is created.
- Want a specific version — patch `0.7.1`, minor `0.8.0`, or breaking `1.0.0` — edit
  `VERSION` in a PR → on merge, that exact version is tagged and released.

## 4. Implementation Phases

### Now — CI (P1–P3)

| Phase | Work | Verify |
|---|---|---|
| P1 | Add `ci.yml` with gating jobs: `secrets` (gitleaks), `backend` (build/vet/test+race+coverage), `frontend` (npm ci/test+coverage/build) — §3.1, minus advisory hygiene | YAML parses (`actionlint` if available / `yq`); push on a test branch and watch the run; gitleaks passes with current allowlists; coverage tables appear in step summary; artifacts uploaded |
| P2 | Add `backend-hygiene` job: staticcheck + govulncheck in **advisory** mode (report to step summary, don't fail) | Findings list visible on PR run; job green even with findings |
| P3 | Triage the staticcheck/govulncheck backlog; fix real issues (TDD); promote staticcheck to gating; docs: CONTRIBUTING.md "CI" section (workflow overview, advisory vs gating policy, how coverage reports are produced, branch-protection checklist) | `backend-hygiene` red on a deliberately broken commit, green otherwise; docs review per `documentation-stewardship.md` |

### Later — CD (P4+, parked until CD design is revisited)

| Phase | Work | Verify |
|---|---|---|
| P4 | **Implemented** — root `VERSION` file (`0.7.0`) + `release.yml` tag job (manual-bump-verbatim / auto-patch off latest tag per revised §3.4), tag + release notes only, no artifacts, no commits to `main` | On next merge to `main`: tag `v0.7.1` appears + GitHub Release with auto-generated notes exists |
| P5 | Add `build-release` job (ldflags injection, binary attach) | Release contains `llm-proxy-linux-amd64`; binary prints `--version` = `v0.7.1` |
| P6 | Manual-bump path test: PR editing `VERSION` (e.g. `0.8.0`), merge, confirm tag `v0.8.0` (no extra bump). Retire `scripts/tag_version.sh`; keep `scripts/build.sh` as-is | Merge result + tag correctness |

P1–P3 are independently mergeable and are the current scope. P4+ reuses the P1 CI file
as-is; nothing in P1–P3 needs rework when CD lands.

## 5. Open Questions (decide before execution)

Resolved (2026-08-30): file holds full `X.Y.Z` and is always tagged verbatim; docs-only
merges release too; race mitigation accepted (all CD decisions — §3.4).
Resolved: staticcheck + govulncheck included in CI, advisory-first, promoted to gating
after triage (P2/P3).

1. **staticcheck pin version** + whether the initial backlog justifies fixing pre-existing
   findings as part of P3 or deferring (advisory remains report-only until then).
2. **Coverage thresholds**: confirm report-only is fine for launch; ratchet/threshold
   design after a baseline run produces real numbers (backend + frontend).
3. **CD scope** (revisit when CD work resumes): initial version `0.7.0`; release artifacts
   (linux-amd64 only, or arm64/macOS?); gitleaks pin version.
4. Commit hygiene during execution: note that gitleaks CI scans **full history** — if any
   pre-existing secret exists in history, the `secrets` job fails on first run. Run
   `gitleaks detect` locally (binary already installed at `/opt/homebrew/bin/gitleaks`)
   before enabling, and extend `.gitleaks.toml` allowlists only for verified fixtures.

## 6. Constraints & Guarantees

- All actions used are first-party (`actions/checkout`, `actions/setup-go`,
  `actions/setup-node`) or run fully in-runner (gitleaks binary, staticcheck,
  govulncheck, shell-based tag job). Free on public **and** private repos; no license
  keys, no external SaaS — coverage/reports stay in GitHub (step summaries + artifacts).
- No changes to backend/frontend code required for CI (buildinfo already exists).
  Coverage adds vitest config (`coverage.include`) and pinned tool installs — no new
  heavy npm/Go dependencies (staticcheck/govulncheck are CI-only installs, never in
  `go.mod`).
- Does not touch the in-flight `task/implement_perfomance_improvements` work — all new
  files are repo-root workflow/config files plus doc updates.
- Constitution: no product-code changes, so no architectural invariants affected;
  secrets handling unchanged (gitleaks config reused as-is).
