---
status: approved
date: 2026-11-05
last_reviewed: 2026-11-05
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.8, II.3, V.1]
evidence: Investigation of origin/main 4b76bacc (v0.47.1) on 2026-11-05; the owner's router (RT-AX86U on Asuswrt-Merlin) captures of dnsmasq query lines reaching vertex the same day; vertex's real findings of the 10:31 audit. Decisions D1–D3 answered by the owner on 2026-11-05.
---

# The stack: modules you switch on and off, attack paths, a phone-home watch and web checks

**Status:** approved (2026-11-05); **P1 and P2 built in v0.48.0** (branch `stack-modules-attack-paths-phone-home`), P3 and P4 not yet. The owner asked to "add all the work we discussed", to "think of a way to stack them … togglable and
modular … think of the UI as well", noting the service may be renamed to something like OneStack, "a stack platform". Owner decisions:
**D1** the stack's Modules tab owns every module's switch (Network → Settings keeps only the details); **D2** nuclei runs in the deep check, with a
nightly switch that is off by default; **D3** the rename is a later PR of its own, so new code uses neutral names (`stack`, `module`) and
`netaudit` keeps its name; **D4** three levels, **Stack → Layer → Module**: stacks are top-level areas (Network now; others later), a
stack's layers are its "sub-stacks", modules are the switches; **D5** this branch also creates a second stack, **Agents** (agent
activity, the secrets pass, with lock down as the stack's action), so the multi-stack design is real from the start; **D6** no further
mock-up: build from the published one (https://claude.ai/artifact/Aq65GPTxn2Tgct6ZZgCA3Y, desktop and phone) with D4's placement. The mobile chat stream fixes are separate work with their own PR (unrelated subsystem).
**Related:** [ADR-0028](../../decisions/ADR-0028-stack-module-registry.md) (the registry), [`security-self-protection-and-agent-watch.md`](security-self-protection-and-agent-watch.md),
[`network-security-roadmap.md`](network-security-roadmap.md) (Phase 5, a source of destinations, is what P3 below delivers), ADR-0023.

## 1. Investigation (what exists, 2026-11-05)

- **Eleven separate switches** (this plan first counted twelve; the nightly audit is an automation, not a switch; corrected 2026-11-05 when P1 was built), each with its own setting in `models.NetauditConfig`, its own handler and route, its own step-up entry in
  `api.stepUpExact`, its own card on Network → Settings, and (for five of them) a `MonitorPart`: presence check, scan on arrival, device
  names, IPv6 discovery, decoys, router log, Internet view (outside lookup), router DNS check, Nearby, weekly summary, daily check-in, plus
  the nightly audit (an automation, not a switch). Dependencies are implicit: scan on arrival and the weekly summary ride on the presence
  check, Nearby and the DNS check follow the Guardrails network switches, the decoys and the router log are inbound sockets.
- **Always on, no switch:** the host watch and privilege marks (part of the audit), the agent security record. **An action, not a module:**
  lock down.
- **The navigation already has a Modules group** holding only Network (`frontend/src/utils/navigation.ts`).
- Nothing lists the modules in one place, says what each sends, or shows what depends on what. The owner cannot see "what is running".

## 2. Architecture: stacks, layers, modules

A **stack** is a top-level area the platform runs (Network; Agents; later others). A **module** is one switchable capability of one stack.
Modules sit on the stack's **layers**; a module may depend only on modules of the same stack on its own layer or below, so a stack reads
bottom-up as "what the upper ones need". Network's layers (fixed in code):

| Layer | Purpose | Modules (v1) |
|---|---|---|
| 1. Discover | who is on the network | presence check, IPv6 discovery, device names, Nearby |
| 2. Listen | what the network says on its own | router log, phone-home watch (needs the router log), decoys |
| 3. Test | look closer, on purpose | scan on arrival (needs presence), web checks nightly (needs nuclei installed) |
| 4. Look outside | what the internet sees | Internet view, router DNS check |
| 5. Understand | turn findings into decisions | attack paths |
| 6. Tell | reach the operator | weekly summary (needs presence), daily check-in |

The **Agents** stack (D5): layer *Watch* holds **agent activity** (the record of refusals, approvals and new destinations; today always on,
becomes a module that can be switched off) and **secrets pass** (the nightly scan of workspace files; same); its **action** is **lock down**
(a step-up action shown on the stack, not a module). A stack may declare actions; a module never is one.

**Backend (`internal/core/stack`).** A descriptive registry, built once in the composition root:

```go
type Module struct {
    ID, Title, Purpose string        // fixed words
    Stack   StackID                  // typed enum: network | agents
    Layer   Layer                    // typed enum, per stack (the table above for Network)
    Sends   Traffic                  // none | lan | internet | inbound (opens a listening socket)
    StepUp  bool                     // switching it needs the passphrase
    Needs   []string                 // module ids below it
    Monitor string                   // MonitorPart id for health, "" when it has none
    Switch  Switch                   // Enabled() bool / Set(bool) error: an adapter to the existing config
    Ready   func() (bool, string)    // optional: false with a reason when it cannot run (nuclei missing, Guardrails off)
}
```

No persisted setting moves: each `Switch` is an adapter to the config that already exists. `GET /admin/api/stacks` returns each stack's summary (title, on/total,
stopped, blocked); `GET /admin/api/stacks/{stack}` its layers and modules with state, health (from `MonitorWatch`), readiness and what
blocks it ("needs the router log"); `PUT /admin/api/stacks/{stack}/modules/{id}` `{"enabled": bool}` switches one module. **Step-up is checked in the handler per module** (`api.StepUpGranted`), because a single
path cannot be listed in `stepUpExact` per module; a test walks every module marked `StepUp` and expects a refusal without the passphrase.
Switching on a module whose need is off is refused with the need named; switching off a module others need warns but is allowed (the
dependents then say they are blocked). The old per-feature `PUT` routes stay for their details (intervals, URLs, names) and keep their step-up.

