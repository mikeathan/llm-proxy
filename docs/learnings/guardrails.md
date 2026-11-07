---
subsystem: guardrails
last_updated: 2026-11-06
---

# Learnings — Guardrails, sandbox and egress (SPEC-006)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Porting a structural `write_file` guardrail from Hermes → **Hermes has none — its "guardrails" are completion gates; renaming `save_file` → `write_file` was cosmetic**. **Lesson:** tool-intent governance is prompt-based in both; a real block needs our own layer. Evidence: [hermes-write-file-guardrail.md](../audits/hermes-write-file-guardrail.md).
- **Tried:** A blanket hidden-directory block on the terminal tool → **rejected**. **Lesson:** `git`, `npm`, `go` need dot-dirs; block a curated sensitive set plus `.sandbox`. Evidence: [terminal-vs-filesystem-guardrail-asymmetry.md](../audits/terminal-vs-filesystem-guardrail-asymmetry.md).

## Root causes and fixes that held

- **Tried:** Terminal path checks that covered only absolute paths and `..` → **`.sandbox` was readable via the terminal while the file tools blocked it**. **Lesson:** share `blocked_filenames` with the terminal; return `path access denied` so it is a silent policy denial. Evidence: [terminal-vs-filesystem-guardrail-asymmetry.md](../audits/terminal-vs-filesystem-guardrail-asymmetry.md).
- **Tried:** Validating a multi-line command as one line → **a newline-separated second command could skip the per-segment whitelist**. **Lesson:** validate per line, same normalization as execution. Evidence: [2026-08-30-llm-smoke-test-incomplete-run.md](../audits/2026-08-30-llm-smoke-test-incomplete-run.md).
- **Tried:** No authentication and a `0.0.0.0` bind (2026-09-30 scan) → **any LAN client can start runs, kill processes, change provider URLs**. **Lesson:** tracked; owner parked authentication. Evidence: [2026-09-30-platform-scan.md](../audits/2026-09-30-platform-scan.md), [admin-api-authentication.md](../PLANS/cross-cutting/admin-api-authentication.md).

## Relaxation inventory and measurements (stacks-as-packages P1, 2026-11-06)

Every way the isolation guarantees are lifted today, read in the code at origin/main `c218fa09`. The isolation library
(`backend/internal/platform/isolation`, suite `isolation/isolationtest`) starts every profile with all of these off.

| Relaxation | Where | Who sets it | Scope | Decision |
|---|---|---|---|---|
| `sandboxing.unix_sockets: true` | `models/host_settings.go:68`, `UnixSocketsAllowed`; wired at `core/toolregistry/registry.go:420` | operator, read at start-up | every agent shell and guarded program | keep; tested by the suite ("operator's switch") |
| Per-spawn `AllowUnixSockets` | `progexec.Invocation`, `wifilook.go:194` | code, one call site (nmcli over D-Bus) | that program only | keep; a new site needs its own review |
| `sandboxing.filesystem: false` (jail off) and `network: false` (host network ceiling) | `HostSandboxingConfig.Filesystem`, `.Network` | operator; changing them is a step-up action (Constitution I.5) | whole host | keep; not reachable from a stack (ADR-0033) |
| `netaudit.allow_unconfined` | `models/netaudit_config.go:11`, `app/netaudit.go:683`, `netaudit/scope.go:110` | operator confirm | guarded programs on a host with no jail (development) | keep; the suite proves a program does not start without it |
| Terminal switch (`terminal.enabled`) | `tools.ValidateTerminalCommand` start | operator | terminal tool | keep |
| Guardrail approval (30-minute TTL, exact call) | `guardrails.go:170` `defaultOverrideTTL`, `MarkApproved` | user, per call | one tool call with the same arguments | see the measurement below |

**Measured: an approved override skips the terminal allow-list checks, and every check after the one the user was shown.**
`ValidateToolCall` returns nil on `hasApproval` right after the hard gates (`guardrails.go:217`), before `validateGlobal` and the
terminal checks. The terminal order is blocked patterns, blocked paths, allow-list, shell launchers, path security. A
blocked pattern or blocked path is refused as a security boundary and cannot be approved. The allow-list comes next, so a command
that fails it and also fails path security (`rm /etc/hosts`) shows the user only the allow-list reason; once they approve, the jail
check is never run for that exact call. Pinned by `TestApproval_SkipsTheTerminalChecksAfterTheOneTheUserWasShown`. Network and
lock-down gates run before the fast path and are not skipped. Not changed in P1 (no behaviour change); the options are a decision
for the owner: re-run the checks after the approved one and keep only that check lifted, or show every failing reason in the prompt.

**Tried:** the conformance suite on macOS and in a Linux container (kernel 6.8, Landlock ABI 4+). **Held:** on Linux every guarantee
was enforced; on macOS the host has no jail, each guarantee skips with the reason Effective reports. Mutating the library to drop
the TCP denial, the Unix denial and the jail made the suite fail on exactly those three.
