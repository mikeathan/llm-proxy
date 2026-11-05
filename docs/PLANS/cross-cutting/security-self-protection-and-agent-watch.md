---
status: partial
date: 2026-11-05
last_reviewed: 2026-11-05
related_specs: [SPEC-003, SPEC-006, SPEC-009]
constitution_references: [I.5, I.6, II.3, V.1]
evidence: v0.46.0 (origin/main f69f0743) read on 2026-11-05; vertex read through its admin API the same day (GET /admin/api/system, /netaudit/findings, /auth/tokens, /auth/status). None of the phases below is built.
---

# Security: protect this program itself, and watch what its agents do

**Status:** partial (2026-11-05). Every phase is built on branch `security-self-protection-agent-watch` (v0.47.0) at the owner's request
("finish all the above phases … all work goes in same PR"), with the UI made calmer on a phone at the same time; nothing new has run on
vertex yet. §2a says how each phase was built and where it differs from what is written below. The owner asked for "new plans, what's
next, still under network and security and monitoring and prevention" after v0.46.0 was deployed.
**Related:** [`network-detection-prevention-next.md`](network-detection-prevention-next.md) (v0.46.0; its verify-first facts
are Phase 0 here), [`network-security-roadmap.md`](network-security-roadmap.md) (Phases 5 and 6 are not repeated here),
[ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md) (rejected and parked ideas; checked against
every phase below), [ADR-0010](../../decisions/ADR-0010-no-level-2-assisted-fixes.md) (the app never changes another machine),
[`admin-api-authentication.md`](admin-api-authentication.md) (decision A7: TLS stays out of the process).

## 1. Why a new direction

The Network module now watches the house network and this computer's files closely. The two weakest points it does
not look at are **this program's own exposure** and **what its agents do**:

- **This program's model server is open to the whole network by default.** `models/app_config.go:58-59` sets both
  `Bind` and `ModelHost` to `AddrAllInterfaces`. On vertex `GET /admin/api/system` returns `"model_host": "0.0.0.0"`, and
  the nightly audit reports `ai_api_exposed` (high) for `192.168.50.60:8081`, which is vertex's own llama-server. The
  audit flags its own product's default and offers a generic guide.
- **The admin page is plain HTTP on the LAN.** vertex answers `/admin/api/auth/status` with `"encrypted": false`, so
  the passphrase and session cookie cross Wi-Fi in clear. A7 keeps TLS out of the process and documents a front end in
  `service_setup.md`; nothing tells the operator that they run without one.
- **Agent runs are the most realistic way in.** Automations run models with tools, terminal access and network grants
  (vertex's `network-audit` automation has `network_grant: lan`). Guardrail blocks (`EventGuardrailBlocked`,
  `core/assistant/agent_events.go:29`) and egress-proxy refusals (`core/egress/egress.go:146,259,319`, a bare 403) go to
  the run's stream and are then forgotten. No record crosses runs, and nothing reaches the operator's alerts.

## 2. Phases, highest value first

### Phase 0. Evidence first (owner and vertex, no code)
Close v0.46.0's verify-first list before building on it: the first privilege baseline and the set-id walk's time
(after the 02:00 audit of 2026-11-06), `/proc/modules` readable by `llm-proxy`, one `sign_in` alert from a new
address, and the DNS-check week (about 2026-11-08, roadmap Phase 3). Also capture: whether the egress proxy is on
(`sandboxing.egress_proxy` in `settings.yml`; `GET /system` does not show it) and a week of `guardrail_blocked` counts
from vertex's run records.
*Accept:* each fact is in the ledger with its value, or the phase that needs it says it is still unknown.

### Phase 1. This program's own posture: a "This program" group with fixes the app may make itself
**What.** Typed findings about the program's own settings, made from facts it already holds, each with a fix that changes
**only this program's settings** (allowed: ADR-0010 is about other machines), behind step-up:
- `app_model_server_open`: `ModelHost` is not loopback and `/v1` has no inference key. Fix: set `model_host` to
  `127.0.0.1` (the proxy reaches the model locally). Once this exists, the audit's `ai_api_exposed` for this computer's own
  model port points to it instead of the generic guide.
