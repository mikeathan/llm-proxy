---
id: ADR-0024
title: Nearby discovery sends two fixed multicast datagrams and reads a bounded set of unicast replies, off by default
status: proposed
date: 2026-11-03
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0024: Nearby discovery sends two fixed multicast datagrams and reads a bounded set of unicast replies, off by default

## Context
The device list comes from the host's neighbour table, so it knows an address and a hardware address but not what a device *is*. Devices announce that themselves over mDNS (DNS-SD service types: printers, TVs, cameras, computers) and SSDP/UPnP (device type, maker, model). Two things in the repo bear on how to listen:

- **`NetworkTools` cannot do it.** The guarded dialer refuses multicast and link-local addresses for every caller (`tools/network.go` `validateIP` → `network.CheckAlwaysBlocked`), and it returns a *connected* UDP socket, which the kernel limits to replies from that one peer. A multicast question is answered by many devices from their own addresses. So this feature needs one socket the guard cannot provide, which is a new Constitution item (I.9, below), not a use of I.1–I.3.
- **What already works stays as it is.** A unicast mDNS reverse question to one known device (`names/prober.go`) and a unicast SSDP datagram to the router (`upnp/upnp.go`) both go through the guarded dialer. They need nothing new and are not changed.

Evidence for the packets (ledger 2026-10-31, 2026-10-30): a unicast mDNS question from an unprivileged socket was answered by a Mac, a TV and a phone (transaction id 0 in one case); the router answered a unicast SSDP M-SEARCH. Whether a *multicast* question from an ephemeral source port gets unicast answers on the owner's Wi-Fi is **not yet measured** (RFC 6762 §6.7 says a responder must answer such a question by unicast; access points may still filter multicast). That is the first probe of the plan.

## Decision
We will add **Nearby discovery** under a new Constitution item **I.9** (text below; `CONSTITUTION.md` is not edited until the owner approves). It is off until the operator turns it on, and the switch states exactly what is sent.

**Design in one paragraph.** One package (`platform/mcast`) opens, per look and per eligible interface, an unconnected UDP socket on an ephemeral port bound to that interface's own IPv4 address, sets the multicast interface and a hop limit of 1 with `golang.org/x/sys` (already a dependency; no `x/net`), sends **two fixed datagrams** (an mDNS `_services._dns-sd._udp.local` PTR question to `224.0.0.251:5353`, an SSDP `M-SEARCH` for `upnp:rootdevice` to `239.255.255.250:1900`), reads replies for at most five seconds under hard counts, and closes. It never joins a multicast group, so it hears no passive multicast. Everything after the first replies (a unicast follow-up question, the UPnP description page, any probe of a camera) goes through the guarded dialer to the replier's own address, so I.1–I.3 apply unchanged. The same code builds on macOS and Linux (both are `unix`); other systems get a stub that reports "not available here".

