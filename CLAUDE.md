# CLAUDE.md

The operating contract for this repository is **AGENTS.md** — follow it; it outranks this file. It is imported below
so Claude Code loads it: Claude Code reads AGENTS.md on its own only when no CLAUDE.md exists (verified 2026-10-09 —
once this file was added, AGENTS.md stopped loading).

@AGENTS.md

Claude-specific notes only:

- **Skills.** The Agent Skills live in [`.agents/skills/`](.agents/skills/) (`name` + `description` frontmatter).
  `.claude/skills` is a symlink to that folder so Claude Code discovers them; edit the files under `.agents/skills/`,
  never a copy, and run `./scripts/check-agent-harness.sh` after changing one.
- **Rules.** The mandatory language rules in [`.agents/rules/`](.agents/rules/) are loaded as AGENTS.md → Before
  coding says (backend: `go-staff-engineer.md`, frontend: `frontend-vue-engineer.md`); they are not skills.
- **Memory.** AGENTS.md → Memory / Knowledge Persistence applies to Claude Code's auto-memory: save durable findings
  there, with `file:line` pointers.
- **Routing.** Start from [docs/INDEX.md](docs/INDEX.md); the constitution is [CONSTITUTION.md](CONSTITUTION.md).
