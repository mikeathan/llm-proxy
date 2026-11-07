---
id: ADR-0033
title: The sandbox is core and cannot be swapped or configured off, and a stack gains no way around it
status: proposed
date: 2026-11-06
subsystems: [guardrails, network-audit, platform]
supersedes:
superseded_by:
---

# ADR-0033: The sandbox is core and cannot be swapped or configured off, and a stack gains no way around it

Proposed: it is in force when the owner accepts it. Companion of [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md).

## Context
Moving Network into a stack package raises the question whether a stack, its manifest or a setting could be used to relax the agent sandbox or
the network guards.

What exists (read 2026-11-06): the OS sandbox in `backend/internal/platform/sandbox` (a Landlock filesystem jail, a Landlock TCP denial, a
seccomp filter that refuses `socket(AF_UNIX)` and `io_uring_setup`) and `platform/progexec` for guarded programs (Constitution II.3,
[ADR-0031](ADR-0031-the-wifi-look-is-a-guarded-nmcli-with-no-network-and-only-unix-sockets.md)). Relaxations already exist:
`sandboxing.unix_sockets: true`; the sandbox and terminal switches, which need the passphrase (Constitution I.5); and guardrail overrides
held for 30 minutes (`defaultOverrideTTL`, `core/assistant/guardrails/guardrails.go:170`). Whether an approved override skips the terminal
allow-list checks is not settled here; the plan's first phase measures it.

Network's guarded programs (`nmap`, `nmcli`) and its connections reach the sandbox and the guard today through `internal/` packages; a stack
would reach them through the SDK.

## Decision
We will keep the isolation code in the core and make these rules:
1. **The isolation library is core, not a stack.** It is a standalone package with a profile type and a conformance test suite. It cannot be
   uninstalled, swapped or switched off by a stack, a manifest or a setting added for stacks.
2. **A stack reaches the sandbox and the network guard only through the SDK** (Constitution IV.6). It cannot start a program, open a
   connection or relax a scan scope by any other route.
3. **No new relaxation is added for stacks.** A stack's manifest cannot declare a privilege, and the unit's hardening
   (`docs/services/llm-proxy.service`) is unchanged by this work.
4. **Every existing relaxation is listed and decided** in the plan's first phase (the Unix-socket setting, the sandbox and terminal
   switches, the 30-minute override), so the sandbox's guarantees are written once and tested by one suite.
5. **Data returning to a model stays typed fields and untrusted**, as in [ADR-0025](ADR-0025-network-timeline-reads-existing-records-summary-is-local-and-typed.md).

## Consequences
- The sandbox's guarantees are tested in one place, wherever a child runs.
- Work that needs a privilege the unit refuses stays blocked until a separate decision (see the follow-on plan).
- If agents or a stack ever run as their own process, this ADR already says what must not move: the isolation library goes with the
  model-driven code, and secrets stay in the core.

## Alternatives considered
- **Make the sandbox an installable stack** — then it could be removed or replaced, which is itself the bypass.
- **Let a stack's manifest request a privilege** — a request an operator approves is a second bypass path to defend; with no separate
  process there is nothing to grant it to.
- **Leave the sandbox inside `internal/` with no conformance suite** — the relaxations above stay undocumented, and a stack could copy a path
  that skips them.

## Links
- Plan: [`stacks-as-packages-and-network-stack.md`](../PLANS/cross-cutting/stacks-as-packages-and-network-stack.md) (P1).
- SPEC-006 (guardrails and sandbox), Constitution II.3, I.5 and IV.6 (proposed).
