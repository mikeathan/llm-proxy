---
status: complete
date: 2026-08-07
related_specs: [CONSTITUTION III.2, CONSTITUTION III.4, CONSTITUTION III.6]
---

> **Post-completion note (2026-08-11):** The two-root XDG layout described below (ConfigDir vs DataDir) is now **superseded by a single-root consolidation**. Per operator request, `Paths.ConfigDir` and `Paths.DataDir` both resolve to the same physical directory (default `~/.config/llm-proxy`). All files — `settings.yml`, `registry.json`, `secrets.json`, `master.key` (+`.hash`), `orchestrator.db`, `templates/`, `meta/`, `runs/`, `logs/` — live under one root. The resolution precedence, key-management encryption, and all typed accessors are preserved; only the directory layout changed. CONSTITUTION III.4/III.6 reflect the new single-root design.

# XDG Config/Data Relocation + Storage Cleanup + Reset Controls

**Status:** complete
**Date:** 2026-08-07
**Scope decision:** legacy-layout support and boot migration (original Phase 8) are **removed** — the app targets the final XDG layout directly; fresh installs seed via `paths.SeedDefaults`, existing installs are advised to delete legacy files / run `factory-reset`. See "Not supporting legacy installs" below.
**Related:** CONSTITUTION III.2 (Atomic Persistence), III.4 (Three-Tier Configuration Model), III.6 (Secrets Are Encrypted)

---

## Implementation Progress (2026-08-07)

**Phases 0–11 complete.** Storage core rewritten with full test scaffolding and race-clean under `-race`:

- **Phase 6** — `models.AppConfig` is the single persisted root over `settings.yml`; `DataManager.System()`/`.Settings()`/`.HostSettings()` are facade views (single-owner invariant: every write persists the merged document under one mutex). `run_logging` is one canonical field. `HostSettingsStore` (storage, `host.json`) **deleted** — sandboxing merged into `AppConfig.Sandboxing`; the three facade views share one `onProjectionChange` helper and project from a shallow read (`GetProjected`). Per-view `OnChange` filters by projection change so a host-settings write no longer fires system/settings subscribers.
- **Phase 7** — relocation onto typed `Paths` accessors: `settings.yml`/`registry.json`/`master.key(+hash)` in `ConfigDir`; `secrets.json`/`orchestrator.db`/`meta/`/`runs/`/`logs/`/`templates/` in `DataDir`. `orchestrator.db` opens from `Paths.DatabaseFile()`. No legacy reads. `main.go` boot reordered: resolve → seed → logger at `Paths.LogsDir()` → stores → `LoadAll` → `Watch`; `--data` is empty-by-default so omitting it reaches env/XDG/home. Templates extracted from `//go:embed` on first run (extract missing only).
- **Phase 9** — `workspaces_dir` defaults to `~/llm-proxy/workspaces` when unset.
- **Phase 10** — `POST /admin/api/system/factory-reset` + `/clear-runtime-data`: staged temp-build → validate → atomic swap (allowlist paths only); file-managed key regenerated, env-managed key reused; stores reloaded quiet (no mid-reset `OnChange`) and the `SecretStore` rebuilt around the effective key; watcher restarted after reset. `clear-runtime-data` deletes `meta/*/sessions`, `process.log`, `.lock`, `runs/`, `logs/` and recreates the required empty dirs. Tests in `reset_test.go`.
- **Phase 11** — `paths.EnforcePermissions()` fails startup if `master.key`/`.hash`/`secrets.json`/`settings.yml`/`registry.json` are group/world-readable or symlinked; roots tightened to 0700.

**Follow-up hardening (same day):**
- **Post-reset secret invariant at runtime** — `AppContext.FactoryReset` re-points the live runtime registrar (`SetSecrets`) at the rebuilt `SecretStore` and issues exactly one reconciliation (`Sync` + `ApplyModelOverrides`) after the quiet reload; the watcher is restarted on the app root context, not `context.Background()`.
- **`secretsStore` pointer swap is mutex-guarded** (`secretsMu`) so concurrent `Secrets()` reads during reset are race-clean under `-race`.
- **Swap failure safety** — `swapFile`/`backupFile` propagate errors; a failed mid-swap rolls the backed-up set back into place (key/ciphertext never diverge). Covered by `TestFactoryReset_SwapFailureRollsBack` and `TestFactoryReset_SecretsConcurrentAccess`.
- **`clear-runtime-data` active-state guard + logger reopen** — refuses while assistant/automation runs or live shell sessions exist; after clearing, the file logger is reopened so writes do not go to a deleted inode. Covered by `TestClearRuntimeData_RejectsActiveWork`/`_AllowsWhenIdle` and `TestFileLogger_ReopenAfterLogDirRemoved`.
- **Factory-reset active-runs guard** — refuses to wipe the registry/settings while assistant/automation runs are executing. Covered by `TestFactoryReset_RejectsActiveRuns`.

---

### Decision: relocate directly to the target layout — no legacy support

We do **not** need to keep the legacy layout working. The "no files move yet / transitional" deferral is dropped. `NewDataManager` and every storage constructor must target the **final** XDG layout from the moment Phase 6/7 lands — no dual-path transitional code, no `config.json`-in-data-root compat, no `RootDir()`-join fallback to legacy locations. This removes a whole class of correctness risk (two sources of truth, silent-empty fallback) that the original transitional plan carried.

Consequence for the `--data` flag: explicit `--data <dir>` sets **DataDir only**; `ConfigDir` follows the env/XDG/home chain (`internal/platform/paths/paths.go:Resolve`). The portable/self-contained option is `LLM_PROXY_HOME=<dir>` (`<dir>/config` + `<dir>/data`). We deliberately do **not** implement the "collapse both roots under `--data`" wording from the precedence table — collapsing would put `settings.yml` and `secrets.json` under one dir, defeating the CONFIG/DATA separation that is the whole point of this plan. The 0700/0600 file modes remain the real protection in portable mode.

