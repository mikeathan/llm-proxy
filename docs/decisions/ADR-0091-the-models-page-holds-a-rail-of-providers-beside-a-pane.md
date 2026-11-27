---
id: ADR-0091
title: The Models page holds a rail of providers beside a pane, not a Providers tab of folding sections
status: accepted
date: 2026-11-26
subsystems: [admin-ui]
supersedes:
superseded_by:
---

# ADR-0091: The Models page holds a rail of providers beside a pane, not a Providers tab of folding sections

## Context
ADR-0090 (accepted the same day) put everything about models on one page with four tabs, one of them **Providers**: a folding section per cloud
provider holding its keys, an always-open "Add a key" form, "Remove all keys" and "Remove all models" buttons, and a hidden "Add a model". The
owner called it "an ugly mess" and asked for one view of providers, keys and models, without a fold per provider and with the Add key form behind
a button. Mocks (`docs/PLANS/cross-cutting/models-providers-mocks/`, options A to D) were drawn from the real data; the owner chose A.
Hermes Agent, Cherry Studio, Open WebUI and Jan were looked at: they all go provider, key, models.

## Decision
1. **The Providers tab is gone.** The tabs are **Models · Engine · Routing**. The Models tab is a **rail** (All models, This computer, then each cloud
   provider by name, each with what it holds: "1 key · 2 models", "Not set up") beside a **pane**. All models is ADR-0090's catalogue unchanged;
   This computer holds the local models (Start / Stop) and the files found on disk, with a link to Engine for the folder, binary and arguments; a
   provider shows its keys and, under each key, the models it serves (`provider_config.api_key_name`), with Add a model per key and Edit.
2. **Addresses:** `/models` (All models), `/models/providers` (the provider list: All models on a desktop, a screen of its own on a phone) and
   `/models/providers/:provider` (`local` is This computer), under the same route name so switching provider never asks to discard unsaved
   configuration edits. Old `/settings/<provider>` links land on that provider.
3. **Phone:** All models opens first with one **Providers** row; the row opens the list, a provider opens its own screen with a back link. No
   side-by-side panes, which is why ADR-0090 rejected list and detail.
4. **Keys:** Add key is a button that opens the form (closed until pressed); the Base URL field suggests the provider's own manifest endpoint, never another provider's; Remove all
   models and Remove all keys are in a ⋯ menu on the provider's header and still ask first.
5. Constitution III.8 needs no change: it already puts every provider's keys and settings on the Models page.

## Consequences
- ADR-0090 decision 1's list of tabs and its description of Providers are replaced by this; the rest of ADR-0090 stands.
- `MODELS_TABS` is `engine`, `routing`; the `providers` address segment is `MODELS_PROVIDERS_SEGMENT`; `toModelsProvider(provider?)` builds the links.
- Files found on disk moved from Engine to This computer, next to the models they become.
- Gemini's Vertex AI fields sit in its pane and edit the shared configuration draft; the page's Save row appears there.

## Alternatives considered
- **B, provider cards with a side panel:** smallest change, but providers, keys and models stay on a different tab from the models.
- **C, one page grouped by provider:** long once many providers hold many models.
- **D, a tab per provider:** the strip overflows with many providers and truncates names on a phone.
- **A phone dropdown for the rail:** hides what is set up; a Providers row and screen shows it.

## Links
- Plan and mocks: `docs/PLANS/cross-cutting/models-providers-options.md`; SPEC-003 2.79; amends ADR-0090.
