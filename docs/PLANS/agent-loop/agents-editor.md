# Agents editor — create and edit profiles (and helpers) in Settings

**Status:** active (2026-10-23). D1–D4 decided as recommended (the user also asked for a good phone layout and a full
desktop one). E0–E3 built; E4 (live check) and E5 (docs) left.
**Related Specs:** SPEC-012 (Prompt Profiles), SPEC-001 (§15 Delegation), SPEC-003 (UI).
**Constitution refs:** II.13 (prompt text only in code, user instructions are data), II.3 (nothing leaves the machine
unintended), IV.4 (no dead code).
**Follows:** [`multi-agent-delegation.md`](multi-agent-delegation.md), Part C ("The Settings → Agents editor").

## 1. Problem

A profile is the unit that decides how a run behaves: its instructions, tools, memory, step budget and, through
`delegates`, which helpers it may use. A helper is a profile too. Today a user can only add or change one by writing a
file by hand into `{data root}/profiles/*.md` (SPEC-012 §2.2), with a YAML header whose errors are only found when the
file is read. The picker lists them but cannot show the instructions, and nothing says which fields exist or what they
do. So "a research profile that always uses my own helper" needs a text editor and the file format open in another tab.

## 2. Goal

A Settings section, **Agents**, where a user can:

- see every profile (built-in and their own), what it is the default for, and what it delegates to;
- read a built-in's instructions and settings;
- create a profile, **duplicate** a built-in to change it, edit and delete their own;
- get the same validation the file parser applies, with the error next to the field.

The file stays the single source of truth (no database, no second format): the editor reads and writes the same
`profiles/<id>.md` files, so hand-edited files and the editor never disagree.

## 3. Design

### 3.1 Backend (`profile` package + handlers)

- `profile.Catalog` gains `Save(p Profile) error` and `Delete(id string) error` (user files only). `Save` writes the
  file atomically (temp file + rename) after `Parse` has accepted exactly what will be written; the file text comes from
  one `Profile.Marshal()` that is the inverse of `Parse` (a round-trip test over every built-in).
- Limits are the existing ones: 64 KB, at most 50 user profiles, id pattern, unknown ids in `delegates` rejected.
- New routes beside `GET /admin/api/profiles` (list, unchanged, no instructions):
  - `GET /admin/api/profiles/{id}` → the profile **with** its instructions;
  - `PUT /admin/api/profiles/{id}` → create or replace a user profile (400 with the parser's message on invalid input);
  - `DELETE /admin/api/profiles/{id}` → delete a user profile; if it replaced a built-in, the built-in returns.
- `GET /admin/api/profiles/schema` → the editable fields with type, allowed values, limits and one line of help each,
  generated from the same table the parser validates against, so the form and the parser cannot drift. The tool
  categories come from `models.ToolCategories`.
- Deleting or renaming a profile that a workspace, automation or other profile still names is not blocked: the existing
  rule ("an id that no longer exists is skipped, the next level applies") already covers it. The response lists who
  names it so the UI can say so.

### 3.2 Frontend

- Settings section **Agents** (checklist: `docs/architecture.md#adding-a-frontend-settings-tab-checklist`): a list on
  the left (built-ins first, a "yours" badge, default-for chips), the form on the right, `Panel` + `FormField`.
- Built-ins are read-only with a **Duplicate to edit** button (asks for a new id). A user file that replaces a
  built-in's id shows "Restore built-in" instead of "Delete".
- Form: name, summary, instructions (a large text area, no markdown preview), the toggles, steps, tool categories (a
  checkbox group), memory, memory save mode, helpers (a multi-select of the other profiles). Help text comes from the
  schema endpoint, not from strings in the component.
- Validation is the server's: the form sends, shows the message next to the field, and keeps the draft. It has one
  unsaved-change guard (the page's), like the other sections.
- A helper-specific hint: a profile listed in another profile's `delegates` runs on the Helper model with only web
  search and page fetch, whatever its own `tools` say (SPEC-001 §15); the editor says so where `delegates` is chosen.

### 3.3 Not in this plan

Per-profile model (a helper still uses `agent.subagent_model`), `on_finish`, the reviewer type, `files` / `cloud_files`,
pipelines, import/export of profiles, and a profile history.

## 4. Safety

- Instructions are data, not code: they are placed after the code-owned core rules and boundaries (SPEC-012 §2.4), so a
  user profile cannot remove a guardrail; the file lives outside every workspace so an agent with file tools cannot edit
  it. The API keeps that: `PUT` is an admin route and is never reachable from a tool.
- A profile can narrow tools (`tools`), never add ones the list lacks; the editor offers categories only.
- A saved profile applies from the next turn (files are read on demand); nothing needs a restart.

## 5. Phases

| Phase | What | Accept |
|---|---|---|
| E0 | `Profile.Marshal` + round-trip test over all built-ins; `Catalog.Save` / `Delete` (user dir only, atomic, limits) | Table tests: valid, invalid, oversized, 51st file, delete restores a built-in |
| E1 | Handlers: `GET /{id}`, `PUT`, `DELETE`, `GET /schema`; SPEC-012 §4 | Handler tests incl. 400 messages and who-names-it |
| E2 | Frontend service, types, composable; the Agents section with list and read-only view | `npm test`, `npm run build` |
| E3 | Create / duplicate / edit / delete forms with server validation | Component tests; `npm run test:visual`; screenshots of the list, the form and an error |
| E4 | Live check: duplicate Research, add a helper profile, list it under `delegates`, run a chat that delegates to it | A helper run with the new profile on vertex |
| E5 | Docs: SPEC-012, SPEC-003, architecture pitfalls; plan status | `./scripts/check-agent-harness.sh` |

## 6. Decisions (taken 2026-10-23, all as recommended)

- **D1:** the section is **Agents**, and every place the interface said "Profile" now says "Agent" (the picker, the
  workspace field, the automation form and details, the picker's accessible label). The file format, API paths and
  field names keep *profile*.
- **D2:** built-ins are read-only with **Duplicate**. A user file with a built-in's id stays editable, with "Restore
  built-in" in place of Delete.
- **D3:** the full editor, good on a phone (one pane at a time with a way back, a 44px touch target per row, the
  save row sticky above the bottom bar) and full on a desktop (list and form side by side).
- **D4:** deleting an agent that others still list as a helper is allowed; the confirmation and the response name them.

## Built (2026-10-23)

- **Backend:** `profile.Marshal` (inverse of `Parse`, round-trip tested on every built-in), `Catalog.Save` / `Delete` /
  `DelegatingTo` / `HasBuiltin` (atomic write, 64 KB and 50-file limits, unknown helpers refused, built-in ids refused),
  `profile.Schema` (one table for the form) and `profile.Explain` (which setting a refusal is about), the four routes
  (SPEC-012 §4).
- **Frontend:** `AgentsSettings` / `AgentList` / `AgentForm` / `AgentFieldControl`, `CheckboxGroup`, `useAgentsEditor`,
  `agentDraft` helpers, `HttpError` (carries the server's `field`), the section in General between Runs and Memory, and
  the rename (labels in `constants/labels.ts`).
- **Found while building:** `useProviders.settingsTabs` is a fixed list, so a new section is invisible in the Settings
  navigation until it is added there as well as to the type, the labels and the section groups.
- **Checked by screenshot** (mocked API, paper and dark themes, desktop and phone): the list, a built-in, an edit, a new
  agent and the sticky save row above the phone's bottom bar.

## Remaining Work

- E4: live check on vertex after the user deploys: duplicate Research, add a helper profile, list it under Helpers, and
  run a chat that delegates to it.
