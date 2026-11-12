---
id: ADR-0049
title: The audit's assessment reads a bounded window digest that the scan result carries
status: accepted
date: 2026-11-12
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0049: The audit's assessment reads a bounded window digest that the scan result carries

## Context
The owner read a real audit (vertex, v0.77.0, run 06356dd5) and found the model's assessment light: it was given only this scan's hosts and ports and one check finding, while the program also kept 54 findings, the finding history, alerts, the activity feed and daily counts. It also got three facts wrong (devices up, a severity, and "no finding for SSH" while a stored one existed). A run's order is: the model scans and writes, then `Augment` adds known vulnerabilities, host watch, UDP, web, router and stack findings, compares with the last scan and writes the baseline. The baseline keeps only the latest scan, so there was no record of how a day's scans differed. The owner asked for the assessment to use the data of the day (a week for a weekly audit), generically, with no scenario text in the prompt and no new way for the model to breach anything.

## Decision
1. **One pure reader, `netaudit.DigestOf`**, builds a bounded typed digest for `[now - window, now]` from records the program already keeps (the scan log, the finding history, the latest saved scan, the alert log, the daily series, the inventory). It starts no scan and sends no packet. Only the scan log is new: one row per saved audit (counts and finding keys; 14 days, 200 rows), written by the report step after the baseline, never by a model.
2. **The scan result carries it.** `network_scan` appends the digest in its own untrusted-data fence for an audit run only (the run has scan settings); an ordinary chat gets the scan alone. It is attached to the result, not offered as a second tool: a local model told to call the scan once may never call another. The model gets **no new trigger**; route trace (I.14), the router read (I.15), the deep check and the capture stay with the operator or the audit's own code.
3. **Aggregated in code, bounded by bytes.** Exact counts, the latest 12 scan rows, 25 open problems worst first, 15 per change list, newest first, and a 9,000-byte ceiling (about 3k tokens on a dense result) for the rendering (the longest list is halved until it fits, and each cut is stated). A record that cannot be read is named in the digest, so it is never read as a quiet network.
4. **What the digest leaves out, by owner decision (2026-11-12):** findings the operator accepted, and known-vulnerability findings (the report adds those after the assessment, by design). Activity is counts by kind; a device's name, address or hardware address and an alert's text are never in it.
5. **The window is a structured setting** (`ScanSettings.Window`: `day` or `week`, a template header `**Window:**`, a form field), capped in code. The weekly audit is a second template over the same profile.
6. **The count in the tool's header is typed** (devices that answered, of addresses looked at), so the model does not count a list. The prompt describes the data and asks for reasoning across the window, and keeps the facts the code writes (counts, severities, the comparison with the last scan) out of the model's hands.

## Consequences
- The model sees the previous scans' history, not this run's add-ons (they are computed after it writes); the latest saved scan holds the earlier add-ons' findings.
- A scan log row exists only from the first audit after this version; a window with no rows says so.
- The weekly audit's own scan rows are only its own (one a week); the finding history, alerts and inventory are workspace-wide. Reading the daily audit's rows for a weekly one is left open.
- Loosens "the model sees only this scan" for audit runs. The model stays local-only, device text stays inside the fence, and the operator's notes and the router key are never in it.
- 2026-11-12 (v0.78.1, from the first vertex run of v0.78.0): **the digest is reconciled with the scan it travels with, and accepting a check hides its port.** The records end at the scan before this one, so a digest that said "gone" for a port the new scan showed open made the model state it as fact (it cannot diff two lists). `DigestOf` now takes the scan being made (`CurrentScan`, built from the full result): an open TCP port the records call gone is moved to "returned", one they call new or open all window but the scan shows closed is moved to "gone" (dated as of this scan), an open finding whose port is closed now is marked `not_in_this_scan`; UDP, whole-device findings and devices the scan did not finish are left as recorded. The digest names when its records end (`records_end_at`), and an empty scan log beside a saved scan says so instead of "no scan was recorded". `HideAccepted` hid only the bare open-port finding, and the findings are rebuilt from the hosts, so an accepted `telnet_open` came straight back: an accepted check finding now takes its port out of the hosts (a known-vulnerability acceptance does not: it is about those CVEs), and the digest hides every finding on that port. Down addresses are no longer listed in the scan result (a /24 listed 254 `{"up":false}` rows); the header keeps the count.