Consequence for first run: a fresh install is created entirely under the target layout by `paths.SeedDefaults` (which already writes the target locations: `ConfigDir/settings.yml`, `ConfigDir/registry.json`, `ConfigDir/master.key`(+`.hash`), `DataDir/secrets.json`, etc.). There is no legacy to read from.

### Not supporting legacy installs / no migration phase

Existing installs that still have files under the legacy locations (`backend/data/{config,registry,secrets}.json`, `~/.config/llm-proxy/<ws>/` metadata) are **not** automatically migrated. Rationale: the value of supporting legacy is low (pre-1.0, single-developer/early-adopter user base) and the cost/risk of a migration path (Phase 8 in the original plan) is high (boot-time assertions, `.migrated` bookkeeping, `--no-migrate` flag, partial-migration resume). Instead:

- **Fresh installs** get the correct layout via `SeedDefaults` — zero migration needed.
- **Existing installs** are advised (release note / log warning) to remove the old `backend/data/*` files and let the app reseed; or run `factory-reset` (Phase 10) which builds a clean target-layout set. We will NOT read legacy files at all, so there is no silent fallback to stale data.
- **Phase 8 (boot migration + `--no-migrate`) is removed** from scope. The boot "old-present/new-absent" assertion from the original plan is replaced by a startup *warning* if legacy `backend/data/{config,registry,secrets}.json` are detected, telling the operator to delete them — not a fatal error, not an auto-move.

This keeps the plan smaller and the code single-path. If, after Phase 6/7, real users report pain, a one-shot migration can be added later as its own small task — but it is out of scope here.

### Done in this pass

- **Phase 7 templates** — `//go:embed` (`backend/data/templates/embed.go`) + extract-on-first-run in `TemplateStore` (extract missing only; never overwrite user-edited templates). Fresh installs now get the 8 shipped defaults at `Paths.TemplatesDir()`.
- **Phase 10 frontend** — Reset Controls (Factory Reset + Clear Runtime Data) added to `SecuritySettings.vue` with confirmation + toast, wired to the two new endpoints.
- **Phase 10 factory-reset guard** — `AppContext.FactoryReset` refuses while assistant/automation runs are active (`TestFactoryReset_RejectsActiveRuns`); clear-runtime-data already guarded (`TestClearRuntimeData_RejectsActiveWork`).

### Resolved / out of scope

- **Phase 10 deep quiesce/resync** — **not implemented by decision.** Factory reset empties the registry, so the single post-reset `Sync()` already reconciles the runtime (an empty catalogue leaves nothing to rebuild), and reset deliberately leaves `workspaces/`/`meta/`/shell state untouched. A process restart after factory reset remains the recommended path, standard for an operator-level destructive action.
- **Full uninstall ("wipeout")** — added after Phase 10 as `POST /admin/api/system/wipeout` (UI: "Wipeout (Uninstall)" in `SecuritySettings.vue`). Unlike factory-reset/clear-runtime-data it removes the entire single root *and* the workspaces directory, then stops the process. Guarded against active work and against wiping `/`, `$HOME`, or an ancestor of `$HOME` (see `storage/reset.go` `validateWipeTarget`).
- **Phase 12 docs** — done in this branch (CONSTITUTION III.4/III.6, AGENTS.md, README, docs/INDEX.md, `.gitignore`, `git rm --cached backend/data/config.json`); the previously-flagged dangling `docs/services/llm-proxy.service` no longer exists.

**Phase 8 (boot migration) is removed** — we do not support legacy installs (see "Not supporting legacy installs" above).

---

## Overview

`config.json`, `registry.json`, and `secrets.json` currently live in the `--data` root (`backend/data/`, inside the git working tree). `settings.yml`, `host.json`, and `master.key` live in `~/.config/llm-proxy`. Per-workspace runtime state — including a 2.8 MB rotating `process.log` — also lives in `~/.config/llm-proxy`.

This plan relocates configuration and application data into a conventional two-directory XDG layout, unifies the three competing config-directory resolvers, fixes the correctness/performance/security defects found in the storage package along the way, and adds two operator reset controls to the Settings UI.

The storage package has **zero test files** today (18.3% statement coverage) and this plan rewrites most of it, so test scaffolding lands before any refactor.

**Scope boundary:** `workspaces/` is user-owned working content and is **not** moved by this plan. Only its default value changes, and only when unset.

---

## Problem Statement

1. **Three independent config-dir resolvers.** `getMetadataDir` (env-aware, `manager.go:165`), `keys.go:27` (hardcoded `$HOME`), `host_settings.go:23` (hardcoded `$HOME`, bare-CWD fallback).
2. **Secrets and registry live in the repo tree.** `backend/data/secrets.json`, `backend/data/registry.json`. `backend/data/config.json` is git-tracked while the other two are gitignored.
3. **Config dir polluted with runtime state.** `~/.config/llm-proxy/<workspace-id>/` holds `process.log`, `.lock`, `state.json`, `sessions/`.
4. **Duplicated config concept.** `run_logging` exists in both `SystemConfig` (`infrastructure.go:18`) and `UserSettings` (`infrastructure.go:69`), reconciled ad-hoc by `RunLoggingEnabled()` (`app_context.go:248-257`) and written by `ApplySystemUpdate` (`app_context.go:371-373`).
5. **Silent-empty failure mode.** `store.go:40-48` returns a zero value for a missing file — any relocation without migration boots with an empty registry and empty secrets, with no error.
6. **Tests mutate real user config.** ~15 `NewDataManager` sites resolve `MetadataDir()` to the developer's real `~/.config/llm-proxy`.
7. **No first-run seeding contract.** Defaults are produced ad-hoc (master key in `keys.go`, `DefaultHostSettings()` in the host store); there is no single "create a valid userdata directory from nothing" path, and nothing tests it.