### Proposed text of Constitution I.9
> 9. **Nearby Discovery** (proposed 2026-11-03, plan `network-nearby-discovery`, ADR-0024; in force only when the owner merges the pull request that carries this text, which is the owner's approval of the security review). I.1 to I.3 send LAN traffic through `NetworkTools`, which refuses multicast and cannot read replies from many senders; this item allows one more socket, opened by `platform/mcast` and by nothing else, on these terms:
> *   **Off until the operator turns it on**, and turning it on or off is a step-up action (`PUT /admin/api/netaudit/nearby`). While off, no socket is opened and nothing is sent. The switch names both datagrams. It follows the Guardrails local-network switch: with that switch off the look does not run and the screen says why.
> *   **Two fixed datagrams, built in code**, never shaped by an operator, a model or a device: an mDNS PTR question for `_services._dns-sd._udp.local` to `224.0.0.251:5353` and an SSDP `M-SEARCH` for `upnp:rootdevice` to `239.255.255.250:1900`. At most once per interface per look, on at most four interfaces that are up, multicast-capable, not loopback and hold an address inside the declared scan networks. Hop limit 1; multicast loopback off; **no group is joined**, so no multicast datagram is ever received.
> *   **An ephemeral port, no privilege.** The socket is bound to the interface's own IPv4 address and an operating-system-chosen port, is open for at most five seconds, and is closed. The program never asks for a capability, never opens a raw socket, never starts a program for it, and never uses port 5353 or 1900 as its own.
> *   **Bounded reads.** At most 512 datagrams per look, 32 per sender, 4,096 bytes each; a 256 KB receive queue, so memory does not depend on what is sent. A datagram from any source that is not an IPv4 address inside the declared scan networks, is this computer's own address, or is on the blocked list is dropped before it is looked at. A look runs at most once every 30 seconds and one at a time (checked on the server, not only by a disabled button).
> *   **Typed parsing; everything a device sends is untrusted.** Only these survive parsing: the sender's address; service types matching `_[a-z0-9-]{1,15}\._(tcp|udp)`, at most 16 per device; port numbers from service records; a device type matching the UPnP URN shape; and names, maker and model cleaned and cut to 40 characters (`names.Clean`). TXT values, `SERVER` text other than a parsed version token, and everything else are discarded while parsing. Kept names are shown as plain text only; they are never put in a finding, an alert, a log line or a prompt, and findings use fixed wording.
> *   **Follow-ups are ordinary guarded connections to the replier's own address.** The UPnP description page is fetched only from the `LOCATION` that is on the replier's own address (the rule `upnp.ParseSSDP` already enforces), with no redirect, at most 32 pages, 256 KB each. Any later probe (the camera check) uses a fixed port table in code, never a port a device announced.
> *   **Silence is never absence.** A device that did not answer is "not heard", never gone, and no finding resolves because a look was quiet; a finding resolves only on a later answer that contradicts it.
> *   **Residual risks, accepted:** every device on the link sees both questions, as it would see any browser's; a device may answer with false claims, which at most produces a hint row or a finding worded "this device says it is…"; Wi-Fi multicast is lossy and an access point may block it, so a quiet look proves nothing (the screen says so); an unprivileged local account on this computer could read the replies of its own socket but not ours.

**Other decisions recorded here.**
1. **Router firmware is out of reach, the UPnP software is not.** The real description page (`upnp/testdata/rootdesc_asus.xml`) holds manufacturer, `modelName`, `modelNumber` (the router's model, RT-AX86U) and `MiniUPnP daemon version 2.3.11`; the `SERVER` header adds `AsusWRT/4.1.52`. No firmware version is anywhere. The finding is therefore "known flaws in the router's UPnP daemon" matched on a versioned CPE for the daemon, and the router's model is shown with "firmware version not given by the router". A versionless value never matches (ledger 2026-10-27).
2. **A printer's IPP is read to identify it, not to raise a finding.** An IPP printer that accepts jobs without a login is the normal home setup; a finding would fire on every printer. `rtsp_no_auth` (a camera stream readable without a password) is a real privacy finding and is kept.
3. **Unauthenticated AI and admin endpoints are port checks, not discovery.** They work with Nearby off, so they are a separate phase of the plan and may ship as a separate pull request.

## Consequences
- The product gains its first socket that reads from many senders. The bounds above, the scope check on every reply and a fuzzed parser are what stand between a hostile device and the program; the I.6 and I.8 precedents (off by default, step-up, fixed table, counts) are followed.
- Devices the neighbour table has not yet seen can appear, and the device list gets a type for the first time; a device is still keyed by hardware address where the inventory knows it, and by address otherwise.
- On a network whose access points filter multicast the feature may hear nothing; the screen says so and never treats it as proof.
- DNS-SD parsing is the second place that reads DNS wire format (`names/mdns.go` is the first). The plan extracts one shared reader and fuzzes it rather than writing a second hand parser (the ledger records a hand-decoding error).
- A look costs two datagrams per interface, up to 32 small page fetches and some unicast follow-ups; it runs on the operator's click and once per nightly audit, never in a loop.

## Alternatives considered
- **Unicast only: ask each device in the inventory (guarded dialer, no amendment).** Needs no new item and reuses the proven path, but it only asks devices already in the neighbour table, which is Linux-only here, so it fails the macOS requirement and never finds a quiet device that has not been talked to. Kept as the **fallback** if I.9 is not approved: it still gives names and service types for known devices.
- **Passive multicast listening (join the groups).** Finds more, but hears every device's chatter all the time and needs an always-open socket; deferred by the owner until the active version proves useful (separate amendment).
- **`golang.org/x/net/ipv4`** for multicast options. Clean API, but a new direct dependency (AGENTS.md: ask first) for three socket options `x/sys` already sets.
- **Run `avahi-browse` or `dns-sd` as a guarded program.** Different tool per OS, text output to parse, and II.3's guarded-program carve-out for a program that talks to every host on the link.
- **Send from port 5353 with the unicast-response bit.** Needs to share the port with avahi or mDNSResponder (`SO_REUSEPORT`) and would make this program a mDNS participant; an ephemeral port makes responders answer by unicast without that.

## Links
- Plan: [`network-nearby-discovery.md`](../PLANS/cross-cutting/network-nearby-discovery.md).
- Related: [ADR-0016](ADR-0016-active-ipv6-discovery-one-echo-unprivileged.md), [ADR-0018](ADR-0018-router-log-only-wakes-the-presence-check.md), [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md), [ADR-0020](ADR-0020-dns-check-only-records-and-never-reads-silence-as-agreement.md), [ADR-0023](ADR-0023-network-ideas-rejected-or-already-built.md).
