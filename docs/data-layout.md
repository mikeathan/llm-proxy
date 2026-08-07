# Data Layout

This document describes where the proxy stores its on-disk state, how the paths
are resolved, and how each artifact class can be cleaned up. It is the canonical
reference for "where are my sessions / runs / recordings".

## Root resolution

All files live under a single root directory. Resolution precedence
(highest → lowest):

1. `--data <dir>` — explicit flag, resolved against CWD.
2. `LLM_PROXY_HOME=<dir>` — environment variable.
3. `~/.config/llm-proxy` — default on macOS **and** Linux.

Source: `backend/internal/platform/paths/paths.go` (`Resolve`).

The single root contains:

```
{root}/
├── settings.yml, registry.json, secrets.json, master.key(.hash), orchestrator.db
├── templates/          task-template library
├── meta/               per-workspace metadata (sessions, state, config)
├── runs/               automation run dirs + LLM recordings
└── logs/               application logs
```

## workspaces/ — agent working files

- Path: `{workspaces_dir}/{workspaceID}/` where `{workspaces_dir}` defaults to
  `{repoRoot}/workspaces` — the repo root is discovered by walking up from the
  current working directory to the nearest ancestor containing `backend/go.mod`.
  This anchors the default to the repo root regardless of which subdirectory the
  proxy is launched from (`go run main.go` from `backend/`, `frontend/`,
  `scripts/`, or the repo root all resolve to the same `{repoRoot}/workspaces/`).
- Accessor: `DataManager.WorkspacesDir()` (`backend/internal/platform/storage/manager.go`).
- Override: set `workspaces_dir` in `settings.yml` (absolute, or relative to the
  data root).
- When no repo root is discoverable (e.g. a packaged/systemd deployment launched
  from an unrelated cwd) and `workspaces_dir` is unset, the default falls back to
  `~/llm-proxy/workspaces` (or `{dataRoot}/workspaces`).
- These are the agent's **working files** — `AGENTS.md`, `heartbeat.md`, and any
  files the agent creates. They are kept **outside** the data root so they never
  sit beside `master.key`/`secrets.json`, and `wipeout`/`factory-reset`/
  `clear-runtime-data` never touch them.
- The per-workspace `meta/` (config, state, lock, process.log) and `sessions/`
  still live under `{root}/meta/{workspaceID}/`, separate from these files.

## meta/ — conversation sessions (assistant)

- Path: `{root}/meta/{workspaceID}/sessions/{sessionID}.json`
- Accessor: `PathResolver.SessionsDir` (`backend/internal/platform/storage/resolver.go`).
- These are the assistant **conversation transcripts** (user/model/tool turns).
  They are stored *outside* the workspace tree so the agent cannot read them via
  file tools — the separation is filesystem-based, not encryption.
- Cleanup:
  - Per-session: `DELETE /admin/api/conversation/sessions/{ws}/{session}`
  - All in workspace: `DELETE /admin/api/conversation/sessions/{ws}`
  - Both also remove the conversation's on-disk run dirs
    (`runs/{ws}/{model}/{sessionID}`) so deleting a chat does not orphan its
    events/recording artifacts.

## runs/ — automation runs and recordings

- Path: `{root}/runs/{workspaceID}/{model}/{task}/{timestamp}_{sessionID}/`
- Produced per automation execution when run logging is enabled
  (`--enable-runs` / `--record`), or via the recording client.
- A single run directory contains: `events.jsonl`, `run-meta.json`,
  `final-report.md`, and (when `--record` is active) `recording.jsonl`.
- Cleanup surfaces:
  - **Per-recording** (removes the whole run dir when nested):
    `DELETE /admin/api/recordings/{id}` (UI: `RecordingsPanel.vue`).
  - **Per-run** (removes a single run by its history ID, plus its on-disk run
    directory and matching `state.json` history/last-run entry):
    `DELETE /admin/api/dispatcher/runs/{ws}/run/{run}` (UI: delete button on
    each card in `WorkspaceActivity.vue` and in the run detail views).
  - **Per-automation runs** (clears every run directory for an automation across
    all model subdirs and purges matching history, without deleting the
    automation itself): `DELETE /admin/api/dispatcher/runs/{ws}/{automation}`
    (UI: "Clear All Runs").
  - **Per-automation** (deletes the automation and its runs):
    `DELETE /admin/api/dispatcher/workspaces/{ws}/automations/{automation}`.
  - **Per-workspace** (removes `runs/{ws}` and sessions):
    `DELETE /admin/api/dispatcher/workspaces/{ws}`.
  - **Bulk** (wipes all runs + logs, keeps config/secrets/db):
    `clear-runtime-data` (UI: `SecuritySettings.vue`).
  - **Full uninstall** (removes the entire single root — config, secrets, DB,
    templates, meta, runs, logs — *and* the workspaces directory, then stops the
    process): `POST /admin/api/system/wipeout` (UI: "Wipeout (Uninstall)" in
    `SecuritySettings.vue`). Guarded against active work and against wiping `/`,
    `$HOME`, or an ancestor of `$HOME`.

All deletion paths are guarded: the target must resolve beneath `runs/`, must
not be a symlink, and the run/automation endpoints reject invalid workspace IDs
and `.`/`..` path segments explicitly so `filepath.Join` can never collapse a
segment into a sibling directory.

## Reset & uninstall controls

Three admin endpoints govern destructive cleanup, in increasing severity. All
refuse to run while assistant/automation runs or live shell sessions are active.

| Control | Endpoint | Removes | Keeps | Process |
|---|---|---|---|---|
| Clear runtime data | `POST /admin/api/system/clear-runtime-data` | `runs/`, `logs/`, `meta/*/sessions`, `meta/*/process.log`, `meta/*/.lock` (dirs recreated) | settings, secrets, master key, `orchestrator.db`, `templates/`, `meta/` structure, `workspaces/` | stays running |
| Factory reset | `POST /admin/api/system/factory-reset` | resets `settings.yml`, `registry.json`, `secrets.json`, `master.key` to defaults (fresh key) | `orchestrator.db`, `templates/`, `meta/`, `runs/`, `logs/`, `workspaces/` | stays running |
| Wipeout (uninstall) | `POST /admin/api/system/wipeout` | the entire single root **and** the workspaces directory | nothing | stops (`os.Exit(0)`) |

`wipeout` is a separate operation rather than a composition of the other two:
neither clears `orchestrator.db`, `templates/`, the `meta/` structure, or
`workspaces/`, and neither stops the process. It additionally refuses to wipe
`/`, `$HOME`, or an ancestor of `$HOME` (`validateWipeTarget` in
`storage/reset.go`) so a misresolved `--data`/`workspaces_dir` cannot destroy the
host. All three are surfaced under "Reset Controls" in `SecuritySettings.vue`;
the wipeout button confirms through the shared `ConfirmDialog`.

## Why two locations?

`meta/` and `runs/` serve different purposes and are intentionally separate:

- **Lifecycle**: sessions persist forever (resume a chat); run dirs are
  ephemeral telemetry/replay output you may prune.
- **Security boundary**: sessions are hidden from the agent (outside the
  workspace); runs are operator-facing logs, not agent memory.
- **Consumers**: sessions feed the UI chat list; runs feed the automation
  dispatcher, SSE event stream, and the recording store.

There is no duplication of "the same thing": `meta/` is chat memory, `runs/` is
run telemetry.

## Startup logging

On startup the proxy logs its resolved layout (root, meta, runs, logs) at the
`data layout` info line in `boot.go`, both to stdout and to `logs/llm-proxy.log`.
