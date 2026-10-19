---
status: proposed
date: 2026-10-19
related_specs: [SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007]
---

# Plan: Keep large tool results instead of cutting them (saved reports)

Follows [`run-efficiency-and-convergence.md`](run-efficiency-and-convergence.md) (M2, "spill"). That plan shaped what the model
sees; this one makes sure the part that does not fit is **kept and readable**, not lost.

## 0. TL;DR

Every tool result is cut to ~3000 characters (`appendToolResult`, `tool_exec.go`), and a cut is final: the rest of a long
page or report is gone for the run, and for an automation that wants it on a later run. The fix is the pattern DeepSeek's
spill package uses (full text saved, the model gets a locator and a read hint), with the lifetime and the controls this
repo needs:

1. **Phase 0 — make a file readable in pieces.** The model cannot read the middle of a long file today. This is a bug on its own.
2. **Phase 1 — automations save an oversized result to the workspace, `reports/<automation>/`.** Not purged by the run reaper.
3. **Phase 2 — chat.** Same mechanism, location per the open decision in §4.
4. **Phase 3 — purge, operator-only.** One file, one automation, the whole folder, by age; pins; manifest.
5. **Phase 4 — UI.** A link from the step or run to the file, a side sheet, a Back link on the Files page.
6. **Phase 5 — optional memory index** (search-only record and a content hash).

Only a result the cut would shorten is saved. A result that fits is untouched and nothing extra is stored.
Rule for every phase: **additive, proven offline first, then live**, as with the sieve and the progress monitor.

## 1. What was verified (and where)

