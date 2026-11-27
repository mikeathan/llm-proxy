---
status: complete
date: 2026-11-26
last_reviewed: 2026-11-27
related_specs: [SPEC-003]
constitution_references: [III.8]
related_plans: [../ARCHIVE/cross-cutting/models-redesign.md]
evidence: Screens of the Providers tab on 2026-11-26 (OpenRouter key dev ••53f5, NVIDIA NIM key nvidia-dev ••5nk9); components/models/ModelProviders.vue, components/settings/ApiKeySettings.vue; mocks in models-providers-mocks/index.html.
---

# Models › Providers: providers, keys and models in one view (options)

**Status:** partial. Option A was chosen and built (v0.146.0, ADR-0091, SPEC-003 2.79); the items under Remaining Work were drawn in the mocks and not built. The owner called the Providers tab "an ugly mess": one folding section per provider, an always-open
"Add a key" form, "Remove all" buttons in the main flow, no sight of which models a key serves. Asked for options (rail, cards, tabs),
whether a provider should show its models, and whether the Models tab becomes redundant. Open
[`models-providers-mocks/index.html`](models-providers-mocks/index.html) (`?theme=light` for paper).

## What is wrong today
- `ApiKeySettings.vue`: the "Add a key" block is always open; "Remove all keys" sits in its header; the base-URL placeholder is the literal
  `https://api.openai.com/v1` for every provider (lines 200 and 237). `ModelProviders.vue`: a Disclosure per provider, "Remove all X models" at the foot.
- Models carry `provider_config.api_key_name` and `modelCountsByKey` already counts them, but the Providers tab shows only the count.

## Options (mocks A–D, plus the shared connect flow)
| | Shape | Models tab | ADR-0090 | Phone |
|---|---|---|---|---|
| **A** | Rail of providers (All models, This computer, each cloud provider) + detail pane: keys, then models grouped by key | becomes the rail's "All models" | amended (one page, no Models/Providers tabs) | rail becomes a full-width picker |
| **B** | Provider cards with key lines and model chips; Manage opens the existing side panel | stays | unchanged | panel is the full-screen sheet |
| **C** | One page grouped by provider, key chips in each header, model cards under it; Providers tab gone | merged into the page | amended | one column; sections collapse |
| **D** | A tab per provider | tab "All" | amended | tab strip overflows (shown) |

Shared by all: **Add key** is a button; the form is closed until pressed. **Connect a provider** is three steps (provider → key with Test →
choose models). Remove-all actions move into a ⋯ menu. Keys keep saving at once; Gemini's Vertex fields and Engine/Routing keep the draft and Save row.

## Answers
- **Should a provider show its models?** Yes, grouped by key (data exists).
- **Is the Models tab redundant?** Only if the layout keeps an "All models" view with the cross-provider filter, the running model and Start/Stop
  (A and C do; B does not need to). Local models have no keys, so "This computer" must absorb Engine's folder, binary and files on disk.

## Research (what other tools do)
- Hermes Agent is CLI-first: `hermes model` chooses provider and model, `hermes setup` the first-time configuration (Nous docs, via search). No GUI to copy; its order
  (provider, key, model) is the flow of the shared mock.
- Cherry Studio: several keys per provider (comma-separated, rotated), a Manage button that lists the provider's models and a + per model, a Test button.
- Open WebUI: Admin Settings › Connections, URL + key, models auto-detected from `/models`, falling back to a typed model id list.
- Jan: Settings › Model Providers, fetches `{base_url}/models` on save, extra keys used on 401/403/429.
Sources were search summaries, not opened pages; labels may have changed.

## Recommendation
**A** if the owner will accept amending ADR-0090 and III.8 (providers, keys and models in one place; scales to many providers); **B** for the
smallest change. D is shown for completeness and breaks on a phone with many providers.

## Remaining work (built: rail + pane, Add key behind a button, per-provider base URL, ⋯ menu, keys grouped with their models, phone Providers row / list / provider screens, rail warning dot, Add key drawer on a phone)
- **Rail warning dot (built):** `useProviderKeys.failed` keeps whether a provider's last key test failed (the open editor clears `testStatus`, not this); `railEntries` marks it `error` with "key test failed" in the summary. A saved key or a passing test clears it. It lives for the page session; a key never tested shows no dot.
- **Add key on a phone (built):** `NewApiKeyForm.vue` is the form; `ApiKeySettings.vue` shows it inline on a desktop and in a teleported `ContextDrawer` (full screen) when `isMobile`. Not a half-height sheet: the repo has no such component and one drawer is the existing overlay.
- **Not built, on purpose: connect a provider as three steps, and Find models.** The path is already two presses: the rail entry's Add key opens the key editor on the new key (Test connection is there), and Add a model in the header then picks that key and fetches its models (`fetchProviderModels`). A wizard would repeat both screens; revisit only if a first-time user stalls there.
- **Owner:** check Models on vertex (OpenRouter and NVIDIA keys, phone width): the red dot after a bad key, the New key drawer.

## Handoff (2026-11-26)
- **Branch and commit state:** everything is uncommitted on `models-unique-name-per-provider`, on top of 33885bad (pushed to origin, not on main). It ships in that PR unless the owner wants a separate branch. `backend/internal/transport/http/frontend_dist/index.html` is rebuilt and goes in the same commit.
- **Checks run** (from `frontend/`): `npm test` (401 files, 3710 tests pass), `npm run build` (lint, palette, contrast, vue-tsc, vite: pass), `npm run test:visual` (454 passed, 12 skipped; baselines refreshed on purpose for models-cards, models-engine and models-list-editor, `models-providers` replaced by models-provider-list, models-provider and models-this-computer), `./scripts/check-agent-harness.sh` (pass). A real-app look at desktop 1400 px and phone 390 px on an isolated binary with a scratch data folder.
- **Not run:** `go test` and the complexity check (no backend file changed); nothing on vertex.
- **Not measured:** the ⋯ button's size on a phone (it uses the same `max-sm:!h-11` classes as the Newsroom menu).
- **First file to open:** `frontend/src/views/ModelsView.vue` (which pane shows), then `components/models/ProviderPane.vue`.
