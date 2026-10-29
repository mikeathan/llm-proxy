---
id: ADR-0007
title: No nmap in CI; verify network scans by hand
status: accepted
date: 2026-10-26
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0007: No nmap in CI; verify network scans by hand

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
The network audit runs nmap under the sandbox's Landlock grants. Adding nmap to the sandbox-conformance CI job would cover that path automatically but enlarges and slows CI.

## Decision
We will keep nmap out of CI. nmap under the Ubuntu Landlock grants and the check scripts are verified by hand on an Ubuntu host, with the procedure recorded in the plan.

## Consequences
CI stays small and fast. A Landlock regression for nmap is caught only by the manual check, so it must be rerun after sandbox or scan-engine changes.

## Alternatives considered
- **nmap in the sandbox-conformance job** — owner declined.

## Links
- [`PLANS/ARCHIVE/cross-cutting/network-audit.md`](../PLANS/ARCHIVE/cross-cutting/network-audit.md) D13, §13.D (owner decision, 2026-10-26)
