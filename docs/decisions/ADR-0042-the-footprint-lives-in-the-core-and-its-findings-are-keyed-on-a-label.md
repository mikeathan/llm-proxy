---
id: ADR-0042
title: The footprint lives in the core, and its findings are keyed on a label, not a device
status: accepted
date: 2026-11-10
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0042: The footprint lives in the core, and its findings are keyed on a label, not a device

## Context

The owner wants to watch their own domains and email addresses outside the network: new certificates, expiry, mail-protection records,
breaches. Every finding, acceptance, fix record and history record is keyed on `Finding.Device`, which holds an address; the computer's
own findings already use the fixed label `this-computer` instead. The sensors reach outside services, one of which needs a paid key.

Measured on 2026-11-10 (real answers, `example.com`): IANA's RDAP bootstrap and Verisign's RDAP answer parse and carry an `expiration`
event; some registries are listed with plain `http://` only; crt.sh returned 502 twice (it is often down). The breach service's
documentation says an API key is required, that a k-anonymity search of an address by the first six characters of its SHA-1 hash exists
only on its Pro plans, and that matches other than the searched address must be discarded at once.

## Decision

- **Placement: a core package, `core/netaudit/footprint`, a sibling of `outsidelookup`.** The Network stack's `Fetch` port has no request
  headers and caps a body at 1 MiB; the breach key must be sent as a header and must never enter a stack. A split (certificates, expiry
  and mail in the stack, breaches in the core) would still leave the settings, schedule and findings in the core and add a second reader.
  This is the ADR-0037 precedent. If a second consumer appears, the three domain sensors can move without changing their results.
- **Finding labels (owner, 2026-11-10): `domain:<name>` and `email:<id>`.** The email id is opaque, assigned when the address is added and
  kept in settings. Not the masked address (two addresses collide) and not a hash (it can be matched against known addresses). The
  page shows the masked address from settings; a finding never carries the address.
- **Only the k-anonymity breach search is built.** A direct search sends the whole address in the URL, and the guarded client logs the
  URL on failure. A key whose plan lacks the private search reads "could not look: key refused".
- **The audit never asks an outside service.** A look has its own schedule (manual, daily, weekly) and "Check now"; the report reads the
  stored results. A watch is replaced in the report only when every switched-on check has a successful look within twice its frequency;
  otherwise its earlier findings are carried and nothing is resolved (the ADR-0019 rule: a look that was not made says nothing).
- **Outside text never reaches a finding.** Certificate names, TXT text and breach names are read as ids, counts, flags and validated
  short names; findings use the core's own words.
- **Each service is off until the operator turns it on** (a step-up write), and the switch says what that service learns.

## Consequences

Domain and email findings have no device actions (rescan, expected ports, deep check, recheck). The first look of the certificate check
is a baseline, so a domain with many certificates does not start with many "new" ones. The history of a footprint finding starts when it
is first reported.

## Alternatives considered
- **A new stack.** New host wiring and a second findings reader, with no isolation gained.
- **Opaque id for domains as well.** Unreadable in logs and the UI for no privacy gain: the operator's own domain is public.
- **Direct breach search.** Sends the full address; the URL is logged on failure.

## Links
- Handover: `docs/PLANS/cross-cutting/network-assistant-fixloop-footprint-posture-handoff-2026-11-10.md`
- Related: [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md), [ADR-0020](ADR-0020-dns-check-only-records-and-never-reads-silence-as-agreement.md), [ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md), [ADR-0037](ADR-0037-where-the-vulnerability-and-ubuntu-feeds-live-when-network-becomes-a-stack.md), [ADR-0038](ADR-0038-the-network-stack-owns-the-sensors-and-the-core-keeps-the-judgement.md)
