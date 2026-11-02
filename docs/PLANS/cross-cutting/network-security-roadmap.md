---
status: proposed
date: 2026-10-31
last_reviewed: 2026-10-31
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.2, I.6, I.7, II.3, V.1]
evidence: Research session of 2026-10-31 (an external analysis the owner pasted), checked against the repo at v0.31.0 on 2026-10-31 (§2). Vendor and tool facts (§7) are from that analysis and unchecked. Nothing here is built or measured.
---

# Network security roadmap: what to add next, and in what order

**Status:** proposed (2026-10-31); Phase 7 (patch hygiene, DNS settings) built 2026-11-02, Phase 8 (connections + Feodo) rejected; Phase 1 (1a, 1b, 1c) built 2026-11-01, the rest is not approved or built. Each phase needs the owner's go-ahead, and the ones marked *amendment* need a Constitution change after a security review (V.1).
**Related plans:** [`network-detection.md`](network-detection.md) (the inventory, presence check, decoys, DNS decision §6a), [`network-guard-module.md`](network-guard-module.md) (the Network page; Phase 6 rogue DHCP dropped), [`network-followups.md`](network-followups.md) (IPv6 scanning proposal, ADR-0017).
**Origin:** the owner (2026-10-31) asked for options for network security, monitoring, detection and prevention, and which could run inside this service so there are no more external dependencies. This plan extracts that analysis and orders it.

## 1. Goal and non-goals

**Goal.** Close the largest blind spots of the audit in an order that fits the owner's setup (the service on **vertex**, a Linux server; an ASUS RT-AX86U router that is also the DNS server; Macs and phones as clients; no mirror port), preferring what an unprivileged service can do with libraries it already has.

**Non-goals.** Blocking traffic or changing a firewall (ADR-0010: fixes are guided, not executed); ARP-spoofing tools; credential guessing (II.3); a DNS server inside the service (§6a of `network-detection.md`: port 53 and every name lookup in the house would depend on this process); Wi-Fi monitoring (needs a monitor-mode adapter); rebuilding Suricata's signature engine; a macOS blocking firewall (needs Apple entitlements a Go service cannot get).

## 2. What exists and what the research claimed (verified 2026-10-31)

| Claim | Check | Result |
|---|---|---|
| The neighbour reader works on Linux only | `platform/neighbors/neighbors.go`: `ErrUnavailable` "(only Linux can)" | **True.** On a Mac the inventory, presence check and IPv6 discovery report "could not check". |
| `fsnotify`, `gopsutil/v3`, `x/sys` are direct dependencies | `backend/go.mod` | **True** (fsnotify 1.9.0 used in `storage/manager.go`, `automation/scheduling.go`; gopsutil 3.24.5; x/sys 0.47.0). |
| `golang.org/x/net` is already available for `route.FetchRIB` | `go list -m all` shows v0.50.0, but `go mod why -m golang.org/x/net` says **the main module does not need it**, and `go doc golang.org/x/net/route` finds no package locally | **Partly false.** Using it is a new direct dependency (ask first, AGENTS.md). Whether x/sys alone can read the macOS routing table is unchecked. |
| The DNS-server comparison of the router (old Phase 3) is unbuilt | No "dns" in `core/netaudit/upnp` | **True.** |
| Rogue DHCP was dropped because port 68 is privileged | `network-guard-module.md` Phase 0(a): `ip_unprivileged_port_start` is 1024 on vertex | **True** (a bind was not attempted on vertex). |
| AdGuard Home is the DNS-log source, no client built | `network-detection.md` §6a, owner decision 2026-10-30; revisit after some weeks of use | **True.** |
| Packet capture is a separate, privileged category | `docs/architecture/network-audit.md` Appendix A | **True:** its own amendment, off by default, summaries only. |
| `gopsutil` shells out to `lsof` on macOS | not checked (module source not read) | **Unchecked.** |