- `app_admin_plain_http`: the bind is not loopback and the last sign-in was not over HTTPS (`api.IsHTTPS`). Guide only:
  the documented front end (A7 unchanged).
- `app_token_stale`: a script token unused for 30 days, or an admin-scope token never used from a second address. Fix:
  revoke (step-up). vertex holds one admin token (`claude-test`, created 2026-10-29).
- **New installs:** change the `ModelHost` default to loopback. This is an owner decision because it changes a default
  (`config-persistence`). Existing settings keep their value, and the finding covers them.
**Needs.** Nothing new. **Verify first:** does any client on the LAN call `:8081` directly instead of through `/v1`? If
one does, the fix must say so before closing the port.
*Accept:* on a test data folder with `model_host: 0.0.0.0` and no inference key, one finding appears. The fix
writes `127.0.0.1`, the next model start binds loopback (checked with the socket table in a Linux test), and the finding
resolves.

### Phase 2. Agent security record: one bounded log across runs, and alerts that reach the operator
**What.** A small append-only store, capped like the alert log, with one row per security-relevant agent event and a
fixed schema: time, workspace, automation or chat, run id, kind, and a vetted plain token (a host name or a tool name).
It never holds arguments, URLs with paths, or model text. Kinds:
`guardrail_blocked`, `egress_denied` (the proxy records the host it refused), `terminal_refused` (allow-list),
`approval_denied`, `network_grant_used` (once per run).
- A Security section on the Timeline reads the record, and Overview shows the count.
- One alert per run when it holds two or more blocks of different kinds (one block is normal operation), worded like the
  other Network alerts.
**Not this:** whole-computer outbound destinations (rejected in ADR-0023). Only the agent proxy's own refusals are
recorded, and those are a decision the product already made.
**Needs.** The egress proxy on for the destination rows (Phase 0 says whether it is). Without it, the guardrail,
terminal and approval rows still work.
*Accept:* a mock run that hits a guardrail block and an egress refusal leaves two rows and one alert. A row never
contains a path, query or argument (fuzz the host vetting).

### Phase 3. New destinations per automation (needs Phase 2 and two weeks of its data)
**What.** An automation's set of egress hosts is small and stable. A run that reaches a host its automation never used
is one row, `agent_new_destination`, with an alert only when the same run also read untrusted content (a web fetch or
an inbound message) before it. That pairing is the prompt-injection exfiltration shape. Learn-then-alert: the first
seven days only record.
**Why this is not ADR-0023's rejected row.** That row is about a server's own connections to the whole internet. This
one is per automation, through a proxy the product owns, and alerts only on a pairing. Phase 3 is built only if two weeks
of Phase 2 data show that new hosts per automation are rare (measure first; drop the phase otherwise and record why).
*Accept:* replayed real records give fewer than one alert a week on vertex.

### Phase 4. Secrets that agents left behind
**What.** A nightly pass over agent outputs (workspace files the agent wrote, saved reports, memory entries) using the
existing secret check of memory capture (`core/memorycapture/extract.go:31`, `SecretCheck`), extended with the
program's own token prefix `lp_`. Finding: `agent_output_secret`, giving where (a workspace-relative path or memory id)
and the kind of secret, never the value. Prevention: a memory write or report save that matches is refused with a
fixed reason.
**Verify first:** what `SecretCheck` matches today, and its false-positive rate over vertex's workspaces (run read-only,
counts only).
*Accept:* a planted `lp_` token in a workspace file is found by path. The value is not in the finding, the log or
the alert.

### Phase 5. Alerts that are known to arrive
**What.** "Silence is never absence" applied to the delivery path:
- an opt-in daily "all quiet" note to the alert channel, so a dead server is noticed by its missing note;
- a `monitor` part `alerts` that stalls when the last send to every enabled channel failed. This reuses `MonitorWatch`
  (`core/netaudit/monitorwatch.go`).
**Verify first:** how connector send failures are recorded today (the alert log and the connector status).
*Accept:* a connector that fails every send raises the `alerts` stall once, and recovery clears it.

### Phase 6. Lock down (manual, reversible, this program only)
**What.** One step-up action, offered from a `device_incident`, an `agent_new_destination` alert or Settings. It pauses
every automation, removes every agent network grant, turns off inbound connectors, signs out every other session and
optionally revokes script tokens. It writes one record of what it changed, and "Undo lock down" restores exactly that.
It never starts on its own.
*Accept:* lock down then undo returns the settings byte-for-byte, and a run that starts during lock down is refused
with a fixed reason.

