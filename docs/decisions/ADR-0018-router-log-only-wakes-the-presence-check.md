---
id: ADR-0018
title: The router's log may only wake the presence check early; nothing in it is kept, shown or acted on
status: proposed
date: 2026-11-01
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0018: The router's log may only wake the presence check early; nothing in it is kept, shown or acted on

## Context
The presence check looks at the network every 10 minutes (ADR-0014), so a new device can go unnoticed for that long. The owner's router (RT-AX86U, stock firmware) already sends its log to this computer on UDP 514 (captured with `tcpdump` on 2026-10-31). It contains the moment a device is given an address (`dnsmasq-dhcp ... DHCPACK(br0) <ip> <mac> <name>`) or joins the Wi-Fi (`hostapd ... STA <mac> IEEE 802.11: associated`), and, many times a minute, firewall drops from internet scanners. Nobody listens on 514 on this computer, and the program may not open a port below 1024. UDP syslog has no authentication, so any device on the LAN can send a line that looks like the router's.

## Decision
We will open one fixed unprivileged UDP port (5514) when the operator switches it on (a step-up action, off by default) under a new Constitution item I.8, and use what arrives for one purpose: when a datagram from the default gateway is an acknowledged lease or a Wi-Fi association for a hardware address the device list does not know, ask the presence check to look soon. The look is paced (a few seconds' pause, never closer than a minute to the previous cycle, wakes merged) and is the same look the check takes every 10 minutes: the list of devices still comes only from this computer's own neighbour table, so a forged datagram can cause an early look and nothing else. Only the two message shapes seen on the wire are parsed; the router's name and the device's name are never read, and nothing from a datagram is stored, shown, logged, put in a finding or sent to a model. The router's setting stays at 514; this computer redirects 514 to 5514 with a firewall rule the screen shows and the program never runs (ADR-0010).

## Consequences
A new device is noticed in seconds instead of minutes, when the presence check is on; when it is off the listener does nothing. The device's own name from DHCP is not used, so alerts do not name the device yet. Every datagram from the router is read and its sender checked, at most ten a second; the firewall drops (the bulk) are discarded after that check. The operator must make one change on this computer (the redirect) and one in the router, and the redirect has to be made persistent by hand. A device on the LAN can make the check look early, at most once a minute.

## Alternatives considered
- **Shorten `presence_interval_minutes`** — needs no listener or amendment, but sweeps every address that often, around the clock; the early look sweeps only when something joins.
- **Let the log create devices or findings** — it would make forged lines a way to raise false alarms or to hide a device, and would need real parsing of untrusted text.
- **Use the DHCP name** — useful in alerts, but it is text a device chooses; if wanted later it should go through the same cleaning as names the program asks devices for.
- **Listen on 514 with a capability or a lower `ip_unprivileged_port_start`** — gives the service or every program a privilege for one socket.
- **Have `rsyslog` write a file for the program to read** — another product to install and keep running.
- **Count or alert on the firewall drops** — one every few seconds from the internet; noise.

## Links
- Plan: [`network-security-roadmap.md`](../PLANS/cross-cutting/network-security-roadmap.md) Phase 4; Constitution I.8.
- Related: [ADR-0014](ADR-0014-presence-check-and-device-inventory.md), [ADR-0015](ADR-0015-decoy-listeners-low-interaction-and-step-up.md), [ADR-0010](ADR-0010-no-level-2-assisted-fixes.md).
