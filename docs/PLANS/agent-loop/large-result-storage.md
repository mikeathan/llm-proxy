---
status: proposed
date: 2026-10-19
related_specs: [SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007]
---

# Plan: Keep large tool results instead of cutting them (saved reports)

**Constitution refs:** §I network security (fetched content is untrusted input), §II resource management (bounded disk, bounded
results), §III data (secrets are never persisted unmasked), §IV code standards (one wiring point, small functions).
**Specs touched (SPEC-first):** SPEC-006 (guardrails: a new system write into `reports/`, the `read_file` interface change),
SPEC-007 (automation task shows its saved reports), SPEC-003 (UI), `docs/data-layout.md` (the `reports/` folder and the manifest).

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
7. **Phase 6 — demote old tool results under context pressure** to one-line stubs that point at the saved file, so the sieve
   can free room without losing anything. Adaptive, model-agnostic, and strictly additive (§6).

Only a result the cut would shorten is saved. A result that fits is untouched and nothing extra is stored.
Rule for every phase (and every model, local or cloud): **additive, proven offline first, then live**, as with the sieve and the progress monitor.

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
| Memory has a search-only mode and an expiry reaper. Its secrets screen is a **yes/no refusal, not a redactor**: `isSecretContent` makes `memory_update` refuse the whole save. | `docs/SPECS/memory.md` (`on_demand`, `expires_at`); `tools/memory_tools.go:42-51` (`ErrSecretContent`, `isSecretContent`). |
| A read-only filesystem guardrail blocks writes. | `validateFilePath`: `isWrite && cfg.ReadOnly`. |
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

- **Automation:** `{workspace}/reports/<automation>/<date>-<tool>-<run-id>-<n>.txt` (the run id keeps names unique when a chat and an automation write at once). Lives and dies with the workspace; no reaper touches it.
- **Chat:** open decision (§4). Recommended: `{workspace}/reports/chat/<conversation-id>/…`, deleted with the conversation.
- **Manifest:** `meta/{ws}/reports-manifest.json` (outside the workspace tree; deleted with the workspace). It records
  every system-saved file (path, tool, run, time, size, pinned) so a purge removes only what the system created.

### 3.3 Rules

- **R1 — never point at a file the run cannot open, and decide it in the agent, not the tool.** Whether filesystem tools
  are enabled, `read_file` is in this run's toolset, the extension is allowed, the path is under `AllowedPaths`, the
  filesystem guardrail is not `ReadOnly` (§4) and the channel is one we spill for are all known to the **agent** at run start.
  When all hold, the agent puts a **save target** into the run context (workspace root, folder, run id). `FetchURL` writes
  to it if present and does nothing otherwise, so the tool stays unaware of policy and the decision lives in one place.
- **R2 — redact before writing, and make the redaction real.** The memory screen only refuses; on a 1 MiB page one
  token-like string would block the whole save and silently fall back to the cut. Phase 1 must either extend that
  matcher into a redactor (replace the match with a marker) or take Hermes's spill redaction as the model, and measure it on
  1 MiB. Any section map or offset is computed **after** redaction, or it points at the wrong text.
- **R3 — what is saved.** The shaped text for HTML, the raw text otherwise. `fetch_url` reads at most `MaxFetchSizeKB` (1 MiB
  default), so "full" means up to that.
- **R4 — a size warning, not eviction.** Eviction is how reports get lost. The warning threshold comes from the measured
  growth per fetch (Wikipedia and weather fixtures), not a guess.
- **R5 — scope.** `fetch_url` only at first; terminal and search output later, each measured.
- **R6 — one wiring point.** The save target (R1) and a small writer interface are wired where the tools are constructed
  (AGENTS.md centralisation); the tool never calls the filesystem directly.
- **R7 — deterministic discovery.** A later run must learn the file exists without relying on the model to list the
  folder. The manifest feeds a short list of **this automation's** saved reports (path, date, title, summary line, capped
  count) into the task text, the way journal notes are shown. This is how "check older findings" works; memory (Phase 5) is
  then only an extra, not a requirement.

## 4. Decisions (user agreed with every recommendation, 2026-10-20)

1. **Chat location: the workspace**, `reports/chat/<conversation-id>/`, deleted with the conversation (one read path, one link
   target, one purge UI). The run folder is rejected: neither the model nor the Files page can open it.
