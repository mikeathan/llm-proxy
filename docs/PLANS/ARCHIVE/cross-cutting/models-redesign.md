---
status: complete
date: 2026-11-26
last_reviewed: 2026-11-26
related_specs: [SPEC-003]
constitution_references: []
related_plans: []
evidence: Screens of v0.141.0 rendered with vertex's /admin/api/state (12 models, Qwen3.6 35B A3B active) on 2026-11-26; mocks in models-redesign-mocks/.
---

# Models: one place for everything about models, set up like a stack

**Status:** complete (built in v0.142.0; ADR-0090, SPEC-003 2.77, Constitution III.8 amended). The owner asked for a redesign ("messy, noisy, not clean"), picked option A of the first mocks (a row opens a side
panel), liked option C's cards, asked to rethink the whole Models area, and asked for the System stats panel to name the running model.
Open [`models-redesign-mocks/index.html`](models-redesign-mocks/index.html) (`?theme=light` for paper); `v2-1.png` to `v2-7.png` and
`system-stats.png` are dark renders; the first options are kept in `v1.html`.

## What is wrong today (v0.141.0)

- **Model settings live in four places:** the sidebar's Models page (`views/ModelsView.vue`: status, Start/Stop, a "Settings" link),
  Settings › Models (`SettingsView.vue` sub-tabs per provider with keys, `ProviderModelsCard.vue` Add/Edit/Remove, "Discovered on disk"),
  Settings › General (`GlobalSettings.vue`: primary and fallback model, model folder, llama-server, bind address, default arguments,
  environment, idle unload, GPU reading, run lanes, helper model) and Dashboard › LLM and host. SPEC-003 describes both model screens
  (`discovery-panel.md`); no ADR gives a reason for the split.
- **The edit form is a wall** (`ModelTuningFields.vue`): 20 inputs, a help sentence under each, server-set numbers drawn as greyed inputs.
- **The running model** is only an "Active" tag; the System stats panel (`layout/HostStats.vue`) does not name it.

## The design (v2)

- **One Models page, four tabs:** **Models** (cards or a list, a switch remembered per device, always cards on a phone; filter All / Local /
  Cloud; the running model first with its live numbers and Stop; Edit opens a side panel, a full-screen sheet on a phone) · **Providers**
  (one card per provider with its masked keys, Find models, Add key; a provider not set up is one quiet card) · **Engine** (local
  llama.cpp: model folder, binary, bind address, default arguments, environment, idle unload, GPU reading, and the files found on disk
  that are not added) · **Routing** (primary, fallback, helper model, run lanes, chat first; requests from other apps folded).
- **The side panel:** Basics · How it runs (arguments as chips) · The agent (steps, temperature, loop, prefill); tool format, time limits and
  safety timeouts folded, saying "all default" until one is set. Numbers the server sets are one sentence, never inputs.
- **Reading stays on Dashboard › LLM and host** (the tab exists): running model with Switch / Stop, calls and tok/s, VRAM, today by model
  (ADR-0085's model stats). The same split as a stack page and its dashboard tab (ADR-0089).
- **Settings** loses its Models tab and the model fields of General.
- **System stats panel:** a "Running model" section on top from `/state.active` (name, port, ready, uptime, last call); "No local model
  running" otherwise.

## Models as a stack?

Not as code: a stack is a plug-in that uses the core through ports (ADR-0032, Constitution IV.6), and models are what the core serves
through the `model` port every stack calls (`stackport.Model`); a stack can be switched off, models cannot. As a screen, yes: v2 gives
Models the stack shape (setup on its page, reading on its dashboard tab).

## Decisions (answered 2026-11-26)

- **D1** yes: the four tabs, Settings › Models and the model fields of General leave Settings; and III.8 amended rather than kept (asked).
- **D2** the recommendation: cards by default, List remembered per device, cards always on a phone.
- **D3** yes: the System stats "Running model" section.
- **D4** in that order, all on one branch (`models-fix-and-cleanups`).

## Open question found while mocking

A local model's output cap follows the training context, not the serving one: Qwen3.6 serves 16,384 tokens (`--ctx-size 16384`) but its
max tokens is 87,381 (262,144 / 3). Check `orchestrator.ApplyMetadataDefaults` and the `/slots` read before the editor states the numbers.

## Remaining Work

None in this plan. The open question above (a local output cap that follows the training context) is not part of the redesign: it is in
`docs/learnings/admin-ui.md` (2026-11-26) for whoever next changes `orchestrator.ApplyMetadataDefaults`.
