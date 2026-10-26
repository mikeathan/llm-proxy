---
id: ADR-0008
title: The terminal guard blocks a curated sensitive set, not every dot-directory
status: accepted
date: 2026-08-20
subsystems: [guardrails]
supersedes:
superseded_by:
---

# ADR-0008: The terminal guard blocks a curated sensitive set, not every dot-directory

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
The file tools block every hidden directory; the terminal tool blocked only absolute paths outside the jail and `..`, so `.sandbox` was readable through the terminal.

## Decision
We will block in the terminal the sandbox runtime directory (hardcoded) and the shared `blocked_filenames` list (credentials and keys), returning the same `path access denied` error as the file tools so it is a silent policy denial. We will not block every dot-directory in the terminal.

## Consequences
`git`, `npm`, `go` keep working. The sensitive list is one shared list for both tools; adding a secret file type means adding it there.

## Alternatives considered
- **Blanket hidden-directory block in the terminal** — breaks normal developer commands.

## Links
- [`audits/terminal-vs-filesystem-guardrail-asymmetry.md`](../audits/terminal-vs-filesystem-guardrail-asymmetry.md)
- [`SPECS/guardrails.md`](../SPECS/guardrails.md)