2. **Final reports:** still open in fact, not in policy. `AutomationRun.Output` is stored in `state.json` history; whether a very
   long final report is cut or purged is unverified and is read in Phase 1, step 1. If it can be lost, it joins `reports/`.
3. **"Clear runtime data" and factory reset:** clear chat reports with the sessions; keep automation reports.
4. **Read-only workspaces:** no saving when the filesystem guardrail is `ReadOnly`; keep today's cut and log why.
5. **Size-warning default:** set from the measured growth per fetch (Wikipedia and weather fixtures), then confirmed.

## 5. Phases (riskiest first; each ends in a check)

### Phase 0 — ranged `read_file` (ships on its own)

- This is a **tool interface change** (`read_file` gains arguments): update SPEC-006's tool list and `tools/manifests/filesystem.json`,
  and `docs/architecture.md`'s tool checklist. Offsets and limits count **bytes**, because `ReadFile` cuts by bytes today and the
  section map (`@4200`) uses the same unit; a chunk never splits a UTF-8 character.
- Add `offset` and `limit` to `read_file`; size each chunk by **encoded length**, as `pageHead` does, so
  `appendToolResult` does not re-cut it. Replace the truncation message with a true one.
- Fold in the open flaw: the size-limit note in `fetchedText` is appended after `pageHead` and can push a result over the cap.
- Acceptance: a 200,000-character file is read end to end in chunks with no chunk re-cut; the message names only tools that exist.

### Phase 1 — automation spill

- Step 1 (read-only): check `AutomationRun.Output` storage/caps and the reaper's effect on `state.json` history; check the
  shipped task templates for `reports/` collisions.
