---
status: proposed
date: 2026-10-30
last_reviewed: 2026-10-30
related_specs: [SPEC-003, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.1, I.2, I.3, I.4, II.3, II.13, III.6, V.1, V.3]
evidence: none yet. Phase 0 exists to collect it; every fact marked "to verify" below is unchecked.
---

# Network detection: new devices, changed identities, phoning home, intruders

**Status:** proposed (2026-10-30). Nothing is built. Owner approval of the phases and the decisions in §6 comes first.
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

**Phase 1: device inventory and identity.** Read the neighbour table after the sweep; keep each device's MAC beside its
address; findings `new_device`, `device_missing` (after N nights, not one), `mac_changed`, `duplicate_mac`. The operator can name a
device, which also makes "unknown device" mean something; this reuses the shape of expectations. An optional vendor name from a
shipped OUI table is shown beside a MAC, never trusted. *Acceptance:* tests from the real capture; a device that joined, one that
changed MAC, and two IPs on one MAC each produce exactly one finding; a first audit reports a baseline, not forty "new" devices.

**Phase 2: identity drift.** Store the SSH host key and TLS certificate fingerprints the checks already read, and diff them between
audits: `hostkey_changed`, `cert_changed` (an expiry renewal on the same key is a lower-severity note). *Acceptance:* a
fixture pair per case; a renewed certificate is not reported as an attack.

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

1. Router make and model, and whether it exports flows or answers UPnP.
2. Is there a Pi-hole or AdGuard Home, or is the router the DNS server? Would the owner run one?
3. Should a new device at night alert the Telegram connector at once, or wait for the report?
4. How many nights before a missing device is reported (a laptop closed for a weekend is not a finding)?
5. Is a decoy listener acceptable on vertex, or only on a separate small machine?
6. How long may DNS history be kept (proposed 30 days, domain names only)?
7. IPv6: a neighbour-discovery sweep is proposed as a later addition, since IPv6 devices are invisible to the IPv4 scan.

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

Everything. Next: Phase 0 (collect the evidence and the owner's answers to §6). No code until the owner approves the
phases.
