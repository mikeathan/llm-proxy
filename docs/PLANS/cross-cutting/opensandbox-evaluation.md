---
status: proposed
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-006]
constitution_references: [I.5, II.3]
evidence: opensandbox-group/OpenSandbox README, docs/architecture/network/egress.md, docs/getting-started/configuration.md, SECURITY.md (read 2026-11-08 via gh); backend/internal/platform/sandbox/sandbox.go (package comment); docs/decisions/ADR-0033. Nothing installed or run.
---

# OpenSandbox evaluation (investigation, not built)

**Status:** proposed. A desk review only. **No code, no dependency and no CI change** without owner approval (AGENTS.md → ask first).

## Question
Can [OpenSandbox](https://github.com/opensandbox-group/OpenSandbox) (Apache-2.0, an Agentic AI Foundation project, release 1.2.0 on 2026-10-27) sandbox our agents, and is it better than `backend/internal/platform/sandbox`?

## What the desk review found (docs read, not run)
- **What it is:** a server that runs agent workloads in containers (Docker, local) or Firecracker microVMs (Kubernetes "Fast Sandbox"); gVisor and Kata are the other secure runtimes. An in-sandbox daemon (`execd`) runs commands and files; a per-sandbox sidecar enforces egress (FQDN allow/deny, DNS plus nftables default-deny, a Credential Vault, fail-closed start). SDKs for Python, Java, TS, C#, Go; a CLI and an MCP server.
- **Ours:** a per-child-process jail in the same process tree: Landlock filesystem jail, Landlock TCP denial (kernel ≥ 6.7), a seccomp filter refusing `AF_UNIX` and `io_uring`. No daemon, no container, almost no start-up cost. macOS has no OS mechanism yet.
- **Verdict:** it confines a different thing. A kernel boundary (microVM, gVisor) is stronger and its egress control works on any host, but it runs a *separate* environment, not the user's workspace. Local use needs Docker plus a Python server; the strong modes need Kubernetes. It does not fit our host-access features (`nmap`, `nmcli`, the privileged helper: ADR-0031, ADR-0034). Replacing the sandbox is ruled out by ADR-0033 (proposed) and Constitution I.5 and II.3.
- **Concerns it states itself:** the current umbrella releases have a tracked signing and provenance gap (its issue #2463); parts of the TLS-interception egress are marked experimental; gVisor cannot run its nftables features.

## Possible use (the only one worth testing)
An **optional, opt-in "isolated execution" tool** for model-written code (a code interpreter, untrusted scripts), beside the Landlock sandbox and never replacing it. The agent's own child processes stay in our sandbox.

## Phases
1. **Trial (needs owner go-ahead to install anything).** Run its Docker mode locally in a scratch directory. Acceptance: written answers to (a) default hardening of the container (capabilities, privileged, `no-new-privileges`, pids and memory limits, user), (b) whether a host-mounted workspace can be made read-only or confined and what an escape would need, (c) start-up cost per sandbox, (d) what runs on macOS and on vertex, (e) the image and release verification steps we would have to do ourselves.
2. **Decide.** An ADR: adopt as an optional tool, adopt as an extra for the macOS network gap, or reject. Inputs: phase 1 answers, SPEC-006 §II.7 downgrade contract (never claim enforcement not applied), ADR-0033 (the tool must not become a way around the core sandbox).
3. **If adopted:** a design for the tool surface, guardrails and approvals, secrets (the vault must not hold our keys), SPEC-006 amendment, tests. Not planned until phase 2 says yes.

## Remaining Work
Everything. First step: owner decides whether to run phase 1. Until then nothing is installed.
