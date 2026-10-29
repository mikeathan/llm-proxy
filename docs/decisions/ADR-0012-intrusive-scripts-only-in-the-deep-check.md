---
id: ADR-0012
title: Intrusive scripts run only in an interactive, step-up deep check of one device
status: accepted
date: 2026-10-29
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0012: Intrusive scripts run only in an interactive, step-up deep check of one device

## Context
Some useful checks (weak TLS versions and ciphers, SSH password login) use nmap scripts in the `intrusive` category. A nightly audit must stay quiet and never disturb a device (§13.B.2, §13.J).

## Decision
We will run intrusive scripts only from a Deep check the operator asks for on one chosen device, in a session with a freshly confirmed passphrase (a token cannot start it), with its own script list, slow timing, one device and a cool-down. The nightly audit and the agent tool refuse the deep mode. Findings carry fixed wording and are dated in the report. The Constitution (II.3, "Intrusive runs") says so.

## Consequences
Intrusive checks exist without ever running unattended. The cost is a manual click per device, and results are carried into later audits only for 90 days.

## Alternatives considered
- **Intrusive scripts in the nightly audit** — a model or a schedule could disturb a device unattended.
- **Never run them** — leaves weak TLS and SSH password login undetected.

## Links
- Plan: [`network-audit.md`](../PLANS/ARCHIVE/cross-cutting/network-audit.md); as built: [`architecture/network-audit.md`](../architecture/network-audit.md).
