---
id: ADR-0050
title: A finding carries typed details, and the host watch keeps file names with a short sum for each
status: accepted
date: 2026-11-12
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0050: A finding carries typed details, and the host watch keeps file names with a short sum for each

## Context
The Results tab of an audit (read 2026-11-12) showed "Startup services changed on this computer" with "The audit does not show what changed". The host watch kept one SHA-256 per watched place (`models.HostFile{ID, Hash, Tag}`), so it could say a place changed and never which file. Other findings packed their facts into one sentence (the changed program files' paths), which a screen could not lay out. The owner wants the report useful: structured, friendly, expandable, and clean on a phone.

## Decision
1. **`ScanFinding.Details`** is an optional list of `{label, value}` rows next to `Evidence`. Evidence stays the check's fixed sentence; Details are the facts behind it. Every value is fixed words or text vetted by the check that wrote it (a plain path, a count): never raw device text. Older saved scans have none and read as before.
2. **The host watch lists the files of a watched place**: each file or link the digest covers, with the first 12 hex digits of its own digest (`hostwatch.FileEntry`), only for plain paths (`^/[A-Za-z0-9._+@/-]{1,200}$`, the shape integrity paths already use), at most 1,000 per place (a bigger place keeps only its digest and says so). No byte of any file is kept; a short sum of a file's content, with its path, is what a later look compares. The first look's list is the reference, as its digest was; the baseline never moves.
3. **A change is named against that list**: Added, Edited, Removed (ten names, then a count). A place first recorded before lists were kept gets its list at the next look that sees it changed, and says that what changed earlier cannot be named; names are given from that look on.
4. **The report's Markdown writes details after the evidence** in the Checks table, so the text a reader (or the model) sees names the files too.
5. **The baseline names the devices that answered** (`ScanBaseline.UpAddrs`, sorted), so the Results tab can list devices that answered with nothing open. Before, "15 devices up" showed ten.

6. **A run's files share one folder, and the audit leaves a typed record in it** (added the same day). A saved report is `reports/<automation>/<start time>-<run id>/report.md`
   and a `ReportHook` may add files beside it; the audit adds `audit.json` (`netaudit.AuditRecord`, version 1: counts by severity, the window digest without its open-problems list,
   exposed services, vulnerability coverage, notes, the model's assessment). It is written next to the Markdown by the same step, masked like it, and the Results tab reads it
   through the workspace file route (no new route) and shows it above the problems when it is of the scan shown. Reports saved before run folders keep their flat path and age
   out; nothing is moved, because the run history holds those paths. The task-file picker leaves `reports/` out: a result is not a task.

## Consequences
- Names of files under `/etc/systemd/system`, the cron folders, sshd config, sudoers and similar are now stored in `host_watch.json` (plain paths and a 12-digit sum, never content) and shown in the report. They are on this computer's own account-readable places; the paths are shape-checked, so a file name cannot carry text a model would read as an instruction.
- A baseline taken before this change cannot name earlier changes. The finding says so instead of staying silent.
- The UI shows details as grouped rows (one label, many values), folded after five values, monospace and wrapping.

## Rejected
- Keeping file content or diffs: the watch's rule is that no byte of a file is kept.
- Re-baselining on accept: not asked for; accept keeps meaning "this is fine".
