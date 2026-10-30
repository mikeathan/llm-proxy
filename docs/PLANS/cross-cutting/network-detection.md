---
status: partial
date: 2026-10-30
last_reviewed: 2026-10-30
related_specs: [SPEC-003, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.1, I.2, I.3, I.4, II.3, II.13, III.6, V.1, V.3]
evidence: Phase 0 partly collected (§2a); the real `/proc/net/arp` capture from vertex is still missing, so the parser was tested on the kernel's documented layout, not on a capture.
---

# Network detection: new devices, changed identities, phoning home, intruders

**Status:** partial (2026-10-29). Phase 1 (device inventory), Phase 2 (identity drift), the frequent presence check and the extra port checks are built on branch `netaudit-device-inventory` (§2a, §5); phases 3 to 6 are not. Owner approval of the phases and the decisions in §6 was given in part (§6).
**Related plans:** [`network-audit.md`](network-audit.md) (the nightly scan, checks, expectations, vulnerability matching and
the deep check this builds on; its Appendix A says packet capture is a separate category and is **not** part of this plan).
**Origin:** the owner's question (2026-10-30): can the audit also tell when a device appears that should not be there, when
one changes identity, when a device is phoning home, and when someone is poking around inside the network?

## 1. Goal, and what this is not

**Goal.** Make the nightly audit answer four questions it cannot answer today, each from the cheapest source that is
honest about what it can see:

1. *Who is on the network, and did that change?* New device, device gone, a MAC address that changed behind an IP, two IPs
   sharing one MAC.
2. *Is a device what it was yesterday?* An SSH host key or TLS certificate that changed without a reason.
3. *Is a device talking to places it should not?* New or listed-bad domains and destinations, per device.
4. *Is something moving around inside?* Any connection to a decoy that nothing legitimate touches.

**Not in this plan:** packet capture or a mirror port (a separate plan, `network-audit.md` Appendix A); Wi-Fi monitoring
(needs a monitor-mode adapter, a separate hardware project); any tool that sniffs by deceiving the network (ARP spoofing);
guessing credentials or default passwords (Constitution II.3 forbids it); storing packet payloads; agent-applied fixes.

## 2. What exists today (and what is unverified)

Verified from the code and the live runs of 2026-10-28 to 30:
- A scan is unprivileged (`nmap --unprivileged`, connect scans), so it finds addresses and ports, not link-layer
  addresses. *To verify in Phase 0:* that no MAC reaches a `Host` today.
- Every audit keeps a baseline of findings (`ScanBaseline`), diffs the next scan against it (new, changed, resolved), lets the
  operator accept or mark a finding fixed, and carries stored results forward (expectations, deep checks).
- Findings are typed, with fixed wording; nothing a device prints reaches a report or a model (`checks.go`, `deep.go`).
- The service runs on a Linux host (vertex), where `/proc/net/arp` is readable without starting a program. *To verify:* that
  it lists the neighbours after the sweep and which interface and VLAN it sees.

## 2a. Phase 0 so far (2026-10-29)

Collected:
- **Router:** ASUS RT-AX86U (owner). **No Pi-hole or AdGuard Home** (owner). So Phase 4 has no query log to read today (§6a).
- **No MAC reaches a `Host`** (verified by search of `backend/internal`: no non-test code reads one). The nightly scan stays unprivileged; the inventory reads the host's neighbour table instead.
- **Owner decisions (2026-10-29):** a new device alerts at once; a named device is missing after 3 nights; the presence check runs every 10 minutes and is off until switched on; work goes on branch `netaudit-device-inventory` from `origin/main`.

Still to collect (the owner runs it on vertex, right after an audit or a rescan): `cat /proc/net/arp; ip -4 neigh; ip route show default; ip link`. Until then `neighbors.ParseARP` and `ParseDefaultGateway` are tested on text in the kernel's documented layout (`platform/neighbors/neighbors_test.go`), which ADR-0007 says is not evidence. The captured output belongs in the test fixtures and in this section. Also open: what the RT-AX86U answers to UPnP (Phase 3), and whether its AiProtection log is reachable (§6a).

## 3. Signals, sources and what each needs

| Signal | Source | Needs | Sees content? | Phase |
|---|---|---|---|---|
| New or missing device | the sweep plus the host's neighbour table | nothing new | no | 1 |
| MAC changed, or one MAC on two IPs | neighbour table, compared night to night | nothing new | no | 1 |
| SSH host key or TLS certificate changed | fingerprints from the scripts already in the checks list | nothing new | no | 2 |
| Router port mappings (what the internet can reach) | UPnP query of the router (`upnp-info`, nmap `safe`) | router answers UPnP | no | 3 |
| New or listed-bad domain per device | the network's DNS server's query log (Pi-hole or AdGuard Home) | the owner runs one; an API URL and token | domain names only | 4 |
| A connection to a decoy | low-interaction listeners | new listening sockets, own amendment | first bytes only, bounded | 5 |
| Destinations, volumes, beaconing | router flow export | a router that exports flows; a receiver | no | later plan |
| TLS server names, intrusion rules | Zeek or Suricata on a mirror port | hardware position | metadata | later plan |

