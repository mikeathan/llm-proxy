---
status: proposed
date: 2026-10-10
last_reviewed: 2026-10-10
related_specs: [SPEC-007, SPEC-003]
constitution_references: [III.2, III.3]
related_plans: [../cross-cutting/xdg-config-data-relocation.md]
---

# Playbook Library Management (add, remove, edit)

**Status:** proposed. Needs investigation before any build; Phase 0 is that investigation.

## Why this plan exists

Playbooks (task templates) are read-only today: the library is exactly the set embedded in the binary
(`backend/data/templates/`). An operator cannot add their own, hide one, or adjust a shipped one for their
setup (for example the news brief's window or sources) without changing the repo and rebuilding.

This plan exists because the earlier design, a library copied to disk and refreshed on startup, went stale
on a live host (a news-brief automation kept running an old task file with no memory step and a 60-day
window). That design was removed on 2026-10-10 in favour of reading the embedded set directly. Any editing
feature must not bring back a second copy that has to be kept in sync with the binary.

## Ground truth (verified in code, 2026-10-10)

| Fact | Source |
|---|---|
| The library is the embedded set, listed and read directly; no directory, no seeding, no manifest | `backend/internal/platform/storage/template_store.go`, wired in `storage/manager.go` |
| API is read-only: `GET /admin/api/templates` and `GET /admin/api/templates/{id}` | `backend/internal/app/routes.go:188-189` |
| A template's identity comes from its `**ID:**` / `## Task:` / `**Category:**` header lines, not its filename | `template_store.go` `parseMetadata` |
| Adding to a workspace is a copy: "New file" writes `<id>.md` and asks before replacing a different file; "append" adds to the open buffer | `frontend/src/composables/assistant/useTemplates.ts:44-80` |
| A workspace copy is never updated when the shipped playbook changes | pitfall 41 in `docs/architecture.md` |

## Open questions (Phase 0 answers these)

1. **Where do operator playbooks live?** An install-level directory under the single root, or per workspace?
   Per workspace means no library feature at all (the workspace file already is the playbook), so the real
   question is whether operators want a shared library.
2. **Override or fork?** If an operator edits a shipped playbook, is that an override stored next to the
   shipped one (shipped stays untouched, reset = delete the override) or a new playbook? Override-by-id is the
   simpler model; confirm it with real use.
3. **Upgrades.** A shipped playbook that changes under an override: ignore silently, or show "a newer shipped
   version exists"? The editor already knows when a workspace file equals the library text, so an
   "out of date" hint on workspace copies may be enough and costs no storage.
4. **Validation.** Required header lines, ID collisions with shipped playbooks, size cap, allowed characters.
   A playbook becomes LLM prompt content, so this is an input-validation and security-review item.

## Phases (outline, not started)

- **Phase 0, investigate:** answer the questions above with the operator; write the decision into this plan;
  check SPEC-007 / SPEC-003 for the contract to amend.
- **Phase 1, backend:** operator-playbook store under the single root with atomic writes (CONSTITUTION III.2),
  validation, `POST/PUT/DELETE /admin/api/templates`, shipped read-only plus override resolution. Tests first.
- **Phase 2, UI:** add, edit, delete and reset-to-default in the playbook panel; mark shipped vs custom.
- **Phase 3, docs:** SPEC amendment, `docs/data-layout.md`, pitfall 41, this plan to `complete`.

## Remaining Work

Everything. Nothing is built.