**UI (D4).** The navigation group "Modules" becomes **Stacks**, holding **Network** and **Agents**. Inside each stack's page a
**Modules** tab is the switchboard (Network: beside Overview, Devices, Findings, Timeline, Settings; Agents: the Agent activity page moves
here from Settings → Safety, with lock down as the stack's action). A Stacks overview page (one card per stack) only when a third stack
arrives. The switchboard: the layers drawn as bands, top (Tell) to bottom (Discover); each module a tile with its name, one
line of purpose, a switch, a health dot only when it is on (working / stopped / blocked), and a small "sends: LAN / internet / listens"
mark. A tile whose need is off is dimmed and says "needs …". On a phone each band is a list of rows (name, purpose, switch), the marks in
one quiet line, no repeated tags. Network → Settings cards lose their switches and keep their details, with a line "Switch: on the Stack
page". The page is the one path to switch a module (owner's rule, memory `feedback-ux-no-hidden-knowledge`).

## 3. Features, as modules

### P1. Registry, the Stacks navigation group and each stack's Modules tab (existing modules)
*Accept:* `GET /stacks/network` lists the twelve existing modules (and `GET /stacks/agents` its two, with lock down as the action) with the state each config reports; a step-up module is refused without the
passphrase; a module whose need is off is refused with the reason; Network → Settings shows no second switch; phone screenshots with
vertex's real data show one row per module and no repeated tags.

### P2. Attack paths (layer Understand, sends nothing)
A pure function over what the audit already has (the latest baseline with acceptances, the inventory, the Internet view's exposed ports,
decoy touches): **ways in** (a port the internet sees, a device that touched a decoy, any device on the Wi-Fi), **steps** (a finding that
lets someone in or further: SSH with passwords, SMB with known flaws, an open model server, RDP, an unauthenticated stream or admin service),
**prizes** (this computer, a device the operator marks as important, the router). It shows at most three paths, ranked by how many paths
one fix breaks; accepted findings stay in a path, quietly marked. Fixed rules in code; the model never builds a path. One Overview card and
a short section in the nightly report.
*Accept:* vertex's real findings (scratchpad fixture, never committed) give the SSH-with-passwords and SMB paths with their one fix; no
path is drawn from an open port alone.

### P3. Phone-home watch (layer Listen, sends nothing; needs the router log)
The router (RT-AX86U, Asuswrt-Merlin, `log-queries` in `/jffs/configs/dnsmasq.conf.add`, `nvram log_level=7`) sends lines of the shape
`dnsmasq[pid]: query[TYPE] name from address` to the existing listener (captured on vertex 2026-11-05 with tcpdump on udp 514/5514;
redacted fixture lines, the shape exactly as received after the syslog header `<30>Nov  5 12:54:33 RT-AX86U-5A60-6FE90A5-C `):
`dnsmasq[31037]: query[A] ccp-lh.googleusercontent.com from 192.168.50.188`,
`dnsmasq[31037]: query[AAAA] ccp-lh.googleusercontent.com from 192.168.50.188`,
`dnsmasq[31037]: query[A] example.com from 127.0.0.1`,
`dnsmasq[31037]: query[PTR] 243.147.66.172.in-addr.arpa from 127.0.0.1`. Recognised by prefix like the drop lines,
under **their own bounded counter**, so they never use the device allowance or starve the DHCP and Wi-Fi lines. Kept per device (keyed by
hardware address through the neighbour table, not by IP): the set of names it looked up (bounded), first-seen time, counts per hour;
`127.0.0.1`, this computer's own addresses, `*.arpa` and A/AAAA duplicates are dropped. Signals:
- **new place** after a device's learning week (names new for that device);
- **beaconing**, only for names new to the device in the last week (refreshes of long-known names such as NTP and push are expected);
- **goes around your DNS**: a device online for a day with no lookup; a one-time finding the operator can accept (Chromecasts use 8.8.8.8 by design);
- **known-bad name**, when a maintained list is chosen (abuse.ch domain lists are the candidate; size and licence to check first).
Names are plain DNS names vetted to `[a-z0-9.-]`, shown as text, never sent to a model verbatim; alerts in fixed words naming the device
and the name. **Needs the I.8 amendment in §4.** Measure the real line rate on vertex before fixing the bounds.

### P4. Web checks with nuclei (layer Test; on demand in the deep check, nightly switch off by default)
nuclei as a **guarded program** (II.3): arguments from a fixed table, HTTP templates only (no code, headless, javascript, file or workflow
templates), interactsh off (no out-of-band callbacks), no update check and no template download by the program itself, a pinned list of
template ids with their checksums, templates downloaded only on the operator's request (like the vulnerability data) and checked against
the list, targets from the scan scope only, run through `progexec` and the sandbox, rate-limited. Readiness says when nuclei is missing.
The exact flags are checked against `nuclei -h` of the installed version before the list is written. **Needs the II.3 note in §4 and the
owner's approval of the template list.**

## 4. Constitution texts for the owner's approval (in force when the PR that carries them is merged)

- **I.8, new item "DNS query lines"**: one more exact shape is recognised from the default gateway, `dnsmasq[pid]: query[TYPE] name from
  address`, by one search of the line, with its own counter and limits, not the device allowance. Kept: the device's hardware address (from
  the neighbour table, never from the line), the name if it is a plain DNS name, and the hour. Dropped while parsing: every other word, the
  router's own and this computer's lookups, reverse lookups. Names are shown as text and named in fixed-word alerts; never logged, never
  sent to a model verbatim. Residual risk accepted: anything on the LAN can forge a line and so cause a false phone-home signal, which is why
  each is a finding to review, never an action.