---

## Target Layout

```
~/.config/llm-proxy/            0700    CONFIG — text you edit
  settings.yml                          (absorbs config.json + host.json)
  registry.json
  master.key
  master.key.hash

~/.local/share/llm-proxy/       0700    DATA — app internals
  secrets.json
  orchestrator.db  (+ -wal, -shm)
  templates/
  meta/<workspace-id>/                  config.yaml, state.json, sessions/, .lock, process.log
  runs/
  logs/llm-proxy.log

<operator choice>/workspaces/           USER FILES — unchanged, governed by workspaces_dir
  <workspace-id>/
```

Rule: **if you would open it in an editor it belongs in CONFIG, otherwise DATA.**

### Path precedence

Order below is **highest → lowest** (first match wins). `--data` only ever matches when explicitly
supplied; an omitted flag is the only way to reach the env/XDG tiers.

```
explicit --data <dir>   sets DATA root only; ConfigDir still follows env/XDG/home.
                        Portable/self-contained mode is LLM_PROXY_HOME=<dir>
                        (<dir>/config + <dir>/data) — see "Decision" above.
LLM_PROXY_CONFIG_DIR    DEPRECATED. Sets ConfigDir ONLY (back-compat with the old
                        metadata-dir-only behaviour). Never overrides an explicit --data.
LLM_PROXY_HOME=<dir>    ConfigDir = <dir>/config, DataDir = <dir>/data.
XDG_CONFIG_HOME / XDG_DATA_HOME
~/.config + ~/.local/share                                 (default, macOS AND Linux)
```

**`LLM_PROXY_CONFIG_DIR` scope is fixed:** it relocates `ConfigDir` only (matching today's behaviour
where it redirects the metadata/settings dir). `DataDir` follows the normal XDG/default resolution.
Startup warns if `LLM_PROXY_CONFIG_DIR` and `LLM_PROXY_HOME` are both set, and prefers
`LLM_PROXY_CONFIG_DIR` for `ConfigDir` only.

**Flag-presence is mandatory.** `main.go` currently defaults `--data` to `"data"`, so the
flag is *always* set and XDG/`LLM_PROXY_HOME` can never take effect. Phase 0 must detect
whether `--data` was explicitly supplied (e.g. a `dataFlag` pointer + `flag.Visit`, or default
empty string and resolve only when non-empty). An omitted `--data` is the only way to reach env/XDG.

**`LLM_PROXY_HOME` expansion is fixed:** `<dir>/config` (CONFIG) and `<dir>/data` (DATA). It does
NOT collapse both roots into `<dir>` itself, and does NOT invent `.config`/`.local` subdirs.

**Independent-root fallback is defined.** The existing fallback only redirects the metadata dir; it
does not define what happens when one root is unwritable. Phase 0 must specify, for each root:

- Config root unwritable but Data root writable → fail startup (config is mandatory).
- Data root unwritable but Config root writable → fail startup (DB/secrets are mandatory).
- Both unwritable → fail startup.
- A configured path is a symlink or a regular file (not a dir) → fail startup, do not follow/overwrite.
- Only `LLM_PROXY_HOME=<dir>` intentionally co-locates both roots (as `<dir>/config` + `<dir>/data`); `--data` sets DataDir only and never collapses roots, and env/XDG modes must keep CONFIG and DATA distinct (see Decision above).

`os.UserConfigDir()` must **not** be used — it returns `~/Library/Application Support` on darwin and would break macOS/Linux parity with the existing install.

### Security rationale

`master.key` stays in CONFIG while `secrets.json` ciphertext moves to DATA. Key and ciphertext are never in the same directory, so no single `cp -r`, backup tarball, or dotfile sync captures both — in the default XDG/home layout. `--data` never collapses the roots (see Decision above), so this key/ciphertext separation holds in every layout; the 0700/0600 file modes remain the real protection. Both directories are 0700, all secret files 0600.

---

## Engineering Findings (addressed by this plan)

### Correctness / data races

| # | Finding | Location |
|---|---------|----------|
| C1 | `Store.Get()` returns `*s.data` — a **shallow** copy. `ModelOverrides` (map) and `Guardrails` (pointer) are handed live into `ApplyModelOverrides` and `GetGuardrails`, which read them under a *different* mutex while `Store.Update` mutates them. Genuine data race + torn reads. | `store.go:79-88`, `app_context.go:340` |
| C2 | Watcher self-reload loop. `WriteAtomic` creates a temp file and renames it; fsnotify fires Create/Rename; the handler reloads the file it just wrote and re-fires `OnChange` (secrets → `AppContext.Sync`). Redundant reload + listener fan-out on every save. | `manager.go:68-97`, `atomic.go:17-38` |
| C3 | Debounce is a blocking `time.Sleep(100ms)` **inside** the event loop — a burst of N events serializes into N×100 ms and no events are coalesced. | `manager.go:77` |
| C4 | Master-key load failure only prints a warning, then constructs the secret store with a nil key. Every decrypt then fails and returns empty silently, so a wrong or missing key presents as "no credentials" rather than an error. | `manager.go:42-45`, `secrets_store.go:26-30` |

### Performance

