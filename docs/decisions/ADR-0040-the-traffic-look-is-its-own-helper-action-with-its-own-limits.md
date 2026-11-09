---
id: ADR-0040
title: The traffic look is its own helper action with its own limits
status: proposed
date: 2026-11-09
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0040: The traffic look is its own helper action with its own limits

## Context

The owner asked for a Wireshark-style view of the computer that runs the program, for any user, on demand, with names as well as addresses and sizes
(2026-11-09). The only packet capture in the program is `capture_window` (Constitution I.11): a 60-second, headers-only look at ARP, DHCP, mDNS and neighbour
discovery, 64 bytes a frame, in the privileged helper. Names need more than 64 bytes (a DNS answer holds them), and a host's own traffic is thousands of frames
a second, not 14. The vantage is not a problem here: a computer sees all of its own traffic whatever the Wi-Fi does with other devices' frames.

## Decision

We will add a second helper action, `capture_host` (Constitution I.13), and leave `capture_window` byte-identical (its constant, its filter, its tests).

- **Pure Go, no new package.** The same helper binary runs a second mode (`capture-host`) with the same packet socket, jail and capability. No `tshark`, `dumpcap`
  or libpcap, so nothing to install and no new privileged parser.
- **A fixed kernel filter with two lengths.** IPv4 and IPv6 TCP, UDP and ICMP only. An ordinary frame is cut exactly after the first four bytes of its L4 header, which
  are the ports (the filter computes the length from the IPv4 header length: 38 bytes without options, 58 for IPv6), so no payload byte is kept; a DNS message keeps
  up to 512 bytes. A first version kept a flat 128 bytes, which is about 70 bytes of payload on a TCP frame; the review caught it before it merged.
- **The volume is bounded by the kernel, not by reading.** Ordinary frames are sampled one in N in the filter (the kernel's random load); the capture program, which
  only counts the frames it keeps, doubles N when a second passes a budget. It never reads a field. Each record carries the N in force, so the service can say the
  figures are estimates. A DNS message is never sampled away.
- **The helper parses nothing; the service aggregates.** `core/netaudit/hosttraffic` reads the headers into totals, a table of at most 512 conversations and 256
  names, and discards the bytes. Names come only from the question of a DNS answer, vetted by I.8's rule, and appear only on the traffic view.
- **The response frame grows to 16 MiB** (it was 1 MiB) because the output cap is 8 MiB of raw bytes, which is base64 in JSON. A request stays at 4 KiB, and only the
  service reads a response, from the helper it trusts.

## Consequences

- A person can see where this computer's traffic went, by name, without installing or running anything beyond the helper they already install. It is step-up, on
  demand and never scheduled.
- The figures of a busy window are estimates (sampled), and the kernel may drop frames while the filter is changed; the screen says both, with the counts.
- The service holds at most 8 MiB of raw output and a bounded table for a moment; it keeps one small summary.
- A TLS server name is not read. It would need the TCP header length in the filter (scratch memory in the BPF program and its test interpreter) and its own amendment;
  QUIC and Encrypted Client Hello hide it anyway.
- The helper's install must list the new action: re-run `sudo ./setup.sh --install --build --unattended --helper` (its action list now holds `capture_host`, and so does the
  default of `scripts/install-helper.sh`), then switch the action on in Settings.
- The route's answer never carries the raw output of a capture (`HelperDeps.OmitRaw`), so up to 8 MiB of other devices' frames is not sent on to the browser; a scan's raw
  output is still sent.

## Alternatives considered

- **`tshark` or `dumpcap` as a guarded program.** A new privileged dependency and parser to install and trust; ruled out by the owner (pure Go).
- **Aggregating flows in the helper.** That is parsing in the privileged process, which I.11 forbids.
- **Widening `capture_window`'s 64 bytes.** It is a constitutional promise with pinned tests; a second action keeps both honest.
- **Reading every frame and capping by count.** A busy host would hit a cap in a second and show only the start of the window.
- **Live capture on the whole network.** Not possible from a Wi-Fi vantage (the owner measured it on 2026-11-01); it would need a mirror port and a larger amendment.

## Links

- Plan: `docs/PLANS/cross-cutting/network-host-traffic-and-findings-ui.md`
- Constitution I.13 and I.11; SPEC-003 changelog 2.55.
