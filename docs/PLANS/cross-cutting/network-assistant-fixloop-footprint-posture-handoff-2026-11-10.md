---
status: partial
date: 2026-11-10
last_reviewed: 2026-11-10
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.2, I.8, I.11, II.3]
evidence: docs/architecture/network-capabilities.md (read 2026-11-10); ADR-0019, ADR-0020, ADR-0023 (read 2026-11-10); backend/models/tool_catalog.go:66-70 and backend/internal/core/toolregistry/registry.go:644 (get_network_info reads the computer's own network info; network_scan is the only netaudit tool); backend/internal/core/netaudit/scantool/scantool.go (the tool module pattern); backend/internal/core/netaudit/followup.go (the one targeted re-scan that exists); stacks/network/dnscheck (the precedent for a list of names, an interval and a step-up opt-in); backend/models/device_inventory.go:45 (FirstSeen exists for devices, not for findings). Nothing below is built. Facts about outside services (crt.sh, RDAP, the breach service) are from memory and UNCHECKED.
---

# Network: four things that are not more sensors (assistant, fix loop, footprint, posture)

Handover for a new session. The owner chose all four on 2026-11-10, after asking for "something away from what we already have, which is getting repetitive". Everything built so far answers "what is wrong on my network?". These answer other questions.

## Status 2026-11-10 (end of the session that built it)

Branch `network-assistant-tools` (cut from `d32cfa1a`, v0.69.0), two local commits, **not pushed**, and later work uncommitted; `VERSION` set to 0.70.0 in the working tree. **All four tasks are built; none has run on vertex.**
- **A:** `network_findings` (`core/netaudit/findingstool`, category `netfindings`, listed in the `general` and `operator` profiles), over the shared `core/netaudit/findingsview`. `network_devices` and `network_timeline` were left out on purpose: each tool costs schema tokens in every prompt, so add them only after the three real questions have been tried.
- **B:** `finding-life.json` per workspace (`AdvanceLife`, `UpdateLife` under a lock), `POST /admin/api/netaudit/recheck`, "Check now" on the finding card, "came back N times", Timeline rows.
- **C:** `core/netaudit/footprint` (ADR-0042): crt.sh certificates, RDAP expiry, SPF and DMARC over DoH, breach k-anonymity range search; settings, schedule loop, Check now, step-up routes, `FootprintCard.vue` in Network → Settings. Findings are `domain:<name>` and `email:<id>`.
- **D:** `standing.go` and `StandingCard.vue` on the Overview, from the finding history only.
- **Checks run:** `go build`, `go test ./...` (one unrelated timing test in `guardrails` failed once under load and passed three times after), `go run ./tools/check-complexity/`, `go test -race` on the history store, footprint and tool packages, `npm test`, `npm run build`, the harness check. **Not run:** `npm run test:visual`, anything on a real server, the breach lookup with a real key (none was available), a live crt.sh answer (it returned 502 twice).
- **Left for the owner, in order:** (1) deploy and press "Check now" on a real domain; (2) paste a breach key whose plan includes the private search, or accept "could not look: key refused"; (3) ask the assistant the three questions (urgent this week, what changed, why is X flagged) and keep the transcript in `docs/learnings/network-audit.md`; (4) after two weeks the standing card shows its first comparison; (5) decide `network_devices` and `network_timeline` from step 3.
- **Traps:** the history starts at deploy, so "first seen" means first seen since this version; a footprint watch is carried, not resolved, when a look is missing or older than twice its frequency; the guarded client logs a request URL on failure, which holds the watched domain (a local log), and never an address (the breach look sends six hash characters only).

## 0. Gate: do this before anything else

1. PR #221 (v0.69.0: closer TLS look, macOS host look, deep-check warnings) must be **merged and deployed**. Check: `gh pr view 221 --json state,mergedAt`, `git log origin/main -3`, and the version the deployed server reports (`GET` its status, or the VERSION file on vertex). If it is not merged, stop and tell the owner; do not build on the branch.
2. Cut a **new branch from `origin/main`** (PRs here are squash-merged, so the old branch's history is not on main). One branch and one PR per task below unless the owner says otherwise; A and B first.
3. `docs/architecture/network-capabilities.md` is the "what already exists" list. Read it, then ADR-0023 (rejected ideas), before touching anything. Add a row there for every capability you add, in the same change (AGENTS.md).
4. Open items from #221 that the owner still has to do on vertex (not blockers): the Traffic Listen check, `verify-deployment.py --compare`, and `apt install testssl.sh` then a deep check (the jailed testssl.sh stalls on Linux; see `network-host-and-tls-looks.md` item 2).

## 1. The owner's brief, and the answers already given

- **A. Let the assistant talk about the network.** In chat: "anything urgent this week?", "why is this device flagged?".
- **B. Fix, then prove it.** "I did it, check now", a record that it worked, and findings that keep coming back called out.
- **C. Footprint outside the LAN.** The owner **has domains and email addresses to monitor**. It must be **settable in the UI: what to monitor and how often**. The owner asked: on demand, or on each audit run?
  - **Answer (recommendation, owner can overrule): both.** Every watch has a **Check now** button, and a scheduled frequency chosen in the UI per watch: *Only when I press Check now / Daily / Weekly* (an interval setting like the DNS check's, ADR-0020, which runs on its own interval and not inside the audit). The nightly audit's report then **reads the latest result** and carries it (the `Exposure.Carry` idea of ADR-0019: a look that could not be made is "unknown", never "clean"). Why not "only on the audit run": it ties a cheap lookup to the audit's schedule and failures, and a slow source (breach lookup) would be asked as often as the audit runs. Why not on demand only: nobody presses it.
- **D. A posture view and a trend.** Last, because it needs A to C's data. Keep it plain; see section 5.

## 2. Rules that apply to all four (read these, they are why earlier sessions lost time)

AGENTS.md, then `CONSTITUTION.md`, the subsystem hub `docs/subsystems/network-audit.md`, SPEC-003, the rule files and skills it names (`go-staff-engineer`, `frontend-vue-engineer`, `task-planning`, `tdd-guide`, `documentation-stewardship`, `engineering-practices` for the ordered checklists). Specifically:
- **TDD**, build after each Go edit, **`npm run build` and commit `frontend_dist/index.html`** with every frontend change, `go run ./tools/check-complexity/` (functions over 12 are refused; split them), `./scripts/check-agent-harness.sh` after docs.
- **Git is read-only** unless the owner approves each mutation. The owner has approved commit, push and PR for #221 only.
- **Untrusted text (I.8):** a device name, a certificate name, a hostname from an outside service, a breach name: text somebody else chose. Findings carry only the code's own wording (ADR-0036); text from outside is validated (plain DNS names only, bounded) or kept out. The model gets findings and typed counts, never raw text.
- **Outside calls (I.2):** only through the guarded client (`NetworkTools.Open` / `OpenAccepting`), **off by default, a step-up switch that says what the outside service learns** (ADR-0019, ADR-0020).
- **Verify before you cite.** Three times a "new" idea already existed. Grep the check ids (`grep -rn 'Check[A-Z][A-Za-z]* *=' backend/internal/core/netaudit`).
- **Memory of the repo:** a finding, trap or failed approach goes in `docs/learnings/network-audit.md`, a choice between alternatives in an ADR, changed behaviour in SPEC-003.

## 3. Task A: the assistant reads the network

**Progress (branch `network-assistant-tools`, uncommitted, v0.69.0 unchanged):** built `network_findings` (`core/netaudit/findingstool`, category `netfindings`, manifest `netfindings.json`), the shared `core/netaudit/findingsview` (the Findings handler now uses it), `prompts.FenceScanData` and `findingsview.Line`; wired in `app/netaudit.go`; `general` and `operator` profiles list the category. Go tests, build and complexity pass. **Left:** run it on a real server and keep the transcript; then decide `network_devices` / `network_timeline` (or one tool with an `about` enum) from what the real questions need; bump VERSION and write the PR. Design reasons are in `docs/learnings/network-audit.md`.

**Verified facts.** The netaudit category has one tool, `network_scan` (`models/tool_catalog.go:70`; it probes hosts, so it is not read-only). `get_network_info` (`registry.go:644`, `Reach: LAN`) reads the computer's own network information, not findings (I did not open its output). The model already sees scan results only as a fenced block (`<untrusted-scan-data>`, `scantool.go`) with accepted findings left out (`AckSource`).

**Build.** Read-only tools over the data the Findings and Devices tabs already read, so there is **one home** (reuse the finding viewer in `transport/http/handlers/netaudit_handlers.go`, `newFindingViewer`; do not write a second reader). Suggested set, to be cut down by what the first real questions need:
- `network_findings` (open and accepted counts; per finding: check title, severity, device label, age, accepted or not),
- `network_devices` (counts and the new-this-week list),
- `network_timeline` (the Timeline's records since a date; `core/netaudit/activity.go`),
- later, `network_explain` for one finding (the incident joins, `incident.go`).
Register each with `Effect: EffectReadOnly` in `models/tool_catalog.go`, a handler through `toolregistry.Handle` like `scantool.Module`, and a tool-manual entry (follow the "tool" checklist in the `engineering-practices` skill). Decide what gate it needs (it reaches no LAN; `GateNetwork` is probably wrong, check `tool_catalog.go`).

**Decisions to take (ask the owner only for the first):** device names are untrusted text: show the address and maker, or fence the name; are accepted findings shown to the model (default: no, as `network_scan`).

**Done when:** three real chat questions (urgent this week; what changed; why is X flagged) are answered from real data on a running server, a transcript is kept in the learnings ledger, and a test proves a hostile device name cannot reach the model unfenced.

## 4. Task B: recheck and closure

**Verified facts.** The one targeted re-scan today is the follow-up of a new device (`followup.go`: `FollowUp.Offer/Step`, `wanted` only for a fresh unnamed device). Accepting a finding is `ack.go`. A finding disappears when the next audit no longer sees it (baseline diff in the nightly report). **No finding has a first-seen or resolved time** (`FirstSeen` exists for devices only, `models/device_inventory.go:45`), so "how long did it take to fix" and "keeps coming back" cannot be answered today. Scans run one at a time (`Service`), and intrusive checks are refused outside the deep check (ADR-0012).

**Build.**
1. **A lifecycle record per finding key** (first seen, last seen, resolved at, resolved by: recheck or audit, times it came back). One small store, written by the report step that already diffs the baseline; read by the Findings view. Check what the key is before designing (`ack.go`, `stackfindings.go`): do not invent a second identity.
2. **A "Check again" button per finding** that re-runs only what produced it: map each check id to a recheck kind: *one-device scan of that port or script*, *host look*, *deep check (opens the existing dialog, step-up)*, or *not recheckable* (router-log events, history). The answer is **fixed / still there / could not check**, and a failed scan is never "fixed" (the same rule as ADR-0019). Refuse politely when another scan is running.
3. **"Keeps coming back"** is shown on the card (`came back N times`), and the Timeline gets a "verified fixed" entry.
**Not in scope:** executing fixes (ADR-0010).

**Done when:** on a real device a finding is fixed by the owner, "Check again" says fixed, the card and Timeline show it, and re-breaking it makes the next audit say it came back.

## 5. Task C: footprint outside the LAN

**Precedent to copy:** `stacks/network/dnscheck` and ADR-0020: a list of names set in the UI (validated), an interval setting, a step-up opt-in that says what leaves, results kept bounded, "could not check" with a fixed reason, never "fine". Sensors live in a stack with typed results; the core words the findings (ADR-0036, ADR-0038); read ADR-0028, ADR-0032, ADR-0035 and the "Adding a Frontend Settings Tab" checklist (`docs/architecture/checklists.md`) before designing. Decide whether this is a new stack or a module of the Network stack; recommendation: its own module under the Modules group (ADR-0022), because it does not look at the LAN.

**UI (the owner's requirement).** A "Footprint" module page: a list of **domains** and a list of **email addresses** (add, remove, validate), for each watch a **frequency** (Only when I press Check now / Daily / Weekly), a **Check now** button, the last result per watch with a plain status (looked / could not look and why / failed), and one step-up switch per outside service saying what it learns. Simple and Advanced as the rest of Network (`feedback`: one path per job, problems shown with their fix, mobile).

**Checks, per domain (sources to be measured read-only first; all UNCHECKED):**
- **New certificate issued** for the domain or a subdomain (Certificate Transparency). Candidate sources: crt.sh JSON (free, no key, often slow or down) or a CT monitoring API with a free tier. A name in a certificate is text: validate as a plain DNS name, bound the count, keep it out of prompts. Finding keyed on the domain.
- **Expiry** from RDAP (public, no key; the server per TLD comes from the IANA bootstrap file): expires in under 30 days, expired.
- **Mail protection records**: SPF and DMARC present and not weak (no SPF, `+all`, DMARC `p=none` or missing). A plain DNS question; reuse the DNS-over-HTTPS client the DNS check built so the router cannot hide a wrong answer.
**Checks, per email address:**
- **Breach lookup.** CORRECTION of what the owner was told earlier: the "only a hash prefix leaves" protection belongs to the breached-**password** service, not to the address lookup. An address lookup **sends the full address** to the service, and (from memory, UNCHECKED) the main service's address lookup needs a **paid API key**. So this check needs: the owner's explicit opt-in per address, a key stored as a secret (secrets store, step-up, never logged), and a measured read-only try before anything is promised. If the terms do not fit, drop the per-address check and say so; a domain-wide lookup (if offered) may be the better shape.
- Keep only the breach name and date as typed fields; the address is not put in a finding, alert, log or prompt beyond a masked label.

**Traps already visible.**
- `Finding.Device` is the key of every finding, acceptance and diff and holds a device address. A finding about a domain has no device. This is a **contract question for the owner** (like ADR-0017, and the open question in the roadmap's Phase 0 about the host): propose a fixed label form (for example `domain:example.com`) and get a yes before building.
- Domains and addresses are personal data: stored in settings (`config-persistence` skill), removed by a factory reset (`AfterFactoryReset`), never in a model prompt.
- An outside service that is down must read as "unknown", never "no problems" (carry the last findings, as `Exposure.Carry`).
- Rate limits and politeness: one request per name per interval; no retry storm; a cap on names (suggest 10 domains, 10 addresses).

**Done when:** the owner adds a real domain and address in the UI, presses Check now, sees a status for each, sees the scheduled frequency honoured, and a deliberately broken input (a name that does not resolve, a service that refuses) shows "could not look" with a reason.

## 6. Task D: a posture view and a trend (last)

**Why last.** It reads A to C: finding age and resolution (B), footprint findings (C), what the assistant can already explain (A).
**Shape.** Not one magic number. On the Overview (`NetworkOverviewPanel.vue` is where the Network page's overview lives) three plain facts with a week-on-week arrow: *what is exposed* (internet-reachable ports, high and medium findings that need you), *how long things stay broken* (age of open findings, from B), and *what is not being checked* (modules off, ports not checked on this machine, a program missing: the `PortsUnchecked` and readiness notices already say these). If a single score is wanted, its formula is shown on screen and a check that was not run lowers confidence, not the score. Trend from the history the Timeline already reads (ADR-0025: read existing records, no new history).
**Done when:** the owner can answer "are we getting better?" from one screen and every number has a sentence saying where it comes from.

## 7. Order, and what each needs from the owner

| Step | Needs from the owner |
|---|---|
| Gate (section 0) | merge and deploy of #221 |
| A, assistant tools | nothing (decide the two small questions yourself and say so) |
| B, recheck and closure | nothing; a real fix on a real device to prove it |
| C, footprint | the **Finding.Device label** yes; the list of domains and addresses to try (the owner has them); a decision on the breach key once the terms are read; step-up approvals |
| D, posture | a look at the first version |

## 8. Where to start

First command: `gh pr view 221 --json state,mergedAt`. First files to open after the gate: `docs/architecture/network-capabilities.md`, then `backend/internal/core/netaudit/scantool/scantool.go` (task A), `backend/internal/core/netaudit/followup.go` and `ack.go` (task B), `stacks/network/dnscheck` and ADR-0020 (task C).