| # | Finding | Location |
|---|---------|----------|
| P1 | `getDecrypted()` AES-GCM-decrypts and JSON-unmarshals the entire secret blob on **every** read. It is on the request hot path via `GetProviderKeys` / `GetResolvedProviderKey`. | `secrets_store.go:20-42` |
| P2 | `isWritableDir` creates and deletes a probe file on **every** `getMetadataDir()` call, and `MetadataDir()` is called repeatedly during startup. | `manager.go:161-200` |
| P3 | `GetDefaultGuardrails` re-reads 5 manifests from disk (via `runtime.Caller`, a doomed `os.ReadFile` in production) plus 2 `json.Unmarshal` each, on every call. Manifests are static. | `app_context.go:342`, `manifests.go:28-60` |

### Security / permissions

| # | Finding | Location |
|---|---------|----------|
| S1 | `WriteAtomic` creates the parent directory with `0755`. Correct for a data dir, too open for config/secrets. | `atomic.go:14` |
| S2 | `master.key` is created with `os.WriteFile`, leaving a umask/TOCTOU window. Needs `O_WRONLY\|O_CREAT\|O_EXCL, 0600`. | `keys.go:67` |
| S3 | No startup check that config/secret files are not group- or world-readable. | — |

### Dead code

| # | Finding | Location |
|---|---------|----------|
| D1 | `AppContext.SetEnvironment` is an unused stub that returns nil. | `app_context.go:337` |
| D2 | `GetDefaultGuardrails(root)` and `LoadManifest(root, ...)` / `LoadManifestAsTool(root, ...)` accept `root` and never use it — they resolve via `runtime.Caller(0)`. The `s.rootDir` argument is threaded for nothing. | `manifests.go:28,62,129`, `app_context.go:342` |
| D3 | `ConversationDeps.RootDir()` declared in the interface, never called in that package. | `conversation_service.go:29` |
| D4 | `SecretData.Version` / `EncryptedSecretData.Version` are read and copied but never written, incremented, or validated. | `models/secrets.go:8,13` |
| D5 | `LOG_FILE` is set in `.env.development` / `.env.production` and never read by any Go code. | `backend/.env.*` |
| D6 | `fmt.Printf` used for storage and security diagnostics instead of the structured logger. | `store.go:140`, `secrets_store.go:28,34,122`, `manager.go:44,94` |

---

## Phases

Each phase ends with `go build ./...` and `go test ./...` from `backend/`. TDD (Red→Green→Refactor) per `docs/skills/tdd-guide.md`.

### Phase 0 — `internal/platform/paths` package (new)

Create the single source of truth for on-disk locations.

```go
type Paths struct{ ConfigDir, DataDir string }

func Resolve() (Paths, error)      // precedence chain above, dirs created 0700
func (p Paths) SeedDefaults() error // idempotent first-run seeding
```

`Resolve()` keeps the `isWritableDir` probe (`manager.go:189`) as a **fail-fast** check that a mandatory root is writable, but computes it **once** and caches the result (fixes **P2**). The old `<data>/.internal` fallback is **deleted**: metadata now lives under `DATA/meta` (a mandatory root), so the fallback target is dead, and the precedence rules above require an unwritable mandatory root to fail startup rather than silently collapse.

`SeedDefaults()` must, when a target is missing:
- create `ConfigDir` and `DataDir` at `0700`
- write a default `settings.yml` (merged defaults: server, workspaces_dir, metrics, sandboxing, local, memory, run_logging)
- write a default `registry.json`
- generate `master.key` + `master.key.hash` using `O_EXCL` (fixes **S2**) — see format note below
- write an `secrets.json` containing a valid encrypted empty payload

**All storage constructors must take resolved paths, not resolve env/home themselves.**
`GetMasterKey()` (`keys.go:12-19`), `NewHostSettingsStore()` (`host_settings.go:16-28`), and
`getMetadataDir()` (`manager.go:165`) each independently resolve `~/.config/llm-proxy`. After Phase 1/2
they must accept the resolved `ConfigDir`/`DataDir`/`MetadataDir` from `paths.Paths`:
```go
GetMasterKey(configDir string) ([]byte, error)
NewHostSettingsStore(configDir string) *HostSettingsStore
NewDataManager(paths Paths) (*DataManager, error)
```
Phase 2 is no longer purely behaviour-preserving: the three resolvers must be deleted and replaced by
calls that thread `Paths`. No storage component may call `os.UserHomeDir()` after Phase 2.

**SQLite opens only after the final DataDir is selected.** `orchestrator.db` (`app_context.go:137`),
`-wal`, and `-shm` are one logical database set and must be created/opened at `Paths.DatabaseFile()`
(`DATA/orchestrator.db`). The database parent dir must be created before `db.Open`, and the DB must be
closed before any reset/data-deletion step. Phase 0/7 must open SQLite only via `Paths.DatabaseFile()` — never against a legacy root (legacy roots are not read; see "Not supporting legacy installs").

**Startup ordering must change.** Today `initLogger()` runs before `NewDataManager()` (`main.go:44-48`)
and the file logger resolves `logs/llm-proxy.log` relative to the executable dir. Because logs move to
`DATA/logs/`, the boot sequence must be: load env → `paths.Resolve()` → create/seed dirs → init logger at
`Paths.LogsDir()` → construct stores → `LoadAll` → start watcher → open SQLite → build runtime services.
A fallback logger (stderr) is required for the window before `Paths` is resolved or seeding fails.

