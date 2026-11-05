---
status: approved
date: 2026-11-05
last_reviewed: 2026-11-05
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.8, II.3, V.1]
evidence: Investigation of origin/main 4b76bacc (v0.47.1) on 2026-11-05; the owner's router (RT-AX86U on Asuswrt-Merlin) captures of dnsmasq query lines reaching vertex the same day; vertex's real findings of the 10:31 audit. Decisions D1–D3 answered by the owner on 2026-11-05.
---

# The stack: modules you switch on and off, attack paths, a phone-home watch and web checks

**Status:** approved (2026-11-05); **P1 to P4 built in v0.48.0** (branch `stack-modules-attack-paths-phone-home`). The owner approved the I.8 and II.3 texts and the 24-template list on 2026-11-05; the Constitution carries both. Open: the DNS line rate (the owner is capturing it), nuclei on vertex, and the abuse.ch list (§6). The owner asked to "add all the work we discussed", to "think of a way to stack them … togglable and
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

## P3 and P4 as built (v0.48.0)

- **P3.** `routerlog.ParseQuery` / `IsQuery` and a second intake in the listener (`Deps.Query`, its own limit of `queriesPerSecond` = 50 lines a second (the owner's 10-minute capture on 2026-11-05 held 362 lines of every kind, about 0.6 a second), apart from the 10-a-second device
  allowance, so a flood of query lines cannot starve a lease; the limit is a judgement until the owner's capture), `core/netaudit/phonehome` (names per device keyed by hardware
  address, bounded, an A and AAAA pair counted once, flushed every five minutes and at shutdown to `phone-home.json`; `Evaluate`: a new place after the learning week, beaconing for new
  names only, a device online that asks nothing for a day), `netaudit.PhoneHomeFindings` (one finding per device and kind keyed by the device's address, evidence in fixed words with
  at most five names, a Version that is a digest of the names so an acceptance holds until the set grows, a fix guide each), the app's `phoneHomeIntake` (turns the sender's address into a
  hardware address with the neighbour table, drops this computer's own lookups) and the `phone_home` module (Listen layer, needs the router log, sends nothing, off by default). The nightly
  report carries the findings. **No immediate alert is sent**: findings are the surface (the plan's "alerts in fixed words" is a rule for any alert, and none was built).
- **P4.** `core/netaudit/nuclei`: the fixed argument table (checked against `nuclei -h` of v3.11.1 and run once against a local server), `Verify` of the reviewed checksums (links refused),
  `ParseHits`, the `Reviewed` list (24 templates at tag v10.5.1), `Checker` (runs through `progexec` and the sandbox in a private folder, refuses to start if any template differs),
  `Download` (fetches only the reviewed files from that tag through the guarded client, keeps a file only if its checksum is the reviewed one), and `progexec.Locate` (readiness asks
  whether nuclei is installed without starting it: nuclei writes files into the folder it starts in). Wired as the `web_checks` module ("Web checks, nightly", Test layer, off by
  default, needs the passphrase, blocked with a plain reason until nuclei is installed and the templates are downloaded), as a step of the operator's deep check of one device
  (whenever ready) and as a section of the nightly report. `GET /admin/api/netaudit/web-templates` and `POST .../download` serve the status and the operator's download; the Network
  Settings card is "Web check templates". The findings are `web_exposure`, one per web service, in the reviewed template's title written by this repository.

## Remaining Work

- Owner: install nuclei on vertex and press Download; decide on the abuse.ch list (§6).
- Optional later: an immediate alert for a new place or a beacon.
- **To do: known-bad names (abuse.ch), approved by the owner on 2026-11-05, not started.** Design agreed in conversation:
  - **Card** Network → Settings → "Known-bad names": a plain-words description, a link to `https://urlhaus.abuse.ch/api/` (where the key is made), a short "how to" under "More about this" (sign in, create an Auth-Key in the profile, paste it here), a field for the key (stored encrypted in the secrets store, never shown again; the box says only "Key saved" or "No key yet"), a **Download list** button and a line saying how many names were loaded and when.
  - **Module** "Known-bad names" (Listen layer, off by default, needs `phone_home`): a name a device asks for that is on the list becomes a high-severity finding (`phone_home_known_bad`, Version a digest of the names) for that device, matched locally; the key only ever goes to abuse.ch.
  - **Constitution:** one sentence under I.8 for the owner's approval (the list is matched on this computer; the key is a stored secret sent only to abuse.ch, through the guarded client, only when the operator presses Download).
  - **Blocked on:** the exact file address from the "host file (domains only)" section of the logged-in URLhaus API page (the owner pastes the URL, never the key). The file is documented as host-file format, so the parser accepts `IP name` lines and bare names, vets every name like the DNS parser, and is checked against the first real download. abuse.ch says its data is free under fair use and commercial use may need a licence; this is a home network.
  - **Facts already checked (2026-11-05):** the old keyless host-file address answers 410 Gone; the documentation says a login and an Auth-Key are required; the list is limited to active hosts and ones added in the last 48 hours, with names under the Tranco top million left out.
## 6. For the owner's approval: the P3 and P4 texts and the nuclei template list (drafted 2026-11-05)

**State of the work.** Approved on 2026-11-05 and built (see "P3 and P4 as built" above).

**What I verified.** nuclei **v3.11.1** (official release, SHA-256 checked against the release's checksum file) was run with `-h` and then once, with exactly the flags in `nuclei.fixedFlags`,
against a local test server. Each flag is in its help: `-duc` (no update check), `-ni` (no interactsh), `-pt http`, `-dr` (no redirects), `-nmhe`, `-rl 5 -c 2 -bs 2 -timeout 8 -retries 0 -rsr 262144`,
`-mt <limit>`, `-j -or -ot -silent -nc`, `-u` and `-t`. The real output line (a `.DS_Store` hit) is the test fixture, and the parser keeps only the template id, host, port and severity, so a template's own text never reaches the screen.
nuclei is **not installed on vertex**; its install and its readiness line come with the wiring.

**Rules the draft list was filtered by** (templates repository v10.5.1, 14,104 files, 2,447 passed the rules, 24 hand-picked below): HTTP templates only in `http/exposures`, `http/misconfiguration`, `http/technologies`, `http/exposed-panels`;
severity info or low; GET or HEAD only; every path under `{{BaseURL}}` or `{{RootURL}}` (a request cannot name another host); no `raw`, `payloads`, `fuzzing`, `attack`, no username or password variables,
no interactsh or OAST, no redirect-following to other hosts; no tag among fuzz, dos, intrusive, bruteforce, default-login, oast, ssrf, rce, sqli, xss, lfi, cve, kev, exploit, takeover.
The program runs each as `-t <file>` of a file whose SHA-256 equals the one below, so a changed template is refused and named.

| id | file | severity | sha256 |
|---|---|---|---|
| `ds-store-file` | `http/exposures/files/ds-store-file.yaml` | low | `eeaa88a701796f263e8e4c32a5d16ae3e18b2cc97d65338632e4ac8b9e49b7a8` |
| `apache-server-status` | `http/misconfiguration/apache/apache-server-status.yaml` | low | `a0b401b09818e26203a06fdfdffdfd3a5a1af18a7b3ebd72eaaa982adb27f456` |
| `nginx-status` | `http/misconfiguration/nginx/nginx-status.yaml` | info | `931c4e8049b9d94d2a79d9a476f295db87fe5f5b65d760491a5ca2b4cb80b307` |
| `openapi` | `http/exposures/apis/openapi.yaml` | info | `1a5541ec8f60d5a5fe56cc15a20b95e7609ebec177ec0ac910b99df8d114981b` |
| `grafana-detect` | `http/exposed-panels/grafana-detect.yaml` | info | `7e9829d7f57dee46f723de23f9f4c95f97ba3855b19e4466080f6d78e606b4d0` |
| `home-assistant-panel` | `http/exposed-panels/home-assistant-panel.yaml` | info | `62dbd8aec3c995d7643dd0a463e6c060d8431fc7cc3f1ef45a3139753668b017` |
| `jellyfin-detect` | `http/technologies/jellyfin-detect.yaml` | info | `2ce8d5a45bdbd93220e4627e2c5dfd29246815b1923217ca7be1f92404ff8578` |
| `kubernetes-dashboard` | `http/exposed-panels/kubernetes-dashboard.yaml` | info | `e794c21a6b83a5273ac22645102b30d4d8049ef996fd5ef9901a8977cdf1700e` |
| `prometheus-exporter` | `http/misconfiguration/prometheus/prometheus-exporter.yaml` | info | `130836c9ff0f5cc58d92b1c41c5ef564c9a2d88c4b1fef17589750b0c4d67a6e` |
| `default-apache-test-page` | `http/technologies/apache/default-apache-test-page.yaml` | info | `f4cdf5e2f3b2b65b5683a26c5d979d5d2011854e7dbd50eab4b4c8299b76feec` |
| `default-nginx-page` | `http/technologies/nginx/default-nginx-page.yaml` | info | `35a39e681f7dd41ddfb06712ec207adb18d676e0c4d2a3d83c3fbdab103c2c2d` |
| `luci-login-detection` | `http/exposed-panels/luci-login-detection.yaml` | info | `1b4a00abedd2f48635cee42219ce4b8d0155bfaac9eb43ab9537b400e21a6290` |
| `asus-router-panel` | `http/exposed-panels/asus-router-panel.yaml` | info | `638808b4b1c68b842067a7820cb0652b4076dc1697167b393d0b5386eb6e393c` |
| `unifi-os-panel` | `http/exposed-panels/unifi-os-panel.yaml` | info | `a995965438ef4bda69162a7cddd9f57790de9fc63e71c3a8817cf300b5db3164` |
| `unifi-securitygateway-panel` | `http/exposed-panels/unifi-securitygateway-panel.yaml` | info | `41ec6b0facc21fd0500711d9a1b1403c7ecfbd4b9d4b50aa60dd6c2ff72de982` |
| `zyxel-router-panel` | `http/exposed-panels/zyxel-router-panel.yaml` | info | `29b89db73d43c363bb89b6a7cb1d237ceb556a990d83e5a57f4f4168cd4fef1e` |
| `ubiquiti-edgerouter-panel` | `http/exposed-panels/ubiquiti-edgerouter-panel.yaml` | info | `e4f15410f5616b5ab9f6ff9a4c0839eff0bfd6f39454dd661944c76aec3cc435` |
| `openvpn-router-management` | `http/exposed-panels/openvpn-router-management.yaml` | info | `0667d1db7f3db3c09a0b648181579b40d2b5d45a390f2ac947c61897e399d630` |
| `truenas-panel` | `http/exposed-panels/truenas-scale-panel.yaml` | info | `bc6a29d94beb01dbc97583f0e0aeebf3357374380fac817b74737239ccf178f1` |
| `qnap-qts-panel` | `http/exposed-panels/qnap/qnap-qts-panel.yaml` | info | `c9652f806bc4832c30c0f61abdf5be9466f92d747f8f9b9d489ccf82b10a77fe` |
| `synology-dsm-system-info` | `http/misconfiguration/synology-dsm-system-info.yaml` | info | `460c13c8f72697021ec8a3ff6b1e808f434ec91fe741d116d36c5cb72c2d1b11` |
| `brother-printer-panel` | `http/exposed-panels/brother-printer-panel.yaml` | info | `ed990f3b20dab167bec8316f2450206f12d05eb92e9e046304595706f1ece875` |
| `samsung-printer-detect` | `http/exposed-panels/samsung-printer-detect.yaml` | info | `c455324209f2ddee8674bdf29268769504eaadfc6b0e0efeb9c99bf5b0a2bc51` |
| `fuji-xerox-printer-detect` | `http/exposed-panels/fuji-xerox-printer-detect.yaml` | info | `54c3addc1d83ffdc16c51f1b163875b8000ce10cbf077c660247927dcaa69c2f` |

Two things to decide with it: whether a smaller list is wanted (the router, NAS and printer panels are the ones most likely to be on a home network), and that a template the project later changes is **not** picked up
until a person reviews it and the checksum here is replaced (the same rule as the vulnerability data: downloaded when you ask, checked, never updated by the program).

**The DNS line rate (P3 bounds).** I cannot reach vertex, so the bounds in `phonehome` (256 devices, 512 names a device, 32 times a name) are judgements, and the listener's own limit for query lines is not set. To measure,
on vertex for ten minutes: `sudo timeout 600 tcpdump -l -n -i any -A udp port 514 > /tmp/syslog.txt`, then `grep -c 'query\[' /tmp/syslog.txt` and the count of all lines. Port 514, not 5514 (ledger 2026-11-04).

**Known-bad names (abuse.ch).** Not built. The owner said yes, but abuse.ch's own page (read 2026-11-05) says its URLhaus host file now needs a login and an abuse.ch Auth-Key (a secret to keep), is free under "fair use" and says use by companies or for profit may need a licence; the old keyless address answers 410 Gone. Its size was not obtainable without a key. That is a new account, a stored secret and a licence question the owner did not have when answering, so the owner is asked again before anything is built.