Also true today (so not "to add"): the nightly nmap audit with about 45 typed checks, KEV/NVD matching, end-of-life software, the device inventory and 10-minute presence check, identity drift, the router's UPnP and port-forward list, passive and active IPv6 discovery, decoy services, the Network page with history, alerts on the bell, per-scan port record and name hints (v0.31.0).

## 3. The blind spots, and what each one needs

| Blind spot | What would close it | Needs |
|---|---|---|
| This host itself (listeners, persistence, config) | Read local state | Nothing new: unprivileged reads, no packet |
| What the internet can reach | One opt-in outside lookup of the public IP | An outside service; I.2 |
| What the router knows (DHCP leases, drops, IPS events) | Receive the router's syslog | A listening socket: *amendment* (like I.6) |
| Where devices connect (phoning home) | A **source of destinations**: DNS log, flow records or packets | Hardware or another tool; see Phase 5 |
| Exploits and malware on the wire | An IDS | Privilege and a vantage point: out of scope in-process |
| Clients' own processes | Host agents on each machine (LuLu, OpenSnitch, osquery...) | The owner installs them; not this service |

**The key dependency:** a threat list (abuse.ch, Spamhaus) only matches **destinations**, and today the app has none: the router is the DNS server, packet capture is out of scope, and the host's own sockets cover one machine. So a list import comes *after* a source of destinations, never before.

## 4. Phases, in order, with why

1. **Phase 0: decisions and evidence (no code).** Cheap, and it decides the shape of everything after.
   - Does the service ever run on a Mac for real use? If not, the macOS neighbour source (below) is dropped.
   - Is a ~$30 managed switch with a mirror port and a small Linux box acceptable? Decides whether Phase 6 is ever worth planning.
   - One captured sample of what the router sends as remote syslog (stock RT-AX86U; does the Remote Log Server setting exist and what does it send?). Needed before Phase 4. A live probe or a firewall change on the owner's network needs their approval each time.
   - What the Network page should use as `Finding.Device` for findings about the host itself (its own IPv4 address, or a fixed label). Every finding, acceptance and diff is keyed by it, so this is a contract question for the owner (the same kind as ADR-0017).
   - Does the router's UPnP description return an external IP (`GetExternalIPAddress`), and is it a public address (not double NAT or carrier-grade NAT)? One read-only request through the existing UPnP client, with approval.
   *Accept:* each answer written in the ledger with the pasted evidence.

2. **Phase 1: the host watches itself (unprivileged, no amendment).** Highest value for the least risk, and it fits patterns already in the code. Three findings families, built and shipped separately:
   - **1a Listeners.** Read `/proc/net/tcp{,6}` and `udp{,6}` each presence tick; a listener not in the baseline is `new_listener`, approved like a device. Process names via `/proc/<pid>` only where readable by the service user; otherwise port only. Same shape as the `/proc/net/arp` reader. Expect noise from Docker and dev tools: baseline-and-approve, not alarm.
   - **1b Changes to what survives a reboot.** A nightly hash of a short reviewed list: systemd units (`/etc/systemd/system`), cron, `authorized_keys` files the service user can read, `sshd_config`, `sudoers`, the app binary. Prefer a nightly **diff** to a live `fsnotify` watcher (no watcher lifecycle, no missed events); fixed wording, never file contents (a file's text is untrusted).
   - **1c Hardening subset.** sshd settings and `/proc/sys` values (IP forwarding, `rp_filter`) read directly; each a typed finding with a fix guide. Anything that needs a program (macOS `csrutil`, `socketfilterfw`) goes through `progexec` with fixed arguments and needs a Constitution II.3 review, so macOS checks are a second step.
   *Accept:* tests from real captures of `/proc/net/tcp` and the file list on vertex (ADR-0007); a first run builds a baseline and reports nothing; a changed unit file produces exactly one finding.

3. **Phase 2: outside exposure.** Small and a real answer to "what can the internet reach": the public IP from the router's UPnP `GetExternalIPAddress` (no third party for that step), then, **opt-in and off by default**, one HTTPS request through `NetworkTools` to an outside scan database (Shodan's keyless InternetDB is the candidate), diffed against `upnp_forward`; finding `internet_exposed_port`. The outside service learns the public IP, so the switch says so. If the router's address is not public, the check says it cannot look. *Accept:* a real response from the chosen service for the owner's address; the unreachable and CGNAT cases are stated, not guessed.