**Tests (`paths_test.go`)**
- table-driven precedence matrix: explicit `--data` vs omitted `--data`, `LLM_PROXY_HOME`, `XDG_*_HOME`, `LLM_PROXY_CONFIG_DIR` (and conflict with `--data`), defaults, and the non-writable-root failure cases
- `Resolve()` creates both dirs at `0700`
- `SeedDefaults()` on a completely empty `$HOME` produces a loadable `settings.yml`, `registry.json`, a `master.key` whose `.hash` verifies, and a `secrets.json` that **round-trips** (encrypt empty → decrypt → empty map, no error)
- **master.key format:** the file contains the 32-byte key as a 64-character hex string (current behaviour, `keys.go:56-72`). The test must assert *valid 64-char hex decoding to 32 bytes*, NOT a raw 32-byte file. Any move to binary storage requires a format version + compatibility read path for existing hex keys.
- `SeedDefaults()` is idempotent and never overwrites an existing file
- file modes: dirs `0700`, `master.key` / `master.key.hash` / `secrets.json` `0600`; `settings.yml` / `registry.json` `0600` (config files can carry provider endpoints)

### Phase 1 — Storage test scaffolding

The package is untested and about to be rewritten. Add before touching anything:

- `store_test.go` — load/save round-trip (JSON + YAML), missing-file zero value, `Get()` isolation, `OnChange` fan-out, concurrent `Get`/`Update` under `-race`
- `atomic_test.go` (extend) — directory and file mode assertions
- `keys_test.go` — generate, reload, hash integrity, tamper detection, `LLM_MASTER_KEY` env path, `O_EXCL` behaviour
- `secrets_store_test.go` — encrypt/decrypt round-trip, masking, masked-key resolution, cascade on delete, wrong-key behaviour
- `host_settings_test.go` — read defaults, write, reload
- `manager_test.go` — `LoadAll`, watcher reload, watcher ignores temp files

### Phase 2 — Unify the three resolvers (refactor + path injection)

Point `keys.go:27`, `host_settings.go:23`, and `getMetadataDir` at `paths`. Everything resolves to `~/.config/llm-proxy` (now via `Paths.ConfigDir`); no files move yet at this phase. This is **not** purely behaviour-preserving: the three standalone env/home resolvers must be deleted and replaced with calls that thread the resolved `Paths` (see Phase 0). Delete the duplicated `$HOME` logic. No storage component may call `os.UserHomeDir()` after this phase. Suite stays green; no migration required.

### Phase 3 — Fix correctness, performance, security defects

- **C1** — `Store.Get()` returns a deep copy (clone maps, clone `Guardrails` pointer). Add the concurrent `-race` regression test first.
- **C2/C3** — watcher: ignore files matching the `WriteAtomic` temp pattern; coalesce events per (dir, filename); replace the in-loop `time.Sleep` with a debounce timer so the event loop never blocks.
- **Watcher lifecycle (new, not just debounce):** `Watch(ctx context.Context) error`; `Close()` waits for the watcher goroutine to exit; debounce timers are stopped on shutdown; listener callbacks must not run after reset/shutdown completes; the watcher must be stoppable and restartable around factory reset; no goroutine may use an unowned `context.Background()`. The current `Watch()` (`manager.go:61-111`) starts an untethered goroutine and `Close()` only closes fsnotify — fix both.
- **C4** — master-key failure becomes a loud structured error with an explicit degraded flag surfaced to the admin API, not a `fmt.Printf` warning.
- **P1** — cache the decrypted secret view; invalidate on `Update` and on `Load`.
- **P3** — cache the merged default guardrails (manifests are static).
- **S1** — `WriteAtomic` takes a `dirPerm`; config/data callers pass `0700`. `WriteAtomic` must also tighten permissions on an *existing* parent directory when it is more open than `dirPerm` (MkdirAll does not tighten); for config/secret dirs this means chmod to `0700` on create-or-exist.
- **D6** — replace `fmt.Printf` diagnostics with the structured logger.

### Phase 4 — Test isolation (gates Phase 7)

Add `t.Setenv("LLM_PROXY_HOME", t.TempDir())` at all 15 `NewDataManager` sites in `internal/app/app_context_test.go`, `bootstrap_test.go`, `app_test.go`. Also update the existing `t.Setenv("LLM_PROXY_CONFIG_DIR", ...)` at `app_context_test.go:65` to `LLM_PROXY_HOME` so it does not conflict with the resolved precedence (an explicit `LLM_PROXY_CONFIG_DIR` would win `ConfigDir` and point the suite back at a shared location).

Without this, Phase 7 makes the suite read and mutate the developer's real `secrets.json` and `registry.json`. The 38 `NewPathResolver` sites pass explicit temp dirs and are unaffected.

### Phase 5 — Dead code removal

Remove **D1**, **D2** (drop the unused `root` parameter from `GetDefaultGuardrails`, `LoadManifest`, `LoadManifestAsTool` and their call sites), **D3**, **D5**. For **D4**, wire `Version` properly — initialise to `1` on write and validate on read so schema drift is detectable — since the field is the natural place to hang any future secrets-format migration.

### Phase 6 — Merge `config.json` + `host.json` into `settings.yml`

Introduce `models.AppConfig` as the single persisted root:

```yaml
server:            # from config.json
workspaces_dir:    # from config.json
metrics:           # from config.json
sandboxing:        # from host.json
local:             # existing settings.yml
guardrails:        # existing
model_overrides:   # existing
memory:            # existing
run_logging:       # single field — resolves the duplication
```

One `Store[AppConfig]` over `settings.yml`. `DataManager.System()` and `.Settings()` survive as thin facades over the shared store so the ~40 call sites and `ApplySystemUpdate` (`app_context.go:352`) do not all churn at once. Delete the `run_logging` overlay in `RunLoggingEnabled()` (`app_context.go:248-257`) and the `sys.Server.RunLogging` write in `ApplySystemUpdate` (`app_context.go:371-373`). Delete `HostSettingsStore`.

**Single-owner invariant (critical).** Because `System()` and `Settings()` are facade views over one store, every read must read the whole `AppConfig`, and every write must update and persist the *complete merged document* under one mutex. Concurrent system vs settings writes must serialize through that single mutex; a failed write must leave both views unchanged; the watcher reload must replace both views atomically; `OnChange` listeners must never observe a half-updated document. Without this, system and user updates overwrite each other.

