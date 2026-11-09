---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.7, I.9, I.11, II.3]
evidence: CONSTITUTION.md I.11 ("A capture is time-boxed and headers only"); docs/PLANS/cross-cutting/network-privileged-helper-design.md sections 4, 7, 11, 12 (D5); docs/audits/2026-11-07-privileged-helper-n0.md (vertex capture, section 2, read 2026-11-08: wlp2s0, 854 frames in 60 s, mean 14.2 per second, maximum 139 in one second, 10 broadcast ARP requests all gratuitous from 5 sender addresses, 0 DHCP); docs/PLANS/cross-cutting/network-stack-privileged-additions.md item A; docs/PLANS/cross-cutting/network-security-roadmap.md Remaining Work (Phase 6 dropped 2026-11-01). C1 and C2 built 2026-11-08 (v0.60.0) at the owner's instruction, without the M1 to M3 measurements; not run on a real link.
---

# `capture_window`: design and plan

Item A of the privileged additions: a short, headers-only look at the broadcast and multicast frames vertex already receives (ARP, DHCP, mDNS, neighbour discovery), through the helper. I.11 already carries the capture wording, so **no new Constitution item is needed**. This plan faces two problems the original text did not.

## 1. Two problems to decide first

**1. It cannot be real-time.** Item A's value was real-time detection: a spoofed gateway or a second DHCP server the moment it shows. The owner decided on 2026-11-08 that the helper is step-up only and a scheduled run never calls it (I.11). A person pressing "Listen for 60 seconds" gets a **snapshot**, not monitoring. That is still useful for a person who suspects something ("who is announcing on my network right now?"), but it is not detection, and the screen must not call it that. Real-time watching of the neighbour table is a different mechanism (`AF_NETLINK`, decision D4, no helper) and is its own ADR if wanted.

**2. The vantage point is narrow.** vertex is on Wi-Fi (`wlp2s0`), without promiscuous mode. The N0 capture saw broadcast and multicast frames and traffic to or from vertex, nothing else: 10 gratuitous ARP requests, no DHCP in 60 s, no ARP reply between other devices. The helper removes the privilege obstacle, not this one. What a 60-second window can show on this link: devices announcing themselves by broadcast (gratuitous ARP, mDNS), a device claiming the router's address by broadcast, a DHCP Discover or Request broadcast while a device joins (the Offer and Ack are normally unicast and not seen: roadmap Phase 6's result). It cannot see a spoofed unicast reply aimed at another device.

**Consequence:** the value is real but small. Build nothing until the measurement in section 3 shows frames worth reading on the owner's link.

## 2. Design (if the measurement says go)

| Part | Decision |
|---|---|
| Catalogue row | `capture_window`, class `net`, as in the helper design section 4. Params: `iface` (must exist and be up), `seconds` (at most 60; default 60). The helper opens `AF_PACKET`, attaches a **fixed** classic BPF filter (ARP, UDP 67/68, UDP 5353, ICMPv6 neighbour discovery), copies at most 64 bytes of each frame, no promiscuous mode, sends nothing, stops at the window, the frame cap or a disconnect. |
| Frame cap | **Pending the burst measurement.** Observed maximum 139 per second (about 8,300 over 60 s as a worst case if sustained); observed total 854. A cap near 4,096 frames (about 300 KB with a 12-byte header each) is the candidate; the number is fixed from M1 and written in the helper design's D5. A window that hits the cap says so ("stopped at the frame limit"), never silently. |
| Result | `records`: fixed-size entries (offset in ms, kept length, 64 raw bytes) and a 32-byte trailer (stop reason, frames, kernel counters). The helper parses nothing (I.11: parsing happens outside the privilege). **Built as a subcommand of the helper binary run as its program under the jail (`privhelper/capture`), layout in `privhelper/wire/capture.go`.** |
| Parser | New unprivileged package `core/netaudit/framewire`: reads the 64 bytes into typed fields only (EtherType, ARP operation, sender MAC and IPv4, DHCP source address (**not** message type or server identifier: DHCP options start at byte 282 and only 64 bytes are kept, so they are never inside the snap), mDNS question count, ND type). Anything shorter or malformed is counted as "unreadable", never guessed. Text a device sent (an mDNS name) is not kept. |
| Summary kept | One typed summary per window, in a stack-owned store (not on `NetworkDevice`, which is per device): counts per kind, the MAC-to-IP claims seen (bounded list), DHCP server identifiers seen, duration, stopped-by reason, time. Timeline text from typed fields only (ADR-0025). |
| Route | `POST /admin/api/helper/capture_window`, step-up, one route per catalogue action, added to the step-up table in `authenticate.go` (a test already holds routes to the catalogue). |
| UI | Network, Devices, Advanced: a "Listen for 60 seconds" card (it is interface-wide, not per device), passphrase through the existing step-up prompt, the last summary with its time and a plain statement of what this link can and cannot see. |
| Findings | **None in the first slice.** Candidates after data: a gateway address claimed by a hardware address other than the confirmed router; one IP claimed by two hardware addresses; a second DHCP server identifier. Each has legitimate causes (repeaters, proxy ARP, VRRP, bonded NICs), so, as with the DNS check (roadmap Phase 3), a finding is built only if real windows show no false alarms. |

