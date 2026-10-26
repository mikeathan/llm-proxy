---
id: ADR-0001
title: Harness-neutral agent instructions
status: accepted
date: 2026-10-26
subsystems: [agent-harness]
supersedes:
superseded_by:
---

# ADR-0001: Harness-neutral agent instructions

## Context
The owner works in this repo with five coding agents: opencode, pi, Command Code and Codex (any model), and
Claude Code (Claude models). Each harness has its own conventions; copying instructions per tool drifts, and
vendor names in shared files make the contract read as tool-specific. Phase 0 of
[`docs-and-agent-harness-restructure.md`](../PLANS/ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md) checked
how each one loads instructions and skills (2026-10-26, from the tools' docs and their published mirrors — no
local install was available, so re-check after a major version bump):

| Harness | Root `AGENTS.md` | Nested `AGENTS.md` | Skills from `.agents/skills/` |
|---|---|---|---|
| Codex | yes | concatenated from repo root **down to the working directory** | yes (project and `~/.agents/skills`) |
| opencode | yes (`CLAUDE.md` only as a fallback when no `AGENTS.md`) | found by walking **up** from the working directory | yes (also `.opencode/skills`, `.claude/skills`) |
| pi | yes (also reads `CLAUDE.md`), walking up from the working directory | only via an extension (lazy, per file) | yes (`.pi/skills` and `.agents/skills`, project skills after the project is trusted) |
| Command Code | not confirmed in its docs | not confirmed | yes (`.agents/skills`, native `.commandcode/skills` wins on a name clash) |
| Claude Code | **no** — reads `CLAUDE.md`, which can `@import` `AGENTS.md` | no (nested `CLAUDE.md` only) | no — reads `.claude/skills` |

The Agent Skills format (`SKILL.md`) recognizes `name`, `description`, `license`, `compatibility`, `metadata`
(string map) and experimental `allowed-tools`; harnesses may ignore or reject other top-level keys, load only
`name` + `description` up front, and load the body and bundled files on demand.

## Decision
We will keep **one vendor-neutral source** and give each harness the thinnest possible adapter:

1. `AGENTS.md` is the contract. It names no tool, vendor or model ("the agent", "the harness").
2. `.agents/skills/<name>/SKILL.md` is the only skill tree. Four harnesses read it natively; Claude Code gets the
   `.claude/skills` symlink to it. Frontmatter uses only spec fields; repo bookkeeping such as `last_reviewed`
   lives under `metadata`.
3. `CLAUDE.md` is the one vendor-named instruction file: `@AGENTS.md` plus a few Claude-only notes. The string
   `claude` may appear only in `CLAUDE.md`, the `.claude/skills` symlink, this ADR (the compatibility record,
   which has to name the tools), and the harness check and git hook that verify them —
   `scripts/check-agent-harness.sh` enforces this over the agent-facing files. Product code, tests, plans and
   audits are out of scope: there the name is provider or model data, or history.
4. No other tool-specific files (no `.claude/rules`, `.claude/settings.json`, `GEMINI.md`, Copilot or
   `.commandcode/` instructions). Path-scoped guidance comes from thin nested `backend/AGENTS.md` and
   `frontend/AGENTS.md` pointers, which help only when a session starts inside that folder; the root
   "Before coding" step, which works everywhere, stays the primary way rule files get loaded.
5. Harness enforcement runs from the tool-neutral git hook and CI, never from one tool's hook system.

## Consequences
- Adding a sixth harness means checking the table above, not editing content.
- Command Code's `AGENTS.md` support is unconfirmed; if it does not read it, it still gets the skills, and the
  owner can point it at `AGENTS.md` once. Re-check when it documents context files.
- Nested `AGENTS.md` files must stay pointers that send the agent to the root file first: opencode and pi stop
  at the nearest file when walking up, so a self-contained nested file would hide the root contract.
- Windows checkouts without symlink support would see `.claude/skills` as a text file; if a contributor needs
  Windows, replace the link with a generated copy checked byte-for-byte by the harness.

## Alternatives considered
- **Per-tool instruction files** (`.claude/rules`, `GEMINI.md`, a Copilot instructions file) — duplicates
  content and adds vendor-named files for tools the owner does not use.
- **`CLAUDE.md` as the canonical file** — three of the five tools would read it only as a fallback, if at all.
- **Path-scoped rules through one harness's own mechanism** — works for one tool only.

## Links
- Plan: [`docs-and-agent-harness-restructure.md`](../PLANS/ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md) §3
- Sources (2026-10-26): opencode docs `opencode.ai/docs/skills`, `opencode.ai/docs/rules`; pi docs
  `pi-coding-agent/docs/skills.md` (npm mirror); Codex `developers.openai.com/codex/skills` and the
  AGENTS.md discovery write-up at `codex.danielvaughan.com`; Command Code `commandcode.ai/docs/skills`;
  Agent Skills specification `agentskills.io/specification`.
