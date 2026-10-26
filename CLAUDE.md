# CLAUDE.md

The operating contract for this repository is **AGENTS.md** — follow it; it outranks this file. It is imported below
so Claude Code loads it: Claude Code reads AGENTS.md on its own only when no CLAUDE.md exists (verified 2026-10-09 —
once this file was added, AGENTS.md stopped loading). This is the repo's only Claude-specific instruction file
(`docs/decisions/ADR-0001-harness-neutral-agent-instructions.md`); keep it a pointer.

@AGENTS.md

Claude-specific notes only:

- **Skills.** The Agent Skills live in [`.agents/skills/`](.agents/skills/). `.claude/skills` is a symlink to that
  folder so Claude Code discovers them; edit the files under `.agents/skills/`, never a copy.
- **Nested instructions.** Claude Code does not read the nested `backend/AGENTS.md` / `frontend/AGENTS.md`; the
  rule files they point to are loaded through AGENTS.md → Before coding, as for every harness.
- **Memory.** Auto-memory is a private, per-machine cache: keep personal preferences there, and put every finding a
  fresh clone would need in the repo, as AGENTS.md → Knowledge and memory says.