**`run_logging` canonical field.** The codebase currently has `SystemConfig.Server.RunLogging` (`infrastructure.go:18`) and `UserSettings.RunOutput` (`infrastructure.go:69`), while `RunLoggingEnabled()` (`app_context.go:248-257`) reads only the system field. Phase 6 must define: the canonical Go field and YAML location for `run_logging`, the read-compatibility mapping for existing `settings.yml`, the write path, and CLI `--enable-runs` precedence over the stored value. Add a test proving UI save, config reload, and runtime execution all read the same value.

**`HostSettingsStore` removal keeps the existing API/frontend contract.** The frontend still calls `GET/PUT /admin/api/host` (`SecuritySettings.vue`, `AdminApiService.fetchHostSettings/updateHostSettings`) and `app_context.go:51` constructs `NewHostSettingsStore()`. Phase 6 must either (a) keep those endpoints as facades over merged `settings.yml.sandboxing`, or (b) update the backend handlers, the frontend API calls, and the `SecuritySettings.vue` state together. Deleting the store alone breaks the Security Settings tab.

Write a characterization test over `ApplySystemUpdate` **before** refactoring it.

`registry.json` stays a separate file — it is machine-written on every UI save and merging it would let the writer destroy hand-authored YAML comments. `secrets.json` stays separate — different format, different lifecycle, different permissions.

### Phase 7 — Relocate (target final layout directly, no legacy reads)

There is no legacy to migrate from (Phase 8 removed). `NewDataManager` and every storage
constructor must point at the **final** XDG layout via the typed `Paths` accessors. No code
path may read `config.json`/`registry.json`/`secrets.json`/`orchestrator.db`/`templates/` from a
legacy root, and no `RootDir()`-join fallback to a legacy location is retained. If a legacy file
is found at a legacy location, it is ignored (and a startup warning advises deletion — see Phase 8
removal note below), never read.

| Item | Final location (via `Paths`) |
|------|------|
| `settings.yml` | `ConfigDir` (`ConfigFile()`) |
| `registry.json` | `ConfigDir` (`RegistryFile()`) |
| `master.key` + `master.key.hash` | `ConfigDir` (`MasterKeyFile()` / `MasterKeyHashFile()`) |
| `secrets.json` | `DataDir` (`SecretsFile()`) |
| `orchestrator.db` (+wal/shm) | `DataDir` (`DatabaseFile()`) |
| `meta/<ws>/` | `DataDir/meta/<ws>/` (`MetadataDir()`) |
| `runs/` | `DataDir/runs/` (`RunsDir()`) |
| `llm-proxy.log` | `DataDir/logs/` (`LogsDir()`) |
| `templates/` | `DataDir/templates/` (`TemplatesDir()`) — `//go:embed` + extract-on-first-run |

`templates/` requires `//go:embed` of the 8 shipped defaults plus extract-on-first-run, mirroring the existing prompt embed at `dispatcher_handlers.go:363` — otherwise a fresh install has no templates. **Template semantics:** extract ONLY missing templates; never overwrite user-edited templates automatically; factory reset leaves templates untouched; if shipped-template upgrades are needed, track an embedded template version separately.

**Typed path accessors replace ambiguous `RootDir()` joins.** Update every direct join so no component derives a managed location from a bare `RootDir()`:
```go
ConfigFile() RegistryFile() SecretsFile() DatabaseFile()
TemplatesDir() MetadataDir() RunsDir() LogsDir()
```
Affected sites include `app_context.go` (System/Settings/Registry store paths), `bootstrap.go`, `executor.go`, `assistant_handlers.go` (recording paths), template paths, and the file logger. `RootDir()` is deleted or kept only as a thin alias to `DataDir()` — never as a storage-location authority.

**SQLite opens from `Paths.DatabaseFile()`.** `orchestrator.db`/`-wal`/`-shm` are one logical set; the DB parent dir is created before `db.Open` and the DB is closed before any reset/deletion step. The "silent empty" risk from the original plan (`store.go` returning a zero value for a missing file) is mitigated by `SeedDefaults` always producing a valid target-layout set on first run, plus the C4 fatal-master-key check.

**Watcher must watch the new dirs.** `Watch` already adds `Paths.ConfigDir` and `Paths.DataDir` (`manager.go`). Confirm every store's file lives under one of those two roots after relocation so no file is silently un-watched. This also feeds Phase 10's restart step.

`resolver.go`'s guarantee that workspace metadata lives **outside** the workspace tree (so agent file tools cannot read their own state, `resolver.go:64-66`) must be preserved — `MetadataDir()` under `DataDir/meta` satisfies this.

### Phase 8 — REMOVED (no legacy support)

Boot migration is out of scope: we do not support legacy installs. There is no `*.migrated`
bookkeeping, no `--no-migrate` flag, no partial-migration resume. Instead:

- A fresh install is fully created under the target layout by `paths.SeedDefaults`. No migration.
- On startup, if legacy files are detected at `backend/data/{config,registry,secrets}.json` or
  `~/.config/llm-proxy/<ws>/` metadata, log a **warning** advising the operator to delete them; do
  **not** read them and do **not** fail startup. The app proceeds with the target-layout set.
- Operators with an existing install who want a clean slate use `factory-reset` (Phase 10), which
  builds a correct target-layout set from scratch.

This removes an entire risky subsystem (atomic copy + rename + boot assertions + flag) and keeps
the code single-path. If real demand appears later, a one-shot migration can be a separate task.

### Phase 9 — `workspaces_dir` default