| Fact | Evidence |
|---|---|
| The cap is one head+tail cut of the **JSON-encoded** result. | `appendToolResult`, `assistant/tool_exec.go` (~1313): `proxy.TruncateResultDefault(string(raw))`; `proxy/utils.go` `MaxReturnChars = 3000`. |
| `fetch_url` already cuts its own output first, then the cap is applied again to the encoded string. | `tools/network.go` `FetchURL`, `fetchedText`, `tools/htmltext.go` `pageHead` (sized by encoded length since #95). |
| **`read_file` takes only `path` and returns head+tail 1500+1500 over 3000 bytes.** Its message says "Use 'read_range' or 'grep'". | `tools/filesystem.go` `ReadFile`; `tools/manifests/filesystem.json`. |
| **No `read_range` and no filesystem `grep` exist.** The filesystem tools are `list_directory`, `read_file`, `write_file`, `edit_file_block`, `append_file`. `grep` is only an allow-listed terminal command. | `filesystem.json` `runtime` keys; `terminal.json:42`. |
| Hidden (dot-prefixed) paths, `..`, and config/state/secrets files are denied to the model. So the folder cannot be `.fetched`. | `validateFilePath`, `tools/filesystem.go:106-135`. |
| The workspace root is `AllowedPaths[0]` for a run. | `guardrails/guardrails.go:363`, `assistant/registry.go:391-397`. |
| The run context carries workspace id, automation name, run id, unattended flag. No conversation id accessor. | `models/config.go`: `GetWorkspaceID`, `GetTaskName`, `GetRunID`, `IsUnattendedRun`, `IsJournalRun`. |
| The sieve shortens **older** tool results to ~275 chars: head+tail around `...[Truncated]...`. | `assistant/sieve.go` (`truncateLongContent`, `sieveTruncationMarker`); seen on a real run. A path line must sit in the first or last ~120 characters. |
| `runs/` is cleaned by a fixed 30-day reaper; it is **not** configurable. Sessions live outside the workspace so the agent cannot read them. | `platform/rundir/runreaper.go` `DefaultRunRetention`; `app/app.go:114-117`; `docs/data-layout.md`. |
| The workspace has file endpoints and a Files page with nested paths. | `app/routes.go:244-246` (GET/PUT/DELETE `.../files/{file...}`), `handlers.isUnsafeFileParam`; `frontend/src/router/routes.ts` `toWorkspaceFile`. The read endpoint returns the whole file as one JSON string, no range. |
| A folder delete helper exists. | `workspaceService.DeleteTaskTree` (`handlers/services.go`). |
| An automation's final report is stored as `AutomationRun.Output` in `state.json` history. | `models/workspace.go` (`AutomationRun.Output`), `automation/executor.go:506,579`. |
| Memory has a search-only mode and an expiry reaper; every save passes a secrets screen. | `docs/SPECS/memory.md` (`on_demand`, `expires_at`, "every save now passes one screen in code"). |
| Workspaces pages are `keepAlive` by route. | `frontend/src/router/index.ts:20` (`WORKSPACES_META`). |

**Not verified, to be checked inside the phase that depends on it:** whether the Files page handles a 1 MiB text file;
whether the reaper or any cap prunes `state.json` history output; how the Steps timeline renders a tool result (where a link
would go); whether switching assistant ↔ files inside the kept-alive workspace view preserves scroll and a live stream;
how workspaces without filesystem tools enabled behave (§5 rule R1).

## 2. Decisions already made (user, 2026-10-19)

- Large results are kept in the **workspace**, not `runs/`: runs are purged after 30 days and the point is not to lose them.
- Only results that **would be cut** are saved; small results stay inline.
- The model **cannot delete** reports. Purge is an operator action (UI/API), with a count and size shown first.
- The raw report is reached from the report by a **link** that opens in the app; a **side sheet** (recommended) so the report
  and a live stream are not left, plus a Back link on the Files page when someone arrives from a report.
- A **map or summary** is secondary: the core is the saved file and a ready read call. A section map is built from the page
  (headings and offsets), never written by a model by default.

## 3. Design

### 3.1 What the model sees

The first line carries the path and size (so the sieve's head+tail keeps it even after the result becomes old):

```
[full text saved: reports/daily-news/2026-10-19-fetch-3.txt · 148,000 chars]
<the head: title, summary lines, the opening — what fetch_url already produces>
Sections: History @4200 · Geography @31000 · Economy @58500 …      (optional, built from headings)
Read more: read_file path="reports/daily-news/2026-10-19-fetch-3.txt" offset=58500 limit=3000
```

The note is written **only when the run can read the file back** (R1).

### 3.2 Where it is saved

- **Automation:** `{workspace}/reports/<automation>/<date>-<tool>-<n>.txt`. Lives and dies with the workspace; no reaper touches it.
- **Chat:** open decision (§4). Recommended: `{workspace}/reports/chat/<conversation-id>/…`, deleted with the conversation.
- **Manifest:** `meta/{ws}/reports-manifest.json` (outside the workspace tree; deleted with the workspace). It records
  every system-saved file (path, tool, run, time, size, pinned) so a purge removes only what the system created.

### 3.3 Rules

- **R1 — never point at a file the run cannot open.** Save and note only if filesystem tools are enabled, the path is under
  `AllowedPaths`, the extension is allowed, and the run's toolset includes `read_file`. Otherwise today's cut stays.
- **R2 — redact before writing.** Reuse the memory secrets screen; do not write a second one. Measure it on 1 MiB first.
- **R3 — what is saved.** The shaped text for HTML, the raw text otherwise. `fetch_url` reads at most `MaxFetchSizeKB` (1 MiB
  default), so "full" means up to that.
- **R4 — a size warning, not eviction.** Eviction is how reports get lost. The warning threshold comes from the measured
  growth per fetch (Wikipedia and weather fixtures), not a guess.
- **R5 — scope.** `fetch_url` only at first; terminal and search output later, each measured.
- **R6 — one wiring point.** `FetchURL` gets a small writer interface injected where the tools are constructed
  (AGENTS.md centralisation), not a filesystem call inside the tool.

## 4. Open decisions (need the user)

1. **Chat location.** `reports/chat/<conversation-id>/` (one read path, one link target, one purge UI; clutters the workspace
   a little, deleted with the conversation), versus the run folder (cleaner workspace, but the model and the Files page
   cannot open it). Recommendation: the workspace.
2. **Do final reports join `reports/`?** `AutomationRun.Output` is stored in `state.json` history. Whether a very long final
   report is cut or purged is unverified (Phase 1, step 1 reads it).
3. **"Clear runtime data" and factory reset.** Recommendation: clear chat reports with the sessions; keep automation reports.
4. **Size-warning default** — set from the measured number, then confirm.

## 5. Phases (riskiest first; each ends in a check)

### Phase 0 — ranged `read_file` (ships on its own)

- Add `offset` and `limit` (characters) to `read_file`; size each chunk by **encoded length**, as `pageHead` does, so
  `appendToolResult` does not re-cut it. Replace the truncation message with a true one.
- Fold in the open flaw: the size-limit note in `fetchedText` is appended after `pageHead` and can push a result over the cap.
- Acceptance: a 200,000-character file is read end to end in chunks with no chunk re-cut; the message names only tools that exist.

### Phase 1 — automation spill

- Step 1 (read-only): check `AutomationRun.Output` storage/caps and the reaper's effect on `state.json` history; check the
  shipped task templates for `reports/` collisions.
- Build: the writer interface, `reports/<automation>/`, the manifest, R1–R3, the first-line note (§3.1), the optional section map.
- Acceptance (offline): replay the saved Wikipedia and weather fixtures; the note fits in the sieve's kept head; nothing is
  saved for a result under the cap.
- Acceptance (live): an automation run twice on a copy of workspace-1; run 2 reads a middle section of run 1's report.

### Phase 2 — chat spill

- Implement per the §4 decision. Needs a conversation id in the run context (none today).
- Acceptance: the five-city task on Qwen 16k still takes ≤ 6 fetches; a long page's middle is readable on request.

### Phase 3 — purge (operator-only)

- Endpoints and UI: one file, one automation, the whole folder, by age; confirm shows count and size; manifest-only deletion
  (anything not in the manifest is left); pins; the same path guards as the existing deletes (under the workspace, not a symlink).
  Reuse `DeleteTaskTree`.
- Acceptance: a purge removes exactly the manifest entries selected; a pinned file survives "whole folder"; a file the user
  put in `reports/` is untouched; workspace delete removes the manifest too.

### Phase 4 — UI

- First steps: find the Steps component that renders tool results; test the Files page with a 1 MiB file; check scroll and
  live-stream behaviour when switching sections in the kept-alive workspace view.
- Build: the link, the side sheet (a Teleport-to-body sheet, as the Run settings sheet), a Back link on the Files page that
  uses the route it came from, and large-file handling (a size limit or "load more").
- Acceptance: link opens the file; closing the sheet returns to the exact scroll position with a live stream unaffected;
  the Back link works after a refresh.

### Phase 5 — optional memory index

- A system-written `on_demand` memory record per saved result (URL or query, time, title, summary line, hash of the shaped
  text) so "seen before / changed since" works without the model. Verify first that a system write path exists
  (`memorycapture`) and that automations get the memory tools.

## 6. Risks

- **Clutter and disk growth** — mitigated by R4 and the purge; no automatic eviction by design.
- **A saved file contains something sensitive** — R2, and the folder is workspace data the operator can see and delete.
- **Chat history and the sieve** — the path line must survive the sieve (first line, §3.1); verified by a test over the real
  `sieve.go` constants.
- **Operators who rely on the old cut** — nothing changes for results under the cap, and when R1 fails the old cut is kept.

## Remaining Work

Everything. Start with Phase 0 (a standalone fix) and Phase 1 step 1 (read-only checks), then ask for the §4 decisions.
