---
id: ADR-0026
title: Packaged program files are compared with the checksums dpkg keeps, and a pass that is cut short never resolves a finding
status: accepted
date: 2026-11-04
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0026: Packaged program files are compared with the checksums dpkg keeps, and a pass that is cut short never resolves a finding

## Context
The owner approved a nightly check that system programs still match their packages (plan `network-attempts-integrity-timeline`, P3). The service user `llm-proxy` is in no privileged group (captured on vertex 2026-11-04: uid 995, groups llm-proxy, video, render), and `/var/lib/dpkg/info/*.md5sums` is world-readable (`-rw-r--r-- root`, lines `<md5>  <relative path>`). Findings may carry only the program's own wording (the ledger rule on text a device or file wrote).

## Decision
1. **Read dpkg's own lists, start no program.** `platform/hostwatch.Integrity` reads each `<package>.md5sums`, skips the package's `.conffiles` (people edit those), and compares only files under `usr/bin`, `usr/sbin`, `usr/lib`, `usr/libexec`, `bin`, `sbin`, `lib`, `lib64`. A missing file, a symlink, a malformed line or a path with `..` is ignored, never reported. MD5 is dpkg's format and is used only to compare, never to protect anything.
2. **Re-check before reporting.** A file that differs is looked at again against its package's list as it is then. A package update that finishes during the pass rewrites the list and the file together, so it is not a finding.
3. **A pass that is cut short is never an all-clear.** The pass has a time budget (3 minutes, `hostIntegrityBudget`). When it ends first, the result is incomplete: it still reports what it found, and when it found nothing the earlier result is carried (`HostWatch.Integrity`), so a night that ran out of time does not resolve last night's finding. A computer without dpkg lists reports nothing at all. Files that could not be read, or are over 512 MiB, are counted as not looked at and join the existing "watched locations could not be read" note.
4. **The finding is `host_file_modified`, high.** One finding with the count and up to ten paths; a path is listed only if it is a plain absolute path (`[A-Za-z0-9._+@/-]`, at most 200 characters), the rest are counted. The wording says a change by hand, by an attack or by a failing disk looks the same. The fix guide points to `dpkg --verify`, `dpkg -S` and a reinstall, and says to distrust the computer if the change is unexplained.

## Consequences
- A computer whose owner patched a packaged binary by hand shows the finding every night until it is accepted or the package is reinstalled.
- Files added by hand, and files no package lists, are not seen; this is not a malware scanner.
- The pass reads the whole program tree once a night. Its time on vertex is not measured yet (the budget bounds it); until measured the first nightly run may be incomplete.
- An attacker with root can rewrite both the file and its list; this check catches changes that do not.