4. **Phase 3: a hijack check, only in a narrow form.** The router's DNS servers are not exposed by UPnP, and comparing answers for names through the router against a public resolver raises false alarms (CDNs answer differently by location). Do it only for names chosen to have stable answers, and only after measuring the false-alarm rate over a week. If it cannot be made quiet it is dropped, like Phase 6 of the module plan. *Accept:* a week of real comparisons with zero false findings, or the phase is dropped.

5. **Phase 4: router log intake (*amendment*).** The router is the one place that sees every device: DHCP leases with hostnames (an instant, named new-device signal), firewall drops, and AiProtection events if switched on. A UDP listener on a high port (not 514, which is privileged), enabled by the operator, treated like the decoys of I.6: bounded rate, bounded memory, bytes parsed into typed fields and never stored or shown verbatim, only fixed-wording findings. Alternative with no listener: `rsyslog` writes a file the service reads, which is exactly the extra product the owner wants to avoid. Needs Phase 0's syslog sample, an ADR and a Constitution item (I.8). *Accept:* the amendment is approved; a captured router log produces the expected findings; a flood cannot grow memory.

6. **Phase 5: a source of destinations, then threat lists.** Order matters.
   - Revisit the AdGuard Home client (`network-detection.md` Phase 4, deferred 2026-10-30) once AdGuard has run for some weeks: it supplies per-device queries, and is the cheapest honest source of destinations.
   - Only then import threat lists the way NVD and KEV are imported (ADR-0011/0013: step-up download through the guarded client, matched offline), and match them against that source and Phase 1a's outbound connections for this host. Licence per feed is checked at that point (Feodo Tracker states CC0; abuse.ch's platform terms of 2025-11-04 say access is for authenticated users; the other feeds are unchecked).
   *Accept:* a real log sample; the first night builds history and reports nothing.

7. **Phase 6: traffic visibility (only if Phase 0 says yes; *amendment*).** A separate privileged helper subcommand (`netsensor`) with `CAP_NET_RAW` (Linux) or BPF group access (macOS), no libpcap or cgo, reading packet headers into capped summaries over a Unix socket. It is the one step up in privilege and the largest risk, and needs its own ADR and amendment (Appendix A of the architecture doc). **Do not assume what it sees on a switched LAN:** broadcasts and multicast only. The research claimed it would see ARP replies and DHCP offers for instant spoofing and rogue-DHCP detection; ARP replies are normally unicast and a DHCP offer is unicast unless the client asks for a broadcast, so both are unverified and the value without a mirror port may be small. Capture on vertex first. *Accept:* a recorded capture of what actually arrives on vertex's port.

8. **Optional, low priority, by owner request only:**
   - **macOS neighbour source** (only if Phase 0 says the service runs on a Mac): read the ARP table from the kernel behind the existing `NeighborSource`; needs a new dependency or a verified x/sys route; verify on a Mac first.
   - **More decoys** (a fake HTTP login, an RDP banner): each is a change to I.6's closed table, so a security review each time. A "decoy credential" in an SSH config pointing at a decoy port is guidance in Help, not code.
   - **Flow receiver (NetFlow/IPFIX):** a listening socket needing an amendment; prefer reading from ntopng or AdGuard.

## 8. Phases added 2026-11-02, and what was rejected

**Rejected or already built: read [ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md) before proposing an idea.** In short: key and certificate drift and gateway/ARP checks already exist; router counters need UPnP on (the owner keeps it off); a public IP change cannot be seen with UPnP off; version drift and new-destination alerts are noise.