The host sees only its own traffic on a switched network. Anything about other devices' traffic must come from the router,
the DNS server, or a mirror port. That is why phases 4 and later depend on decisions about the owner's equipment (§6).

## 4. Rules this design follows

1. **Metadata, never payloads.** Domains, counts, first-seen times, addresses. No packet content is stored, and none is shown
   to a model. Text from a device or a log (a domain name, a hostname) is untrusted: it never enters a finding, a prompt or
   a connector message verbatim. Findings use fixed wording, as today.
2. **Typed findings, keyed like every other.** Each new check is one finding type with a title, a fixed one-line fix, a guide and a
   test (`remediation.go`, `fixguide.go`), so acceptance, fix marks, carry-over and the report work unchanged.
3. **Read-only, and through the existing doors.** Anything fetched from a LAN service (a DNS server's API) goes through
   `NetworkTools` (I.1 to I.3); anything that starts a program uses `progexec` (II.3). A new listening socket (phase 5) is a
   new surface and needs its own reviewed amendment (V.1), not a reading of I.4.
4. **Operator-enabled, bounded, and stated.** Each source is off until the operator adds it. Secrets (an API token) live in
   the secrets store (III.6). Stored history is capped by count and by age.
5. **Unknown is not fine.** A source that was unreachable, a device the scan did not finish, or a log with a gap produces a
   plain "could not check" line, never an empty list that reads as clean (the lesson of the deep check).
6. **Alerts are counts and fixed words.** The connector summary says how many changes, not what devices called themselves.

## 5. Phases

Each phase is one PR with tests first and real captured data in the fixtures (ADR-0007).

**Phase 0: facts and decisions (no code).** Verify §2's two unverified facts on vertex; capture a real `/proc/net/arp`; learn
the router make and whether UPnP answers, and whether a Pi-hole or AdGuard Home exists; collect the owner's answers to §6.
*Acceptance:* a short evidence section here with outputs pasted, and every §6 decision answered.

**Phase 1: device inventory and identity (built 2026-10-29, pending the vertex capture).** Read the neighbour table after the sweep; keep each device's MAC beside its
address; findings `new_device`, `device_missing` (after N nights, not one), `mac_changed`, `duplicate_mac`. The operator can name a
device, which also makes "unknown device" mean something; this reuses the shape of expectations. An optional vendor name from a
shipped OUI table is shown beside a MAC, never trusted. *Acceptance:* tests from the real capture; a device that joined, one that
changed MAC, and two IPs on one MAC each produce exactly one finding; a first audit reports a baseline, not forty "new" devices.

*As built:* `platform/neighbors` reads `/proc/net/arp` and `/proc/net/route` (Linux only; elsewhere `ErrUnavailable`, which the report states as "could not be checked", never as an empty network). `core/netaudit/inventory.go`: the pure `Observe` folds a read into one app-wide inventory (`devices.json` in the data folder, `models.DeviceInventory`), keeps only addresses inside the declared scan scope, and takes the first look as the baseline. Findings are derived from that state, keyed by IP like every other, in fixed words that never contain a name or a MAC: `new_device` (medium; low for a locally administered MAC, which is how a phone hides its address), `mac_changed` (an address a device you approved had is now held by one you did not; two approved devices swapping a lease is not reported), `gateway_mac_changed` (critical; the approved gateway MAC never follows the network, only the operator's confirmation, a step-up route, moves it), `duplicate_mac` (one MAC on several addresses, the gateway excepted), `device_missing` (low; only for a device the operator named, not a private-address one, after `MissingAfter` = 3 days), and `hidden_device` (low; answers the neighbour table but no port answered or refused). The nightly `RunReporter` refreshes the inventory after the scan and appends the findings of the devices that scan looked at. Naming a device approves it. The cap is 1024 devices (the longest-unseen unnamed ones go first).

**IPv6 was looked at and left out of this PR (2026-10-29).** It is not the small step it looked like. The host's neighbour table lists only devices it has already talked to, so finding IPv6 devices needs active discovery: a multicast echo to `ff02::1`, which needs an ICMPv6 socket (Constitution I.1 forbids code opening its own sockets for LAN traffic) or a `ping`/`ndisc6` program run through `progexec` (which would talk to every host on the link, not one scanned address, outside II.3's guarded-program rules). The scan scope (`scanscope`) is also IPv4-only by design, so every IPv6 neighbour would be filtered out as out of scope. It needs an owner decision and a Constitution amendment first, as the decoys do.

**Frequent presence check (built 2026-10-29; the owner's gap 1, so a stranger is noticed in minutes and not at the next nightly audit).** `core/netaudit/presence.go` sweeps the declared scan ranges through `Service.Scan` in `ModeDiscover` (quick connection attempts that make the host ARP every address; a running scan skips the cycle), reads the neighbour table into the same inventory, and, when something is new since the operator was last told, sends one short message to every connector: counts and fixed words, no names. A message is marked as told only after it was sent. It never starts a model. It is off until switched on (`netaudit.presence_check`, every 5, 10, 15 or 30 minutes; default 10) and is started once, in `app.New`, tethered to the app context (`StartNetauditPresence`). Settings and status: `GET/PUT /admin/api/netaudit/presence`; the devices: `GET /admin/api/netaudit/devices`, `PUT`/`DELETE …/devices/{mac}`, `POST …/devices/gateway/confirm`. UI: Settings → Network scan → "Devices on your network" (`DevicesCard.vue`). See ADR-0014.

**Port checks added alongside (built 2026-10-29).** Seven checks of the Telnet/Docker kind, where listening is the problem, judged from the port list alone and ranked higher when the scan confirmed the service: `adb_exposed` (5555), `open_proxy` (1080, 3128; "an open proxy"), `elasticsearch_exposed` (9200), `x11_exposed` (6000), `rsync_exposed` (873), `kubelet_exposed` (10250), `etcd_exposed` (2379), and Telnet's alternate port 2323 under `telnet_open`. They are one table (`portchecks.go` `exposures`) instead of one function each. The ports were added to `DefaultPorts` and `ThoroughPorts`, or the rules could never fire; `DefaultPorts` feeds the thorough sweep's liveness ports, so the sweep now tries nine more ports on silent addresses (not measured). Mining-pool and IRC ports were left out: a compromised device connects out to them, so a scan of LAN devices cannot see them. The wording was checked against nmap 7.99's port table by reading, not against a real device's output; `-sV` names such as `adb` and the `Elasticsearch`/`etcd`/`Kubelet` product strings are unverified, and the port-only rule is the fallback.

**Phase 2: identity drift (built 2026-10-29).** Store the SSH host key and TLS certificate fingerprints the checks already read, and diff them between
audits: `hostkey_changed`, `cert_changed` (an expiry renewal on the same key is a lower-severity note). *Acceptance:* a
fixture pair per case; a renewed certificate is not reported as an attack.

*As built:* `core/netaudit/identity.go` reads the fingerprints from the output of the `ssh-hostkey` and `ssl-cert` scripts the checks already run (nothing new is sent), keeps them per device, port and kind in `identities.json` in the data folder (`models.DeviceIdentity`), and reports `hostkey_changed` (medium) and `cert_changed` (low when the subject, issuer and key type and size are unchanged, called a renewal; medium otherwise). An SSH server that keeps at least one old key and adds or drops others is not a change; one whose keys are all new is. A first sighting is a baseline. A port the scan did not read is left alone ("unknown" is never "changed"), a change stays in the report for 30 days or until accepted (derived from stored state, like the inventory), and an identity unseen for 90 days is forgotten. Identity is keyed by IP address, so a device that gets a new address by DHCP starts afresh without a finding. Only the fingerprint and a short hash of the certificate's names are stored, never the text a device sent. The wording was tested on `ssl-cert` output captured from nmap 7.99 (`nmap/testdata/scripts_ssl_cert.xml`); the `ssh-hostkey` line shape is the one the existing weak-key check already parses, not a fresh capture.

**Phase 3: router exposure.** Add `upnp-info` to the reviewed list (nmap `safe`; checked against nmap 7.99 first) and report each
mapping that forwards a port to a device: `upnp_forward`. Compare the router's reported DNS servers with the expected ones when
the operator sets them. *Acceptance:* a real capture from the owner's router, or the phase is dropped if it does not answer.

**Phase 4: phoning home from DNS.** The operator adds a Pi-hole or AdGuard Home URL and token. Nightly the audit pulls per-device
query summaries (domain, count, first seen, last seen) through `NetworkTools`, keeps a bounded history per device (top N
domains, 30 days), and reports: `new_domain` for a device (against its own history), `listed_domain` (against a blocklist the
operator imports as a file; no list is fetched from the internet), and `dns_bypass` (a device using another DNS server, only if
the source exposes it). *Acceptance:* a real log sample; the first night builds history and reports nothing; unreachable source
is a stated "could not check".

**Phase 5: decoys (needs its own review).** A small, separate, low-interaction listener (fake SSH banner, SMB, Telnet) on high
ports, enabled by the operator, that records source address, port and time, reads at most a few bytes, executes and replies with
nothing meaningful, and is rate limited. A connection becomes `decoy_touched`. This opens listening sockets, so it gets a
design review and a Constitution amendment before any code. *Acceptance:* the amendment is approved; a connection from the
LAN is reported once per source per night; the listener cannot be made to write, execute or exhaust memory.

**Phase 6: report, alerts, UI.** One section, "Changes on your network", in the nightly report and the Advanced view; counts in the
connector summary; a Help part; each finding a fix and a guide. Delivered with each phase's findings, not at the end.

## 6. Decisions the owner must make before Phase 0 closes

1. Router make and model, and whether it exports flows or answers UPnP. **ASUS RT-AX86U** (2026-10-29); flow export and UPnP still unchecked.
2. Is there a Pi-hole or AdGuard Home, or is the router the DNS server? Would the owner run one? **None exists; the router is the DNS server** (2026-10-29). Whether to run one is open: see §6a.
3. Should a new device at night alert the Telegram connector at once, or wait for the report? **At once** (2026-10-29), by the presence check, to every connector, counts only.
4. How many nights before a missing device is reported (a laptop closed for a weekend is not a finding)? **3 nights**, for named devices only (2026-10-29).
5. Is a decoy listener acceptable on vertex, or only on a separate small machine?
6. How long may DNS history be kept (proposed 30 days, domain names only)?
7. IPv6: a neighbour-discovery sweep is proposed as a later addition, since IPv6 devices are invisible to the IPv4 scan.

## 6a. The DNS question: why a DNS server, and what else could do it (owner asked 2026-10-29)

Only a DNS server sees which device asked for which domain, and the RT-AX86U does not hand that log to anything. Pi-hole and AdGuard Home are DNS servers that keep a log and can be queried. This service cannot read the router's log. Three ways to get the same signal, none built here:
1. **AdGuard Home (or Pi-hole) on vertex**, then set the router's DHCP "DNS server" to vertex (LAN → DHCP Server on the router). This is Phase 4 as designed; the service only reads the log through `NetworkTools`. Lowest risk; the owner runs one more program.
2. **A DNS forwarder built into this service.** It would answer devices' queries, log domain and client, and forward to an upstream. It removes the extra program, but it opens a listening socket on port 53 that every device on the network can reach and puts this service in the path of all name lookups, so it needs a Constitution amendment and a security review (V.1) like the decoys of Phase 5, and an outage of this service would break name lookups for the whole network.
3. **The router's own AiProtection / Network Analysis.** The owner should check whether it is on and what it logs (Adaptive QoS "Web History" lists visited sites per device on some firmware). Whether any of it can be read by this service is unknown; stock firmware has no documented API for it. Worth one look before building anything.
Nothing of this is in the PR. The owner's choice decides Phase 4.

## 7. Risks

| Risk | Handling |
|---|---|
| False alarms from phones that randomise their MAC | Phase 0 measures it; a device is keyed by address plus MAC history, and a randomised-MAC pattern is a note, not an alarm |
| A hostile device name or domain reaches a model or a message | Fixed wording only; untrusted text is never copied (rule 1) |
| The log source is wrong or silent | "Could not check" is stated, never "clean" (rule 5) |
| New listening sockets widen the attack surface | Phase 5 is separate, amended, bounded and off by default |
| Stored history grows | Caps by count and age, tested |
| A source token leaks | Secrets store only; never in a finding, a log or a prompt |

## Remaining Work

- **Done 2026-10-29 (branch `netaudit-device-inventory`, v0.25.0, uncommitted until the owner says):** Phase 1, Phase 2, the presence check (§5), the seven port checks.
- **Next:** paste the vertex capture (§2a) and add it as a fixture; deploy and watch the presence check on vertex for a day (false alarms from DHCP lease churn and phones are the main risk, `mac_changed` and `hidden_device` are the likely noisy ones); run a real scan with `-sV` on devices that expose the new ports to confirm the service/product names.
- **Then, in this order:** the decision on IPv6 discovery (see the paragraph above Phase 1's presence check: it needs an amendment and an IPv6 scope), Phase 3 (UPnP: nmap's `upnp-info` is UDP-only and never runs under `-sT --unprivileged`, so it needs another mechanism, and the RT-AX86U may not answer), the owner's choice in §6a for Phase 4, Phase 5 decoys (own amendment).
- **Idea, not designed:** a bundled free malware/botnet-C2 list imported like the vulnerability data (abuse.ch feeds are candidates; licence unchecked) to mark destinations once any source of destinations exists.
- Questions 5 to 7 of §6 are not yet answered.
