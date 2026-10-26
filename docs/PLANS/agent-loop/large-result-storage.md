---
status: partial
date: 2026-10-19
related_specs: [SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007]
---

# Plan: Keep large tool results instead of cutting them (saved reports)

**Constitution refs:** §I network security (fetched content is untrusted input), §II resource management (bounded disk, bounded
results), §III data (secrets are never persisted unmasked), §IV code standards (one wiring point, small functions).
**Specs touched (SPEC-first):** SPEC-001 §2 (Phase 6 changes the sieve's compression stage), SPEC-006 (guardrails: a new system write into `reports/`, the `read_file` interface change),
SPEC-007 (automation task shows its saved reports), SPEC-003 (UI), `docs/data-layout.md` (the `reports/` folder and the manifest).

Follows [`run-efficiency-and-convergence.md`](run-efficiency-and-convergence.md) (M2, "spill"). That plan shaped what the model
sees; this one makes sure the part that does not fit is **kept and readable**, not lost.

## 0. TL;DR

Every tool result is cut to ~3000 characters (`appendToolResult`, `tool_exec.go`), and a cut is final: the rest of a long
page or report is gone for the run, and for an automation that wants it on a later run. The fix is the pattern DeepSeek's
spill package uses (full text saved, the model gets a locator and a read hint), with the lifetime and the controls this
repo needs:

1. **Phase 0 — make a file readable in pieces (built, PR #99).** The model could not read the middle of a long file. This was a bug on its own.
1b. **Phase A — size the result cap from the room left (built, a switch).** A 7 KB file or a search result should not be cut at all on a model whose budget is ~44k characters; the marker, saving and demotion then apply only to what truly does not fit. See §5, Phase A.
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
- **R2 — redact before writing, and make the redaction real.** (`tools.RedactForRecording`, built for run recordings, is the redactor to reuse here: it masks, it does not just refuse, and it skips text with nothing to mask.) The memory screen only refuses; on a 1 MiB page one
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
6. **Decided (2026-10-21): final reports go in `reports/<automation>/`** (the name matches what they are). Saved cut tool results, if ever built, get a different folder.

## 5. Phases (riskiest first; each ends in a check)

### Phase 0 — ranged `read_file` (ships on its own) — BUILT in PR #99

Built: `read_file` takes `offset`/`limit` (bytes; numbers or numeric text via `tools.FlexInt`), reads only the requested window, and sizes each piece by encoded length (`tools/result_fit.go`: `fittingPrefix`, `encodedLen`); `fetchedText` reserves room for the size-limit note. SPEC-006 is 1.5; architecture pitfall 56. Acceptance met offline: a 200,000-character file is read end to end in chunks with no chunk re-cut. **First live check (Vertex, Qwen 16k, 2026-10-20) found a flaw in the default path:** the note telling the model about `offset`/`limit` sat in the middle of an over-cap result and was removed by the second cut, so none of three runs used the new arguments (they fell back to terminal `grep`/`sed`, one waited on an approval). A first follow-up (#100: note first, whole result sized to the cap) made it worse: the note was visible and the model ignored it, answering "no Budget section" in 3 of 3. Tested against the real model offline (same history, only the tool result swapped, `/v1/chat/completions`, 5 tries per variant): the current note read the missing range 3 of 5; a loud marker at the gap with the missing range, its share and a real call read it 5 of 5, and so did the same plus a map of the headings in the missing part (5 of 5); the old generic truncation marker 3 of 5. The marker at the gap is built (`prompts.FileGapMarker`; 8 of 8 on the exact output). **The live check of #101 then failed on a different cause:** 1 of 6 runs used `offset`, the model's own reasoning said "read from offset 1363" and the call had only `path`. Rebuilding the request with the real 12 tools reproduced it (3 of 8), bisected it (real tools without the tool reference 4 of 8; 3 hand-written tools with the reference 8 of 8) and found the cause: the product sent `read_file`'s properties alphabetically (`limit`, `offset`, `path`) because Go sorts map keys. With the real tools in manifest order (`path`, `offset`, `limit`) 10 of 10; alphabetical 1 of 10. Fixed for every tool (`models.OrderedProperties`, architecture pitfall 57). The heading map stays an option. **Still to check live:** the Budget question after deploy, plus the usual five-city and weather runs as the regression bar, because the argument order of every tool changed.

- This is a **tool interface change** (`read_file` gains arguments): update SPEC-006's tool list and `tools/manifests/filesystem.json`,
  and `docs/architecture.md`'s tool checklist. Offsets and limits count **bytes**, because `ReadFile` cuts by bytes today and the
  section map (`@4200`) uses the same unit; a chunk never splits a UTF-8 character.
- Add `offset` and `limit` to `read_file`; size each chunk by **encoded length**, as `pageHead` does, so
  `appendToolResult` does not re-cut it. Replace the truncation message with a true one.
- Fold in the open flaw: the size-limit note in `fetchedText` is appended after `pageHead` and can push a result over the cap.
- Acceptance: a 200,000-character file is read end to end in chunks with no chunk re-cut; the message names only tools that exist.

### Phase A — size the result cap from the room left (BUILT, switch `agent.adaptive_result_cap`)

**Why (the real fix).** Phase 0 and its follow-ups make a cut result readable, but the cut is the root cause. Measured on Vertex
(Qwen3.6 16k, 2026-10-20/21): every `internet_search` result was 5.5–7.4 KB before the cut (median ~6.6 KB) and was cut to 3000;
a 7 KB file needed three reads, and a model stopped after two in 4 of 8 runs (its own reasoning: "I've read up to byte 4163. Let me
check if there's more… Actually, looking at the content I've read, I can see the headings clearly"), while with the whole file in
front of it (7,170 characters as a JSON result, no marker) it listed all 11 headings in **8 of 8**. No wording of the continue note
helped offline (7, 7, 6 of 8): the model decides to stop, so the file has to arrive whole.

**Design (as built).** The cap is **per tool turn**, not per run (a fixed share per call would let five parallel `fetch_url` calls
take the window):

`cap = clamp(3000, 12000, 0.5 × (requestLimit − request size) ÷ calls in the turn)` — `assistant/result_cap.go`.

- Request size is the turn's `measureRequest` (kept on `runSession`); the cap is fixed once before the turn's tools run
  (`beginResultCap`), reaches the tool through `models.WithResultCap` and is the cap `appendToolResult` applies: one value, so the
  tool's own cut and the agent's agree (pitfall 56). Consumers: `read_file` (whole file, marker and `limit=`), ranged reads (piece
  size), `fetch_url` (`pageHead`, size-limit note, final cut).
- The floor is today's 3000, so no result is ever smaller; a model with no budget (`ContextBudget` 0) stays at 3000; each result takes
  half of what is left, so the caps shrink as the request fills and their total cannot reach the room.
- Switch `agent.adaptive_result_cap` (default **on**: the floor means no result is worse than before, and the operator can turn it off
  in Settings → Runs without a deploy); chat and automations; applies to the next run. This departs from this plan's first
  "off by default" because the experiment above passed and the offline check below kept the five-city request unchanged.

**Offline check (real sizes from today's recordings).** Base request 22.7k characters (10k messages + 12.8k tool schemas), sieve limit
about 40k. Five parallel fetches: floor, request 37.9k, unchanged. One read of a 7 KB file: whole, request 29.5k. A 4-step weather run
ends at 41.2k against 35.3k today and a long news run reaches the limit two results earlier: **the floor adds 3000 characters per
result after the first big one, so long runs meet the sieve sooner.** Measure live.

**Live acceptance (after deploy, against today's numbers).** Whole-file question at least 7 of 8 (4 of 8 today); Budget question still
correct; five-city no worse than 60 s and 5 fetches; weather and news answers and call counts no worse, `tool result cut` lines for
`internet_search` mostly gone, `cached_tokens`, and how often the sieve fires; the next nightly `daily-llm-news` run compared with its
history.

**Live acceptance result (Vertex v0.8.39, Qwen3.6 16k, 2026-10-21).** Whole-file question 8 of 8 complete, each in one read and 2 model
calls (4 of 8 before); Budget question 3 of 3; five-city 57.5 s and 57.7 s with 5 fetches (60 s before); weather 42–48 s with 2 calls
(51–64 s, 4–8 calls before; part of that may be model variation); news 154 s and 191 s, both answered (no earlier baseline for this
question). `tool result cut` lines: 17, **all `internet_search`**, 5.4–7.3 KB results kept at 3000 (13 of 17) or 4124–4360 (4 of 17), so
the "mostly gone" expectation did not hold for search on a 16k model; the whole-file fix comes from `read_file`. `fetch_url` cuts are
**not** in that count (the tool cuts its own output before `appendToolResult` logs). `cached_tokens` were 44% of prompt tokens over 42 calls
(no earlier figure). The sieve fired 5 times across the window (not attributed per run). Still open: the next nightly `daily-llm-news` run.

### Phase 1 — decision 2026-10-21: keep the final report first, park saving cut results

The user's need is that a run's **final report is not lost**. That is built separately and is small: **Phase 1-lite — saved final reports
(PR #109)**. A finished run writes `reports/<automation>/<UTC start time>-<run id>.md` into the workspace
(`WorkspaceManager.WriteReport`, `automation/executor.go` `saveFinalReport`); quiet heartbeats and runs with no reply write nothing;
credential-shaped text is masked; a write failure is logged; nothing purges them (Phase 3 stays the operator purge). SPEC-007 1.16,
`docs/data-layout.md`.

**Saving cut tool results (everything below in this phase, R1–R7 and the manifest) is parked.** Build it only if a measurement shows it
helps: how often a real run cuts a `fetch_url` page (the `tool result cut` log does not count these, because the tool cuts its own
output first) and whether the answer suffers. Folder for it, if ever built: not `reports/` (the final reports and the user's own
convention live there); open decision 6 is answered for final reports as `reports/<automation>/`.

### Phase 1 (parked) — automation spill of cut tool results

- Step 1 (read-only) — **done 2026-10-21**:
  - `AutomationRun.Output` is the full final report with a short header (`executor.go:508-510`), not cut. It is stored in
    `state.json`, where `AgentState.History` keeps only the last `MaxStateHistory = 30` runs (`models/workspace.go`), and
    `LastRuns[automation]` holds only the latest run of each automation. A run's older report is therefore lost when the
    same automation runs again and, for a frequent automation (a heartbeat), from `History` within a day. The run folder
    also keeps `final-report.md` (`rundir.WriteFinalReport`), but only when run logging is on and only for the 30-day reaper's
    window. So the answer to §4.2 is **yes, a final report can be lost**; keeping final reports is a separate need from saving
    cut tool results and is not solved by this phase unless the scope says so.
  - **Name collision:** `reports/` is already a user convention. `prompts/AGENTS.template.md:5` gives "reports go in reports/" as the example
    of a workspace preference, and tests use `reports/scan.txt`. Saved raw pages next to a user's own reports would mix two kinds
    of file; the manifest protects deletion but not the listing. Decide the folder name before building (open decision 6).
  - The journal (`tools/automation_journal.go`) is the existing cross-run channel: its text is capped and sanitised
    (`models.SanitizeJournal`). R7 should reuse that injection point, not add a second one.
  - Not checked yet: whether a real automation's toolset (e.g. `daily-llm-news`) includes `fetch_url` and `read_file`, which
    decides whether R1 would ever allow saving there.
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

**Where (SPEC-001 §2).** The physical sieve already has a first stage, "compression": older messages longer than 4,000 characters (reasoning
2,000) are cut to head+tail around `...[Truncated]...`, and only if that is not enough does it drop messages down to the locked head and a
priority tail. Phase 6 **upgrades that first stage**; it does not add a stage before the sieve. For an old tool result that carries a
recovery handle, the stage writes the one-line stub instead of head+tail; for everything else it behaves exactly as today. The sieve's
note, ledger and one-time warning, the locked head and the tail rules are unchanged.

**Two triggers, as DeepSeek has (`pressure` and `context-overflow`).**
1. *Pressure:* `measureRequest` against `requestLimit()` at `session.go:1152`, before `applyPhysicalSieve`. Needs `ContextBudget > 0`.
2. *Overflow:* in `handleContextSizeError`, the stub-first compression runs before the reactive and aggressive sieves, and the request is
   retried only if it freed enough. This covers a model whose `ContextBudget` resolves to 0 (`measureRequest` returns an empty size, so the
   pressure trigger never fires). Check in the first step which registered models resolve to 0, so the plan names who relies on this path.

**Which history it edits.** The sieve rewrites the run's working history in place (`*history = a.applyPhysicalSieve(...)`), and the
`RepairToolPairing` fix in #92 exists because sieve-made orphans reached saved conversations, which suggests the working history is what is
persisted. To confirm in the first step: trace `s.history` to the session file (`finishTurnRun`, `conversation_service.go`). The plan
follows the sieve: demotion edits the working history in place, so a demoted result stays demoted on every later turn (the prefix is stable
without any extra latching) and the persisted result is the stub. Nothing is lost because the stub names the saved file.

**Steps**

1. Pure function over (history, limit, measure) returning the demoted copy and a report; table tests over several budgets and tool shapes.
2. Wire it into the sieve's compression stage, on both triggers, in shadow mode: one log line per pass (`would demote N results, free X tokens`).
3. Replay recorded histories (including the 141 s run's request 8) offline across budgets 4k, 16k and 128k.
4. Live on Vertex in shadow mode, then enabled, comparing steps, time, answer, `cached_tokens` and whether the sieve still fires against a
   20-run baseline (the bar used for the progress monitor).

**Acceptance**

- Idempotent and deterministic: running the pass twice, or on consecutive turns, gives byte-identical stubs and requests.
- Docs updated in the same change: SPEC-001 §2, `.agents/skills/agent-loop/SKILL.md`, this plan, and `docs/architecture.md` if a pitfall is added.
- No handle means unchanged output; the existing sieve tests stay byte-for-byte unchanged and green.
- Pairing intact on every generated history; the persisted conversation holds stubs, never a call without its result.
- Adaptivity: the same history at 4k, 16k and 128k demotes progressively fewer results, never more than needed.
- Live, against a 20-run baseline on the same build and model: no run answers worse; median steps and time not higher, and the worst run
  not worse than the baseline's worst (the same build varies by a few calls); the cache hit rate after the first pass is within 10 points
  of before it.

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

Saved final reports (#109, #110 merged and live-checked 2026-10-21; the "Saved reports" history panel is built in a follow-up PR, SPEC-007 1.17). Parked until measured: saving cut tool results (the rest of Phase 1), Phases 2–6. Phase 0 is built and live-checked (#99–#102); Phase A is built (#106) and passed its live acceptance (above), except the next nightly `daily-llm-news` comparison. Phase 1 step 1 is done (findings above). Next: live-check #109 on Vertex, then measure `fetch_url` cuts before deciding whether to build the parked work. Phase 6 waits for Phases 1–2.
