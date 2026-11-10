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
| Capture window (frames of the link, IPv6 check) | helper action `capture_window`, `core/netaudit/framewire/` | Linux | Network |

## This computer (the host watch)

`platform/hostwatch/` read by `core/netaudit/hostwatch.go` (`HostSource`). The program behind a port is **deliberately not named** (its name is text any account can set).

**Linux** (all of it): open ports and new listeners (`new_listener`) · startup and change files: systemd units, cron, SSH server config, sudoers, DNS settings, authorized_keys, this program's own file (`files.go`) · program files against dpkg's checksums (`host_file_modified`, `integrity.go`) · installed packages against Canonical's USN OVAL (`packages.go`, `flaws.go`) · patch state (`patches.go`) · hardening: firewall, auto-updates, reboot pending, disk, rp_filter, SSH root and password login (`hardening.go`) · accounts, groups, set-id files, kernel modules, preload (`privilege.go`, `hostprivilege.go`) · key-file permissions (`keyfiles.go`) · Docker bridge (`docker.go`) · failed logins from btmp (`platform/loginlog/`, `loginwatch.go`).

**macOS** (since 2026-11-09, measured on a Mac, plain file reads, no program started): startup files compared, adding **LaunchDaemons and LaunchAgents** (`launch_daemons`, `launch_agents`, `FileSpecsFor` in `files.go`), sudoers, sshd_config, DNS settings, authorized_keys including `/Users/*` · **open ports and new listeners** (`new_listener`), read in pure Go from the kernel's socket tables (`platform/hostwatch/pcblist.go`, `sockets_darwin.go`; since 2026-11-10; if the table cannot be read the report says **open ports were not checked**, `HostLook.PortsUnchecked`, and no ports baseline is kept) · accounts, packages, integrity, patches and the SSH server's settings are not judged (`hardenerFor`: macOS ships `sshd_config` whether or not Remote Login is on). **Not covered on macOS:** open ports (listing them needs a program, and a guarded program does not start on macOS unless the operator allows running unconfined; the osquery and `netstat` options are in plan `network-host-and-tls-looks.md`), packages, integrity, privilege, hardening.

## Reading and presenting

Findings tab (Needs you / Notes, By kind / By device) · Devices tab · Timeline (`activity.go`) · attack paths (`attackpaths.go`) · incident joins (`incident.go`) · fix guides (`fixguide.go`, `firewallguide.go`, `remediation.go`) · acknowledgements (`ack.go`) · alerts on the bell (`alertlog.go`) · weekly summary and daily check-in · the nightly report (`runreporter.go`) and its baseline diff.

## The model and the agent

The agent reads **findings and typed counts**, never raw text from a device or a program (Constitution I.8, ADR-0036, ADR-0025). `network_scan` is the agent's tool (`core/netaudit/scantool`); intrusive modes are refused to it and to scheduled runs (ADR-0012).

## Programs the core runs (guarded, `platform/progexec`, `platform/isolation`)

`nmap` (both) · `nmcli` (Linux) · the privileged helper's pure-Go actions (Linux). `testssl.sh` (a closer look inside the deep check, `core/netaudit/testssl/`); osquery was measured and is **not** being integrated (it cannot name another account's program on Linux without root, and the host watch already covers Linux).

## When an optional program is missing

Say so, and say what to run: `isolation.Present` checks the PATH without starting anything, `Readiness.Notices()` (`core/netaudit/readiness.go`) returns a notice with the install command as a copy fix (precedents: `nmap_missing`, `testssl_missing`), and the readiness banner shows it. A feature that needs a program must work, or say plainly that it cannot, without it. A dialog that states what will be sent must say it for the case in force.

## Before you add something: the four questions

1. Is it in the tables above, or a `Check*` constant in `core/netaudit/`? (`grep -rn 'Check[A-Z][A-Za-z]* *=' backend/internal/core/netaudit`)
2. Does `ADR-0023` already list it as built or rejected?
3. Which one home shows it, and does that screen already show the same fact? If yes, feed the existing finding (same check id, same key) instead of adding a section.
4. Is it intrusive (many handshakes or crafted packets)? Then it belongs to the deep check, not the nightly audit (ADR-0012).
