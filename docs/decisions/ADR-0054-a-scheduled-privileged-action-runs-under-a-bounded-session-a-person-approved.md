---
id: ADR-0054
title: A scheduled privileged action runs under a bounded session a person approved
status: proposed
date: 2026-11-13
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0054: A scheduled privileged action runs under a bounded session a person approved

Proposed 2026-11-13; becomes accepted when the owner merges the pull request that carries Constitution I.17. Builds on [ADR-0034](ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md) (the helper, its three gates and one-use grants) and [ADR-0052](ADR-0052-radio-is-its-own-stack-with-generic-sensors-and-the-core-keeps-capture-and-judgement.md) (duty-cycled windows).

## Context
Every privileged request needs a one-use grant minted for a person's step-up (Constitution I.11). The Radio stack listens in windows: 20 seconds a channel, a cycle every 5 minutes, about six channels, which is about 1,700 requests a day. Nobody can confirm a passphrase 1,700 times, so the sensor would either not run unattended or the rule would have to go. The owner asked for a production-ready, reusable elevated type that a person allows and that is carefully bounded, because other actions may need it later.

## Decision
We will add **bounded sessions** (`platform/privhelper/session`, Constitution I.17):

- A person confirms the passphrase once (a step-up route) and approves an **exact list** of (action, checked parameters) for **one named stack**, for a time and a number of requests.
- The core serves a request from that stack that matches an entry exactly, through a per-stack port (`port.ForStack`). A request that carries a grant takes the grant path unchanged.
- **No token.** The stack asks with no grant and holds nothing to leak, copy or lend; the authority is the session in the core's memory. A restart ends every session.
- **Only actions the catalogue marks `Standing`** (`radio_probe`, `radio_listen`). A scan, a capture of the computer's own traffic and anything that sends is not standing. A new standing action is a security review.
- **Hard limits in code:** 12 hours, 5,000 requests, 64 list entries, 4 sessions open. The action's hourly cap, the target scope, the action's switch (checked on every use) and the helper's own three gates and scope still apply.
- **Visible and revocable:** list, end one and end all need no step-up (they only remove authority); starts and ends are logged with id, stack, limits and use count; the list keeps the last ten that ended with the reason.

## Consequences
- The scheduler (R3) calls the stack's `Privileged` port with no grant and is served only while a session approved that exact request. The Radio page needs a start-listening control (hours, adapter, channels), a visible "listening until" state and a stop button; none is built yet.
- A leaked stack cannot widen anything: it can only repeat requests a person approved, within the time and count, and only while the action is on.
- A session lasting up to 12 hours is a standing risk the owner accepts; a longer or indefinite sensor needs an amendment.
- Adding a standing action means a code change plus a line in `TestOnlyTheRadioActionsMayRunUnderASession`.

## Alternatives considered
- **A long-lived token handed to the stack:** rejected; a token can leak or be lent, and it outlives the person's attention.
- **Amend I.11/I.16 for a standing sensor:** rejected for now; it removes the "a person just approved this" guarantee entirely and ADR-0052 chose windows to avoid it.
- **Manual windows only:** rejected; no unattended monitoring.
- **A new field on the SDK's `PrivilegedRequest` for a session token:** rejected; the session lives in the core and the stack's port carries it, so the SDK does not change.
