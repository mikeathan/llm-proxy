---
id: ADR-0039
title: How the listening and discovering sensors reach the Network stack
status: proposed
date: 2026-11-09
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0039: How the listening and discovering sensors reach the Network stack

## Context

ADR-0038 moves the Network sensors into the stack one at a time. The send-and-look sensors are done (`udpchecks`, `webprobe`, `upnp`): each takes addresses in
and returns ids and numbers, over the scoped dial. What is left is different in kind, and reading the code showed that "the ingest port (S4)" in the S6 plan
covers three things that need three answers:

1. **Listeners that keep the sockets in the core and push data in:** `routerlog` (router syslog lines: drops, DNS queries, new devices), `phonehome` (what each
   device calls out to, from those lines), `knownbad` (a downloaded list of host names matched against what devices asked for) and `decoy` (listening ports
   that record who connected). Their input is **text from the network**: router log lines, and the domain names devices ask for.
2. **A discovery that sends and listens in a window:** `nearby` (`platform/mcast.Exchange`: multicast mDNS and SSDP questions out of each interface, replies read
   for a window). Its input is **device text**: names, maker, model, service types. It is not pure ingest: it sends.
3. **Programs:** `nmap`, `portprobe`, `scantool`, `wifilook` (S5; see "Programs" below).

ADR-0038 rule 3 says no text from the network crosses a port, ids and numbers only. That rule was written for sensors whose answers are a yes or a number.
It cannot be applied unchanged to (1) and (2), whose whole job is to carry names.

## Questions this ADR must settle (proposed answers, for the owner)

**Q1. Do validated host names count as allowed text?** `StackFinding.Names` already lets a stack hand the core host names, which the core drops unless they are
valid host names (ADR-0036). *Proposed:* yes, for **domain names and IP addresses only**, validated by the core at the port (the same host-name check, a length
bound, a count bound), and never in a finding's wording except where the core's own table puts a name in a fixed sentence (as the DNS finding does).
Device-chosen free text (a friendly name, a maker, a model) stays out of the port: the core keeps the Nearby card's text handling, and the stack hands over only
the typed facts (which service types, which ports, which addresses).

**Q2. Does Nearby's send side need its own port?** Yes: it sends multicast, which is neither the scoped dial (unicast, scoped to declared ranges) nor ingest.
*Proposed:* leave `platform/mcast.Exchange` in the core as an **exchange port** (`Exchange(ifc, questions, window) -> datagrams`), plain data in and out, and
move the parsing and the judgement of the replies into the stack. The datagrams are untrusted bytes; the stack parses them, as `webprobe` parses pages. Without
this port Nearby stays in the core.

**Q3. Which direction does listener data travel?** *Proposed:* the core owns every socket (Constitution I.9 puts multicast and listeners in one place) and **pushes
typed observations in** (`Observe(Observation)`), where an observation is plain data: a device address, a kind id, a number, and for phone-home a validated domain
name. The stack keeps what it needs under its own storage claim and offers findings through `Stack.Findings` (ADR-0036), worded by the core.

## Programs (S5): what the scoped dial does and does not cover

The scoped dial applies the declared scan scope to **connections the stack opens itself**. A program such as `nmap` runs as a separate process under the isolation
port and opens its own sockets, so the scoped dial never sees them. Reading the code: the targets of a scan are authorised **before** the engine is reached
(`netaudit.PolicyAuthorizer.Authorize` in `scan.go`: names resolved, every address checked against the scope), and the engine receives `[]netip.Addr`. `scantool` is
the agent's `network_scan` tool: it depends on `core/assistant/prompts`, `core/toolregistry` and the `models` context values, and is not a sensor. Two real options:

- **(a)** `nmap` and `portprobe` move behind a typed method (addresses in, typed hosts out) whose host adapter checks every address against the scope before the stack
  is called, and the isolation adapter checks the target arguments of a `RunRequest` against the program table. The second check is the only defence if a stack
  built a different argument list, and is only as good as the table of which arguments are targets.
- **(b)** `nmap`, `scantool` and `wifilook` stay in the core; only the pure leaves move. Cost: the Network stack is not self-contained; benefit: no new enforcement
  surface for a process that can reach anything.

*Proposed:* (b) for `scantool` (it is a tool, not a sensor) and decide `nmap`, `portprobe`, `wifilook` after the owner chooses between (a) and (b).

## Decision

Not taken. The status stays `proposed` until the owner answers Q1 to Q3 and chooses (a) or (b). Nothing is built from this ADR.

## Consequences if the proposed answers are accepted

- The SDK gets two small things: an observation push (Q3) and an exchange port (Q2), each plain data, each with its manifest requirement and a drift test.
- A names-only allowance (Q1) is a change to ADR-0038 rule 3 and must be recorded there when accepted.
- Real multicast and listener behaviour changes owner (the core still holds the sockets), so each move needs a run on vertex, not only tests.

## Links

ADR-0038, ADR-0036 (findings by id, `Names`), ADR-0035 (ports are plain data), `docs/PLANS/cross-cutting/network-stack-s6-plan.md`,
`docs/PLANS/cross-cutting/network-stack-p3b-design.md` (D6).