- **Phase 7: host patch hygiene and DNS settings (built 2026-11-02, v0.39.0).** `host_reboot_pending` (low, medium after 14 days) and `host_updates_pending` (low, medium when a standard security update waits) from `/var/run/reboot-required` and `/var/lib/update-notifier/updates-available`; `host_changed_dns_settings` (medium) from a hash of `/etc/resolv.conf`, `/run/systemd/resolve/resolv.conf`, `/etc/systemd/resolved.conf` and `resolved.conf.d`. Files only, no program, fixed wording with numbers from the file. A host without the updates file says nothing about updates. The findings are the existing "This computer" group, so the UI needed no new screen; the visual spec covers a phone and a desktop width. Fixture: the owner's real `updates-available` and `resolv.conf` (2026-11-02). Added the same day from the owner's `20auto-upgrades` and `ufw.conf`: `host_auto_updates_off` and `host_firewall_off` (both low; vertex shows the firewall one because ufw is `ENABLED=no`). Not yet seen on the server.
- **Phase 8: this computer's outbound connections against a threat list: REJECTED 2026-11-02 (ADR-0023), not built.** The Feodo Tracker file was downloaded read-only to size it: 5 addresses in the default list (last updated 2026-03-04), 0 in the recommended one, 7,606 in the aggressive one, which includes every address ever listed and would raise false alarms. Do not plan it again unless a maintained list of current addresses with a clear licence turns up. Phase 5's "threat lists" step is withdrawn for the same reason.

## 5. Existing tools worth using now (no code, from the research; the specifics are unchecked)

- **Router:** switch on ASUS AiProtection (two-way IPS and infected-device blocking); block outbound port 53, DoT and DoH except to the DNS server so devices cannot bypass it; Asuswrt-Merlin as a log source if the owner accepts third-party firmware.
- **Each Mac:** LuLu (outbound firewall), BlockBlock/KnockKnock (persistence), Lynis (hardening). **Linux:** OpenSnitch, Lynis, CrowdSec or fail2ban. osquery on both.
- **Outside view:** Shodan Monitor for the public IP, GRC ShieldsUP. **Docker on vertex:** Trivy for image CVEs (run by hand, not rebuilt here).
- **Traffic, if hardware is added:** a mirror switch feeding Zeek and Suricata (or Security Onion). Their alerts could later be read as files (like the router log) into the Network page.

## 6. Strain and safety

Phases 1 to 3 add no listening socket and no new probe of other devices; Phase 2 sends one request to a third party only if enabled. Phases 4 and 6 add a listener or a privileged process and are the only ones that change the threat model; each is an amendment with its own review. Findings use fixed wording; nothing a log, file or device says reaches a message or a model verbatim.

## 7. Risks and unknowns

| Item | Status |
|---|---|
| Stock RT-AX86U remote syslog (existence, content) | captured 2026-10-31 (ledger); custom port above 1024 unchecked |
| Merlin/softflowd NetFlow export | unchecked; not needed unless flow intake is chosen |
| Shodan InternetDB terms, limits, keyless access | keyless and "free for non-commercial use" (its page, 2026-11-01); weekly refresh; no limit stated; the full terms page returned 404, so **not read** |
| `gopsutil` and `lsof` on macOS | unchecked |
| Passive ARP/DHCP visibility on a switched port | doubted (§4 Phase 6); needs a capture |
| Noise of `new_listener` on a Docker host | unmeasured |

## Remaining Work