Default to a directory next to the running binary (`./workspaces` relative to the
executable), falling back to the launch directory under `go run`/test harnesses
(where the binary lives in a throwaway OS temp dir). Existing configured values
are untouched and no workspace files are ever moved. Fix the already-incorrect
helper text at `GlobalSettings.vue`.

**Default/relativity semantics.** `WorkspacesDir()` (`manager.go:368`) resolves:
an absolute `workspaces_dir` as-is; a relative value against the data root; an
*empty* value to the executable-relative default (falling back to the working
directory). `workspaces_dir` remains an override, settable via the UI or
`settings.yml` before launch. Changing `workspaces_dir` at runtime requires
restart: `SetWorkspacesDir()` (`app_context.go:179`) only mutates
`AppContext.resolver`, but `WorkspaceManager` is built separately in
`bootstrap.go:75` with its own resolver — already-created services keep the old
path.

### Phase 10 — Reset controls

Two endpoints and two Settings buttons, each behind a confirmation modal with toast feedback. Reset
endpoints operate on a **known allowlist** of paths derived from `paths.Paths` — never a recursive wipe
of a directory root — so a misresolved path cannot delete unrelated data.

**Reset is not a file delete + reseed against a live process.** The running app holds in-memory store
values, the active master key, a decrypted secret cache, the runtime model catalogue, model processes,
the dispatcher, and active shell sessions. Merely deleting files on disk does not reset the running
application. Phase 10 must define an explicit reset lifecycle:

1. Reject/quiesce new work (stop accepting requests or mark app quiescent).
2. Stop the dispatcher and active model processes.
3. Stop the storage watcher.
4. Clear runtime caches (including the decrypted secret view and merged guardrails cache).
5. Perform the staged file reset (see below).
6. Reload stores **without firing `OnChange`** into the live runtime: use a reload-without-notify path (or suppress the `AppContext` subscribers) so the empty/default `registry.json`/settings do not trigger a mid-reset `manager.Sync()`. After the full reload completes, issue exactly one reconciliation.
7. Reconstruct the `SecretStore`/`DataManager` with the **new** master key — `Store.Load()` only re-reads the file and cannot swap the key that the secret store captured at construction.
8. Rebuild/resync runtime services and restart the watcher against the new `ConfigDir`/`DataDir` (not the old `--data` root).
9. Resume — or return `restart_required` and let the existing restart mechanism bring the app back.

**`POST /admin/api/system/factory-reset`** — staged protocol:
- Build the complete replacement files in a private temp dir: default `settings.yml`, default `registry.json`, a freshly generated `master.key` + `master.key.hash` (via `O_EXCL`), and an encrypted empty `secrets.json` using the effective key.
- Validate the replacement set (settings/registry load, key/hash verify, secrets round-trip) before activation.
- Under a reset lock, swap the active files into `Paths.ConfigDir` / `Paths.DataDir` (atomic rename per file, or a directory swap). Keep a recoverable backup of the previous set until activation succeeds.
- Leaves `orchestrator.db`, `meta/`, `templates/`, and `workspaces/` untouched. (Accepted side effect: the DB still holds memory/ledger/ICU-budget rows that reference the pre-reset model catalogue; the empty post-reset registry makes those rows inert. No cleanup required.)

This replaces the naive "delete then SeedDefaults" sequence, which is not failure-safe: a crash or
permission error mid-delete can leave key and ciphertext inconsistent, or settings deleted while
registry remains.

**`LLM_MASTER_KEY` handling (two reset modes).**
- *File-managed key:* remove old `master.key`/`.hash`, generate a new key, verify `FactoryReset_NewKeyDiffersFromOld`.
- *Environment-managed key:* do NOT delete or regenerate the effective key. Clear and recreate `secrets.json` using the environment key. Skip `FactoryReset_NewKeyDiffersFromOld`; report key as externally managed. The reset must reject, document, or require clearing `LLM_MASTER_KEY` before it claims a fresh key.

**`POST /admin/api/system/clear-runtime-data`** — deletes, derived from `Paths`:
- `DATA/meta/*/sessions/`
- `DATA/meta/*/process.log`
- `DATA/meta/*/.lock`
- `DATA/runs/`
- `DATA/logs/`

NOT `DATA/sessions` (that path does not exist in the target layout — sessions live under `DATA/meta/<ws>/sessions/`). Also decide whether legacy workspace-local `sessions/` (`workspace.go:312`) is included; because `ReadSession`/`ListSessions` read both locations, clearing only the new path does not actually clear all session data. **Active-state guard:** clear-runtime-data must not remove a `.lock`/`process.log` belonging to a currently active workspace, shell session, or automation run. Stop/quiesce active sessions/runs first, or reject the operation while active, then recreate the required empty directories. **File logger handling:** the running file logger holds an open fd on `DATA/logs/llm-proxy.log`. The quiesce step must close/reopen the logger after the log dir is recreated, otherwise subsequent log writes go to a deleted inode.

**Post-reset secret invariant (must be asserted before reporting success):**
```
master.key/.hash mutually valid; secrets.json decrypts with the effective key;
the live SecretStore uses that same key; write-then-read of a secret succeeds;
no old ciphertext remains active.
```

