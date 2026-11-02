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
| Router counters or traffic volume from the host's own interfaces | It would see one machine, not the house; low value. |

**Parked (a later decision, not a "no"):**
- Wi-Fi neighbourhood scan for an evil-twin access point (vertex is on Wi-Fi; unknown what it can see; macOS needs a location permission).
- Failed-login counts from `auth.log` or `last` (needs read rights and a real capture from vertex first).
- DNSBL check of the public address; extra nmap scripts (`dns-recursion`, `http-title`, `ssl-enum-ciphers`: each is a Constitution II.3 review and scan time).
- osquery, Lynis, Suricata or Zeek on the host: optional external programs, so a separate plan.
- UDP scans and OS detection (need root).

**Built instead (v0.39.0, this record's PR):** host patch hygiene (`host_reboot_pending`, `host_updates_pending`) and a watch of the computer's DNS settings (`host_changed_dns_settings`), both read from files with no program started. Added in the same PR after the owner's vertex captures: `host_auto_updates_off` and `host_firewall_off` (vertex has automatic updates on and ufw `ENABLED=no`, so the second shows there until accepted).

## Consequences
- A future plan or session that proposes one of the rejected rows must first say what changed (for example, the owner turning UPnP on).
- The patch checks read Ubuntu's own login-message files; a computer without them reports nothing about updates, never "up to date" (`Patches.UpdatesRead`).
- The standard-security-updates line of `updates-available` is written from the package's documented wording, not from a capture: the captured file had none because none was waiting. Check it the first time a security update is pending.
- `/run/systemd/resolve/resolv.conf` was captured on vertex 2026-11-02: three `nameserver` lines (94.140.14.14, 94.140.15.15 and the router 192.168.50.1) under a comment header, then `search .`. A DHCP renewal that changes the list is reported once and accepted by the operator. `reboot-required.pkgs` on vertex lists `gnome-shell`, so vertex runs a desktop session.
