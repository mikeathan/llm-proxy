---
id: ADR-0090
title: Everything about models is set up on one Models page, and what is read about them is on the dashboard
status: accepted
date: 2026-11-26
subsystems: [admin-ui]
supersedes:
superseded_by:
---

# ADR-0090: Everything about models is set up on one Models page, and what is read about them is on the dashboard

## Context
Up to v0.141.0 models were set up in four places: the sidebar's Models page (a list with Start / Stop and a "Settings" link), Settings › Models
(one sub-tab per provider with its keys, Add, Edit, Remove and the files on disk), Settings › General (primary and fallback model, the local
engine, idle unload, GPU reading, run lanes and the helper model) and Settings › System (the model processes). Constitution III.8 put model
and provider management "exclusively in Settings" to end an older split between keys and models. The owner called the result "messy, noisy and
not clean" (2026-11-26), and editing a local model could not even be saved (the browser refused a reasoning budget the server had filled in,
pitfall 78). From mocks (`docs/PLANS/ARCHIVE/cross-cutting/models-redesign-mocks/`) the owner chose option A, a list whose rows open a side panel,
with option C's cards as a view; asked for the whole Models area to be rethought; asked whether Models could be a stack (answered: not as code,
since stacks use models through the core's `model` port and can be switched off, but yes as a screen shape); and chose to amend III.8.

## Decision
1. **One Models page** (`/models`, the sidebar's Models) holds everything about models, in four tabs: **Models** (the page without a tab: every
   model, local and cloud, as cards or a list, remembered per device, always cards on a phone; the running model first; a filter and All / Local
   / Cloud; Start / Stop; Edit and Add open a side panel), **Providers** (one folding section per cloud provider with its keys, Add a model,
   Gemini's Vertex AI fields, Remove all of its models), **Engine** (the local llama.cpp settings, the files found on disk not added yet, the model
   processes) and **Routing** (primary, fallback and helper model, run lanes, chat priority, tool result size, external requests).
2. **The editor is a side panel** over the list (`ContextDrawer`, teleported to the body; a full-screen sheet on a phone): Basics, Limits, The
   agent, and the tool format, time limits and safety timeouts folded with a count of what is set. A local model's limits (output cap, thinking
   budget, history) are one sentence, never inputs, because the server sets them; closing with unsaved edits asks first.
3. **What is read about models is on Dashboard › LLM and host** (the running model, how long it has run, its last call, today's calls by model),
   and in the System stats panel's new "Running model" section; the Models page links to that tab. This is the split a stack has (ADR-0089).
4. **Settings holds nothing about models**: its tabs are General (Agents, Memory & reminders, Storage & logging), Safety, Connectors and
   Appearance. An old `/settings/local-models`, `/settings/local`, `/settings/processes`, `/settings/runs` or provider link lands on the Models tab
   that holds it now. Constitution III.8 is amended to name the Models page.

## Consequences
- The Engine, Routing and Vertex AI fields edit the same configuration draft as Settings (`useConfig`), with one Save row and one leave guard on
  the Models page; leaving it discards an abandoned draft, as Settings does.
- `SettingsTab` no longer includes providers; `PROVIDER_LABELS` is keyed by `ProviderType`; `useProviders` no longer lists Settings sections.
- All provider keys load at once on the Models page, which exposed a race in `useProviderKeys` (the last answer overwrote the others); fixed.
- Messages that named "Settings → Runs" or "Settings → Models" (the helper model, the tool call format hint) now name the Models page.

## Alternatives considered
- **Keep III.8: the four tabs under Settings › Models, the sidebar's Models opening that tab** — offered; the owner chose one page of its own.
- **Option B, list and detail side by side** — two screens on a phone, and a narrow list for long local model names.
- **Option C alone, cards with an edit page of its own** — kept as the Cards view; an edit page leaves the list, and turning llama.cpp flags into
  fields needs a parser that round-trips the owner's flags.
- **Models as a stack in code** — models are what the core's `model` port serves to every stack; a stack can be switched off and models cannot.

## Links
- Plan and mocks: `docs/PLANS/ARCHIVE/cross-cutting/models-redesign.md`; SPEC-003 2.77; Constitution III.8 (amended).
