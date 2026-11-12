---
status: partial
date: 2026-11-12
last_reviewed: 2026-11-12
related_specs: [SPEC-003, SPEC-007]
constitution_references: [I.8, I.14, I.15, II.13]
evidence: vertex v0.78.0 to v0.81.0 audit runs 06356dd5, 122538f7, adb7df0e, 9362b046, 037b81db, 7f0ea49e read through the admin API and their recordings; PRs #232 to #238; ADR-0049; docs/PLANS/cross-cutting/network-audit-assessment-window.md; the owner's confirmation that the UI works (2026-11-12).
---

# Network audit · the assessment and its report (handoff)

The work of 2026-11-12 is **built, merged (#232 to #238, v0.78.0 to v0.81.0), deployed on vertex and checked by the owner and from the runs' recordings**.
Read this file first; `network-audit-assessment-window.md` holds the design history and ADR-0049 the decisions. Nothing below blocks anything.

## Second session, 2026-11-12 (Results tab and run folders): built, committed on `audit-handoff-2026-11-12`, v0.82.0, not run on vertex

Owner asked for a useful, structured report (friendly, expandable, clean on a phone), a folder per run, and the new files kept out of pickers. Built (ADR-0050, SPEC-007 1.26):
- **Finding details:** `ScanFinding.Details` (label/value rows); the host watch names the files of a changed startup place (added/edited/removed, plain paths + 12-digit sum); the integrity finding lists its files; the Markdown report writes details after the evidence.
- **Devices that answered:** the baseline keeps `UpAddrs`; the Devices tab folds the ones with nothing open under "Answered, nothing open" (it showed 10 of 15 up). Header says "and N more you accepted"; the DNS tally says "comparisons".
- **Run folder:** `reports/<automation>/<start>-<run>/report.md` with `audit.json` beside it (`AuditRecord`, version 1); `ReportHook.Files`; old flat reports still list and age out; prune keeps a person's sub-folder.
- **Overview on the Results tab:** `AuditOverview.vue` above the problems (assessment, figures, one folded line each for the window, exposed services, background checks, notes), read from `audit.json` through the workspace file route; shown only for the scan the findings come from.
- **Picker:** the automation task-file picker no longer lists `reports/`.
**Checks run:** Go build, `go test ./...`, complexity (no new violations); frontend build (lint, palette, contrast), 3,031 tests; all 294 netaudit visual specs at 1440 and 390 px (screenshots of the new parts read at 390 px, no sideways scroll); harness check. **Not run:** `-race` (no concurrency changed), the non-netaudit visual specs, anything on vertex.
**On vertex after deploy:** the first audit writes the first run folder and `audit.json`, so the Overview appears after that run; older flat reports stay listed; the first changed look of each startup place says earlier changes cannot be named, then names changes; the Devices list appears after the next audit. **Check these by hand:** open the Results tab after a run, open each folded part, and look at the History list (old and new reports together).
**Left, in order:** (a) a **per-run findings record** (the baseline keeps only the latest scan) so the Results tab can show any earlier scan or the change between two: put the findings in the run folder beside `audit.json` (it needs a retention rule and a size look; the run folder already prunes with the report); (b) retire the Markdown regex parser (`utils/netaudit/auditReport.ts`) once old flat reports have aged out (200 reports, so months at one a day), and show the record in the History list's opened report; (c) the weekly audit's own scan rows and the router log item below.

## State

- **Branch / PRs:** all merged to `main` (last: #238, `c778f1b0`, v0.81.0). This handoff is a docs-only change on `audit-handoff-2026-11-12`, uncommitted.
- **What the audit report is now** (`RunReporter.Augment`, `core/netaudit`): the Observed section is **all code**: what was scanned; New / Resolved / Changed since the scan
  before; **Still open** (every check finding not accepted and not under New); known-vulnerability coverage; **Exposed services worth reviewing**
  (`exposedservices.go`, confirmed service names only); **Last 24h / 7d before this scan** (`windowreport.go`, the same `WindowDigest` the model reads, plus the router DNS
  check's 24 h tally); notes; **Ways in that matter**. The model's assessment is **at most three items on what to look at first** (`security-auditor.md`).
- **What the model is given** (`scantool`): a compact scan view sized to the agent's tool-result cap (answering devices, open ports, no open-port findings, **no guessed
  service or product names on any path**, `ConfirmedText`), then the window digest fitted to the room left. Accepted checks hide their port everywhere (`CoveredPorts`).
- **UI:** `AuditReport.vue` (saved reports, run drawer) shows the assessment first, what is new and still open in view, reference parts folded with a one-line summary;
  saved reports fold to one line with the opened report above them; the running tab is brand-filled; the Automations page refreshes on a tab's return and ignores a late reply.
- **Checks run on the final code:** Go build and tests, complexity check, stack SDK and network module tests, harness check; frontend 3,008 tests, build (lint, palette,
  contrast), 366 visual tests. **Run on vertex:** v0.81.0 audit `7f0ea49e` (report structure, DNS line, assessment: 3 items, 91 words); the UI confirmed by the owner.

## How to verify an audit run (the recipe that found everything)

Judge a model's output against its **recorded input**, not against the report text. With the admin token in `LLM_PROXY_TOKEN`:
`GET /admin/api/dispatcher/activity` (the run's `run_dir_name`), then `GET /admin/api/recordings/content/<workspace>/<model>/<automation>/<run_dir_name>/recording.jsonl`.
Take the last `request`: the `tool` message is a JSON-encoded string (parse with `strict=False`). Search it for `SYSTEM TRUNCATED` first. To try new prompt wording without a scan,
replay that request against `/v1/chat/completions` with the configured model name (not the gguf file name) and the new system prompt and task text; it loads the owner's model.
Details in `testing-guide` (SKILL.md, "Analysing a Run") and `docs/learnings/network-audit.md` (2026-11-12 entries).

## What is left, in order

1. **Router log sends nothing.** The listener is on port 5514 and listening, but `queries_read` is 0: the router is not sending its log to vertex, so "connection attempts the
   router refused" is always absent. *Owner:* a router setting (remote syslog to vertex:5514). No code needed unless the listener itself is suspect.
2. **Weekly audit's own scan rows.** The weekly template reads a week, but the scan log's rows for it are only the weekly audit's own (one a week). Reading the daily audit's rows
   for it needs a decision on mixed targets. *Owner decides*; small.
3. ~~"90 checks" in the DNS line~~ now says "comparisons" (second session).
4. **The model's one-line scan summary before its items** appeared on one run and not the next (the profile forbids it). A prompt patch risks more drift than it fixes; if it
   becomes common, strip a leading summary line in code rather than rewording again.
5. **Known-vulnerability findings stay hidden from the model** (owner's decision, 2026-11-12); they are in the report's "Still open". Changing it is a decision, not a fix.
6. **The digest does not include this run's own add-ons** (computed after the model writes); the report's window section is built at the same moment and has the same limit.

## Traps found (so they are not found again)

- A prompt or task text that **describes the report must match what `RenderObserved` writes on a repeat scan**, not only on a first scan (v0.79.0's profile promised
  "every check's problem" and a repeat scan listed only what changed).
- A workspace's `network-audit.md` is **an older copy of the template and is never refreshed**: every rule belongs in the profile, which ships with the binary. After a template
  change the owner re-copies the task file.
- The run's context carries the audit's name and scan settings but **not the workspace** (a tool call adds it): code called from `Augment` takes the workspace and audit explicitly.
- The agent's tool-result cap cuts a result's middle; a tool that returns a lot reads `models.ResultCapFrom(ctx)` and sizes itself.
- A stack's data reaches the core only as typed fields (ADR-0036); port types are checked as plain data (no pointers; `omitzero` for an optional struct).
- Marking a guessed name "(guess)" does not stop a local model repeating it; leave the name out.

## First command or file to open

`docs/learnings/network-audit.md` (the last six 2026-11-12 entries), then `backend/internal/core/netaudit/report.go` (`RenderObserved`).