**Tests**
- `FactoryReset_DeletesConfigAndSecrets` — files gone; `orchestrator.db`, `meta/`, `templates/`, `workspaces/` untouched
- `FactoryReset_ReseedsDefaults` — after reset, `settings.yml` and `registry.json` load, a new `master.key` verifies against its hash, and `secrets.json` round-trips (write a key → read it back)
- `FactoryReset_NewKeyDiffersFromOld` — only in file-managed key mode
- `FactoryReset_EnvKeyModeReusesKey` — with `LLM_MASTER_KEY` set, key is NOT regenerated; secrets still round-trip
- `FactoryReset_FailureInjection` — crash/permission error at each staged step leaves a consistent, recoverable state
- `ClearRuntimeData_DeletesSessionsRunsLogs` — asserts config, secrets, db, templates, workspaces are all untouched; asserts `DATA/meta/*/sessions`, `runs`, `logs` removed
- `ClearRuntimeData_RejectsActiveState` — refuses or quiesces when a shell session / automation run is active
- `Reseed_CreatesMissingUserdata` — with `~/.config/llm-proxy` entirely absent, defaults and secrets are created correctly
- permission assertions after reseed (dirs `0700`, `master.key`/`.hash`/`secrets.json` `0600`)
- handler-level tests for both routes mirroring the existing admin POST test style

### Phase 11 — Hardening

`O_EXCL` on master.key creation (**S2**); `0700`/`0600` enforcement on create (**S1**); startup permission policy for existing paths (**S3**), following the `hermes-agent/utils.py:246` pattern.

**Permission policy is explicit, not warning-only, for sensitive files.** Define, for each path, whether an unsafe permission is *repaired*, *rejected*, or *warned*:
- Root dirs `ConfigDir`/`DataDir`: if mode is more open than `0700`, **repair to `0700`** (or fail startup if repair is not permitted).
- `master.key`, `master.key.hash`, `secrets.json`: if mode is more open than `0600`, **fail startup** (warning-only is insufficient — these gate ciphertext).
- `settings.yml`, `registry.json`: define explicit mode (`0600`) and repair-or-fail.
- temp files, `templates/`: define explicit mode. (No `.migrated` files exist — Phase 8 migration was removed.)

`WriteAtomic` must tighten an existing parent dir to `dirPerm` on create-or-exist (MkdirAll does not tighten). Reject symlinked key/secret files unless deliberate support is added; a symlink to a world-readable location defeats the permission model. Check ownership where the platform allows.

### Phase 12 — Documentation

- `CONSTITUTION.md:85-96` — III.4 rewritten (three files → two files plus canonical locations), III.6 updated for the new `secrets.json` location
- `AGENTS.md:17`, `README.md:69`, `docs/skills/llamacpp-setup.md:70-85`
- `.gitignore:55-59` — remove the now-obsolete `backend/data` entries
- untrack `backend/data/config.json` (currently the only tracked file of the three)
- catalog rows in `docs/PLANS/README.md` and `docs/INDEX.md`
- flag (do not fix) the dangling `docs/services/llm-proxy.service` referenced by `backend/install.sh:11` and `docs/INDEX.md:120`

---

## Risks

| Risk | Mitigation |
|------|-----------|
| Silent empty config from a stale/legacy file | We do not read legacy locations at all; `SeedDefaults` always produces a valid target-layout set on first run, and the C4 fatal master-key check rejects a corrupt key — no silent-empty fallback (Phase 8 migration removed). |
| Tests mutate real user secrets | Phase 4 gates Phase 7 |
| Phase 6 touches the admin write path | Facades preserve `System()`/`Settings()`; characterization test on `ApplySystemUpdate` first; **single-owner store** so concurrent system/settings writes cannot clobber each other |
| Deep-copy `Get()` cost | One map clone per settings read — negligible against the per-request AES decrypt it replaces |
| Reset endpoint deletes the wrong thing | Known-path allowlist derived from `paths.Paths`; never a recursive root wipe; both covered by untouched-assertions in tests |
| Factory reset loses API keys irreversibly | Confirmation modal stating exactly what is deleted; keys are unrecoverable by design once the master key is gone |
| Fresh install has no templates | `//go:embed` + extract-on-first-run in Phase 7; never overwrite user-edited templates |
| Running server reads stale legacy file | We never read legacy locations (Phase 8 migration removed); startup warns if legacy `backend/data/*` files are detected but proceeds with the seeded target-layout set |
| `--data` default silently blocks XDG/`LLM_PROXY_HOME` | Main detects explicit flag; omitted flag is the only path to env/XDG resolution |
| `LLM_PROXY_HOME` ambiguity causes misresolved roots | Fixed `<dir>/config` + `<dir>/data` expansion; documented precedence |
| Reset leaves the running app on old state | Phase 10 defines a quiesce→stop→swap→reload→resync lifecycle; not file delete + reseed against live process |
| Reset is not atomic on crash | Staged build-in-temp + validated swap + recoverable backup; `FactoryReset_FailureInjection` test |
| `LLM_MASTER_KEY` makes "new key" invalid | Two reset modes: file-managed regenerates; env-managed reuses and skips `NewKeyDiffersFromOld` |
| `clear-runtime-data` clears wrong/active state | Deletes `DATA/meta/*/sessions`,`runs`,`logs` (not nonexistent `DATA/sessions`); quiesces active sessions/runs first |
| Permission model is warning-only on key/secret | Phase 11 fails startup on unsafe `master.key`/`.hash`/`secrets.json`; repairs dirs to `0700` |
| Watcher goroutine leaks across reset/shutdown | Phase 3 adds ctx-tethered `Watch`/`Close` with goroutine exit and timer stop |

---

## Out of Scope

- Moving `workspaces/` — user-owned content; only its default changes
- OS keychain (macOS Keychain / libsecret) integration for `master.key` — possible follow-up
- Fixing the pre-existing dangling `docs/services/llm-proxy.service` — flagged only
- Windows support — this plan targets Unix and macOS

---

## Verification

Per phase:

```bash
cd backend && go build ./...
cd backend && go test ./...
cd backend && go test -race ./internal/platform/... ./internal/app/...
cd backend && go run ./tools/check-complexity/
cd frontend && npm run build
```

Storage package coverage must rise from 18.3% to a meaningful level before Phase 2 begins.