- Build: the save target and writer interface (R1, R6), `reports/<automation>/`, the manifest (written with the existing
  `WorkspaceManager` atomic-write and locking pattern), R2–R3, R7 (the task's list of saved reports), the first-line note (§3.1),
  the optional section map. Update `docs/data-layout.md` and SPEC-006/007 in this phase.
- Acceptance (offline): replay the saved Wikipedia and weather fixtures; the note fits in the sieve's kept head; nothing is
  saved for a result under the cap.
- Acceptance (live): an automation run twice on a copy of workspace-1; run 2 is **told** run 1's report exists (R7) and reads a
  middle section of it with the ranged `read_file`.

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

- Optional, because R7 already gives the guarantee. A system-written `on_demand` memory record per saved result (URL or query, time, title, summary line, hash of the shaped
  text) so "seen before / changed since" works without the model. Verify first that a system write path exists
  (`memorycapture`) and that automations get the memory tools.

### Phase 6 — demote old tool results under pressure (adaptive, model-agnostic, resilient)

**Why.** Today a result is cut at entry whatever the pressure, and the history sieve later shrinks old results to head+tail with no way
back. DeepSeek and Hermes do the opposite (see the research note in §7): nothing is trimmed below pressure; under pressure old tool
output is replaced first, with a pointer to the preserved original, and only then is anything summarised or dropped. The stubbing built
and reverted earlier in `run-efficiency-and-convergence.md` failed because nothing could be recovered and because it edited history in
place. The saved files from Phases 1–2 supply the recovery path; this phase supplies the policy.

**Where.** In the existing pressure path: `session.go` measures the real request each turn (`measureRequest`, `size.over()`) before
`applyPhysicalSieve`. The demotion pass runs **first** on that same trigger, re-measures, and the physical sieve runs only if the request
is still over.

**Design rules**

- **One measure, no new thresholds.** The trigger and the target are `measureRequest` against `requestLimit()` — already scaled per model by
  `ContextBudget` and the server-learned token ratio for a local model, and the message-only allowance for a cloud model. Nothing is keyed on
  a model, provider or tool name; local and cloud differ only through that measure.
- **Eligibility by shape, not name.** A tool-result message is eligible when it is outside the newest rounds, longer than a minimum, and
  **carries a recovery handle** (the `[full text saved: …]` first line from §3.1). No handle, no demotion: that message is left to the
  existing sieve, so behaviour without Phases 1–2 is exactly today's.
- **Protected tail sized by budget.** The newest results stay whole until they total a fraction of the request limit (starting at 30%,
  tuned from the replay), never a fixed message count, so a 4k window, a 16k window and a 128k window all behave sensibly.
- **Just enough.** Demote oldest-first only until the request is under the limit (with a small margin), not everything eligible.
- **Stable prefix.** A demoted result stays demoted on every later turn of the run (a latched set of call ids), and the stub text is a pure
  function of the original (tool, size, handle). The same input gives the same request, so the server's prefix cache is rebuilt once after
  the pass and then holds, instead of being broken again each turn.
- **A projection, not an edit.** The pass builds the request copy; the saved conversation and the run's persisted history are untouched.
- **Pairing by construction.** Only the content of a tool message changes; no message is added or removed, and the call/result pairing
  helper from `sieve_test.go` is run over every output in tests.
- **Short stub.** One line, under ~150 characters, tool name, original size and the read call, so it survives a later head+tail cut.
- **Fail safe.** Any error, or a run where the pass would not get under the limit, logs one line and falls through to the existing sieve.
  A setting turns the pass off. No path makes a request larger or an answer worse than today.
- **Shadow first.** The first release only logs what it would demote and the tokens it would free; it is switched on after the live data
  shows it helps.

**Steps**

1. Pure function over (history, limit, measure) returning the demoted copy and a report; table tests over several budgets and tool shapes.
2. Wire it before `applyPhysicalSieve` in shadow mode: one log line per pass (`would demote N results, free X tokens`).
3. Replay recorded histories (including the 141 s run's request 8) offline across budgets 4k, 16k and 128k.
4. Live on Vertex in shadow mode, then enabled, comparing steps, time, answer, `cached_tokens` and whether the sieve still fires against a
   20-run baseline (the bar used for the progress monitor).

**Acceptance**

- Idempotent and deterministic: running the pass twice, or on consecutive turns, gives byte-identical stubs and requests.
- No handle means unchanged output; the existing sieve tests stay byte-for-byte unchanged and green.
- Pairing intact on every generated history; the saved conversation is not modified.
- Adaptivity: the same history at 4k, 16k and 128k demotes progressively fewer results, never more than needed.
- Live: no run answers worse, no run takes more steps, and the cache hit rate after the first pass is within 10 points of before it.

**Open risks specific to this phase**

- The sieve's one-time "wrap up now" warning ended a measured loop. If demotion keeps a run under the limit, that warning no longer
  fires. The loop detectors and the progress monitor still apply; the live comparison must confirm no run gets longer.
- A cloud provider that does not report cached tokens cannot be measured for the cache effect; the design does not depend on it.
- Demoting the oldest results moves the point where the prefix cache breaks to the front of the history. Mitigated by the stable prefix
  rule and measured live.

## 6. Risks

- **Clutter and disk growth** — mitigated by R4 and the purge; no automatic eviction by design.
- **A saved file contains something sensitive** — R2, and the folder is workspace data the operator can see and delete.
- **Chat history and the sieve** — the path line must survive the sieve (first line, §3.1); verified by a test over the real
  `sieve.go` constants.
- **Operators who rely on the old cut** — nothing changes for results under the cap, and when R1 fails the old cut is kept.

## 7. Research note (existing harnesses, checked 2026-10-19)

- **DeepSeek harness** (`docs/subsystems/compaction.md`, `compaction-tool-result-pruner`): compaction runs at `agent/pre-step` only on
  `pressure` or `context-overflow`; it trims oversized tool results first (default over 8,192 chars to the first 4,096 plus the last 1,024,
  no model call), re-measures, and summarises an older range only if still over. Originals stay in the session log; tool call/result
  pairing is checked before and after.
- **Hermes** (`agent/context_compressor*.py`, `micro_compaction.py`, `context_compressor_tail.py`): compresses at a threshold percent of the
  window scaled by model; prunes old tool output first, then a cheap auxiliary model summarises the middle with head and tail protected; old
  results become one-line recovery stubs naming the preserved original. Rolling micro-compaction is off by default because every rewrite
  breaks the prompt cache. Not read in full: the 5,800-line compressor.
- **Claude Code** (from behaviour, not source): clears old tool results before summarising, keeps the recent part, large output goes to a file.

## Remaining Work

Everything. Decisions in §4 are made. Start with Phase 0 (a standalone fix) and Phase 1 step 1 (read-only checks). Phase 6 waits for Phases 1–2.
