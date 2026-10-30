---
status: partial
date: 2026-10-30
last_reviewed: 2026-10-30
related_specs: [SPEC-003, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.1, I.2, I.3, I.4, II.3, II.13, III.6, V.1, V.3]
evidence: Phase 0 partly collected (§2a); the real `/proc/net/arp` capture from vertex is still missing, so the parser was tested on the kernel's documented layout, not on a capture.
---

# Network detection: new devices, changed identities, phoning home, intruders

**Status:** partial (2026-10-30). Built: Phase 1 (device inventory), Phase 2 (identity drift), the frequent presence check, the extra port checks (v0.25.0, #166), the UPnP check of the router, passive IPv6 and the decoy services (v0.26.0, #167), the router's port-forward list and addresses in the new-device message (v0.27.0, #168), and on branch `netaudit-decoy-address-ipv6-discovery` (v0.28.0) the decoy alert with the source address (Constitution I.6 amended), real-capture checks of the port checks' service names, and active IPv6 discovery (Constitution I.7 and ADR-0016, proposed: in force when the PR merges). Not built: DNS-based phoning home. Owner approval of the phases and the decisions in §6 was given in part (§6).
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

**Real capture from vertex (owner, 2026-10-30)** — `platform/neighbors/testdata/arp_vertex.txt` (hardware addresses replaced by RFC 7042 documentation addresses, locally-administered bit kept). What it showed:
- The host is a laptop on **Wi-Fi** (`wlp2s0`, `192.168.50.60`, default via `192.168.50.1`), with Docker bridges (`br-…`, `docker0`) on the side; the `/proc/net/arp` layout matches the parser. 14 of the ~96 rows are complete (flags `0x2`); the rest are the dead addresses the sweep made the kernel ARP for (flags `0x0`, zero MAC). The scope filter, not the interface name, is what keeps the bridges out.
- **One row has flags `0x0` and a non-zero MAC** (`ip neigh` calls it FAILED): the kernel remembers the old MAC of a device that stopped answering. The parser skips it, as it must; a parser that looked only at the MAC would have counted a departed device as present.
- **Four of the 14 devices use a private (locally administered) MAC** — phones. They will show as low-severity notes when they first appear, and iPhones/Androids that rotate their private address can reappear as "new".
- `ip neigh` shows `STALE` entries for devices that may have left. The kernel's `gc_thresh1` (default 128, from the kernel documentation, not measured here) means a table smaller than that is never purged, so a departed device keeps its `0x2` entry until a sweep makes the kernel re-check it (`DELAY`, then `FAILED`). The sweep runs before every read, so the effect is that a device is counted as seen for **one more cycle** after it leaves (one night, if only the nightly audit runs). It does not stop `device_missing`, which waits three days.
- Still unchecked: `/proc/net/route` itself (the default route above came from `ip route`, and agrees with the parser's layout), the router's answer to UPnP (Phase 3), whether AiProtection is reachable (§6a).

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
6. **Alerts are counts, fixed words and addresses.** The connector summary says how many changes and where the new devices are (their IPv4 or IPv6 address from the host's own neighbour table, with a note for a private hardware address), never what devices called themselves or what the operator named them. (Changed 2026-10-30: the first version was counts only, and the owner could not tell which device it meant.) The decoy alert lists the source addresses too (amended 2026-10-30, Constitution I.6): the address is the connection's remote address as the kernel reports it, never bytes the peer sent.

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

**IPv6 was looked at and left out of this PR (2026-10-29).** It is not the small step it looked like. The host's neighbour table lists only devices it has already talked to, so finding IPv6 devices needs active discovery: a multicast echo to `ff02::1`, which needs an ICMPv6 socket (Constitution I.1 forbids code opening its own sockets for LAN traffic) or a `ping`/`ndisc6` program run through `progexec` (which would talk to every host on the link, not one scanned address, outside II.3's guarded-program rules). The scan scope (`scanscope`) is also IPv4-only by design, so every IPv6 neighbour would be filtered out as out of scope. It needs an owner decision and a Constitution amendment first, as the decoys do. **Built later (v0.28.0, see below).**

**IPv6 (passive only, built 2026-10-30).** `platform/neighbors` reads the IPv6 neighbour table over netlink (`RTM_GETNEIGH`: a local request to the kernel, Linux only, no network traffic, no program), tested against a real Linux kernel in a container (`NEIGH_EXPECT_ADDR`, `neighbors6_linux_test.go`) as well as the parser against the documented layout. Only neighbours on a network interface that also has an in-scope IPv4 device count (a container bridge's do not). The addresses are stored beside the device by hardware address (`NetworkDevice.IPv6`, newest first, six at most), so a dual-stack device is not a `duplicate_mac` and its address never flips between families. A device seen only over IPv6 is listed and reported to the operator (card and alert) but has **no finding**: every finding is keyed by the IPv4 address, and privacy addresses change daily. The table lists only hosts this computer has already talked to, so this finds few IPv6-only devices; active discovery is below.

**IPv6 (active discovery, built 2026-10-30, v0.28.0; Constitution I.7, ADR-0016, proposed until the PR merges).** Owner decisions: one echo to `ff02::1` per interface, no IPv6 scanning (the scan scope stays IPv4 and II.3(3) is untouched), `x/sys` only, and the operator sets `net.ipv4.ping_group_range` once. `neighbors.Discover6` (`platform/neighbors/discover6*.go`) opens the kernel's unprivileged ICMPv6 datagram socket (never a raw socket, never a program), sends the fixed echo, waits 1.5 s, closes; the replies are never read, they only fill the kernel's table, which `read6` lists. `Inventory.discover` calls it inside `Refresh` (so both the presence check and the audit) on the interfaces of in-scope IPv4 devices (at most eight), then reads the table again. A host that refuses gives `ErrNotPermitted`, shown as the problem code `not_permitted` with the exact command (`GET/PUT /admin/api/netaudit/ipv6-discovery`, switch on the Devices card; off by default; an ordinary signed-in request, not step-up, because it opens no socket that listens). **The first look that ran discovery is a baseline for IPv6-only devices** (`DeviceInventory.Baselined6`): the owner's server showed 7 hardware addresses the passive table did not list, and without a baseline switching it on would have alerted for each of them. **Evidence:** the owner's output (§ learnings) and a real-kernel test in containers (`discover6_linux_test.go`). **Not verified on vertex yet:** the end-to-end run after the sysctl is set.

**Frequent presence check (built 2026-10-29; the owner's gap 1, so a stranger is noticed in minutes and not at the next nightly audit).** `core/netaudit/presence.go` sweeps the declared scan ranges through `Service.Scan` in `ModeDiscover` (quick connection attempts that make the host ARP every address; a running scan skips the cycle), reads the neighbour table into the same inventory, and, when something is new since the operator was last told, sends one short message to every connector: counts and fixed words, no names. A message is marked as told only after it was sent. It never starts a model. It is off until switched on (`netaudit.presence_check`, every 5, 10, 15 or 30 minutes; default 10) and is started once, in `app.New`, tethered to the app context (`StartNetauditPresence`). Settings and status: `GET/PUT /admin/api/netaudit/presence`; the devices: `GET /admin/api/netaudit/devices`, `PUT`/`DELETE …/devices/{mac}`, `POST …/devices/gateway/confirm`. UI: Settings → Network scan → "Devices on your network" (`DevicesCard.vue`). See ADR-0014.

**Port checks added alongside (built 2026-10-29).** Seven checks of the Telnet/Docker kind, where listening is the problem, judged from the port list alone and ranked higher when the scan confirmed the service: `adb_exposed` (5555), `open_proxy` (1080, 3128; "an open proxy"), `elasticsearch_exposed` (9200), `x11_exposed` (6000), `rsync_exposed` (873), `kubelet_exposed` (10250), `etcd_exposed` (2379), and Telnet's alternate port 2323 under `telnet_open`. They are one table (`portchecks.go` `exposures`) instead of one function each. The ports were added to `DefaultPorts` and `ThoroughPorts`, or the rules could never fire; `DefaultPorts` feeds the thorough sweep's liveness ports, so the sweep now tries nine more ports on silent addresses (not measured). Mining-pool and IRC ports were left out: a compromised device connects out to them, so a scan of LAN devices cannot see them. The wording was checked against nmap 7.99's port table by reading, not against a real device's output; `-sV` names such as `adb` and the `Elasticsearch`/`etcd`/`Kubelet` product strings are unverified, and the port-only rule is the fallback.

**Phase 2: identity drift (built 2026-10-29).** Store the SSH host key and TLS certificate fingerprints the checks already read, and diff them between
audits: `hostkey_changed`, `cert_changed` (an expiry renewal on the same key is a lower-severity note). *Acceptance:* a
fixture pair per case; a renewed certificate is not reported as an attack.

*As built:* `core/netaudit/identity.go` reads the fingerprints from the output of the `ssh-hostkey` and `ssl-cert` scripts the checks already run (nothing new is sent), keeps them per device, port and kind in `identities.json` in the data folder (`models.DeviceIdentity`), and reports `hostkey_changed` (medium) and `cert_changed` (low when the subject, issuer and key type and size are unchanged, called a renewal; medium otherwise). An SSH server that keeps at least one old key and adds or drops others is not a change; one whose keys are all new is. A first sighting is a baseline. A port the scan did not read is left alone ("unknown" is never "changed"), a change stays in the report for 30 days or until accepted (derived from stored state, like the inventory), and an identity unseen for 90 days is forgotten. Identity is keyed by IP address, so a device that gets a new address by DHCP starts afresh without a finding. Only the fingerprint and a short hash of the certificate's names are stored, never the text a device sent. The wording was tested on `ssl-cert` output captured from nmap 7.99 (`nmap/testdata/scripts_ssl_cert.xml`); the `ssh-hostkey` line shape is the one the existing weak-key check already parses, not a fresh capture.

**Phase 3: router exposure (partly built 2026-10-30).** Add `upnp-info` to the reviewed list (nmap `safe`; checked against nmap 7.99 first) and report each
mapping that forwards a port to a device: `upnp_forward`. Compare the router's reported DNS servers with the expected ones when
the operator sets them. *Acceptance:* a real capture from the owner's router, or the phase is dropped if it does not answer.

*As built (v0.26.0):* nmap's `upnp-info` was not used (UDP-only, never runs under `-sT --unprivileged`). `core/netaudit/upnp` sends one unicast discovery datagram to the router's own address (the default route the inventory found, which must be inside the declared scan networks) through the guarded dialer (`NetworkTools.DialContext`, so the LAN guardrails apply and no raw socket is opened) and reads the router's description page through the guarded HTTP client. An answer whose `LOCATION` is not an `http` IP-literal URL on the router itself, with a path and no query, is refused; redirects are not followed; the page is capped at 256 KiB; a control URL that is not a plain path is ignored; nothing the router wrote is copied into a finding. `upnp_enabled` (low) is reported in the nightly audit only, never by the presence check. A router that does not answer gives no finding and no note (UPnP may be off, or a datagram may have been lost); a refused dial or an unreadable page is a stated "could not be checked". The fixtures are the owner's real RT-AX86U answers (2026-10-30: `AsusWRT/4.1.52 UPnP/1.1 MiniUPnPd/2.3.11`, a `WANIPConnection:1` service at `/ctl/IPConn`) with everything derived from its hardware address replaced. **Port-mapping list (built 2026-10-30, v0.27.0).** With the owner's approval for one more read-only request, the real router answered entry 0 with the SOAP fault 713 `SpecifiedArrayIndexInvalid` (HTTP 500): **its forwarding list was empty**. That is the real end-of-list reply (`testdata/soap_no_more_entries.xml`). `Client.Mappings` reads entries one at a time (at most 64) until a 713 and reports each enabled mapping as `upnp_forward` on the device it forwards to (`ip|proto|internal port|upnp_forward`; high for remote-login, file-sharing, desktop and database ports and for a forward to an address outside the network, otherwise medium). The router's description text for an entry is never read; the service type put in the request header must match a fixed pattern. **The reply for an entry that exists has not been captured** (there was none to capture): `testdata/soap_entry_from_spec.xml` is written from the UPnP specification and says so; a reply that does not parse is an error ("could not be checked completely"), never an empty list. Check it the first time a game or camera opens a port. Router DNS-server comparison is not built.

**Phase 4: phoning home from DNS.** The operator adds a Pi-hole or AdGuard Home URL and token. Nightly the audit pulls per-device
query summaries (domain, count, first seen, last seen) through `NetworkTools`, keeps a bounded history per device (top N
domains, 30 days), and reports: `new_domain` for a device (against its own history), `listed_domain` (against a blocklist the
operator imports as a file; no list is fetched from the internet), and `dns_bypass` (a device using another DNS server, only if
the source exposes it). *Acceptance:* a real log sample; the first night builds history and reports nothing; unreachable source
is a stated "could not check".

**Phase 5: decoys (built 2026-10-30; the amendment is proposed in the pull request and in force only if the owner merges it).** A small, separate, low-interaction listener (fake SSH banner, SMB, Telnet) on high
ports, enabled by the operator, that records source address, port and time, reads at most a few bytes, executes and replies with
nothing meaningful, and is rate limited. A connection becomes `decoy_touched`. This opens listening sockets, so it gets a
design review and a Constitution amendment before any code. *Acceptance:* the amendment is approved; a connection from the
LAN is reported once per source per night; the listener cannot be made to write, execute or exhaust memory.

*As built (v0.26.0):* `core/netaudit/decoy`: three listeners (2222 with a fixed SSH banner, 4445, 8023), unprivileged ports that are in neither scan port list (a test pins it, so the audit of the host that runs them cannot trip them or put the banner into the identity tracking). Off by default; `PUT /admin/api/netaudit/decoys` is a step-up route. Each connection is read for at most 64 bytes under a deadline, which are discarded; only a fixed string is sent; only source, decoy port, count and times are kept (`decoys.json`, 200 entries). A source counts only if inside the declared scan networks and not one of this computer's own addresses. At most 16 connections at once, one per source per second per decoy. The alert is counts and fixed words to every connector, at most every five minutes, marked as told only after it was sent. `decoy_touched` (high) joins the nightly report. They listen on all IPv4 interfaces, because the audit host is a laptop on DHCP whose address changes; sources are filtered to the scan networks instead. A host firewall (ufw) can hide them; the card says so. Constitution I.6 is the amendment (separate commit). A scanner app on the owner's own phone will touch a decoy: accept that finding.

**Phase 6: report, alerts, UI.** One section, "Changes on your network", in the nightly report and the Advanced view; counts in the
connector summary; a Help part; each finding a fix and a guide. Delivered with each phase's findings, not at the end.

## 6. Decisions the owner must make before Phase 0 closes

1. Router make and model, and whether it exports flows or answers UPnP. **ASUS RT-AX86U** (2026-10-29); flow export and UPnP still unchecked.
2. Is there a Pi-hole or AdGuard Home, or is the router the DNS server? Would the owner run one? **None exists; the router is the DNS server** (2026-10-29). Whether to run one is open: see §6a.
3. Should a new device at night alert the Telegram connector at once, or wait for the report? **At once** (2026-10-29), by the presence check, to every connector, counts only.
4. How many nights before a missing device is reported (a laptop closed for a weekend is not a finding)? **3 nights**, for named devices only (2026-10-29).
5. Is a decoy listener acceptable on vertex, or only on a separate small machine?
6. How long may DNS history be kept (proposed 30 days, domain names only)?
7. IPv6: a neighbour-discovery sweep is proposed as a later addition, since IPv6 devices are invisible to the IPv4 scan. **Built 2026-10-30 as one echo per link (v0.28.0).**

## 6a. The DNS question: why a DNS server, and what else could do it (owner asked 2026-10-29)

Only a DNS server sees which device asked for which domain, and the RT-AX86U does not hand that log to anything. Pi-hole and AdGuard Home are DNS servers that keep a log and can be queried. This service cannot read the router's log. Three ways to get the same signal, none built here:
1. **AdGuard Home (or Pi-hole) on vertex**, then set the router's DHCP "DNS server" to vertex (LAN → DHCP Server on the router). This is Phase 4 as designed; the service only reads the log through `NetworkTools`. Lowest risk; the owner runs one more program.
2. **A DNS forwarder built into this service.** It would answer devices' queries, log domain and client, and forward to an upstream. It removes the extra program, but it opens a listening socket on port 53 that every device on the network can reach and puts this service in the path of all name lookups, so it needs a Constitution amendment and a security review (V.1) like the decoys of Phase 5, and an outage of this service would break name lookups for the whole network.
3. **The router's own AiProtection / Network Analysis.** The owner should check whether it is on and what it logs (Adaptive QoS "Web History" lists visited sites per device on some firmware). Whether any of it can be read by this service is unknown; stock firmware has no documented API for it. Worth one look before building anything.
**Decision (owner, 2026-10-30):** run AdGuard Home on an always-on machine and use its own screen (it already shows which device asked for which domain and blocks known-bad domains from its filter lists); **no client is built in this service for now**. Revisit after some weeks of using it: the only thing a client would add is a "new domain for this device" finding and an alert, at the cost of a token in the secrets store, a readiness entry, settings, a UI card and docs, and it cannot be tested before AdGuard Home runs (fixtures must be real captures, ADR-0007). Do not make the laptop (vertex, on Wi-Fi) the only DNS server: every device would lose name lookups whenever it sleeps, and a second DNS server handed out by the router lets devices bypass the log.

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

- **Done:** Phase 1, Phase 2, the presence check, the seven port checks (v0.25.0, #166); the UPnP check of the router, passive IPv6 and the decoy services (v0.26.0, #167); the router's port-forward list and addresses in the alert (v0.27.0, #168); the decoy alert with the source address, the port checks' names checked against real nmap captures (X11 fixed; adb, etcd and the Kubelet reduced to the port rule), and active IPv6 discovery (v0.28.0).
- **Next:** after deploying v0.28.0 on vertex, set `net.ipv4.ping_group_range` (the card shows the command), switch IPv6 discovery on and check the card lists the dual-stack addresses and the IPv6-only devices (the first look is the baseline, so no alert burst is expected); confirm the next decoy touch (from a phone) names the address; when something opens a port on the router, capture one real mapping entry (`soap_entry_from_spec.xml` is from the specification).
- **Open decisions:** DNS-based phoning home stays with AdGuard Home's own screen (§6a). Scanning IPv6 addresses would need an IPv6 scan scope and a change to II.3 item (3); nothing needs it today.
- **Unverified:** adb and the Kubelet service names (nmap 7.99 has no match, so only the port rule exists); the cost of the nine extra sweep ports is bounded by arithmetic (about 8 s per /24 worst case), not measured; noise tuning of `mac_changed`, `hidden_device` and rotating private MACs waits for the owner's evidence.
- **Idea, not designed:** a bundled free malware/botnet-C2 list imported like the vulnerability data (abuse.ch feeds are candidates; licence unchecked), once any source of destinations exists.
