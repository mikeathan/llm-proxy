---
id: ADR-0023
title: Network detection ideas that were weighed and rejected, parked, or found already built
status: accepted
date: 2026-11-02
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0023: Network detection ideas that were weighed and rejected, parked, or found already built

## Context
The owner asked (2026-11-02) what else the service could monitor or detect without an external machine (no AdGuard box, no mirror port), on macOS and Linux. A list of about twenty ideas came out of the plans and a review. Several were then dropped after looking at the code or at the owner's setup. This record keeps the reasons, so the same list is not rebuilt in a later session. Facts are from the repo at v0.38.0 and the owner's captures of 2026-11-02.

## Decision
**Already built; do not propose again** (found by reading the code, 2026-11-02):
- SSH host-key and TLS certificate drift: `core/netaudit/identity.go` (`hostkey_changed`, `cert_changed`).
- Gateway and ARP anomalies: `inventory.go` (`gateway_mac_changed`, `duplicate_mac`, `mac_changed`).

**Rejected, with the reason:**

| Idea | Why it is rejected |
|---|---|
| Router traffic counters (UPnP `WANCommonInterfaceConfig`) | They exist only while the router's UPnP service runs. The owner keeps UPnP off on purpose, and the audit itself reports `upnp_enabled` as a finding, so a feature that needs UPnP on would push the owner to weaken the network. The app has no router login. |
| Public IP change finding | With UPnP off the address is the one the owner typed (ADR-0021), so nothing can tell it changed. Learning it from an outside "what is my address" service would be a new third party for a low-value signal. Revisit only if the owner asks for an opt-in echo service. |
| Service and version drift per device | Every normal update changes a version, so it would fire constantly; `known_vulns` and `end_of_life` already read versions and say what matters. |
| New-destination alerts for this computer's outbound connections | A server talks to new addresses all day (mirrors, cloud providers, CDNs). Without a threat list it would be noise, and the list was measured and rejected (next row). |
| **Threat-list import (Feodo Tracker) and reading this computer's outbound connections against it** | **Measured 2026-11-02 by downloading the feed read-only:** the default list `ipblocklist.txt` holds **5 addresses** and says "Last updated: 2026-03-04"; `ipblocklist_recommended.txt` holds **0**; `ipblocklist_aggressive.txt` holds 7,606 but includes every address ever listed (old residential ISP ranges among them), so a server's connection to a reused address would be a false alarm. The plan had assumed "a few hundred" current botnet addresses; that was wrong. A download pipeline (step-up, store, card, route) for a near-empty list is not worth its surface, and connections without a list are noise (previous row). Revisit only if a maintained list with thousands of *current* addresses and a clear licence is found, or the owner has a source of destinations (AdGuard, a mirror port). |
| Failed-login counts from `auth.log`, `last` or `lastb` (decided 2026-11-02 from vertex captures) | `/var/log/auth.log` is `syslog:adm 0640` and `/var/log/btmp` was unreadable (`lastb: Permission denied`); the service user `llm-proxy` would need the `adm` group, a new privilege. The captured tail held no failed password, only the audit's own scans from the host's address (`Unable to negotiate ... no matching host key type`, `Connection closed ... [preauth]`), which a counter would score as attacks. `last` lists successful logins only. vertex runs fail2ban. Value is low and the privilege and false-count risks are real. **Owner decision 2026-11-02: leave it; do not add `llm-proxy` to `adm`.** `adm` reads all of `/var/log` (usernames, source addresses, every `sudo` command, apt history) and this service runs agents with tools, so a compromised or prompt-injected run would get a map of the machine. The narrower route, `setfacl -m u:llm-proxy:r /var/log/auth.log`, is lost when logrotate recreates the file unless a default ACL or hook is added. Revisit only if fail2ban is turned off and a real brute-force problem shows. |
| Wi-Fi evil-twin scan (decided 2026-11-02 from a vertex `nmcli dev wifi list`) | Linux needs `nmcli` run as a program (Constitution II.3 review; `iw` is not installed on vertex). The capture shows the owner's own SSIDs on several access points (a normal mesh, plus a guest network on a locally-administered address), so "same name, different address" is not a signal without a trusted-address list the owner must maintain. A WPA2 twin cannot take over a client that does not know the key, and a server on Wi-Fi does not roam. A scan result is third parties' network names and addresses. `iw` was installed on vertex afterwards (`sudo apt install iw`); that does not change the verdict, because `nmcli` already gave the list. |
| nmap `dns-recursion` (decided 2026-11-02 by reading the script on nmap 7.99) | Its port rule is **UDP 53 only**, but the product scans TCP, unprivileged; UDP needs root (parked, above). It asks the device to resolve `www.wikipedia.org`. A router answering recursive queries from the LAN is normal ("expected on your own internal nameservers", the script says), so a LAN scan would flag every router; whether it is open to the Internet is not learnable from inside. A disabled server prints nothing, so "off" cannot be told from "no answer". |
| nmap `http-title` | A page title is text the device wrote, and findings may carry only the code's own wording; the `http-*` scripts follow redirects. No finding follows from it. |
| Router counters or traffic volume from the host's own interfaces | It would see one machine, not the house; low value. |

**Parked (a later decision, not a "no"):**
- DNSBL check of the public address.
- osquery, Lynis, Suricata or Zeek on the host: optional external programs, so a separate plan.
- UDP scans and OS detection (need root).

**Built instead (v0.39.0, this record's PR):** host patch hygiene (`host_reboot_pending`, `host_updates_pending`) and a watch of the computer's DNS settings (`host_changed_dns_settings`), both read from files with no program started. Added in the same PR after the owner's vertex captures: `host_auto_updates_off` and `host_firewall_off` (vertex has automatic updates on and ufw `ENABLED=no`, so the second shows there until accepted).

**Correction (2026-11-02, v0.40.0 session):** `ssl-enum-ciphers` is not parked: the opt-in deep check already runs it (`core/netaudit/deep.go`, `evalTLSCiphers`; 58 connections in 28 s measured). `dns-recursion` and `http-title` are rejected (table).

**Built later (v0.40.0):** snap mount units (`snap-*.mount`) are skipped by the `systemd_units` watch, with the owner's approval; the parked items above are otherwise unchanged.

## Consequences
- A future plan or session that proposes one of the rejected rows must first say what changed (for example, the owner turning UPnP on).
- The patch checks read Ubuntu's own login-message files; a computer without them reports nothing about updates, never "up to date" (`Patches.UpdatesRead`).
- The standard-security-updates line of `updates-available` is written from the package's documented wording, not from a capture: the captured file had none because none was waiting. Check it the first time a security update is pending.
- `/run/systemd/resolve/resolv.conf` was captured on vertex 2026-11-02: three `nameserver` lines (94.140.14.14, 94.140.15.15 and the router 192.168.50.1) under a comment header, then `search .`. A DHCP renewal that changes the list is reported once and accepted by the operator. `reboot-required.pkgs` on vertex lists `gnome-shell`, so vertex runs a desktop session.