## 2a. How each phase was built (2026-11-05)

- **P0.** The first v0.46.0 audit was read from vertex's API (ledger 2026-11-05): it ran, but the model named problems no check found, the
  privilege look's parts could not be confirmed, and the Overview counted ports as findings; all three are fixed in this branch. The
  privilege facts (`/proc/modules`, the set-id walk) are still open: the next report now says them.
- **P1.** Built as two extensions, not a new group: the existing Sign-in → Exposure card gains `model_server_open` and `token_unused`, and
  the audit relabels the AI-server finding on this computer's own model port as `own_model_server_open` (`core/netaudit/ownmodel.go`). No
  in-app "fix" button: the setting already exists in Settings → System and the guide names it. `app_admin_plain_http` was already the
  Exposure card's `unencrypted` item, so it was not added again. The new-install default of `model_host` is now 127.0.0.1 (Constitution
  I.5 says a new install listens on this machine only); existing settings keep their value.
- **P2.** `core/agentwatch` fed by `Bus.SetTap`. Kinds kept: `tool_refused`, `approval_asked` (and P3/P4's). **Alert rule changed from the
  plan's "two kinds in one run"**: a refusal is followed at once by its approval prompt, so two kinds is every approval; and vertex's log of
  1–5 Nov held no refusal at all. So an unattended automation's first refusal of the day alerts, a chat's never does (the person is there).
  `egress_denied` and `network_grant_used` were not built: the egress proxy is off by default and has no per-run identity.
- **P3.** Built from the agent's own `fetch_url` calls, not the egress proxy, so it works with the proxy off and knows the automation. The
  plan's "measure two weeks first" became a rule in code: an automation that reaches more than 8 hosts in its learning week is marked as one
  that browses and only records; the pairing with "read untrusted content" was dropped because every fetch is untrusted content.
- **P4.** Nightly pass over the workspace folders with `tools.SecretKindIn` (the one list the guardrails already use, with names added);
  memory needed no pass, since `memory_update` already refuses a secret (`tools/memory_tools.go`).
- **P5.** `core/netaudit/checkin.go` (daily check-in, off by default) and the monitor part `alerts` (stalled while sends kept failing for an
  hour; "no connector" stays the existing finding).
- **P6.** Built as one flag enforced at three places (guardrail hard gate, automation pre-flight, webhook handler) instead of changing
  settings and restoring them, so "lift" is exact by construction; sessions and tokens it ended stay ended (the dialog says so).

## 3. Checked against what was rejected or parked

- ADR-0023: whole-computer destinations, threat lists, failed-login counts and Wi-Fi scans are not proposed again.
  Phase 3 says what differs.
- Parked (osquery or Lynis, UDP and OS scans, DNSBL) stays parked. Roadmap Phases 5 and 6 are not repeated. IPv6 scanning
  stays with ADR-0017. Global-IPv6 counting stays in `network-detection-prevention-next.md`.
- A7 (no TLS in the process) is unchanged. Phase 1 only reports and guides.

## 4. Strain and safety

No phase adds a probe, a socket or a privilege. Phases 1 and 6 change only this program's settings, behind step-up.
Phases 2 to 4 store vetted tokens and counts, never content. Every message uses fixed wording. Linux and macOS behave the
same, because every source is this program's own state.

## Remaining Work

- **Next (owner):** deploy v0.47.0 on vertex; then (1) read the next audit's notes for the kernel modules and the set-id walk, (2) set
  Settings → System → model server address to 127.0.0.1 (vertex keeps 0.0.0.0 from its old settings) and see `own_model_server_open`
  resolve, (3) open Settings → Safety → Agent activity, try Lock down and lift it, (4) switch on the daily check-in if wanted.
- **Unverified:** the secrets pass on vertex's real workspaces (false positives: run it, then read the record); the agent alert's noise on
  a real week; that no LAN client called llama-server's port directly before the default changed.
- **Not built (decided above):** `egress_denied`, `network_grant_used`, an in-app fix button for `model_host`.
