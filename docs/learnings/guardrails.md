---
subsystem: guardrails
last_updated: 2026-10-26
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
