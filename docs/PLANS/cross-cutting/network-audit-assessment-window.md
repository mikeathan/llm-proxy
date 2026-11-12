---
status: partial
date: 2026-11-12
last_reviewed: 2026-11-12
related_specs: [SPEC-003]
constitution_references: [I.1, I.8, II.13]
evidence: vertex v0.77.0 run 06356dd58f9747f4 (2026-11-11 20:02Z), read through the admin API; backend/internal/core/netaudit/scantool/scantool.go; backend/internal/core/netaudit/runreporter.go (Augment); backend/internal/core/assistant/prompts/profiles/security-auditor.md; the owner's request of 2026-11-12.
---

# Network audit · the assessment reads the window's data

**Status:** partial (branch `audit-assessment-uses-the-days-data`, from `origin/main` at v0.77.0, v0.78.0: phases 1 to 4 built and tested, uncommitted; **not run on vertex**; ADR-0049).

## Why

The owner's reading of a real audit (vertex, v0.77.0): the model's assessment is a port-and-service triage with no reasoning across time, while the
system holds much more. Measured on that run:

- The model was given only `network_scan`'s result: `{engine, hosts[ports], findings, warnings}` (`scantool.go:112`). Script output and CPEs are `json:"-"`.
- The saved result held 54 findings (known vulnerabilities, web exposure, UPnP, SSH password login, host-watch findings, attack paths); the model saw 1 check finding.
- Its own text then got three things wrong: "10 up" (the system counted 15; the header prints `len(res.Hosts)`), Telnet "critical" (the system said high), and "no check findings for SSH" while a stored SSH finding existed.
- **Order of a run** (`runreporter.go:337`): the model scans and writes, **then** `Augment` computes known vulnerabilities, host watch, UDP, web, stack and router findings, compares with the last scan and writes the baseline. So nothing the model can call during its turn holds this run's add-ons; what is stored then is the **previous** scan's.

## How often scans run (vertex, measured 2026-11-12)

30 recorded runs of `network-audit` over 9 days: one scheduled run a night (`0 2 * * *`, 9 of 30 at 02:xx) and the rest manual: 1 to 8 a day (8 on 5 Nov
while testing; 2 to 4 a day since 9 Nov). A run takes 24 s to 5.7 min. The presence check (model-free, every 10 minutes) is a look, not a scan. So a "day" holds
**about 1 to 8 scans**, a "week" at most a few dozen, and **volume is not a problem if the digest is aggregated in code and bounded**; reading
the raw findings of every scan would be.

## Design (generic, no scenario text)

1. **One pure, model-free reader** over records the program already keeps, like `findingsview.Of` and `ActivityOf`: it returns a bounded, typed **window digest**
   for `[now − window, now]`. It starts no scan and sends no packet.
   - Per scan in the window (a small **scan journal**, one row per audit: time, how it started, engine, devices up, open ports, findings by severity, new and resolved counts; kept 14 days, at most 200 rows; this is the only new store, because the baseline keeps only the latest scan).
   - Per finding (existing `FindingLife`): first seen, open since, came back, resolved, by whom.
   - The previous scan's open findings with their severity (the baseline holds every add-on's findings), worst first.
   - The window's activity rows and alert counts (existing `ActivityOf`), the daily series, devices by kind from the inventory.
2. **Aggregated in code, not by the model:** counts per scan; findings **present in every scan** vs **flapping** vs **new in the window**; the device count's low and high; a fixed cap per list, worst first, with the cut stated; a token budget set from the model's context (a dense result once overflowed 16k; `findingstool.go:21`).
3. **Attached to the `network_scan` result**, not a second tool: a local model told to "call once" may never call another, and attaching costs no turn. The digest is fenced with `prompts.FenceScanData` (device text is data), wording in `templates.go` (II.13).
4. **The window is a structured automation setting**, parsed like Depth: `**Window:** day|week`, capped in code. Weekly is a second template with the same profile and a longer window; the prompt describes the data sections and asks for reasoning across the window, with no per-scenario instructions.
5. **Profile rewrite:** the model is told what the sections are and to reason over them; it no longer writes the count table or severities (the "Observed" section has them); the "never say new, changed or gone" rule is replaced by "do not repeat the Observed tables".

## Security (kept in code)

- The model gets **no new trigger**: only `network_scan`, as today. Route trace (I.14), the router read over SSH (I.15), the deep check and the capture stay on the operator's press or in the audit's own code. The reader never calls them.
- The audit model stays local-only (`local_only: true`); the digest is typed fields and fenced device text; the operator's notes, the router key and the router's connection lines are not in it.
- Findings the operator **accepted** stay hidden from the digest (as `HideAccepted` does today), and known-vulnerability findings stay added after the assessment: both are design choices from the owner. Showing them to the model reverses that and is a separate decision (below).

## Phases

| | Phase | Acceptance |
|---|---|---|
| 1 | Scan journal: a bounded store written by the report step after the baseline | tests: written per audit, trimmed by age and count, a write failure never changes the report |
| 2 | `windowdigest`: the pure reader, aggregation and caps | tests with a day of 8 scans and a week; size bound; accepted findings absent; nothing a device wrote outside the fence |
| 3 | `network_scan` attaches the digest; header counts from typed fields (devices up); `ScanSettings.Window` through settings, template, API and form | scantool test; config round trip |
| 4 | Profile and templates (day, week) | a replay on the local model that writes a recording, compared with run 06356dd5 |

## Decisions (owner, 2026-11-12: both defaults taken)

- The assessment does **not** see operator-accepted findings or known-vulnerability findings (as today).
- The window of a manual run is the last 24 h (week: 7 days).

## Built (v0.78.0)

Phase 1 `scanlog.go` + `persistence/scanlog.go` (named "scan log": `journal` is the automations' journal) and `RunReporter.WithScanLog`; phase 2 `windowdigest.go` (pure, 9,000-byte ceiling, a mutation check on the accepted-findings rule); phase 3 `digestsource.go`, `scantool` attaches the digest in its own fence for audit runs only, a typed "devices answered of addresses" header, `ScanSettings.Window` through validation, template header, form field; phase 4 the rewritten `security-auditor.md` profile, `**Window:**` on `network_audit.md` and a new `network_audit_weekly.md`. Checks run: `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`, frontend `npm test` and `npm run build`, `./scripts/check-agent-harness.sh`.

## Remaining Work

1. **Deploy and replay on vertex** (the phase 4 acceptance, not done): press Run on `network-audit`, then read the report and the digest in the run record. Compare with run `06356dd5`: devices up, severities, whether it uses the window (needs a recording; the run had none). The first digest after deploy has one scan row, so the window grows over the next days.
2. The weekly audit's scan rows are only its own; reading the daily audit's rows for it needs a decision on mixed targets.
3. The digest does not include this run's add-ons (computed after the model writes); the latest saved scan carries the earlier ones.
4. If the local model's context is small, lower `DigestMaxBytes` (`windowdigest.go`).