- **II.3, guarded programs**: nuclei joins nmap under items (1)–(6); its reviewed list is the pinned template ids, and any template that
  contacts a host other than the target, guesses credentials, fuzzes or can deny service is never on it.

## 5. Order and remaining work

1. This plan and ADR-0028. 2. P1 registry, navigation and Modules tabs (Network and Agents). 3. P2 attack paths. 4. P3 phone-home watch (parser and store first, from the
captured lines; the listener reads them only once the amendment is approved). 5. P4 nuclei (flags verified, list drafted for approval,
code behind the module's readiness). Each phase its own commit; one PR. The mobile chat stream fixes: their own PR.

## P1 as built (v0.48.0)

Registry `internal/core/stack`, modules in `internal/app/stacks.go`, `GET /admin/api/stacks`, `GET /admin/api/stacks/{stack}`, `PUT /admin/api/stacks/{stack}/modules/{id}`;
the Stacks navigation group with Network and Agents; each stack's Modules tab; the switches left Network → Settings; Settings → Safety → Agent activity moved to
Agents → Activity. Differences from the text above: **eleven** Network modules (see §1); a fifth kind of traffic, `message` ("sends a message"), for the Tell modules and the two
Agents parts, which send to the operator's connectors and not to "the internet"; a module's `Monitors` is a list (the router log has two parts: its socket and its quiet log);
a module whose Guardrails are off is shown **blocked** and can still be switched (Nearby always could); the Agents modules need the passphrase to switch. The layers Understand
(P2) and the phone-home watch (P3) and web checks (P4) modules are not registered yet: a layer with no module is not drawn.

## P2 as built (v0.48.0)

`core/netaudit/attackpaths.go`, the Overview card and the report section, the `attack_paths` module (Understand layer), and a new operator mark, `NetworkDevice.Important`, because
the plan's "devices marked important" had no source (nothing in the inventory marked a device). Details: `docs/architecture/network-audit.md` → Attack paths. Checked against
vertex's real findings of 2026-11-05 (read-only, not committed): one path, SSH accepts passwords on this computer, reached from any device on the network. The plan expected an SMB
path too; vertex's SMB entries are `known_vulns` with no flaw known to be exploited, which the rule deliberately leaves out (a possible match is not a way in), and its other
devices are not marked important.

## Remaining Work

- P3 phone-home watch, P4 nuclei (see §5).
- Owner: approve the two §4 texts and, for P4, the template list.
- Measure the DNS line rate on vertex (P3 bounds); install nuclei on vertex (P4 readiness).
