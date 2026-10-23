---
id: SPEC-012
title: Prompt Profiles
version: "1.5"
status: draft
last_updated: 2026-10-23
constitution_references: [II.13, II.3]
related_specs: [SPEC-001, SPEC-004, SPEC-007, SPEC-011]
supersedes:
---

# SPEC: Prompt Profiles

## Changelog

- **1.5 (2026-10-23)** — **Settings → Agents** (SPEC-003): the user's own profiles can be created, edited, duplicated and
  deleted from the UI. In the interface a profile is called an **agent** ("Agent" in the chat picker, "Agent for new
  chats", an automation's "Agent"); the file format, the API paths and the field names keep the word *profile*.
  - The editor reads and writes the same `profiles/<id>.md` files (§2.2), so a hand-edited file and the editor never
    disagree. `Marshal` is the inverse of `Parse`; a save is parsed before it is written and written atomically.
  - API (§4): `GET /admin/api/profiles/{id}` (with the instructions), `PUT` (create or replace one of the user's own),
    `DELETE`, and `GET /admin/api/profile-schema` (the settings with help text, so the form and the parser cannot
    drift). A built-in cannot be saved or deleted: it is duplicated under a new id. A user file with a built-in's id can
    be edited, and deleting it brings the built-in back. A save naming an unknown helper is refused.
  - Deleting an agent that others still list as a helper is allowed; the response names them and the run skips the id.
- **1.4 (2026-10-22)** — `GET /admin/api/helpers` adds `problem` (`""` when helpers can run, else `unset`, `missing`
  when the Helper model was removed from the models, or `not_cloud`).

- **1.3 (2026-10-22)** — The chat request accepts `use_helpers` and `GET /admin/api/helpers` reports whether helpers can
  run (§4). A profile's `delegates` is what makes the switch appear in the composer.

- **1.2 (2026-10-22)** — `delegates: [ids]` lists the helper profiles a profile may hand tasks to (SPEC-001 §15).
  - The `delegate_task` tool exists only for a profile that lists helpers. Its `delegation` category cannot be named
    in `tools`.
  - Ids must be valid and never the profile's own.
  - A seventh built-in, **Web researcher** (`web-researcher`): web tools only, memory off, 15 steps. Research lists it
    as its helper. It also shows in the chat picker, where it works as a plain web-only chat profile.

- **1.1 (2026-10-17)** — `memory_guidance` is replaced by `memory_save: model|explicit|off`, the one switch for who
  may save to memory in a run (§2.1). `explicit` and `off` do not offer `memory_update` and `memory_forget`, send no
  save guidance and no pre-compaction nudge; `off` also stops the capture of "remember that…". The built-ins set it:
  General and Task runner `model`, Research, Operator and Builder `explicit`, Watcher `off`. A file that still has
  `memory_guidance` is invalid (unknown key). Motivation: the evaluation (`docs/audits/2026-10-17-prompt-profiles-eval.md`)
  showed a model logging its own edits as memory facts when it was told to save.
- **1.0 (2026-10-17)** — First version. A profile bundles the behaviour block of the system prompt with the loop policy
  that must match it, is chosen per conversation, per workspace and per automation, and is rebuilt into the system
  message on every turn. Plan: `docs/PLANS/cross-cutting/prompt-profiles.md`.

## 1. Intent

One system serves any input — a question, a pasted document, a file, a look-up, a reminder, a scheduled task — without
leaning in one direction. How it behaves comes from a **profile**, not from rules fixed in code. The default for chats
is a generic, conversational assistant; specialised behaviour is opt-in. The code keeps only a floor of rules no profile
or file can remove.

## 2. Functional requirements

### 2.1 What a profile is

A profile is a markdown file: YAML front matter, then the instructions (the behaviour block).

| Field | Meaning |
|---|---|
| `name`, `summary` | Shown in pickers. `name` is required. |
| `answer_first` | A plain text first turn is a complete answer: no prior tool result is needed (`checkTaskCompletion`), the no-tool output cap (`enforceRelaxedCap`) does not apply, and the "call a tool" nudges are replaced by ones that allow an answer. |
| `may_ask` | The turn may end with one clarifying question. Unattended profiles leave it false. (Behaviour comes from the instructions; the flag is for pickers and tooling.) |
| `max_steps` | Replaces the model's step budget when greater than 0 (0–200). |
| `tools` | Tool categories exposed (`models.ToolCategories`); empty exposes all. The `system`, `security` and `automation` categories are never hidden. |
| `memory` | `on`/`off`: the profile's default for hot memory; absent inherits the global default. |
| `memory_save` | Who may save to memory in a run. `model` (default when left out): the write tools are offered and, in the operator's own chats, the save guidance and reminder are sent (SPEC-004 §4.1). `explicit`: `memory_update` and `memory_forget` are not offered, no guidance or flush is sent, and code saves what the user explicitly asks to remember. `off`: nothing is saved, not even an explicit request. Reading memory (`memory`, `memory_search`) is not affected. |
| `reminders` | The reminder tools are exposed (SPEC-011). |
| `delegates` | Helper profile ids this profile may delegate to; the `delegate_task` tool is offered only when it is not empty (and a cloud sub-agent model is set). A helper's own `delegates` are never used: delegation is one level deep. |

Boolean fields left out are false. Unknown keys, an unknown tool category, `reminders: true` without the reminders
category in a non-empty `tools`, `delegation` named in `tools`, an invalid or self-referencing `delegates` id, a
missing name or empty instructions make the file invalid.

### 2.2 Catalog

Built-in profiles are embedded files (`prompts/profiles/*.md`, CONSTITUTION II.13): **General** (default for chats),
**Research**, **Operator**, **Builder**, **Task runner** (default for automations), **Watcher** (the heartbeat's) and
**Web researcher** (Research's helper; SPEC-001 §15).
User profiles are `*.md` files in `{data root}/profiles/` (`PathResolver.ProfilesDir`), never in a workspace, so an agent
with file tools cannot rewrite the instructions it runs under. The file name (lowercase letters, digits, dashes, at most
40) is the id; a user file with a built-in's id replaces it. Files are read on demand; an invalid, oversized (64 KB) or
surplus (beyond 50) file is skipped with a warning and never stops a run.

### 2.3 Resolution

The profile of a chat turn is the first existing of: the one the message asked for (which the conversation then keeps,
stored on the session), the conversation's, the workspace's `assistant_profile`, the default for chats. An automation's is
its `profile`, else the default for automations; the heartbeat's is always Watcher. An id that no longer exists is
skipped. Explicit settings beat the profile beneath them: hot memory is the workspace's `assistant_memory` /
automation's `memory_mode`, else the profile's `memory`, else the global default; an automation's `allowed_tools`
narrows the profile's tool set further (a profile can hide tools, never add one the list lacks).

### 2.4 System prompt

Layers, in order: the profile's instructions; the code-owned core rules (`prompts.CoreRules`: tool use, no UI
imitation, loop protection, finalization) and boundaries (`FileSystemRules`, `InstructionBoundaryRule`); the workspace
notes (`AGENTS.md`, comments removed) under `WORKSPACE NOTES:`. A chat wraps this with its conversation id, context
version and timezone (the conversation's own, else the request's). The system message is **rebuilt every turn** from the
current profile and notes and replaces the stored head, so a profile switch or an edit of a profile file applies on the
next turn and existing conversations get fixes (notes are cached for up to 30 minutes); it contains no clock
(the local time rides on each turn's user message, SPEC-011 §1.4), so it is identical turn after turn and the model
server's cache holds. `AGENTS.md` seeded by older versions is recognised (`prompts.IsLegacyDefaultNotes`) and read as no
notes; a file the operator changed is kept.

### 2.5 Loop policy

The profile's flags are read by the loop (SPEC-001): answer-first completion and cap, the nudge wording
(`AnswerFirst*Nag`), memory guidance, and the tool schema. An agent built with no profile runs as `profile.Unprofiled()`
(tool-first, guidance and reminders on, every tool), the behaviour before profiles.

## 3. Data model

`models.WorkspaceConfig.AssistantProfile` (`assistant_profile`), `models.Automation.Profile` (`profile`),
`models.AssistantSession.Profile` (`profile`), `AssistantMessage.profile` on the request. All are optional ids.

## 4. API

- `GET /admin/api/profiles` → every profile (`id`, `name`, `summary`, `source`, the flags, `default_for`), sorted by id;
  the instructions are not included.
- `GET /admin/api/profiles/{id}` → one profile with its `instructions`, `delegating_to` (the profiles that list it as a
  helper) and `replaces_builtin` (a user file that has a built-in's id). `404` for an unknown id.
- `PUT /admin/api/profiles/{id}` → create or replace one of the user's own profiles from `{name, summary,
  instructions, answer_first, may_ask, max_steps, tools, memory, memory_save, reminders, delegates}`. `400
  {"error", "field"}` when the file would be invalid (`field` names the setting), `409` for a built-in's id with no
  user file, `200` with the saved profile otherwise. At most 50 files of 64 KB.
- `DELETE /admin/api/profiles/{id}` → delete the user's own file: `{restored_builtin, delegating_to}`. `409` for a
  built-in, `404` for an unknown id.
- `GET /admin/api/profile-schema` → the settings of a profile in drawing order (`key`, `label`, `kind`, `help`,
  `required`, `max`, `options`).
- `POST /admin/api/conversation/message` accepts `profile`; an unknown id is `400` naming the known ones. It also
  accepts `use_helpers: true`: the message asks the assistant to hand its independent parts to helpers (SPEC-001 §15).
  It is ignored, never an error, when the run's profile names no `delegates` or helpers are not set up.
- `GET /admin/api/helpers` → `{available, model, problem}`: `available` is true when a cloud helper model is set up (the
  sub-runner's `Enabled()`), `model` is the Helper model Settings names (`agent.subagent_model`), `""` when none, even
  when it is not usable, so the page can say why.
- The workspace config `PUT` and automation create/update validate `assistant_profile` / `profile` the same way.

## 5. Error handling

An unknown profile id at the API boundary is `400`. A profile that disappears later (a deleted file) is skipped and the
next level applies. A malformed built-in is a build defect: startup panics (`profile.MustCatalog`) and a test guards it.
