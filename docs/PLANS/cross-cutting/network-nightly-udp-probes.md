---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.2, I.7, I.9, I.11, II.3]
evidence: CONSTITUTION.md I.2, I.7, I.9, I.11 and II.3 item 3 (read 2026-11-08); backend/internal/core/netaudit/names/prober.go (unicast mDNS through the guarded dial); backend/internal/core/netaudit/servicechecks.go (the TCP service-check table, "send no credential", Carry); backend/internal/core/netaudit/probefindings.go (rules for the helper's kept UDP results); ADR-0023 (dns-recursion rejected); docs/PLANS/cross-cutting/network-stacks-s2-handoff-2026-11-08.md item 3. Nothing built or run.
---

# Nightly UDP probes without the helper

**Approved by the owner 2026-11-08 (probe list and Constitution item 12); built in v0.59.0 (backend, module, docs); not yet run on a real network.** This is a new kind of outbound traffic, so it gets its own Constitution item. It does **not** touch I.11: nothing here calls the helper, no capability is added, the unit's socket families do not change.

## 1. What it is

The nightly audit finds devices and scans their TCP ports. It never looks at UDP, because a UDP scan needs raw packets (the helper, by step-up only). This adds two ordinary unicast datagrams per device, sent through the guarded dial (`NetworkTools.DialContext`, as `names.Prober` already does for mDNS), so the audit can report the two UDP services whose presence is itself a finding, with no raw socket and no privilege.

## 2. The probes, and what was left out

| Probe | Datagram (built in code, fixed) | Reply that makes a finding | Finding |
|---|---|---|---|
| **SNMP v3 engine discovery** (161) | The standard empty `msgFlags=reportable`, empty engine-id discovery message. **No community string, no credential.** | Any well-formed SNMPv3 `report` PDU (typed fields only: the agent answered) | `udp_service`, SNMP, medium (the existing `udpRules` wording) |
| **SSDP unicast `M-SEARCH`** (1900) | The same fixed `upnp:rootdevice` datagram as I.9, sent to the device's own address, not to the group | An `HTTP/1.1 200` first line | `udp_service`, UPnP, low |

Left out on purpose, each with its reason, so a later reader does not re-add them by accident:

- **SNMP v1/v2c with the community `public`.** That is guessing a default password. `servicechecks.go` says its checks "send no credential" and the nuclei list excluded `default-login`. A v2c-only device will not answer v3 discovery, so it is **missed**: that is the cost of sending no credential, and the Help text says so. If the owner wants the default-community check, it is a separate decision with its own wording; I do not recommend it.
- **DNS (53).** `dns-recursion` was rejected in ADR-0023 (a router answering the LAN is normal; the script asks the device to resolve a name).
- **TFTP (69).** Any probe is a file request. The helper's kept-result rule for 69 stays; the nightly audit does not ask.
- **NTP (123).** An answer is not a finding, so a datagram with no use is not sent.

## 3. Behaviour and bounds

- **Scope.** Only IPv4 addresses inside the declared scan scope (II.3 item 3), checked by the guarded dial; none declared means none sent. Public, CGNAT, IPv6, loopback, link-local and multicast addresses are refused as for every other probe.
- **Off until the operator turns it on**, a switch in Network settings next to the other audit switches. An ordinary signed-in request, as I.7 and I.9 (fixed packets, no listening socket).
- **Bounds** (as `names`): at most 4 in flight, at most 64 devices per look, 2 datagrams per device, a 2 s wait, a 1,500-byte read cap, the socket **connected** to the device so an answer from elsewhere is never read, closed after the answer.
- **Silence is unknown, never closed.** No reply carries the earlier finding (as `ServiceResult.Carry`), so a device that did not answer tonight is not read as fixed.
- **Replies are untrusted.** Only a typed yes/no per probe is kept; no text from a device reaches a finding, an alert or a prompt.
- **Duplicates.** A nightly finding and a helper-kept `udp_service` finding for the same device and port are merged on `Finding.Key()` at build time (confirm the key includes the protocol), with the nightly one winning because it is newer. A cleared helper result does not clear a nightly finding.

## 4. Draft Constitution text (to be item 12; in force only when the owner merges the pull request that carries it)

> 12. **Unicast UDP Service Probes** (proposed 2026-11-08, plan `network-nightly-udp-probes`). I.2 sends LAN traffic through `NetworkTools`; the audit's TCP checks use it. This item allows two more fixed datagrams through the same guarded dial, sent by `core/netaudit` and by nothing else, on these terms:
>   * **Off until the operator turns it on** (an ordinary signed-in request).
>   * **Two fixed datagrams, built in code**, never shaped by an operator, a model or a device: an SNMPv3 engine-discovery message (no community string, no credential) to UDP 161 and an SSDP `M-SEARCH` for `upnp:rootdevice` to UDP 1900, each to one device's own unicast address inside the declared scan scope. Nothing is sent to a group, a broadcast or a routed address.
>   * **Bounded**: at most four in flight, 64 devices per look, a two-second wait, a 1,500-byte read, the socket connected to the device.
>   * **No privilege, no program, no helper.** The datagrams go through the ordinary guarded dial; no capability, no raw socket (I.7 and I.9 stand), no child process.
>   * **Replies are never trusted or shown**: only whether the device answered with the expected form is kept; silence is "unknown" and never resolves a finding.
>   * **Residual risks, accepted:** every probed device sees two datagrams and may log them; a device that only speaks SNMP v1 or v2c is missed; some cheap devices mishandle unexpected UDP (one datagram each, once per look).

## 5. Build slices (after approval)

1. `core/netaudit/udpchecks` (typed rules, the two builders, parsers, tests from real captured replies when the owner supplies them; fixtures built from the RFC form until then, said so).
2. Wire into `RunReporter` beside `WithServiceChecks`, with `Carry` and the merge.
3. Setting, Network card, Help text, SPEC-003 section, the Constitution item, learnings.

## Open questions for the owner

1. Approve the two probes and the Constitution text (and keep the v2c `public` check out)?
2. Is "one switch, default off" right, or should it follow the existing audit's setting?
3. A real SNMPv3 reply and a real SSDP unicast reply from one device each would make the fixtures real; the owner can run a one-line `nc`/`snmpget` capture, or I build from the RFC shape and mark the tests as such.

## Remaining Work

Built 2026-11-08 (v0.59.0): `core/netaudit/udpchecks` (slice 1), the reporter wiring with `Carry` and the merge (slice 2), the `udp_probes` module and its switch, SPEC-003 2.46, Constitution I.12, learnings. Nothing was run on a real network.
Left: (1) run the module on vertex once and keep one real SNMPv3 report and one real SSDP unicast reply as fixtures (the tests use RFC-shaped bytes); (2) Help text beyond the module's About lines if the owner wants more; (3) open question 2 was answered by building one switch, default off; change it only if the owner asks.