- Phase 0 (2026-10-31), recorded in the ledger: 1 Ubuntu real, macOS dev only; 2 mirror switch acceptable; 3 the owner pasted the router's System Log page (not the wire format; port 514 is the router default and privileged; no DHCP lines seen; a key value and MACs inside); 4 fixed label for host findings (label text open); 5 not obtained, but the log's `DST=192.168.0.137` suggests double NAT. 3 is now a real `tcpdump` capture (BSD `<PRI>` format, DHCPACK lines with hostnames exist; ledger). **Still open:** whether the router accepts a port other than 514, and the host label. Work continues on branch `network-security-roadmap-plan`.
- **Phase 1a and 1b built 2026-11-01 (v0.32.0), uncommitted** on this branch, at the owner's go-ahead (`/proc/net` capture pasted; host label chosen as `this-computer`). Differences from the plan are in the ledger (audit-time not presence-tick, no process names, non-loopback only, own sockets and ephemeral UDP left out). Mutation-checked; all backend and front-end tests, lint, build and a new visual spec pass. **Not yet seen on the owner's server.**
- **Phase 1c built 2026-11-01 (v0.32.0), uncommitted:** sshd password, root and empty-password login and `rp_filter` off, from the owner's real `sshd_config` and sysctl values (`ip_forward` 1, `rp_filter` 2 and 2, so neither is a finding). macOS checks (`csrutil`, `socketfilterfw`) stay a separate step needing a Constitution II.3 review; the owner runs Ubuntu.
- **Phase 4 built 2026-11-01 (v0.33.0), branch `router-log-intake`, uncommitted:** the router's log only wakes the presence check early (ledger and ADR-0018). Constitution I.8 is a proposal in force only if the owner merges it. Left out: the DHCP name, firewall drops, lease expiry.
- **Phase 2 built 2026-11-01 (v0.35.0), branch `network-outside-exposure`, uncommitted (ADR-0019):** the router's own UPnP answer gives the public address (never guessed; private, CGNAT, link-local and "UPnP off" are stated as "cannot look"); an opt-in, step-up switch sends it once per audit to a configurable https service (default Shodan InternetDB); `internet_exposed_port` is keyed on the router's LAN address; a look that could not be made carries earlier findings instead of resolving them. Real captures from the owner's address are the fixtures. **Not yet run by the program on vertex.** The service's full terms and rate limit are unread. The same branch fixes a phone layout bug in the port rows and folds the long text under each Network switch (and three others elsewhere) into a "More about this" expander; the router is asked for its address only when the lookup is on; "internet access off" has its own reason; Help says what the Internet view sends.
- **Phase 3 built 2026-11-01 (v0.36.0), same branch as Phase 2 at the owner's request (ADR-0020), as the narrow form: a collector that only records, off by default, no finding.** Names `dns.google`, `one.one.one.one`, `dns.quad9.net`; outside resolver Cloudflare's DNS-over-HTTPS JSON endpoint (both are settings); hourly by default; the router is asked at the default gateway unless a private DNS address is set; both Guardrails switches (local network and internet) must be on, and the card says which one is off. One real sample matched on all three names (fixtures from it). **Next step is the owner's:** switch it on (Network → Devices → Router DNS check), leave it a week, then bring back the "Copy the results" text (Advanced view). **A finding is built only if the week shows zero false alarms; otherwise this phase is dropped and recorded here and in the ledger.**
- Phase 6: owner-run capture done 2026-11-01 on a Mac on Wi-Fi (ledger): broadcast and multicast only, 0 other unicast, 0 ARP replies, DHCP Request seen but no Offer or Ack. **Dropped by the owner 2026-11-01** (it would need a wired box on a mirror port; vertex is on Wi-Fi). No ADR, amendment or helper written. A capture from vertex itself was not run. Phase 5: not started (needs the AdGuard client first). **More decoys parked** by the owner 2026-11-01 (low value). **Phase 2 seen working on vertex** (`internet_exposed_port` for 1194, 2026-11-01); with UPnP off an **entered public address** (v0.38.0, ADR-0021) lets it keep looking. **Phase 3** is running on vertex since 2026-11-01 ~18:19Z with an alive indicator and a differences list on its card; the owner brings back "Copy the results" about 2026-11-08.
- **2026-11-02:** Phase 7 built on branch `host-patch-hygiene-resolver-connections` (uncommitted); Phase 8 rejected after measuring the feed. Captures from vertex used: `updates-available`, `/etc/resolv.conf`, `resolvectl status` (upstreams include AdGuard DNS 94.140.14.14 and the router), `/proc/net/tcp` (listening rows only). Captured the same day: `/run/systemd/resolve/resolv.conf` (3 nameservers) and `reboot-required.pkgs` (`gnome-shell`). Still to capture: an `updates-available` that has a standard-security line, and a real `/proc/net/tcp` established row (the first paste had a typo).