## 3. Measurement before code (owner runs on vertex; no code)

Scripts exist: `docs/audits/2026-11-07-privileged-helper-n0/capture_frames.py` (needs the helper unit's capture capability; read the folder's README first).

- **M1, burst rate:** five 60-second windows at different times: idle, a phone joining Wi-Fi, a laptop waking, a smart-TV or printer discovery burst, evening use. Record frames per second maximum and total each time. Sets the frame cap.
- **M2, what reaches vertex:** during one window, from another machine on the LAN run `arping -U` (gratuitous ARP) and join a phone to the Wi-Fi. Record whether the broadcast ARP and the DHCP Discover and Request are seen, and whether any DHCP Offer or Ack is. Say which interface and whether it was Wi-Fi.
- **M3, overlap:** compare what the window shows with what the existing 10-minute neighbour-table check already reports for the same period. If the window adds nothing the neighbour table does not, park the feature.

Results go to `docs/audits/` with a link from the learnings ledger. **Go/no-go is the owner's, after M1 to M3.**

## 4. Slices (each its own change set)

| Slice | Content | Needs |
|---|---|---|
| C0 | M1 to M3 on vertex; results in `docs/audits/` | The owner |
| C1 | Catalogue row, helper code for the packet socket and the fixed filter, frame cap, the fake in `helpertest`, fitness tests, the `isolationtest` run on the helper's jail profile (helper design test 6), measured under the real units | C0 numbers; the helper unit already holds `CAP_NET_RAW` |
| C2 | `framewire` parser, route and step-up entry, summary store, the UI card, Help text, SPEC-003 section, learnings | C1 |
| C3 | Findings from windows, only if C0/C2 data shows no false alarms | Real windows over time |

Each is its own PR and its own branch; none is combined with stack slice S2 (the privileged-additions plan says so).

## 5. Decisions for the owner

1. **Accept the snapshot?** A manual 60-second window, not real-time (recommended), or leave capture out until a real-time route (D4 netlink) is decided.
2. **Run C0?** Without the measurements the frame cap and the value are guesses.
3. **No findings in the first slice**: agree (recommended)?

## Remaining Work

**Decisions made 2026-11-08 (owner):** a manual 60-second snapshot (not real-time); **no findings** in the first slice; build C1 and C2 without C0, with a frame cap of **4,096** taken from the one earlier vertex capture (854 frames in 60 s, maximum 139 in one second), and let the summary (frames, busiest second, kernel drops, stop reason) supply the M1 numbers from real presses.

**Built (v0.60.0):** C1 (catalogue row, the capture program, the fixed filter with a portable interpreter test, the wire layout, the helper binary's `capture` mode, the allowlist entry in `archtest`, the step-up route entry, `setup.sh` action list) and C2 (the `framewire` reader, `models.CaptureSummary`, the store, the default-interface hook on the helper route, `GET /admin/api/netaudit/capture-window`, the card, SPEC-003 2.48). Deviation: the summary store is in the core, not the Network stack; S6 moves it.

**First real press (2026-11-09, v0.60.1):** done on vertex. The first press failed (the jail denied the exec of the helper's own binary; fixed in #212 and moved to the shared runner on branch `network-capture-window-jail-test`); the second ran the full window: 143 frames, at most 14 in one second, 0 unreadable (numbers in `docs/learnings/network-audit.md`). The IPv6 count was 0 and is unproven (a check is in the ledger entry).

**Not done, in order:** (1) more real windows at different moments, one while a device joins, and the IPv6 check; (2) design test 6 (the `isolationtest` run on the helper's jail profile) and a measurement under the real units: the Linux packet code was only compiled for linux (amd64 and arm64), never run; (3) the owner's M3 judgement: does a window show anything the 10-minute neighbour check does not? If not, park; (4) C3 findings only after real windows show no false alarms.
