# Network audit: what already exists (read this before proposing or building anything)

**Why this file exists.** Twice in one session (2026-11-09) a "new" network feature turned out to be built already: a nightly TLS-protocol check (the manual deep check already makes `tls_weak_protocol` and `tls_weak_cipher` with nmap's `ssl-enum-ciphers`) and an osquery host look (the host watch already reads open ports, startup files, packages, accounts and hardening on Linux). Both were found only after the plan and some code were written. **Before you propose, plan or build a network feature: search this list, then `ADR-0023` (what was weighed and rejected), then the check ids in `backend/internal/core/netaudit/`.** If it is here, extend it. If you add a capability, add a row here in the same change.

Verified against the code on 2026-11-09 (v0.68.0); the platform column comes from reading the code (`/proc`, systemd and helper dependencies, `ErrUnavailable`), not from running each item on both systems. Platform: **both** = macOS and Linux, **Linux** = needs `/proc`, systemd or the privileged helper. The screen is where a person sees it; a finding's severity lives only in the Findings tab.

## Looking at other devices

| Capability | Code | Platform | Where it shows |
|---|---|---|---|
| Device discovery, scan modes `discover` / `ports` / `services` / `checks` / `deep` | `core/netaudit/nmap/`, `scan.go`, `mode.go`; a limited Go prober when nmap is absent | both | Findings, Devices |
| **Deep check of one device** (intrusive, operator only): **TLS protocol versions and cipher grade** (`tls_weak_protocol`, `tls_weak_cipher`), SSH password login | `deep.go`, `scandeep.go` (`DeepScan`), `encryptionfindings.go` (a closer look with `testssl.sh` when installed, replacing nmap's answer for a port it judged), `DeepCheckDialog.vue`, `DeepCheckResult.vue` | both (needs nmap) | the device's deep-check dialog and Findings |
| Safe nmap script checks: SMBv1, SMB signing, anonymous FTP, open Redis/Mongo/MQTT/RTSP/VNC/RDP/X11/NFS/rsync/memcached/etcd/kubelet/Docker/ADB/Elasticsearch, Telnet, open proxy, weak SSH algorithms and host keys | `checks.go` (`CheckScriptNames`) | both | Findings |
| Web pages and **certificate** checks (expiry, changed, self-signed, weak key, weak signature), 24 fixed pages | `stacks/network/webprobe/` (`tls.go`, `checks.go`), words in `core/netaudit/webfindings.go` | both | Findings; module "Web and certificate checks, nightly" |
| UDP checks (SNMPv3, UPnP) | `stacks/network/udpchecks/`, `core/netaudit/udpfindings.go`, `probefindings.go` | both | Findings; module "UDP checks, nightly" |
| Known vulnerabilities from the local NVD + CISA KEV index; end-of-life software | `core/netaudit/knownvulns.go`, `eol.go`, `core/vulndata/` | both | Findings |
| Device class (what a device should not have open) | `deviceclass.go` | both | Findings |
| UDP scan and OS detection of one device (privileged helper) | helper actions `scan_udp`, `os_detect`; `DeviceProbe.vue` | Linux | device row, Traffic place panel |
| Device names, maker (OUI), MAC, IPv6, presence check, new-device and MAC-change alerts | `inventory.go`, `presence.go`, `discovery6.go`, `stacks/network/oui/` | both | Devices |
| Nearby (mDNS and SSDP announcements) and rogue gateway | `core/netaudit/nearby/`, `platform/mcast/` | both | Devices |
| Scan on arrival; follow-up scan of a new device | `followup.go` | both | message |
| **Footprint** (outside the LAN): new certificates (crt.sh), domain expiry (RDAP), SPF and DMARC (DoH), email breaches (k-anonymity range search, own key); each service opt-in per switch, per-watch frequency and Check now; findings labelled `domain:<name>` / `email:<id>` | `core/netaudit/footprint/`, `persistence/footprint.go`, `FootprintCard.vue`; `GET/PUT /admin/api/netaudit/footprint`; ADR-0042 | both | Network → Settings, Findings |
| **Standing** ("are we getting better?": age of open problems, came back, week against week) | `standing.go` (from the finding history only), `StandingCard.vue` | both | Overview |
| **Finding history** (first seen, resolved by audit or recheck, came back N times) and **Check again** for one finding (a scan of its own device and port: fixed / still there / could not check; never "fixed" when the check was not made) | `lifecycle.go`, `recheck.go`, `findingsview/`, `FixGuidePanel.vue`, `useRecheck.ts`; endpoint `POST /admin/api/netaudit/recheck`; Timeline rows `finding_fixed` / `finding_returned` | both (needs nmap) | Findings card, Timeline |

## Listening and watching

| Capability | Code | Platform | Where it shows |
|---|---|---|---|
| Router log: refused attempts, new devices, sign-in failures, Wi-Fi disconnect bursts | `core/netaudit/routerlog/`, `blocks.go` | both (needs a router that sends syslog) | Findings, Timeline |
| Phone-home watch: where devices call out to; new place, beaconing, encrypted DNS | `core/netaudit/phonehome/`, `phonehomefindings.go` | both | Findings, Devices |
| Known-bad names (abuse.ch) | `core/netaudit/knownbad/` | both | Findings |
| Decoy services | `core/netaudit/decoy/`, `decoyfindings.go` | both | Findings |
| Router DNS check; internet view of open ports; router UPnP and forwarded ports | `stacks/network/dnscheck/`, `upnp/`, `core/netaudit/outsidelookup/`, `exposure.go` | both | Overview, Findings |
| Wi-Fi look-alikes (evil twin) | `core/netaudit/wifilook/` (nmcli) | Linux | Findings |
| **Traffic on this computer**: a listen of up to 60 s, destinations by name, kinds, per-second chart, conversations, click-through | helper action `capture_host`, `core/netaudit/hosttraffic/`, `TrafficTab.vue`, `TrafficReport.vue`, `TrafficDetail.vue` | Linux | Traffic tab |
| Names for a destination from the **server name of a TLS ClientHello** this computer sent (`name_from: tls`), beside the DNS answer's (Constitution I.13, ADR-0043) | `capture/filter_host.go` (768 bytes of a hello only), `core/netaudit/hosttraffic/tls.go` | Linux | Traffic tab |
| **Who runs an internet destination** (network operator, AS number, country; ranked by holder and by country) from a public table downloaded on a press and looked up on this computer; no address is sent anywhere (ADR-0043) | `core/netaudit/ipowners/`, `hosttraffic/owners.go`, `handlers/netaudit_destinationdata_handlers.go`, `DestinationDataCard.vue`, `TrafficWhere.vue` | both (the traffic look itself is Linux) | Traffic tab |
| **Where an internet destination is used** (city, region, the table's country and a point) from DB-IP Lite City, an index file searched on disk (ADR-0044, index version 2) | `core/netaudit/ipplaces/`, `app/netaudit.go` (`currentOwners`) | both (the look is Linux) | Traffic tab, Settings → Downloaded data |
| **Map of the internet traffic**: a dot per city and the route of a traced place, on a built-in land outline (ADR-0046) | `TrafficMap.vue`, `constants/worldLand.ts`, `utils/netaudit/traffic.ts` (`mapDots`, `routePoints`) | both | Traffic tab |
| **Route trace** to one internet address the last look saw: each router, its operator and city from the local tables, its time; unprivileged UDP with `IP_RECVERR` on the operator's press (Constitution I.14, ADR-0045) | `platform/routeprobe/`, `core/netaudit/traceroute/`, `handlers/netaudit_routetrace_handlers.go`, `TrafficRoute.vue`, `useRouteTrace.ts` | Linux | Traffic place panel |
| **Which way traffic went and what is this program's own**: direction by this computer's addresses (scope listing) and its cards' hardware addresses (`/sys/class/net`), the service's port of a conversation, this program's own page set apart | `core/netaudit/hosttraffic/` (`Self`, `servicePort`), `app/hosttraffic.go` (`trafficSelf`) | Linux | Traffic tab |
| **Links to the problem**: a finding or a device's findings from an attack-path step, a Timeline row, the Overview, a device row and the Traffic detail | `router/routes.ts` (`toFinding`, `toDeviceFindings`), `useFindingFocus.ts`, `useDeviceProblems.ts` | both | Findings, Overview, Devices, Traffic, Timeline |
| Capture window (frames of the link, IPv6 check) | helper action `capture_window`, `core/netaudit/framewire/` | Linux | Network |

## This computer (the host watch)

`platform/hostwatch/` read by `core/netaudit/hostwatch.go` (`HostSource`). The program behind a port is **deliberately not named** (its name is text any account can set).

**Linux** (all of it): open ports and new listeners (`new_listener`) · startup and change files: systemd units, cron, SSH server config, sudoers, DNS settings, authorized_keys, this program's own file (`files.go`) · program files against dpkg's checksums (`host_file_modified`, `integrity.go`) · installed packages against Canonical's USN OVAL (`packages.go`, `flaws.go`) · patch state (`patches.go`) · hardening: firewall, auto-updates, reboot pending, disk, rp_filter, SSH root and password login (`hardening.go`) · accounts, groups, set-id files, kernel modules, preload (`privilege.go`, `hostprivilege.go`) · key-file permissions (`keyfiles.go`) · Docker bridge (`docker.go`) · failed logins from btmp (`platform/loginlog/`, `loginwatch.go`).

**macOS** (since 2026-11-09, measured on a Mac, plain file reads, no program started): startup files compared, adding **LaunchDaemons and LaunchAgents** (`launch_daemons`, `launch_agents`, `FileSpecsFor` in `files.go`), sudoers, sshd_config, DNS settings, authorized_keys including `/Users/*` · **open ports and new listeners** (`new_listener`), read in pure Go from the kernel's socket tables (`platform/hostwatch/pcblist.go`, `sockets_darwin.go`; since 2026-11-10; if the table cannot be read the report says **open ports were not checked**, `HostLook.PortsUnchecked`, and no ports baseline is kept) · accounts, packages, integrity, patches and the SSH server's settings are not judged (`hardenerFor`: macOS ships `sshd_config` whether or not Remote Login is on). **Not covered on macOS:** packages, integrity, privilege, hardening.

## Reading and presenting

Findings tab (Needs you / Notes, By kind / By device) · Devices tab · Timeline (`activity.go`) · attack paths (`attackpaths.go`) · incident joins (`incident.go`) · fix guides (`fixguide.go`, `firewallguide.go`, `remediation.go`) · acknowledgements (`ack.go`) · alerts on the bell (`alertlog.go`) · weekly summary and daily check-in · the nightly report (`runreporter.go`) and its baseline diff.

## The model and the agent

The agent reads **findings and typed counts**, never raw text from a device or a program (Constitution I.8, ADR-0036, ADR-0025). `network_scan` is the agent's tool (`core/netaudit/scantool`); intrusive modes are refused to it and to scheduled runs (ADR-0012). **`network_findings`** (since 2026-11-10) reads the saved result of the workspace's latest audit for chat: fixed-wording header, a bounded worst-first list (25, open ports only when new), accepted findings left out and counted, no guide or fix text, every device-supplied string inside the untrusted-data fence (`core/netaudit/findingstool`, category `netfindings`, offered only where an audit exists; the Findings screen and the tool share `core/netaudit/findingsview`).

## Programs the core runs (guarded, `platform/progexec`, `platform/isolation`)

`nmap` (both) · `nmcli` (Linux) · the privileged helper's pure-Go actions (Linux). `testssl.sh` (a closer look inside the deep check, `core/netaudit/testssl/`); osquery was measured and is **not** being integrated (it cannot name another account's program on Linux without root, and the host watch already covers Linux).

## When an optional program is missing

Say so, and say what to run: `isolation.Present` checks the PATH without starting anything, `Readiness.Notices()` (`core/netaudit/readiness.go`) returns a notice with the install command as a copy fix (precedents: `nmap_missing`, `testssl_missing`), and the readiness banner shows it. A feature that needs a program must work, or say plainly that it cannot, without it. A dialog that states what will be sent must say it for the case in force.

## Before you add something: the four questions

1. Is it in the tables above, or a `Check*` constant in `core/netaudit/`? (`grep -rn 'Check[A-Z][A-Za-z]* *=' backend/internal/core/netaudit`)
2. Does `ADR-0023` already list it as built or rejected?
3. Which one home shows it, and does that screen already show the same fact? If yes, feed the existing finding (same check id, same key) instead of adding a section.
4. Is it intrusive (many handshakes or crafted packets)? Then it belongs to the deep check, not the nightly audit (ADR-0012).
