---
id: ADR-0029
title: The service keeps MemoryDenyWriteExecute, so the nuclei web checks are removed
status: accepted
date: 2026-11-06
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0029: The service keeps MemoryDenyWriteExecute, so the nuclei web checks are removed

## Context
The web checks (v0.48.0) ran nuclei as a guarded program. On its first real run on vertex (2026-11-05) it died at start with `panic: permission denied`. After `progexec` kept the whole error (v0.50.2) the trace named `github.com/bytedance/sonic/loader.mprotect`: nuclei's JSON library builds machine code in memory at start-up and needs a page that is writable and then executable. The service unit sets `MemoryDenyWriteExecute=true` (`docs/services/llm-proxy.service`), which systemd applies to the service and every program it starts; a child cannot lift it, and the Landlock jail has nothing to do with it. A test in a container without that setting passed, which is why the fault was not found before. The same unit's `ProtectKernelModules=true` hides `/usr/lib/modules` from the service, which is why the package-file pass counted 13,828 files it could not read.

## Decision
We will keep `MemoryDenyWriteExecute=true` and remove the nuclei web checks and everything that served only them: the `nuclei` package, the `web_checks` module, the `web_exposure` finding, the template download route and its Settings card, the Constitution II.3 sentence that admitted nuclei, and `progexec.Locate`. The persisted `web_checks_enabled` key is no longer read (an old key in a settings file is ignored).

## Consequences
- The proxy and every program it starts keep the memory protection; a flaw in a program that reads untrusted replies from LAN devices is harder to turn into running code.
- The audit loses 24 checks of the kind "this device's admin page is open" or "this web server leaks its status"; nmap, the vulnerability matching, the deep checks and the host checks are untouched.
- Any future program that needs executable memory (a JIT, a Go program that uses `sonic`) cannot run as a guarded program under this unit. Check that before choosing a tool.

## Alternatives considered
- **Remove `MemoryDenyWriteExecute` from the unit** — nuclei would run, but the whole service, including its login-protected admin API and every agent tool, would lose a layer, for a modest feature.
- **Run nuclei as its own systemd service** — keeps the hardening, but needs a second unit, a way to start it and a way to read its output; more machinery than the checks are worth.
- **Keep the code switched off** — dead code with a Constitution carve-out for a program that cannot run here.

## Links
- Plan: `docs/PLANS/cross-cutting/stack-modules-and-new-detection.md` (P4, removed). Ledger: `docs/learnings/network-audit.md` (2026-11-06).
- PR: v0.50.3.
