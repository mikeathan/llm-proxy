---
status: complete
last_reviewed: 2026-09-28
---

# Frontend Redesign — Retro Theme System, Shell, Tree & Notifications

**Status:** complete — Phase 0 signed off 2026-09-28; Phases 1–4 delivered 2026-09-28; Phases 0–5 delivered (Phase 5 closed out 2026-09-30); Phase 6 (hardening) and Phase 7 (documentation) delivered 2026-09-30. The shipped contract is SPEC-003 v2.0; follow-ups not taken are listed under Remaining Work
**Date:** 2026-09-27
**Decisions locked:** D3 (`localStorage` theme persistence), D9–D11 (navigation — six destinations, persistent collapsible left sidebar, `vue-router`), D12 (kept `AgentIde/` path), D13 (dark base — warm ink `#0c0b0a`), D14 (Hermes as a layout-only reference); **D15–D25 confirmed at Phase 0 sign-off (2026-09-28)** with the V10 amendments to D16/D21, Inter + JetBrains Mono (D17) and Playwright approved (D23) — see [Architecture decisions](#architecture-decisions)
**Related Specs:** SPEC-003 (Discovery Panel — requires amendment), SPEC-007 §V (run scheduler — read-only consumer)
**Constitution refs:** V.2 (spec-first), V.3 (no half-finished work), IV.4 (no dead code — retiring the legacy tab navigation and the transitional palette bridge)
**Skills:** `task-planning`, `frontend-vue-engineer`, `clean-code`, `engineering-practices`, `tdd-guide`, `documentation-stewardship`

---

## Problem

The frontend has grown into a dense three-column IDE with no theme layer, a flat
file model that cannot render a tree, and run-state visibility that is confined to
whichever page the user happens to be on. Specific measured issues:

1. **No theme system.** `frontend/tailwind.config.js` has an empty `theme.extend`,
   no `darkMode` setting, and no semantic tokens. `styles/theme.css` holds only
   assistant-run tokens. Every component hardcodes Tailwind `gray-*`/`blue-*`, so a
   palette change means touching 44+ component files.
2. **Layout does not degrade.** `AgentIde.vue` is a fixed `lg:w-72` sidebar + main
   pane + `lg:w-72` right rail. Metrics are permanently visible and compete with
   the work area. `Settings.vue` stacks its full category list above the content
   below `lg`. `useResponsiveLayout` only exposes a boolean.
3. **Files cannot be shown as a tree.** `WorkspaceManager.ListFiles`
   (`platform/persistence/workspace.go:268`) uses non-recursive `os.ReadDir` and
   explicitly `continue`s on `entry.IsDir()`. The read/write/delete routes are
   registered as `files/{file}` — a single path segment that cannot match a slash.
   The flat model is load-bearing.
4. **Run visibility is page-local.** `useGlobalRunActivity` and
   `useRunningActivity` already poll authoritative active-run state globally, but
   nothing surfaces a *transition* — a run finishing or failing while the user is
   in Settings is invisible.
5. **Inconsistent primitives.** 44 component files contain a raw `<button>`;
   only 11 use `BaseButton`. Card/pill/label markup is duplicated per feature.
   Number, cost, duration and timestamp formatting is done ad hoc per component.
6. **Thin test coverage.** 153 source files, 23 test files, **6 component tests**.
   A layout overhaul will break several of them, and the new subsystems (theme,
   tree, notifications) have no precedent tests to copy.

## Design language

The design is **original**: no product's marks, palette values, glyphs or
signature motifs are reused. Retro comes from a warm ink-and-cream material, one
persimmon brand accent, monospace chrome, and a 1-bit pixel layer. Every value
below lives in `frontend/src/styles/tokens.css`; the preset tests
(`src/__TESTS__/theme/presets.test.ts`, Phase 0: `check-contrast.cjs`) prove every contrast pair in every preset (ratios are on canvas `#0c0b0a` /
lifted `#151412` / raised `#1a1816` / hover `#211f1c`).

| Role | Token | Dark value | Contrast | Use |
|---|---|---|---|---|
| Canvas / surface | `--canvas`, `--surface` | `#0c0b0a` warm ink | — | page and panels; surfaces are separated by borders, not fills |
| Raised / hover / active | `--surface-raised` … `--surface-active` | `#1a1816` · `#211f1c` · `#292623` | — | chips, hover, selected rows |
| Hairline / strong border | `--border-hairline`, `--border-strong` | `#262421` · `#3a3732` | ≤1.27 | **decorative panel edges only** |
| Control border | `--border-control` | `#78736b` | 4.18 / 3.91 / 3.76 / 3.49 | inputs, buttons, focus (≥3:1, WCAG 1.4.11) |
| Text | `--text-primary` … `--text-faint` | cream `#f2ede3` … `#8c867d` | faint: 5.45 … 4.56 | body text; faint is the lowest tier for metadata (`retro-dark-lifted` raises it to `#969086`) |
| Decorative text | `--text-decorative` | `#646059` | 3.15 … 2.63 | ornament only (section rules) — never text |
| **Brand** | `--accent-brand` | persimmon `#ff6a3d` | 6.91 / 6.47 / 6.22 / 5.78 | eyebrow mark, active nav, focus ring, wordmark, primary-button offset — **never state** |
| Info | `--accent-info` / `--accent-info-text` | cobalt `#6c8cff` / `#8ea6ff` | 6.40 … 5.35 / 8.47 … 7.08 | charts, links, info tags |
| Success / live | `--state-success`, `--state-live` | mint `#2fd4a3` | 10.36 … 8.66 | success, live dot |
| Running / queued | `--state-running`, `--state-queued` | citrine `#e8cf4a` / `#bfac62` | 12.60 … 10.53 / 8.70 … 7.27 | active runs, queue |
| Error | `--state-error` | `#ff4d5e` | 6.06 / 5.68 / 5.46 / 5.07 | failures |

Brand, info, success, running and error each sit on a distinct hue (≈14°, 228°,
162°, 51°, 355°), so no state can be misread as another or as the brand.
`retro-paper` (warm cream, `#f3efe4`) carries its own darker accent set, proven
by the same gate.

**Rules**

1. **Surfaces are separated by 1px borders**, not fills, soft shadows or
   gradients; small radii; no glass. The one shadow allowed is the hard,
   unblurred 2px full-brand offset under the primary button (see Motifs).
2. **Colour is reserved for data and state.** Chrome is ink and cream; the brand
   accent is a marker, never a fill behind text.
3. **Two type families.** Monospace for chrome (navigation, buttons, tabs), micro
   labels (10px uppercase, wide tracking), IDs, paths, costs, timestamps and
   numerals — always `tabular-nums`. Sans for body copy and the big, tight page
   titles (30px, −0.025em).
4. **High-precision figures**: costs to 4–5 decimals, compact token counts
   (`334.1M`), durations `8.403s`.

**Motifs** (named so they are not lost in implementation)

| Motif | Where |
|---|---|
| **Brand mark** — the *split disc*: two half-discs, cream (client side) and persimmon (model side), parted by a thin gap — the proxy is the seam. One solid shape; never dots-in-a-row, square or nested-square cells | the wordmark and favicon; in decorative ink for empty and not-found states |
| **Ruler meter** — continuous fill over a dithered track, tick scale every 10% | continuous quantities (memory, VRAM) |
| **Slot bar** — exactly one block per real slot | run-lane capacity |
| **Dot-matrix sparkline** — history muted, the latest bar in full colour | throughput, load |
| **Numbered sections** — `01 ── HEALTH` via a CSS counter | panel headings |
| **Eyebrow** — small brand square + muted mono label above the title | page headers |
| **Hard offset shadow** — no blur; full brand at `--offset-control` (2px) under primary buttons | primary buttons |
| **Dotted page frame** — only when there is a gutter (container query) | content column |
| **Tags** — borderless tinted band + square dot (hollow when queued); text always present | status |
| **`✱` dashed tag** for non-state emphasis | `✱ UNSAVED` |
| **Block-caret loading** (`loading▌`) + dithered skeleton rows | loading states |

## Goals

- A **theme system with presets + user-supplied custom themes**, dark default and a
  **warm "paper" light** theme that is deliberately not white — covering colour,
  **typography, and motion** tokens.
- The retro palette visible **app-wide from Phase 1**, not only on redesigned screens (D16).
- A **responsive shell** that works at 360px and at 4K, with one primary pane on
  mobile.
- **Real, deep-linkable routes** with preserved state and unsaved-change protection.
- A **workspace file tree** with the backend support it requires.
- **Global run notifications** when the user is not on the relevant page — correct
  under repeated runs, failed polls, and hidden tabs.
- Redesigned: workspace/explorer, settings (global + workspace), the live activity
  stream (→ **Activity**), system metrics (→ **Overview**), automations + edit
  flow, assistant chrome, buttons, logs.
- **No regressions**: existing behaviour preserved, existing suite green,
  characterisation tests written **before** each refactor, new coverage for every
  new subsystem.

## Non-goals

- Backend behaviour changes beyond: the file tree (Phase 3), the SPA history
  fallback required by the router (Phase 2, D18), and — only if V3 finds no run
  identity — one additive read-only field on the lane snapshot (D20).
- Replacing Tailwind or adding a component-library dependency (repo rule: avoid
  unnecessary dependencies).
- User-editable *theme marketplace*/sharing. v1 is preset + local custom themes.
- Rewriting the assistant event pipeline — `useMessageBuilder` / `ChatMessages.vue`
  as the single consumer is a fixed invariant.
- Folder create / rename / move in the tree. v1 renders directories and supports
  nested **file** read / write / delete only.
- A push channel for run lifecycle. Notification latency is bounded by the existing
  poll interval and accepted as such (D20).
- Per-request usage ledger (tokens / latency / cost per call) **unless V1 confirms
  the backend already records it** — see Phase 5, Activity.

---

## Architecture decisions

**D1 — Tokens, not palettes.** Semantic CSS custom properties on `:root`, with
`[data-theme]` overrides. Tailwind `theme.extend.colors` maps names to token
variables (format fixed by D15). Components reference semantic names
(`surface-raised`, `text-muted`, `state-running`) and never a raw hex or
`gray-800`. This is what makes the design cheap to iterate: change tokens, not
components. Token groups: **colour, typography, radius/spacing, motion** (D17).

**D2 — Themes are data, and custom themes are validated, never executed.** A
`ThemeDefinition` declares a required token set plus optional overrides. Built-in
presets and user themes share one shape, so a custom theme is a validated partial
merged over a base preset — no branching per theme. User themes are stored as JSON
under the same namespaced `localStorage` key family as the selection (D3) and merged
through the same validator.

A custom theme reaches the app by **duplicating a preset and editing it**, or by
**importing JSON**; both paths run the identical validator, and a UI is required
(Phase 5 → Settings · Appearance). Only recognised token **names** are accepted,
and each value must match a strict grammar for its token kind — colour
(`#rgb` / `#rrggbb`, `rgb()`, `hsl()`, **opaque only**), or a bare number for
spacing/radius/duration. Font and easing tokens are **not user-editable** in v1
(they select from preset-defined values). Values are **normalised on import** to the
canonical stored form (D15). A rejected value fails the import loudly and is never
written to a CSS custom property.

Contrast policy by source:
- **Presets** — a contrast failure is an error; the preset test fails the build.
- **Custom themes** — a contrast failure is a **blocking warning in the editor**
  listing each failing pair; the user may save only after explicit acknowledgement.
  Grammar and unknown-name failures are always hard rejections.

**D3 — Theme persistence: `localStorage` (decided 2026-09-27).** v1 persists to
`localStorage` under a namespaced key. Three reasons, in order of force:

1. **The no-flash requirement forces a local pre-paint read anyway.** The stored
   theme is applied before Vue mounts (D19 boot script); a synchronous
   `localStorage` read can do that, a network round-trip cannot.
2. **There is no user model to justify sync.** This is a single-operator admin
   panel (`AdminHeader` has no account concept), so backend storage would push a
   cosmetic preference through six files (`models/` → `registry_handlers.go` →
   `admin_handlers.go` → `admin_view.go` → `manager.go` → mocks) for no
   multi-user benefit.
3. **The upgrade path is purely additive.** If cross-device sync is ever wanted,
   the standard shape is "backend is source of truth, `localStorage` is the
   pre-paint cache" — it layers on without reworking the render path.

**Consequence — theme export/import is REQUIRED, not optional.** Custom themes are
*content*, not merely a preference: they are device-local and vanish on a browser
data wipe. The JSON export/import in Phase 5 (Settings · Appearance) is a mandatory
part of the theme feature.

**One persistence primitive.** Theme selection, custom themes, sidebar collapse
(D10) and pinned sessions (Phase 5) all persist through a single
`usePersistedState(key, validate, fallback)` composable: namespaced key, a
`version` field for schema migration, and corrupt/absent data degrading to the
fallback instead of throwing. No feature talks to `localStorage` directly.

**D4 — The tree needs backend work.** This is not a frontend-only change:
`ListFiles` must gain a bounded recursive listing, and the routes must move to the
Go 1.22 wildcard `files/{file...}` so nested paths match. `isUnsafeFileParam`
already permits `subdir/file.md` (`filepath.Clean` equality) and rejects `a/../b`,
but it is **purely syntactic** — it resolves nothing, so it does not stop symlink
escapes. Containment is enforced separately (Phase 3). Nested paths are currently
**untested** and must be covered before the tree ships.

**D5 — Notifications add no new timers, and terminal status comes from the ledger.**
`useGlobalRunActivity` and `useRunningActivity` already poll the authoritative
endpoints as module-level singletons, each with its own `setInterval`. Phase 4
*reuses the existing `useGlobalRunActivity` tick* for any extra read and adds no
third poller.

**Scope:** the always-visible global run pill already covers **ongoing**
visibility. Phase 4 adds **terminal transitions only**: a run ending, or an
automation failing. Starting a run does not notify; it updates the pill.

**Source limits (verified 2026-09-27):** the lane snapshot **cannot distinguish
failure from completion**. `LaneHolder` / `QueuedRun` carry no status field
(`types/assistant.ts:94-119`); `ActiveRunsResponse` is booleans only (`:121`); and
the SSE lifecycle has no failure phase — `LIFECYCLE_PHASES` is `agent_thinking |
still_thinking | session_started | session_progress | session_completed |
completed` (`types/dispatcher.ts:93`). A vanishing holder means the run **ended** —
never that it succeeded. Assistant runs are therefore labelled **"ended"** only.

**Automation failure** comes from the global activity ledger's
`AutomationRun.error`. The ledger (`GET /admin/api/dispatcher/activity`, 100-entry
rolling history, `MaxHistorySize = 100`, `automation/history.go:9`, rows carrying
`output` and `events`) is **never fetched per tick** — only after an observed
transition, coalesced per batch. Adding a global assistant terminal-status source
is out of scope and requires an explicit plan revision. The correctness rules for
the detector are in D20.

**D6 — Mockups before Vue, against one canonical token file.** Phase 0 produces
standalone HTML. **`frontend/src/styles/tokens.css` is the single source of truth**;
the mockups link that file directly by relative path, so mockup and app cannot
drift. Once Phase 1 lands, the dev-only `/design` route (D23) supersedes the static
mockups as the living reference; `docs/design/retro/` is then **frozen** as the
signed-off Phase 0 record and not maintained.

**D7 — Extract on second use, where "use" includes the signed-off mockups.**
Shared primitives are extracted when a second consumer exists, per repo rule — not
speculatively. The Phase 0 mockups count as evidence: a primitive the signed-off
screens show in ≥2 places is extracted **on its first implementation** rather than
inlined and refactored a PR later. `BaseButton` consolidation is a targeted
migration of the 44 raw-`<button>` files, not a rewrite. The expected primitive set
is listed in Phase 5.

**D8 — Reconcile, don't duplicate.** Absorbs `assistant-ui/automation-edit-form-reactivity.md`
(proposed) since the automation edit form is redesigned here. Must not conflict
with `assistant-ui/overhaul-chat-history-layout.md` (active, Phases 4–5 open) —
that plan keeps ownership of chat streaming/mobile/refresh-resilience; this plan
restyles the chrome around it.

**D9 — Navigation destinations (decided 2026-09-27).** Six top-level destinations
replace the current four: **Overview · Workspaces · Automations · Models ·
Settings · Activity**. Today's "Dashboard" is really the model-config page with
metrics attached, so it splits into **Overview** (health + recent activity) and
**Models**; "Agent IDE" becomes **Workspaces**; **Automations** are promoted to
top-level because they are managed as a fleet (`groupedByWorkspace`); **Process
Logs** merges with run history into **Activity**. The destination set is expressed
as route names (D18), not a separate `AppTab` union.

**D10 — Persistent collapsible left sidebar (decided 2026-09-27).** Navigation
moves from the top header bar to a persistent left rail, which also resolves the
brand + run-pill + nav crowding in `AdminHeader.vue`. Expanded (icon + label) ↔
collapsed (icons only); collapse state persisted via `usePersistedState` (D3); the
rail becomes an overlay drawer below `lg`. Collapsed items keep accessible names
(tooltip + `aria-label`).

**D11 — `vue-router` (decided 2026-09-27; dependency addition).** Navigation
moves from `activeTab` ref switching to real routes. Today there is **no router**
(`package.json` runtime deps: `vue`, `marked`, `cronstrue`,
`@tailwindcss/typography`), so there is no deep-linking, back/forward, or
bookmarking, and no way to link a notification to its run. `vue-router` is the
**only new runtime dependency** in this plan.

`App.vue` currently `provide`s `setActiveTab` / `setActiveSettingsTab`, and
`BannerAction.settingsTab` (`types/ui.ts`) deep-links through that injection. Once
routes exist these are a competing navigation system and are **deleted with their
consumers** (IV.4). `BannerAction.settingsTab` is replaced by
`BannerAction.to: RouteLocationRaw`, built with the typed route helpers (D18).

**D12 — `components/AgentIde/` path is kept (decided 2026-09-27).** D9 retires
"Agent IDE" as a *user-facing* term only. `AgentIde/` holds more than workspaces
(`assistant/`, `automation/`, `common/`, `memory/`, `recordings/`, `system/`,
`workspace/`), so renaming it to `workspaces/` would be less accurate, and it
would touch ~20 files while `overhaul-chat-history-layout` still references those
paths. Any rename is a separate mechanical PR after this redesign. The term must
not appear in any label, route, or user-facing copy.

**D13 — Dark base is warm ink `#0c0b0a`, with a lifted variant mocked alongside
(decided 2026-09-27; amended from pure `#000000` in Phase 0, pending
confirmation).** A near-black ink keeps the contrast that makes meters and
micro-labels read, while the warmth sets the material apart. Phase 0 mocks both `retro-dark` (`#0c0b0a`) and
`retro-dark-lifted` (`#151412` content surfaces); adopting the lifted variant later
is a one-file token change. All tokens in the design language are verified
on both bases.

**D14 — Hermes Agent Desktop is a LAYOUT-ONLY reference (decided 2026-09-27).**
Structure only — its serif wordmark, cream palette and brand typography are
rejected. Adopted: chat-as-canvas; a rail holding nav *and* a searchable, pinnable,
time-grouped session list; model + reasoning-effort selectors inside the composer
(**conditional on V6**); a thin status strip rather than a heavy indicator; and an
empty state that states what the agent will do ("explain my plan, and check in
before risky steps"), framing the guardrail approval moment as expected behaviour.
Window-level affordances of a native app do not transfer. Reconciliation with
`overhaul-chat-history-layout.md` is required (D8).

**D15 — Colour tokens are stored as opaque RGB channel triples (proposed).**
Tailwind v3 opacity modifiers (`bg-surface-raised/50`) only work when a colour is
defined as `rgb(var(--x) / <alpha-value>)`, which requires the variable to hold
bare channels (`153 71 233`), not hex. Therefore:
- `tokens.css` stores every colour token as `R G B` channels.
- Tailwind maps each semantic name once, through a single helper
  (`withAlpha('--surface-raised')` → `rgb(var(--surface-raised) / <alpha-value>)`),
  so no mapping is hand-written per colour.
- Users still author hex / `rgb()` / `hsl()`; the validator **normalises** to
  channels on import and export emits hex for readability.
- Colour tokens are **opaque**. Translucency is a usage decision (opacity modifier),
  not a token property — which also means the contrast validator never has to
  composite alpha over an unknown surface.

If V2 finds Tailwind v4, the same contract is expressed through `@theme` and the
helper is unnecessary; the token file is unchanged.

**D16 — Legacy palette bridge: the whole app themes in Phase 1 (proposed).**
Without this, Phase 1 would ship a theme picker while 44+ components still hardcode
`gray-*`, so `retro-paper` would render broken screens until Phase 5 ends — the
half-finished state V.3 forbids. Instead, Phase 1 remaps the Tailwind palette
shades actually in use (`gray`, `blue`, `green`, `red`, `yellow`, `purple`,
`white`, `black` — exact set from the Phase 0 audit) onto semantic token variables
in one module, `frontend/theme/legacyPaletteBridge.ts`, consumed by
`tailwind.config.js`. Every existing screen picks up the retro palette and theme
switching immediately. The bridge is **transitional**: Phase 5 migrates each screen
to semantic class names, the lint ratchet (D21) prevents new palette use, and the
bridge file is **deleted** in the final Phase 5 PR (IV.4). `dark:` palette variants
remain functional through the bridge until then; `darkMode: 'class'` is kept
permanently only for `prose-invert` and third-party content.

**D17 — Typography and motion are tokens (proposed).**
- **Fonts:** one monospace and one sans family, self-hosted as `woff2` under
  `frontend/src/assets/fonts/` (the panel runs on a LAN and must not depend on a
  font CDN). Candidates, chosen by looking in Phase 0: JetBrains Mono / Geist Mono /
  IBM Plex Mono with Inter / Geist / IBM Plex Sans (all OFL). `@font-face` lives in
  `styles/fonts.css`; the primary mono weight is `<link rel="preload">`ed in
  `index.html`; `font-display: swap` with a metric-compatible system fallback stack.
  No font npm package — static assets only.
- **Type scale tokens:** `--font-sans`, `--font-mono`, `--text-micro` (10px),
  `--tracking-micro`, sizes for body / heading / numeral, and a
  `.tabular` utility applying `font-variant-numeric: tabular-nums`.
- **Motion tokens:** `--motion-fast` (≈120ms), `--motion-base` (≈200ms),
  `--motion-slow` (≈320ms), `--ease-standard`, `--ease-emphasised`. Transitions are
  specified per surface: route change (fade), drawer (slide), run pill
  (expand/collapse), list row insert (fade + height), meter fill (grow).
  `prefers-reduced-motion: reduce` sets all durations to `0ms` in one place.
- **Native UI tokens:** each theme sets `color-scheme` (so scrollbars, `<select>`
  popups, date pickers and autofill follow the theme), `::selection`, and scrollbar
  colours. `@tailwindcss/typography` prose variables (`--tw-prose-body`,
  `--tw-prose-headings`, `--tw-prose-code`, `--tw-prose-pre-bg`, …) are mapped to
  tokens so assistant markdown matches the theme.

**D18 — Route table, typed helpers, SPA fallback (proposed).**
- **History mode** with `createWebHistory(import.meta.env.BASE_URL)` — the router
  base comes from Vite's `base`, never hardcoded, so there is one source of truth
  for the mount path.
- **SPA fallback (backend):** a `GET` under the admin mount that is not an API path
  and not a static asset serves `index.html`, so refreshing a deep link does not
  404. Implemented where `app/routes.go` mounts the admin static files; covered by a
  Go handler test (deep path → `index.html`; `/admin/api/*` unaffected; missing
  asset → 404).
- **Route names** are a `const` object in `types/routes.ts`; navigation anywhere in
  the app goes through typed builders in `router/routes.ts`
  (`toWorkspaceFile(ws, path)`, `toAutomation(id)`, `toSettings(section)`,
  `toActivity(filters)`) — no string-concatenated paths in components.

| Path | Name | Notes |
|---|---|---|
| `/` | — | redirect → `/overview` |
| `/overview` | `overview` | |
| `/workspaces` | `workspaces` | list |
| `/workspaces/:ws` | `workspace` | redirect → `files` section |
| `/workspaces/:ws/files/:path(.*)*` | `workspace-files` | nested file path in the URL; bookmarkable |
| `/workspaces/:ws/assistant/:conversationId?` | `workspace-assistant` | notification target |
| `/workspaces/:ws/:section(memory\|security\|playbooks\|settings)` | `workspace-section` | workspace settings live here (Settings · workspace) |
| `/automations` | `automations` | list, no workspace required |
| `/automations/new` | `automation-new` | |
| `/automations/recordings` | `automation-recordings` | static segment ranks above `:id` |
| `/automations/:id` | `automation` | detail |
| `/automations/:id/edit` | `automation-edit` | |
| `/models` | `models` | |
| `/activity` | `activity` | filters in query: `kind`, `status`, `workspace`, `q`, `from`, `to` |
| `/settings/:section?` | `settings` | global; `appearance` is a section |
| `/design` | `design` | **dev-only** (D23), excluded from production build |
| `/:pathMatch(.*)*` | `not-found` | rendered **inside the shell** so a bad link is visible, never a silent redirect |

Every route is a lazy chunk. `scrollBehavior` restores `savedPosition` on
back/forward and scrolls to top otherwise.

**D19 — State preservation, unsaved-change guard, pre-paint boot (proposed).**
- **Keep-alive by route meta.** `meta.keepAlive: true` on Workspaces routes (explorer
  expansion, editor buffers, chat scroll and composer draft) and Activity (filters,
  scroll). `App.vue` wraps `<RouterView>` in `<KeepAlive :include>` driven by route
  meta — one rule, no per-component special cases. V4 confirms which views keep
  state across tab switches today; anything that does must keep it after the router.
- **One unsaved-change guard.** `useUnsavedChangesGuard(isDirty: Ref<boolean>)`
  registers `onBeforeRouteLeave` + `beforeunload` with cleanup on unmount, and is
  the only implementation used by `FileEditor`, `AutomationForm`, `Settings`
  sections and `ThemeEditor`.
- **Pre-paint boot script.** `useTheme` persists the **resolved, validated** token
  map (`{ version, themeId, tokens }`), so the pre-paint step only applies it and
  never validates. The step is a small same-origin blocking script
  `frontend/public/theme-boot.js` loaded in `<head>` — not inline — so no CSP change
  or hash is needed regardless of V5. It re-checks each value against a cheap
  grammar regex before `setProperty`, and falls back to the default preset on any
  error. A contract test runs the boot script in jsdom and asserts the resulting
  CSS variables equal `apply.ts` output for the same stored map, so the two cannot
  drift.

**D20 — Notification detector correctness (proposed).**
- **Run identity, not lane identity.** A lane key (`chat:<workspace>`) names a lane,
  not a run; deduping on it would suppress every later run in the same workspace.
  The dedupe key is `laneKey + runIdentity`, where `runIdentity` is the holder's run
  id or start timestamp (V3). If `LaneHolder` carries neither, one additive
  read-only `startedAt` field is added to the lane snapshot (the only lane-snapshot
  backend change in this plan). If that is declined, the fallback is: clear the
  dedupe entry when the lane key reappears, and document the blind spot (a run that
  ends and a new one that starts between two ticks is not observed).
- **Baseline and failed ticks.** `useGlobalRunActivity` exposes tick status
  (`ok | error`) alongside the snapshot. The detector sets its baseline on the
  **first `ok` tick** (no notifications for runs already finished at load) and
  **ignores `error` ticks entirely** — an empty/failed response must never be read
  as "every holder vanished".
- **Ledger race.** On an observed automation end, the ledger is read once and the
  entry matched by automation id + run identity. If no matching entry exists yet,
  it is re-checked on the **next existing tick** (one retry, no new timer); after
  that the run is reported as "ended".
- **Hidden tab.** The global poller pauses while `document.visibilityState` is
  `hidden` and ticks immediately on return. Several endings observed in one tick are
  **coalesced** into a single summary notification ("3 runs ended").
- **Latency.** Detection latency ≤ one poll interval; accepted (see Non-goals).
- **Document title badge.** While unread notifications exist, `document.title` is
  prefixed with the count (`(2) …`), restored when cleared.

**D21 — Palette lint ratchet (proposed).** The "no raw palette class" rule is
enforced by lint, not grep, and tightened per directory:
- **Templates:** `vue/no-restricted-class` with regexes covering bare **and
  prefixed** palette utilities (`dark:`, `hover:`, `focus:`, `group-hover:`, …),
  e.g. `/^(?:[a-z-]+:)*(?:bg|text|border|ring|from|to|via|fill|stroke|divide|outline)-(?:gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|white|black)(?:-\d{2,3})?(?:\/\d+)?$/`.
  Today there are **36 `dark:*` palette utilities**, 25 of them in
  `AgentIde/memory/MemoryPanel.vue` (16) and `MemoryDetail.vue` (9).
- **Script / TS:** `no-restricted-syntax` on string literals and template literals
  matching the same pattern, catching class maps in `utils/` and computed `:class`
  bindings built in script.
- **Ratchet:** `warn` globally from Phase 1; `error` in an `overrides` list of
  migrated directories that grows with each Phase 5 PR; `error` globally and the
  overrides list deleted in the final Phase 5 PR (together with the D16 bridge).
- V7 confirms `eslint-plugin-vue` is present and supports the rule as configured.

**D22 — Characterisation tests precede refactors (proposed).** Tests for an
existing module are written **at the start of the phase that changes it**, against
current behaviour, then updated deliberately as behaviour changes (`tdd-guide`).
The former "backfill after the fact" register is distributed into each phase's
**Characterisation first** list; Phase 6 is hardening only.

**D23 — Living design route + visual regression (proposed; dev dependency needs approval).**
- From Phase 1, a dev-only `/design` route (guarded by `import.meta.env.DEV`,
  excluded from the production bundle) renders every primitive and state — loading,
  empty, error, disabled, overflow — with fixture data, in each preset. It replaces
  the static mockups as the reference (D6).
- **Visual regression:** Playwright (`@playwright/test`, **dev dependency only**)
  screenshots `/design` and each destination with fixture data at 390px and 1440px
  in `retro-dark` and `retro-paper`. Unit tests cannot see a visual regression; this
  is the net for a redesign. Requires approval under the dependency rule. **If
  declined**, the fallback is a written manual review checklist run against
  `/design` at each Phase 5 PR.

**D24 — Shared formatting and data-display utilities (proposed).** All
user-visible numbers, costs, durations and timestamps go through one module,
`utils/format/`: `formatTokenCount` (compact: `334.1M`), `formatCost` (4–5
significant decimals below $0.01), `formatDuration` (`8.403s`, `3h 7m`),
`formatRelativeTime` / `formatAbsoluteTime` (`Intl`, user's locale and time zone;
relative in lists with the absolute value in a tooltip). No component formats
numbers inline.

**D25 — Product name in one place (proposed).** The name is expected to change,
so it is data, not copy. `frontend/src/config/brand.ts` exports
`PRODUCT_NAME` (display name, today `llm-proxy`) and nothing else hardcodes it:
the header wordmark, `document.title` (including the unread badge, `(2) Activity ·
${PRODUCT_NAME}`), aria-labels (`${PRODUCT_NAME} home`), empty/error copy and the
`/design` route all import it. `index.html`'s `<title>` is filled from the same
module at build time by a small `transformIndexHtml` hook in `vite.config`, so
there is no second copy. A unit test scans `frontend/src` and `index.html` for
the literal name outside `brand.ts` and fails on any hit. The favicon and
wordmark SVG come from the brand-mark component, not a separate asset.
Out of scope: backend binary, module path, config directory (`~/.config/…`) and
storage keys — renaming those is a migration, not a copy change. The Phase 0
mockups follow the same rule (`docs/design/retro/harness/brand.js`).

### Accessibility gate (applies to every phase)

- Both themes and every state colour meet **≥4.5:1** for body text and **≥3:1**
  for large text and for **UI component boundaries needed to identify a control**
  (inputs, buttons, focus indicators — `--border-control`), against their actual
  surface, on both dark bases.
- Decorative-only tokens: `--border-hairline` / `--border-strong` and
  `--text-decorative` (≤3.15:1) — ornament only, never helper text or metadata.
- Meters and slot bars: `role="meter"` with `aria-valuenow` / `aria-valuemin`
  / `aria-valuemax` and an accessible label; the percentage is also rendered as
  text. State is never conveyed by colour alone (pills carry text).
- Every interactive element keeps a visible focus state; icon-only controls get
  accessible names; `prefers-reduced-motion` zeroes motion tokens (D17).

### Performance & leak budget

- No new `setInterval`/listener without a named owner and a cleanup path.
- Tree: bounded payload, reveal-on-expand, `shallowRef` + `markRaw` for large node
  arrays; no per-node deep reactivity.
- Notifications: capped ring buffer with dedupe by run identity (D20); no unbounded growth.
- Client-side metric history (if V8 requires it): capped ring buffer inside the
  existing `useMetrics` singleton, no new timer.
- `useAssistant` remains a single module-level singleton — no second instance.
- Keep-alive is opt-in by route meta (D19); no global `<KeepAlive>`.

### Open verifications (resolved by code reading in Phase 0, not user decisions)

| # | Question | Where | Affects |
|---|---|---|---|
| V1 | Does the backend record per-request usage (tokens in/out, latency, model, cost, trace id)? | proxy request path, `models/`, admin API | Phase 5 Activity · Requests |
| V2 | Tailwind major version (v3 assumed from `tailwind.config.js`) | `frontend/package.json` | D15 |
| V3 | Does `LaneHolder` carry a run id or start timestamp? | `types/assistant.ts:94-119`, lane snapshot handler | D20 |
| V4 | Which views keep state across tab switches today (`v-show` / `KeepAlive`)? | `App.vue`, `AgentIde.vue` | D19 |
| V5 | Does the server send a CSP? | admin static handler | D19 (external boot script is CSP-safe either way; confirm) |
| V6 | Does the assistant API accept per-turn model and reasoning-effort? | assistant send handler, `useAssistant` | Phase 5 composer selectors |
| V7 | `eslint-plugin-vue` present and version supports `vue/no-restricted-class` with regex? | `frontend/package.json`, ESLint config | D21 |
| V8 | Does `useMetrics` return history or only current values? | `composables/…/useMetrics.ts`, metrics endpoint | Overview sparklines |
| V9 | Go version in `go.mod` (≥1.24 enables `os.Root`) | `backend/go.mod` | Phase 3 containment |
| V10 | Exact set of Tailwind palette shades in use, per file | `grep` over `frontend/src` | D16 bridge mapping, D21 |

### Phase 0 findings (2026-09-27)

- **V1 — No per-request usage ledger.** `icu_ledger` (`platform/ledger/store.go:34`)
  is written only by `BudgetManager` pre-flight (`orchestrator/budget_manager.go:132`)
  with *estimates* (`request_tokens = ContextChars/2`, `response_tokens = MaxTokens`),
  no latency, cost or trace id, and no read endpoint (only the ICU balance).
  → **Activity · Requests is a follow-up**, not in this plan.
- **V2 — Tailwind `3.4.19`.** D15's `withAlpha()` helper stands.
- **V3 — Run identity exists.** `runlane.Holder.Since` (`core/runlane/types.go:124`,
  JSON `since`) is the run start time. Dedupe key = `key + since`.
  → **The D20 additive `startedAt` field is not needed**; no lane-snapshot backend change.
- **V4 — Nothing keeps state across top-level tabs today.** `App.vue:57-60` switches
  views with `v-if`, so leaving Agent IDE / Settings unmounts them. Within a view,
  `AgentIde.vue:407,490,574` (mobile panels) and `Settings.vue:226-369` (categories)
  use `v-show`. D19's keep-alive is therefore an improvement, not a preservation duty.
- **V5 — No CSP header** anywhere in `backend/`. The external boot script (D19)
  needs no CSP work. Separately, `AdminPageHandler` (`handlers/admin_handlers.go:362`)
  **already** serves `index.html` for any missing path under `/admin/` — including
  missing *assets*, which D18 requires to 404. Phase 2 narrows that fallback
  (asset-like paths → 404) rather than adding one.
- **V6 — No per-turn model or effort.** The send payload (`handlers/assistant_handlers.go:26-31`)
  carries `workspace_id, conversation_id, context_version, message, timezone,
  exclude_tools`. Reasoning effort is per-model config (`assistant/reasoning/`).
  → **Composer model/effort selectors are a follow-up** (backend change), not in this plan.
- **V7 — `eslint-plugin-vue` `9.33.0` ships `vue/no-restricted-class`** — but see V10:
  it cannot see `@apply`.
- **V8 — `useMetrics` returns a current snapshot only** (`SystemMetrics`,
  `composables/system/useMetrics.ts`). Overview sparklines need the capped client
  ring buffer inside the existing singleton (already in the performance budget).
- **V9 — Go `1.26.2`**: `os.Root` is available for Phase 3 containment.
- **V10 — Palette audit** (`docs/design/retro/palette-audit.md`, since removed — see the Phase 7 note).**
  1,530 palette utilities in 65 files; **84% are in `@apply`**. Three consequences:
  1. **D16 amendment:** the bridge maps **per utility** (`backgroundColor` /
     `borderColor` / `textColor` / …), because each gray shade is used as bg,
     border and text at once (gray-600: 22 / 21 / 24).
  2. **D16 open decision:** 29 rules put `text-white` on a solid accent fill, which
     no palette mapping can make pass in `retro-paper`. Recommended: convert them to
     a `--text-on-accent` token in the Phase 1 bridge PR.
  3. **D21 amendment:** `vue/no-restricted-class` would see ≈14% of the debt.
     Recommended: a dependency-free `frontend/scripts/check-palette.mjs` scanning
     templates, `@apply` and TS literals, with a shrinking allowlist, in `npm run lint`.
  Also: all 36 `dark:` utilities are in `MemoryPanel.vue` (23) and `MemoryDetail.vue`
  (13); `hover:bg-gray-750` (`InfrastructurePanel.vue:58`) is a no-op today.

**Primitive inventory (D7)** — measured by the harness (Reference → Primitive
inventory), counting ready + loading/empty/error renders of the 11 product screens.
Extracted on first implementation (≥2 screens): button, micro label, panel, status tag, icon, section heading (`01 ──`), empty / error / loading state,
page header, select, data table (with stacked mobile layout), segmented control,
field, search input, text input, toggle, ID chip. Inline until a second consumer:
log viewer, meter, slot bar, sparkline, stat tile, toast, tree.

---

## Phase 0 — Design lock, verifications, mockup harness

**Deliverables**
1. **Verifications V1–V10** answered in this plan (a short "Findings" note per item),
   with any scope consequence applied before sign-off.
2. **Palette audit (V10):** every palette shade in use mapped to a semantic token —
   the input for the D16 bridge.
3. **Standalone HTML mockups** under `docs/design/retro/` linking
   `frontend/src/styles/tokens.css` (D6), rendering: Overview, Models, Workspaces +
   tree, Workspace assistant, Automation edit, Activity, Settings · Appearance,
   Settings · global, Run pill, notification toast, not-found — at 1440px and
   390px, sidebar **expanded and collapsed** (D9/D10), in `retro-dark`,
   `retro-dark-lifted` (D13) and `retro-paper`.
4. Every screen shows **loading, empty, error** states, plus the Activity table's
   **stacked mobile layout**.
5. **Design choices by looking:** font pair (D17), own wordmark (**decided 2026-09-28: split disc**), light-theme accent
   set, motion samples (a page with the specified transitions).
6. **Primitive inventory:** which primitives appear in ≥2 signed-off screens (D7).

**Layout reference (D14):** Hermes Agent Desktop — structure only.

**Why first:** the design is iterated by looking, so the iteration surface must be
cheap. Mockups are throwaway HTML; Vue is not.

**Acceptance:** all deliverables present; every screen in all three themes and both
widths; changing one token file changes every mockup; derived tokens pass the
accessibility gate on both dark bases; user has signed off; proposed decisions
D15–D25 confirmed or amended.

**Verify:** open `docs/design/retro/index.html` in a browser; no build required.

---

## Phase 1 — Tokens, presets, custom themes, palette bridge

**Status: delivered 2026-09-28.** Implementation notes (deviations from the text
below are deliberate and recorded here):
- **Presets parse `tokens.css`** (`theme/tokenSheet.ts`, via Vite `?raw`) instead of
  restating values, so D6's single source of truth holds for presets too. Vitest
  needs `css.include` for that file (see `vitest.config.ts`).
- **Tailwind modules are TypeScript** — `frontend/theme/tailwindTokens.ts` and
  `frontend/theme/legacyPaletteBridge.ts` (not `.cjs`); Tailwind's loader imports
  them, and `tsconfig.node.json` type-checks them.
- **Bridge mapping (D16):** per utility (`colors` / `textColor` / `borderColor` /
  `ringColor` / `placeholderColor`). Accent shades 300–700 are solid; the pale
  (50–200) and dark (800–950) ends are tints — token × 0.08 / 0.12 / 0.2 via
  `calc(<alpha-value> * f)`, so opacity modifiers still compose; accent text of any
  shade → the state text token. `bg-black/N` → `canvas` (recessed wells); the three
  modal backdrops were migrated to semantic `bg-scrim/N`.
- **`--text-on-accent`:** 39 source rules converted (the audit's 29 plus `hover:`
  variants and light-accent text on solid 700 fills); `AppBanner` actions moved
  from 60% to 90% fills — no single text token passes on a mid-alpha tint in both
  themes.
- **D21 as built:** `frontend/scripts/check-palette.mjs` (`npm run lint`) is the global
  ratchet — no file may gain a palette class and counts only go down
  (`palette-allowlist.json`, 1,492 left). `vue/no-restricted-class` runs at
  **`error`** on migrated directories (`src/views/**`) instead of `warn` globally,
  which would have added ~200 standing warnings.
- **`/design`** is mounted from `main.ts` behind `import.meta.env.DEV` until the
  Phase 2 router exists; the dynamic import keeps it out of the production bundle
  (verified).
- **Playwright** (`@playwright/test`, dev only) runs the installed Chrome
  (`channel: 'chrome'`, no browser download): `npm run test:visual` — `/design`
  at 1440/390 × dark/paper, plus no-flash and follow-OS checks. Baselines in
  `frontend/e2e/__screenshots__/`. **Not in CI** (CI changes need approval).
- **D25 delivered here:** `src/config/brand.ts`, `%APP_TITLE%` in `index.html`,
  and a test scanning `src` + `index.html` for the name (backend env var names such
  as `LLM_PROXY_HOME` are out of scope).
- **Seen, deferred to Phase 5:** legacy Tailwind drop shadows (`shadow-lg/xl`)
  read heavy on `retro-paper`; they are not palette classes, so the bridge leaves
  them to the screen redesigns (design language: no shadows).

**Characterisation first:** snapshot the rendered class/colour output of
`RunActivityPill` and `NotificationDot` (the `--color-live` consumers) before the
token rename.

**Files (new):**
- `frontend/src/styles/tokens.css` — semantic variables (channel format, D15),
  typography, radius/spacing, motion, `color-scheme`, prose mapping (D17)
- `frontend/src/styles/fonts.css` + `frontend/src/assets/fonts/*.woff2` (D17)
- `frontend/src/types/theme.ts` — `ThemeDefinition`, `ThemeTokens`, `TokenKind`,
  `StoredTheme` (**single home**; the ESLint `types-must-live-in-src-types` rule
  forbids exporting types from `theme/`, `utils/`, or `composables/`)
- `frontend/src/theme/tokenRegistry.ts` — the one list of token names with their
  kind (colour / length / duration) and user-editability; the validator, the
  editor UI and the Tailwind mapping all derive from it (no parallel lists)
- `frontend/src/theme/presets.ts` — `retro-dark` (`#0c0b0a`, default),
  `retro-dark-lifted` (`#151412`, D13), `retro-paper` (light, own accents)
- `frontend/src/theme/colour.ts` — parse / normalise (hex, `rgb()`, `hsl()` →
  channels), relative luminance, contrast ratio — pure functions
- `frontend/src/theme/validate.ts` — name, grammar, required-token and contrast
  validation; returns a result object (`errors`, `contrastFailures`) — the single
  validator for presets and custom imports (D2)
- `frontend/src/theme/contrastPairs.ts` — the explicit pair list: each text tier on
  each surface tier, `--border-control` on each surface, each state accent (success /
  running / queued / error / info) on the surface it is drawn on
- `frontend/src/theme/apply.ts` — write tokens + `data-theme` + `dark` class to `<html>`
- `frontend/theme/legacyPaletteBridge.ts` — transitional palette → token mapping (D16)
- `frontend/theme/tailwindTokens.ts` — `withAlpha()` + semantic colour map
  generated from the token registry (D15), prose variables (D17)
- `frontend/src/theme/tokenSheet.ts` — reads token declarations from `tokens.css`
- `frontend/scripts/check-palette.mjs` + `palette-allowlist.json` — D21 ratchet
- `frontend/playwright.config.ts`, `frontend/e2e/design.visual.spec.ts` — D23
- `frontend/public/theme-boot.js` — pre-paint apply of the stored resolved map (D19)
- `frontend/src/config/brand.ts` — `PRODUCT_NAME`, the single source of the product name (D25)
- `frontend/src/composables/ui/usePersistedState.ts` — the single persistence primitive (D3)
- `frontend/src/composables/ui/useTheme.ts` — singleton: current, presets, custom,
  resolve + persist; follows `prefers-color-scheme` via a `matchMedia` listener
  (owned, cleaned up) only while the user has not chosen a theme
- `frontend/src/views/DesignView.vue` — dev-only `/design` route (D23); initially
  tokens, type scale, colour pairs with their measured contrast, motion samples

**Files (changed):**
- `frontend/tailwind.config.js` — `darkMode: 'class'`; `theme.extend` colours from
  `tailwindTokens.ts`; palette overridden by `legacyPaletteBridge.ts`; font
  families and transition durations/easings from tokens
- `frontend/index.html` — `<script src="/theme-boot.js">` in `<head>` before any
  stylesheet; mono font preload; `<title>` filled from `brand.ts` (D25)
- `frontend/vite.config.ts` — `transformIndexHtml` hook injecting `PRODUCT_NAME` (D25)
- `frontend/src/style.css` — import `fonts.css` + `tokens.css`; remove hardcoded body colours
- ESLint config — D21 rules at `warn` globally
- `frontend/src/styles/theme.css` — **absorbed**: `--color-live` and the
  assistant-run tokens move into `tokens.css` as semantic state tokens
  (`--state-live`); the file and its `@import` in `AssistantChat.vue:309` are
  removed. All **12 references across five consumer files** are renamed in this
  phase: `common/NotificationDot.vue` (4), `layout/RunActivityPill.vue` (3),
  `AgentIde/workspace/WorkspaceExplorer.vue` (2),
  `AgentIde/assistant/AssistantActivity.vue` (1),
  `AgentIde/common/MobileTabBar.vue` (2 — file deleted in Phase 2). This fixes a
  **latent bug**: the token's only definition is a `:root` block inside
  `AssistantChat.vue`'s **scoped** style, so it is only registered while the
  assistant is mounted — other consumers silently fall back to `#22c55e`.

**Acceptance**
- Switching theme repaints **every existing screen** (via the bridge) with no reload
  and **no flash**; the stored theme is applied by `theme-boot.js` before Vue mounts.
- Opacity modifiers work on semantic colours (`bg-surface-raised/50`).
- A partial custom theme merges over a base preset and validates; hex / `rgb()` /
  `hsl()` input is normalised to channels and exported as hex.
- An unknown token name, a non-editable token, a translucent colour, or a value that
  is not valid for its kind (`url(...)`, raw CSS) is **rejected at import** and never
  reaches a CSS custom property.
- Presets: a missing token or failing contrast pair **fails the test suite**.
  Custom themes: contrast failures are returned as a list for the editor (D2).
- `prefers-color-scheme` is respected only when the user has not chosen a theme,
  and live-updates when the OS setting changes.
- Native controls, scrollbars and assistant markdown follow the theme (D17).
- Changing `presets.ts` alone restyles the app (no component edits).
- No `--color-live` remains anywhere; assistant-run styling reads `--state-live`.
- `/design` renders in dev and is absent from the production bundle.

**Tests (new)**
- `theme/colour.test.ts` — parse every allowed form; reject alpha and malformed
  input; luminance/contrast against known values (the Design language table).
- `theme/presets.test.ts` — every preset defines every required token; every
  contrast pair passes in every preset.
- `theme/validate.test.ts` — rejects missing token, unknown name, non-editable
  token, non-colour value, translucent colour; accepts valid partial; reports
  contrast failures without throwing for custom themes.
- `theme/tokenRegistry.test.ts` — Tailwind map, validator and editor derive the same name set.
- `theme/apply.test.ts` — writes expected CSS vars + attributes; idempotent.
- `theme/themeBoot.contract.test.ts` — boot script output equals `apply.ts` output
  for the same stored map; corrupt map → default preset; malicious value skipped.
- `composables/ui/usePersistedState.test.ts` — round-trip; version migration;
  corrupt/absent → fallback; namespaced keys.
- `composables/ui/useTheme.test.ts` — preset switch; custom merge; export → import
  round-trip; failed validation never applied; OS scheme listener added only when
  no explicit choice and removed on choice.

**Verify:** `cd frontend && npm test && npm run lint && npm run build`

---

## Phase 2 — App shell, routing & responsive layout

**Status: delivered 2026-09-28.** Implementation notes (deviations from the text
below are deliberate and recorded here):
- **Decided with the user (2026-09-28):** (1) `Dashboard.vue` is split **now**,
  minimally — `/overview` is `views/OverviewView.vue` (`SystemStatus`: active
  model + host/GPU readings, which also replaces the rail's `MetricsPulse` /
  `MetricsMini` / `MetricsExpanded`, deleted as duplicates) and `/models` is the
  rest of `Dashboard.vue`; Phase 5 restyles both. (2) The right rail becomes
  `layout/ContextDrawer.vue` at **every** width, opened by a **Monitor** button;
  its content is `AgentIde/common/MonitorPanel.vue` (assistant sessions + lane
  blocker, run history, dispatcher metrics). Run / Stop moved inline onto the
  automation page (`AgentIde/automation/AutomationRunActions.vue`), not into the
  closed drawer. (3) `AgentIde.vue` is **split and deleted**:
  `views/WorkspacesView.vue` and `views/AutomationsView.vue`; the `AgentIde/`
  folder stays (D12). Other plans that cite `AgentIde.vue` line numbers
  (`automation-edit-form-reactivity`, `overhaul-chat-history-layout`) now map to
  these two views.
- **Header vs rail (follows the signed-off mockup):** brand mark + product name +
  version live at the top of `AppSidebar`; `AdminHeader` is the top strip —
  page title, run pill (so it is visible on every destination, including mobile
  with the drawer closed) and, below `lg`, the drawer toggle + brand mark. The
  version fetch moved to `AdminApiService.fetchVersion` (no `fetch` in components).
- **Workspace sections are `memory | security | playbooks`.** `security` renders
  the existing guardrail page (`WorkspaceSettings.vue`); `settings` joins the
  pattern when Phase 5 builds that page — no empty route. `playbooks` shows the
  template library dialog; closing it returns to the file that was open.
- **Unsaved-change guard ownership:** the file buffer (`useFileEditor`, now with
  `isDirty`) is owned by `WorkspacesView`, not `FileEditor`, because it outlives
  section and destination changes (keep-alive). The guard takes a `discards(to)`
  predicate and prompts only when a navigation would replace the buffer (another
  file, another workspace), on the editor's close button (`confirmLeave`), and on
  tab close (`beforeunload`). Leaving to another destination keeps the buffer.
- **Keep-alive:** `App.vue` builds the `<KeepAlive :include>` list from
  `meta.keepAlive` (the resolved route component's name), bounded by the number
  of keep-alive destinations. Kept-alive views read `useDestinationRoute(dest)` —
  a route snapshot that only follows their own destination — so a background view
  does not re-render for another page and unmount the chat / editor it preserves.
  Their 10 s refresh uses `usePolling`, which pauses while in the background and
  refreshes once on return.
- **Selection in the URL:** workspace, file path, section and conversation are
  route params (`workspaceLocation()` in `router/routes.ts` parses them);
  `useViewManager` keeps only the run opened from a history list and the memory
  entry. `AssistantChat` takes `conversationId` and reports session changes via
  `update:conversationId` (URL replaced, not pushed). Automations derive the
  selection from `:id`; an id that no longer exists shows an in-page "No
  automation …" state. `/workspaces` no longer auto-selects the first workspace:
  it is the addressable all-workspaces list.
- **`AutomationForm`** lost its in-sidebar collapse (dead once the form has its
  own `/automations/new` and `/:id/edit` routes); Cancel emits `cancel` in both modes.
- **Unknown `/settings/:section`** re-resolves to the in-shell not-found page at
  the same URL once provider manifests have loaded (a silent redirect is avoided).
- **SPA fallback (D18)** is `handlers/admin_ui.go` (`serveAdminUI`): missing
  `api/*`, `assets/*` and root-level files (`/admin/favicon.png`) 404; any other
  missing path — including nested ones with an extension such as
  `workspaces/ws/files/notes.md` — serves `index.html`.
- **Breakpoints:** `src/theme/breakpoints.ts` (Tailwind defaults incl. `2xl`)
  feeds Tailwind `screens` via `theme/tailwindTokens.ts` and
  `useResponsiveLayout()` (`breakpoint`, `isMobile` = below `lg`, one
  `matchMedia` listener per breakpoint).
- **Found and fixed:** `InfoTooltip` hidden hints (`white-space: nowrap`,
  `visibility: hidden`) still took layout space and made Settings scroll
  sideways at 360px; hidden hints are now `display: none` and wrap at 280px.
- **Found, not fixed (pre-existing, out of scope):** `GlobalSettings.vue` uses
  `icon="power"`, but `assets/svg/power.svg` has never existed, so Settings logs
  a failed dynamic import on every visit.
- **Verified against the real binary** (isolated `--data`), 1440px and 360px:
  every D18 route on hard load, back/forward, keep-alive buffer, guard
  prompt/Stay, collapse persistence, drawer and monitor, no horizontal overflow on
  any destination at 360px, destination reachable in 2 taps on mobile. The
  `/design` visual baselines are unchanged. Destination screenshots with fixture
  data (D23) arrive with each Phase 5 screen.

**Characterisation first:** `composables/ui/useViewManager.test.ts` and
`composables/ui/useResponsiveLayout.test.ts` against **current** behaviour (both
missing today); `RightPane`, `MetricsExpanded` render tests (their content moves).

**Files:** `App.vue`, new `frontend/src/router/index.ts`, new
`frontend/src/router/routes.ts` (typed builders), new `frontend/src/types/routes.ts`,
new `frontend/src/components/layout/AppSidebar.vue`, new
`frontend/src/components/layout/ContextDrawer.vue`, new
`frontend/src/views/NotFoundView.vue`, new
`frontend/src/composables/ui/useUnsavedChangesGuard.ts`, `layout/AdminHeader.vue`,
`AgentIde/AgentIde.vue`, `composables/ui/useViewManager.ts`,
`composables/ui/useResponsiveLayout.ts`, `types/app.ts`, `types/ui.ts`,
`package.json`, `vite.config.ts` (`base`), and — backend — the admin static
handler mounted in `app/routes.go` (SPA fallback, D18).

**Deleted in this phase:** `components/AgentIde/common/MobileTabBar.vue` (replaced
by the sidebar drawer) and `components/AgentIde/common/SidebarNavTabs.vue`
(replaced by workspace sections) — with their consumers, not backfilled with tests.
`AppTab` is deleted; destinations are route names.

**Work**
- **Routes per D18**, lazy per route; `BannerAction.settingsTab` →
  `BannerAction.to` via typed builders; delete `provide('setActiveTab')` /
  `provide('setActiveSettingsTab')` and all consumers (D11).
- **SPA fallback** in the backend (D18) with its Go test.
- **State preservation** via route-meta keep-alive; **unsaved-change guard**
  composable (D19), wired into `FileEditor` in this phase (the others adopt it as
  they are redesigned in Phase 5).
- **Persistent collapsible sidebar** (D10): expanded ↔ collapsed, overlay drawer
  below `lg`, persisted via `usePersistedState`, accessible names when collapsed,
  active destination derived from the matched route (no separate active-state ref).
- **Decouple automation state from workspace selection.** `selectedAutomation`,
  `editAutomation`, and `canOpenAssistant` gate on `selectedWorkspace` in
  `useViewManager`; the Automations routes must not require an active workspace.
  Workspace / automation / conversation selection move into route params;
  `useViewManager` keeps only view state that is genuinely not addressable.
- **Responsive shell:** single primary pane below `lg`; `ContextDrawer` replaces
  the permanent right rail for detail/monitor content.
- Metrics leave the permanent rail; the run pill stays global. `AdminHeader`
  reduces to brand (own wordmark from the brand mark + `PRODUCT_NAME`, D25; version badge) + run pill.
- `useResponsiveLayout` exposes named breakpoints (`sm | md | lg | xl`) read from
  the same values as the Tailwind `screens` config — one source of truth.
- Route transitions use motion tokens (D17).

**Acceptance:** no horizontal overflow at 360px; every destination reachable in ≤2
taps on mobile; keyboard-navigable nav with visible focus; collapsed rail shows
icons only **with accessible names**; a deep link opens the intended destination
**including after a hard refresh**; back/forward moves between destinations and
restores scroll; switching destination and back preserves workspace state that V4
found preserved today; leaving a dirty file editor prompts; an unknown path renders
the in-shell not-found view; run pill visible on all destinations; **no
`setActiveTab` / `setActiveSettingsTab` / `AppTab` remains**; automations render
without a selected workspace.

**Tests**
- `components/layout/AppSidebar.component.test.ts` — expand/collapse, persistence,
  icon-only accessible names, active destination from route, keyboard nav.
- `router/index.test.ts` — every route in the D18 table resolves; static
  `recordings` outranks `:id`; unknown path → `not-found`; lazy components;
  `scrollBehavior`.
- `router/routes.test.ts` — typed builders produce the documented paths, including
  nested file paths and Activity query filters.
- `composables/ui/useUnsavedChangesGuard.test.ts` — blocks leave when dirty, allows
  when clean, `beforeunload` registered and removed.
- `composables/ui/useViewManager.test.ts` / `useResponsiveLayout.test.ts` — updated
  from the characterisation baseline; listener cleanup on unmount.
- `components/layout/AdminHeader.component.test.ts` — brand + run pill render.
- **Update** `components/layout/RunActivityPill.component.test.ts` (DOM moves).
- **Backend** — SPA fallback handler test (deep path → `index.html`, API untouched,
  missing asset → 404).

**Verify:** `cd frontend && npm test && npm run lint && npm run build` ·
`cd backend && go build ./... && go test ./...`

---

## Phase 3 — Workspace file tree (backend + frontend)

**Status: delivered 2026-09-28.** Implementation notes (deviations from the text
below are deliberate and recorded here):
- **Containment is `os.Root`** (Go 1.26): `WorkspaceManager.withWorkspaceRoot`
  (`platform/persistence/workspace.go`) is the one helper; read, write, delete and
  the tree listing all go through it — no check-then-use gap, so no residual to
  document. Atomic writes use the new `storage.WriteAtomicInRoot` (same file-class
  permission policy as `WriteAtomic`; temp file named from the target's base name
  beside it, which fixes the nested-write `pattern contains path separator` bug).
- **Tree:** `ListTree` in `platform/persistence/workspace_tree.go`, cap
  `MaxTreeEntries = 5000`, breadth-first. Symlinks are listed as files only when
  they resolve inside the root **to a regular file**; directory links are omitted
  (never followed), as are escaping and broken ones. `models.TreeEntry` /
  `models.WorkspaceTree`.
- **The root-only `GET …/files` listing and `ListFiles` were removed** (IV.4): the
  UI's only consumer now reads `GET …/tree`, and the automation task-file picker
  derives its list from the tree (`useDispatcher().workspaceFiles`, a computed over
  `workspaceTrees`). `fetchWorkspaceFiles` is now `fetchWorkspaceTree`.
- **`isUnsafeFileParam`** also rejects non-local paths (`../x`, `/abs`) via
  `filepath.IsLocal`, so they are a 400 before containment is ever reached.
- **`FileTree.vue`** renders only the rows of expanded folders as a flat list of
  `treeitem`s with `aria-level` / `aria-setsize` / `aria-posinset` and a roving
  tabindex (one DOM row per visible node), rather than recursive components. The
  selected file's folders open automatically (deep links). Row controls come from
  `actions` / `below` slots (the explorer's delete + inline confirm).
- **Route builders do not use `encodeFilePath`**: vue-router already encodes each
  segment of the repeatable `:path(.*)*` param; `utils/workspace/filePath.ts`
  is used by the service calls only (encoding twice would corrupt `%`).
- **Found and fixed:** a file created from the explorer never appeared until a
  reload (the create had no refresh callback, pre-existing); it now refreshes the
  tree and opens the new file.
- **Verified against the real binary:** nested tree incl. empty dir and collapsed
  `node_modules`, escaping symlink absent; `%20`/`%23` paths read and survive a
  refresh; nested write; read/write through an escaping link refused; `..%2F`
  traversal 400; keyboard open; nested task file offered in the automation form;
  no overflow at 360px.

**Characterisation first:** existing `ListFiles` behaviour (root-only, exclusions)
and handler traversal table, so the change in behaviour is explicit in the diff;
`WorkspaceExplorer` and `FileEditor` / `useFileEditor` save paths (**missing today**
— data-loss risk).

**Backend**
- `platform/persistence/workspace.go` — add **one recursive** listing (chosen over
  per-directory lazy: no round-trip per expand, and the frontend tree is a pure
  function of one payload). `filepath.WalkDir`, which does not follow symlinked
  directories.
- **Endpoint contract (settled):** `GET …/workspaces/{ws}/tree` returns
  ```json
  { "entries": [{ "path": "docs/plan.md", "type": "file" },
                { "path": "docs", "type": "dir" }],
    "truncated": false }
  ```
  Workspace-root-relative slash paths, sorted. **Directories are included**, so
  empty directories render. Symlinks are listed as `"type": "file"` only if they
  resolve inside the workspace; escaping symlinks are omitted.
- **Bounded breadth-first.** The entry cap (constant, documented) is applied in
  breadth-first order, so truncation drops the deepest entries rather than whole
  top-level folders that sort late. `truncated: true` is surfaced in the UI.
- **Exclusions, scoped correctly.** Today exclusion is name-based at the top level
  (`slices.Contains(workspaceFiles, filepath.Base(name))`, `workspace.go:289`).
  - Dotfiles and `.sandbox`: excluded **at every depth**.
  - Reserved legacy names: excluded at the **workspace root only** — a nested user
    file legitimately named `config.yaml` stays visible.
  - Legacy `sessions/` (`sessionOldDir`, `workspace.go:489`): excluded by path.
  - **Heavy directories** (`node_modules`, `.venv`, `venv`, `__pycache__`, `dist`,
    `build`, `target`): listed as a `dir` entry with `"collapsed": true` and **not
    descended into** — visible, never silently hidden, and they cannot exhaust the cap.
  - Do **not** list `logs` / `.lock`: `ProcessLog` and `Lock` resolve into the
    metadata root (`InternalDir`, `resolver.go:53-62`); `config.yaml` / `state.json`
    live there too (`MetadataRoot()`, `workspace.go:240`).
- **Fix nested writes.** `WriteTaskFile` builds the temp pattern from the raw
  filename, so `dir/task.md` yields `dir/task.md-*.tmp` and `os.CreateTemp` fails
  with *"pattern contains path separator"* (verified empirically). Use
  `filepath.Base(filename) + "-*.tmp"` in the target file's directory.
- **Symlink containment.** One helper, used by every read / write / delete / listing
  path. If V9 finds Go ≥1.24: open the workspace with `os.OpenRoot` and perform all
  file operations through the `*os.Root` (traversal- and symlink-escape-resistant
  by construction, no check-then-use gap). Otherwise: `filepath.EvalSymlinks` on the
  target and verify it stays under the resolved root, with the check-then-use
  residual documented in `docs/architecture.md`.
- `transport/http/handlers/dispatcher_handlers.go` — `ListWorkspaceTree` handler;
  nested-path handling on read/write/delete.
- `app/routes.go` — move file routes to the `{file...}` wildcard.

**Frontend**
- `types/workspace.ts` — `TreeEntry`, `TreeResponse`, `FileTreeNode`.
- `utils/workspace/fileTree.ts` — pure builder (entries → nodes), sorted dirs-first.
- `utils/workspace/filePath.ts` — **one** path encoder: `encodeURIComponent` per
  segment, joined with `/`; used by every service call and by the route builders.
- `components/common/display/FileTree.vue` — recursive, reveal-on-expand, keyboard
  navigable (`role="tree"` / `treeitem`, arrow keys, `aria-expanded`), truncated
  and collapsed-heavy-dir notices.
- `components/AgentIde/workspace/WorkspaceExplorer.vue` — consume the tree; the
  selected file is the `workspace-files` route param (D18).
- `services/automation/dispatcherService.ts`, `composables/automation/useDispatcher.ts`.
- **`components/AgentIde/automation/AutomationForm.vue`** — its task-file selector
  keeps `workspaceFiles: string[]`, derived from the tree by filtering
  `type === 'file'` in the composable (one derivation, not re-listed); the dropdown
  gains nested paths without a shape change.

**Acceptance:** nested directories render and expand; empty directories render;
large trees do not block the main thread; traversal is rejected server-side;
**symlink escapes are rejected**; **nested writes succeed**; a `truncated` tree says
so; heavy directories are visible but not expanded; a workspace with only root files
renders correctly; the selected file survives refresh via the URL.

**Tests**
- `utils/workspace/fileTree.test.ts` — nesting, dirs-first ordering, empty dirs,
  empty tree, deep paths, duplicate names, `truncated`, collapsed heavy dir.
- `utils/workspace/filePath.test.ts` — segment encoding (spaces, `#`, `%`, unicode).
- `components/common/display/FileTree.component.test.ts` — expand/collapse,
  reveal-on-expand, selection, keyboard navigation, notices.
- **Backend** `platform/persistence/workspace_test.go` — nested listing; dirs
  included; breadth-first cap + `truncated`; dotfile/`.sandbox` excluded at depth;
  nested `config.yaml` **not** excluded; legacy `sessions/` excluded; heavy dir
  listed collapsed and not descended; escaping symlink omitted.
- **Backend** `platform/persistence/workspace_test.go` — nested write regression
  (`sub/task.md` round-trips).
- **Backend** `dispatcher_handlers_test.go` — nested read/write/delete; traversal
  rejection (extend existing table); URL-encoded nested paths (`%2F`);
  symlink-escape rejection on read, write and delete.
- **Update** `WorkspaceExplorer.component.test.ts`; **add** `AutomationForm` nested
  task-file selection.

**Verify:** `cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/`
then `cd frontend && npm test && npm run lint && npm run build`

---

## Phase 4 — Global run notifications

**Status: delivered 2026-09-28.** Implementation notes (deviations from the text
below are deliberate and recorded here):
- **No backend change:** V3 holds — `LaneHolder.since` is the run identity
  (`utils/runs/runTransitions.ts`: `key@since`). The automation lane key is the
  automation id, so automation targets are `toAutomation(holder.key)`.
- **Tick contract:** `useGlobalRunActivity` exposes `lastTick`
  (`{ seq, status: 'ok' | 'error', holders }`, a new object per poll) and pauses its
  single interval on a hidden tab (one `visibilitychange` listener for the app's
  lifetime; immediate tick on return). No new interval anywhere.
- **Detector:** `createRunNotifications` (pure dependencies, fully unit-tested) +
  `startRunNotifications(router)`, called once from `main.ts`. Ticks are processed
  serially; `inbound` (external /v1) holders never notify; the seen-set is bounded
  (200); the buffer caps at 20 runs, oldest first out.
- **Automation outcome from the run record:** matched by workspace + automation
  name + `timestamp ≥ since` (records are stamped when a run finishes). With a
  record, the outcome is `failed` (with `AutomationRun.error`) or `completed`;
  without one after the single retry, `ended`. Assistant runs are always `ended`.
- **Conversation targets:** `useRunningActivity` now reports `polledWorkspaceId`,
  and a non-starting `runningActivitySnapshot()` accessor feeds the notifications
  (calling `useRunningActivity` at boot would have bound its one interval to a
  null workspace). Otherwise the target is `toWorkspace(ws)`, per the plan.
- **"Already looking at it"** means the automation's detail route, or the chat
  route of that workspace; such runs are not announced and are cleared on
  navigation (`router.afterEach`).
- **UI:** `common/display/RunNotifications.vue` (top-strip bell + panel,
  presentation only) is passed into `AdminHeader`'s new `actions` slot by
  `App.vue`, which also feeds `AppSidebar`'s `badges` (dot + count, and in the
  link's accessible name). A new batch is announced once with a toast
  (`useToast`). `document.title` = `utils/documentTitle.ts` (`(2) Page · app`),
  driven from `main.ts`.
- **Fixed on the way:** the top strip was `z-20`, the same layer as the app
  banner, so the banner painted over the notification panel; the strip is now
  `z-30` and the mobile nav scrim `z-[35]` (still above the strip).
- **Known limits (accepted, D5/D20):** a run that starts and ends between two
  polls (under ~10 s) is never seen, so it does not notify; and notifications are
  in memory, so a page reload clears them.
- **Verified against the real bundle** with the lane and ledger endpoints
  intercepted: a failed automation + an assistant run ending while on Settings
  → one toast "2 runs ended, 1 failed", bell and title `(2)`, badges on
  Automations and Workspaces, the error text from the run record; the ledger was
  read once, and zero times over two idle ticks; opening each run cleared it and
  restored the title; the panel fits at 360px.

**Characterisation first:** `useGlobalRunActivity` tick behaviour (interval, error
handling, singleton lifetime) before exposing tick status.

**Files:** new `composables/assistant/useRunNotifications.ts`; new
`utils/runs/runTransitions.ts` (pure diff of two snapshots → ended runs, no Vue);
new `components/common/display/RunNotifications.vue`; changed
`useGlobalRunActivity` (tick status + visibility pause); wire in `App.vue` /
`AdminHeader.vue`. Reuse `NotificationDot`, `useToast`, `useAppBanner`. If V3
requires it: the additive `startedAt` field on the lane snapshot (backend model →
handler → types).

**Work**
- **Ongoing visibility stays with the pill (D5).** No "run started" toast.
- **Detection per D20:** baseline on first `ok` tick; ignore `error` ticks; diff
  with `runTransitions.ts`; dedupe on lane key + run identity.
- **Assistant runs:** "ended" only — never "completed", never a guessed failure.
- **Automation failure:** on a just-observed end, read the ledger once per batch;
  match by automation id + run identity; one retry on the next existing tick.
- **Targeting:** the lane key carries the workspace but not the conversation id,
  so conversation-level targets exist only for the current workspace
  (`useRunningActivity.assistantConversationId`); elsewhere the target is
  `toWorkspace(ws)` via the typed builders (D18).
- **Visibility:** emit only when the target route is not the current route; pause
  polling on a hidden tab; coalesce multiple ends into one summary.
- **Findability:** badge the owning destination in the sidebar; title-count badge (D20).
- Capped ring buffer; navigating to a target clears its notification.

**Acceptance:** an assistant run ending while the user is in Settings produces
exactly one **"ended"** notification; a **second run in the same workspace also
notifies**; a failed poll produces **no** notification; runs already finished at
page load produce none; an automation failure is reported as failed from
`AutomationRun.error`, including when the ledger entry lands one tick late;
navigating to it clears the notification and the title badge; several ends in one
tick produce one summary; **no additional polling interval**; polling pauses on a
hidden tab; the ledger is fetched **only on a transition**.

**Tests**
- `utils/runs/runTransitions.test.ts` — ended detection; same lane, new run
  identity → new end; identical snapshots → nothing; reappearing lane.
- `composables/assistant/useRunNotifications.test.ts` — baseline on first ok tick;
  error tick ignored; suppression when target is current route; dedupe by run
  identity; buffer cap; **no ledger call on an idle tick** (call-count assertion);
  ledger retry once then "ended"; neutral label; workspace-level target when the
  conversation id is unknown; coalescing; title badge set and restored.
- `composables/…/useGlobalRunActivity.test.ts` — tick status exposed; pause on
  hidden, immediate tick on visible; no extra interval created.
- `components/common/display/RunNotifications.component.test.ts` — render,
  dismiss, navigate action, summary form.

**Verify:** `cd frontend && npm test && npm run lint && npm run build`

---

## Phase 5 — Primitives and screen redesigns

**Progress:** primitives PR **delivered 2026-09-28**; **Overview** and **Models**
**delivered 2026-09-29**; **Activity delivered 2026-09-29**; **Workspaces delivered 2026-09-29**;
**Automations + edit delivered 2026-09-29**; **Settings (global)** and
**Settings · Appearance**, **Settings (workspace)**, **Run pill** and
**Assistant chrome**, **Buttons** and **Close-out delivered 2026-09-30** — Phase 5 is
complete; next: **Phase 6 — Hardening**.

Close-out PR as built:
- Last 90 palette classes migrated (`ExecutionAuditTrail`, `SystemMetricsPanel`,
  `MarkdownViewer`, and the provider "styles" — dead code with the emoji icons:
  `getIcon` / `getStyle` / `PROVIDER_ICONS` / `PROVIDER_STYLES` removed,
  `PROVIDER_LABELS` is now a plain `Record<SettingsTab, string>`).
  `check-palette`: **0 classes in 0 files**; the allowlist is empty.
- **`frontend/theme/legacyPaletteBridge.ts` deleted.** The colour keywords it
  also supplied (`transparent` / `current` / `inherit`) moved to
  `tailwindTokens.ts → colourKeywords`. Proven: the built CSS has the **same
  785 selectors** before and after; a declaration diff then caught the one real
  change — Tailwind preflight colours placeholders from `colors.gray.400`, which
  fell back to a fixed `#9ca3af` — pinned back to `--text-muted` in `style.css`
  (effective CSS identical again). A sweep of 12 pages × 2 themes found no
  element in a stock Tailwind grey, no overflow, no page errors.
- **D21 lint at `error` for every `.vue` file**; the per-directory overrides list
  is gone (`no-palette-classes`).
- **Found and fixed — XSS:** model output reached `v-html` unsanitised, in
  `MarkdownViewer` (every chat answer and reasoning) and via an inline
  `marked.parse` in `ExecutionAuditTrail`. New `utils/markdown/renderMarkdown.ts`
  is the only markdown path: raw HTML in the markdown is escaped (shown as text),
  link / image URLs keep only `http`, `https`, `mailto` or relative; both
  components use it (`marked` is imported nowhere else). No dependency added —
  DOMPurify on top would be defence in depth and needs approval.
- Also: the audit trail's result summary had `outline-none` (invisible focus) and
  a hover-only copy button; a failed log-level change used `alert()` (now a toast).
- **Phase 5 acceptance, as verified:** no raw palette class, enforced by lint at
  error everywhere ✔; bridge deleted ✔; every destructive action through
  `ConfirmDialog` — no `window.confirm` / `alert` left in `src` ✔; tables and
  pages usable at 390 px (headless Chrome's minimum; 360 px not measured) ✔.
  **Not audited:** "every screen has all four states" and "every user-visible
  number through `utils/format/`" were enforced row by row, not re-checked
  globally — Phase 6 should sweep both.

Buttons PR as built:
- **Scope, decided per button:** raw `<button>` stays where it is not an action
  button — list rows and cards that open something, disclosure headers,
  `SegmentedControl` radios, `IdChip`, the run pill, the bell (badge + popup),
  the sidebar collapse (a nav item) and `/design`'s theme toggle. Every ad-hoc
  *action* button moved to `BaseButton`: AppBanner dismiss, Toast dismiss,
  GuardrailBanner's three choices, `CopyButton` (rebuilt on it, document icon —
  the two-squares copy glyph read as a box-in-a-box), WorkspaceActivity's delete,
  the header menu, the drawer close (focus moves to it via the component's
  `$el`), notification Clear all / dismiss.
- **Found and fixed:** (1) `GuardrailBanner` POSTed the decision itself *and*
  emitted it, and both parents (chat, automation) POSTed it again — every
  decision went out twice, and a second "allow & remember" could hit the
  backend's late-decision path and persist the override again. The banner now
  only asks; the owner's `submitDecision` is passed in as `submit` and awaited
  (choices locked while in flight, an error shown on failure). (2) Toasts were
  never announced — now one polite live region, errors `role="alert"`, named
  dismiss. (3) AppBanner: errors are alerts, notices / standing warnings a polite
  status; dismiss named. (4) WorkspaceActivity cards were clickable `div`s with a
  delete button nested inside — now an open button and a separate, named,
  `ConfirmDialog`-confirmed delete; `InlineConfirm.vue` (its last user) deleted.
- Tokens throughout those files; `TOAST_CLOSE` and the local `formatDuration`
  copy removed (D24 formatter used). New component tests: GuardrailBanner,
  Toast, AppBanner, CopyButton; WorkspaceActivity's moved to the new list.
- Legacy palette classes 186 → 90, left in 5 files (`ExecutionAuditTrail`,
  `SystemMetricsPanel`, `MarkdownViewer`, `constants/providers.ts`,
  `composables/models/useProviders.ts`) — the Close-out row.
- Verified in the real bundle: standing banner as a status with its action, a
  save's toast in the live region and dismissable, Monitor drawer close focused
  and closing, mobile menu button; no page errors.

Assistant chrome PR as built:
- **Reconciled, not forked:** `assistant-ui/overhaul-chat-history-layout.md`
  Phase 4 is complete (status strip = streaming progress; mobile drawer verified)
  and its Phase 5 is recorded as delivered by later work (detached runs,
  `POST /conversation/cancel`, reload reconnect, the complete
  persist-assistant-run-state plan) — except the backend SSE-bleed half, left
  open (not re-verified). V6 holds: no composer model / effort selectors.
- Characterised first (D22): `useAssistant` (connect-before-POST, conversation
  adoption, cross-conversation event drop, failed send, webhook runs, load with
  cancelled turns, delete) and `useAssistantSSE` (channel filter, dedup,
  lifecycle forwarding, guardrail approval raise / withdraw, 1000-event cap) —
  unchanged by this row.
- **Session list** (`ChatSessionList.vue`): client-side title search; pins per
  workspace (`composables/assistant/usePinnedSessions.ts` on
  `usePersistedState`, dropped when a conversation is deleted); groups Pinned ·
  Today · Yesterday · This week · Older, webhook conversations kept in their
  foldable folder with bulk delete (`utils/assistant/sessionSections.ts`, pure;
  replaced `groupSessionsBySource`). Row actions are named per conversation and
  show on hover *or keyboard focus* (always on mobile) — they were hover-only.
- **Chat shell:** a thin status strip (StatusTag) with the real state — Ready /
  Working / Waiting for the model / Waiting for your approval / Reconnecting to
  the run — replaces the fixed "Agent Online". Deletes (one, all, a group) go
  through `ConfirmDialog` (were `window.confirm`); header controls named; the
  mobile scrim lost `backdrop-blur` (GPU guidance). D14 empty state: says what
  the agent does and frames the guardrail approval as expected.
- **Found and fixed:** every user message showed *the current time* (a
  `formatStamp()` per render; messages carry no timestamp) — removed; the retry
  button was invisible to keyboard focus; Send / Stop / scroll-to-latest had no
  accessible name; `AssistantActivity` cards were clickable `div`s (now buttons);
  tool-call expanders gained `aria-expanded`.
- **Restyle only where the chat skill sets mechanics:** `ChatBubble` changed
  colours / radius / ARIA only (diff-audited) — the 320 px inset cap, grid
  collapse, `v-show` inset, `visibility: hidden` thinking dots and gated
  animations are untouched. `ArcOrbitLoader` keeps its hard-coded ring colour
  (GPU-measured component; left for Phase 6).
- `components/AgentIde/assistant/` is palette-free and joined the D21 lint
  override (legacy classes 314 → 186).
- Verified in the real bundle at 1440 / 390 px: empty state, Ready status,
  Ctrl/⌘K, conversation drawer, no overflow, no page errors. A live run was not
  exercised (no model in the isolated backend) — covered by the
  characterisation tests.

Run pill PR as built:
- **Backend (approved 2026-09-30, additive):** `GET /admin/api/active-runs`
  gained `lanes` — per lane `lane`, `limit`, `running`, `waiting`,
  `holder_keys` (idle lanes included; inbound callers hold no lane slot) —
  `handlers.LaneSummary` from `runlane.LaneState`; `lane_holders` / `queued`
  unchanged. Handler test pins the wire keys. Documented in
  `api-reference.md` (which lacked the active-runs and queue routes entirely —
  added), `SPECS/automation-dispatcher.md` and `architecture.md` #35. Verified
  live: a limit saved in Settings reaches the API and the slot bars (the
  scheduler already resized lanes live).
- `RunActivityPill.vue` rebuilt on tokens: a quiet **Idle** mark at rest (dot
  only below `sm`) that still opens the lanes; "n running · m queued" while busy;
  "Run state unavailable" on a failed poll (no stale lists or bars). The panel
  (`role="dialog"`, close button, Escape, outside click) shows each lane as a
  `SlotBar` with the runs holding it and queued for it, then **API callers**
  (inbound holders and waiters) with icon-only **Serve now** — now confirmed via
  `ConfirmDialog`, since it cancels a run — and **Dismiss**. Below `sm` it is a
  bottom sheet over a scrim. It renders nothing until the first answer.
- New primitive `common/display/SlotBar.vue` (one block per slot, `role="meter"`
  with "n of m slots in use"); second use: Overview **Running now** shows lane
  capacity even when idle. On `/design`. Run kind / lane labels moved to
  `constants/runs.ts` (the pill said "Chat" / "External", Overview "Assistant" /
  "API caller" — unified on the latter).
- Found in the real-app check: each lane `<section>` repeated its meter's name,
  announcing a duplicate region per lane — removed.
- Not exercised live: a running / queued state (the isolated backend has no
  models); covered by the component tests.

Settings (workspace) PR as built:
- **Found first:** the backend merges a workspace guardrail layer over the global
  policy **additively** (`models/config.go → MergeWith`): lists are unioned,
  switches can only be turned on (`require_review` only off), numbers replace
  only when > 0, session idle always comes from the layer, and a saved layer —
  always written with a network block — always sets network access. The old page
  copied the whole global policy into an editable form, so removing a global
  entry or switching a tool off looked possible and did nothing; and its
  `ensureStructure` defaults (e.g. 512 KB file size) silently overrode the global
  policy in a layer. **And list edits never saved at all** (global Guardrails
  too): `useGuardrailEditor` only synced config → textarea.
- `domain/guardrailLayers.ts` mirrors the merge (`mergeGuardrails`, `seedLayer`
  — a layer that changes nothing — `normalizeLayer` — a stored layer as only its
  own contribution, meaning unchanged — `fieldSource`: inherited / overridden /
  exception, where exception = the workspace loosens the global policy). A shared
  fixture (`__TESTS__/fixtures/guardrailMerge.*.json`) runs against the Go
  `MergeWith` (`backend/models/config_merge_contract_test.go`, proven to fail on
  a wrong expectation) and the TS mirror, so they cannot drift; `MergeWith`'s doc
  comment points at the mirror. Backend change: that test + the comment only.
- The workspace section moved to **`/workspaces/:ws/settings`** (header: Files ·
  Assistant · Memory · Playbooks · Settings); `/workspaces/:ws/security`
  redirects there. `WorkspaceSettings.vue`: loading / error (Retry); without a
  layer "uses the global policy" + **Customise for this workspace**; with one a
  summary (overrides · exceptions) and the layered form; `SettingsActions`
  (Discard / Save — saves the whole config back, the endpoint replaces it);
  **Reset to global policy** is separate and confirmed (removes the layer). Its
  own unsaved-change guard asks only for navigations that unmount it (another
  section or workspace; the page is kept alive across destinations).
- `GuardrailForm.vue` rebuilt, data-driven, in two modes: whole policy (Settings ·
  Guardrails) or a layer over `inherited`: global list entries shown fixed with
  the workspace's additions editable (entries the global already has are
  pointed out), switches the global turns on locked on with the reason, numbers
  empty = inherit (global value as placeholder), a source tag on every field,
  external-path warning. New primitive `common/forms/ListField.vue` (one entry
  per line; keeps what is typed, reports the parsed list); `FormField` gained a
  `tag` slot. Labels disambiguated (Command / Fetch timeout). **Deleted:**
  `GuardrailSection.vue`, `composables/useGuardrailEditor.ts`.
- Characterised first (D22): `GuardrailForm` (the list-edit test failed on the
  old code — the bug), `WorkspaceSettings`.
- Verified in the real bundle (isolated `--data`): old link redirects; customise
  → exception tag, locked terminal switch, redundant-entry note, external-path
  warning → leaving the section asks (Stay) → save writes a layer with no stray
  overrides → reload shows only the workspace's entries → reset removes the
  layer from disk; a global Guardrails list edit now persists; no overflow at
  390 px; no page errors. Legacy palette classes 411 → 345.

Settings · Appearance PR as built (UI only — the Phase 1 engine was complete):
- `/settings/appearance`, first in the System group (the group now has an
  explicit order, `domain/settings.ts → SYSTEM_TABS`).
  `components/settings/AppearanceSettings.vue`: preset cards as a radio group
  (each card renders in its own `[data-theme]` block, so it previews itself on
  any page theme) applied the moment they are picked; the follow-the-OS switch;
  a custom-themes `DataTable` (contrast status, In use, Use / Edit / Duplicate /
  Export / Delete named per theme); **Duplicate <current preset>** and **Import**.
- **Editor** `components/settings/ThemeEditor.vue`: one field per editable token
  from the registry (colours with swatch; shape and motion with unit hints),
  rename via Name (a new theme's id follows the name — `uniqueThemeId`; an
  existing or imported theme keeps its id), live validation through the one
  validator — a bad value marks its row, blocks Save and never reaches the
  preview; failing contrast pairs listed with ratio and target, Save only after
  the acknowledgement (reset when the failing set changes). The preview is a
  scoped sample (`data-theme` = base + validated overrides); the page's theme
  changes only when a saved theme is used. Pure model in `theme/themeDraft.ts`
  (`draftFromPreset` / `draftFromTheme` / `draftToInput` — only values differing
  from the base are stored — `tokenErrors`, `previewOverrides`); types
  `ThemeDraft`, `ThemeInput` in `types/theme.ts`.
- **Import:** a file over 64 KB, invalid JSON or any validator error rejects the
  file with its reasons in a Callout — nothing is imported; a same-id theme asks
  before replacing; a theme with contrast failures opens in the editor for the
  acknowledgement. **Export** downloads `<id>.theme.json` (hex colours) through
  the new `utils/download.ts → downloadText`. **Export all was not built:** the
  import format is one theme per file and a multi-theme file would need
  per-theme acknowledgements; per-theme export satisfies D3.
- Delete is confirmed (names the fallback preset when the theme is in use);
  closing or replacing an edited editor asks first; the open editor's edits feed
  the page guard (`dirty-change`, like Security / Search / Communication).
- Tests: `theme/themeDraft.test.ts`, `ThemeEditor.component.test.ts`,
  `AppearanceSettings.component.test.ts` (against a real theme controller on
  memory storage), Settings page test for the new section.
- Verified in the real bundle: preset switch repaints live; duplicate → edit →
  contrast warning → acknowledge → save → use applies the override to `<html>`;
  export downloads hex JSON; a `url(...)` import is rejected and never reaches the
  page; reload keeps the custom theme (boot script); delete falls back to the
  base; no overflow at 1440 / 390 px; no page errors.

Settings (global) PR as built:
- `views/SettingsView.vue` replaces `components/settings/Settings.vue` (deleted):
  `PageHeader` with the one **Restart backend** (confirmed; Security's duplicate
  "Restart runtime" removed) and a page-level `UnsavedTag`; `SettingsNav` —
  grouped category links (labels only, no emoji; `security` relabelled from the
  misleading "Host Terminal" to "Security & sandboxing") and, below `lg`, a
  Category picker; load / error (Retry) states. Provider-key logic moved to
  `composables/settings/useProviderKeys.ts` (timer cleared on unmount).
- **One save model.** `useConfig` keeps a saved snapshot: `isDirty`,
  `discardChanges`; housekeeping (`reconcileModelRefs`, `ensureProvider`) is
  applied to both sides so it never reads as an edit. Every editable section ends
  with `components/settings/SettingsActions.vue` (UnsavedTag · Discard · Save,
  both live only while dirty, a failed save shown inline as `role="alert"` and
  the edit kept). Sections with a draft of their own (Security host settings,
  Search / Communication secrets — `useToolSecrets` gained `isDirty` / `discard`)
  report it via `dirty-change`; **one** `useUnsavedChangesGuard` on the page
  covers all of it and does not prompt on a category switch (sections stay
  mounted, edits survive). Abandoned edits are discarded on unmount.
- **Long forms split:** Local engine = Routing · Local engine · Workspace storage ·
  Model lifecycle · GPU metrics · Run scheduler · External requests · Logging
  panels; paths, args, env, ports in mono; fields write copies (the old component
  mutated its prop via `v-model="cfg.x"`). `LogLevelPanel` deleted
  (`SegmentedControl`, "applies immediately"). Security = Sandboxing (Callouts,
  `StatusTag` effective state) · Resource limits · Effective enforcement (`dl`) ·
  Active host terminals · Reset controls. MCP, connectors, terminal sessions,
  processes and provider models are `DataTable`s with icon-only row actions named
  per item. `ModelTuningFields` tooltips became visible hints.
- **New primitives:** `BaseToggle` rebuilt as a native `role="switch"` checkbox
  (was a clickable `div` — not keyboard-operable; `label` now required, the one
  unlabelled use in `GuardrailForm` named); `common/feedback/Callout.vue`;
  `common/display/UnsavedTag.vue` (the dashed marker was duplicated in
  `AutomationForm` / `WorkspacesView`, now shared). All on `/design`; visual
  baselines re-recorded.
- **Confirmations:** restart, search-key removal (were `window.confirm`), API key
  remove / remove all (were `InlineConfirm`), connector removal, webhook delete
  and terminal reset (were unconfirmed) go through `ConfirmDialog`.
- **Polling:** the terminal monitor and the processes list poll only while their
  section shows (both were mounted behind `v-show` and polled on every category);
  `useProcesses` gained `loaded` / `error`, so the table has real loading / error states.
- **Found and fixed:** (1) cancelling a search-key replacement left the typed key
  queued, so the next Save stored it; (2) adding a connector under an existing
  name silently replaced it; (3) editing a connector put the stored token's mask
  into the password field; (4) a discovered GGUF that was already configured had
  a silent no-op Add (now "Added"); (5) the Local models card rendered nothing
  with no models (now an empty state); (6) a Gemini deep link lost the Vertex AI
  fields when the stored config had no Gemini entry (load replaced the ensured
  entry); (7) the missing `power.svg` console error (Restart uses `refresh`).
- Characterised first (D22): `Settings.vue`, `GlobalSettings`, `SecuritySettings`,
  `ModelTuningFields`, `CommunicationSettings`, plus `ProviderModelsCard`; the
  tests find fields by label (`__TESTS__/helpers/fieldByLabel.ts`) so most
  survived the redesign unchanged. `SearchSettings` tests moved from CSS-class
  selectors to labels / roles. Page tests mount through a `RouterView` so the
  leave guard takes part.
- `components/settings/` and `components/infrastructure/` are palette-free and
  joined the D21 lint override (legacy classes 841 → 411). `GuardrailForm`
  internals stay for the Settings (workspace) row.
- Verified in the real bundle (isolated `--data`, all 10 categories at 1440 px
  retro-dark and 390 px retro-paper): no overflow, no page or console errors;
  edit → category switch keeps it → leave prompts (Stay) → save → reload persists.

Automations PR as built:
- `views/AutomationsView.vue` is one page per route: **list** (`/automations` —
  `AgentIde/automation/AutomationList.vue`: `DataTable` with workspace filter and
  search, schedule via `utils/automation/automationDisplay.ts → triggerLabel`
  (cron through cronstrue; Go intervals shortened, `2h0m0s` → `2h`), status tag
  (running › queued #n › last run failed › idle), icon-only Run / Stop / Edit /
  Delete named per automation, cancel-queued and delete confirmed; an automation
  whose workspace is busy is locked), **detail** (`/automations/:id` — header with
  status, Run now / Stop, Edit, confirmed Delete; `AutomationDetails` as
  Configuration · Last result · Console · Runs panels; a run opens
  `HistoricalRunDetails` in a wide `ContextDrawer`), **new / edit**
  (`AutomationForm` grouped Basics · Model & access · Schedule · Review, name
  checked against `constants/validation.ts → RESOURCE_NAME_PATTERN` (shared with
  `WorkspaceList`), workspace fixed when editing, unsaved-change guard; navigates
  to the saved automation only once the save succeeded), **recordings**
  (`/automations/recordings`, `RecordingsPanel` on tokens) and an in-page error
  for an unknown id. Monitor stays a drawer.
- New primitive `common/forms/FormField.vue` (label, hint or `role="alert"`
  error, `aria-describedby` via slot props) and a `.form-control` component class;
  `CronEditor` rebuilt on them. `AutomationPayload` typed in `types/automation.ts`
  (dispatcher service and `useDispatcher` take it instead of `any`).
- `HistoricalRunDetails` gained `embedded` (no close button inside a drawer — both
  Activity and Automations pass it), D24 `formatDuration`, confirmed deletes.
- **Deleted:** `AutomationsPanel.vue`, `AutomationRunActions.vue`,
  `SystemPulseDashboard.vue` (last consumer) and their tests.
- Characterised first (D22): `useAutomationForm`, `useAutomationRunner`,
  `AutomationForm`, `AutomationDetails`, `HistoricalRunDetails`,
  `RecordingsPanel`. `useLiveConsole` is unchanged and left to the Assistant
  chrome row (it shares the chat renderer).
- **Absorbs `assistant-ui/automation-edit-form-reactivity.md`:** its
  derive-don't-sync refactor was already in place; its tests now exist
  (`useAutomationForm.test.ts`) and the plan is marked complete.
- **Found and fixed:** (1) the form repopulated whenever the edited automation
  arrived as a new object — the page polls every 10 s, so edits were wiped and the
  save's navigation was blocked by the guard; the populate watch is keyed on the
  automation's id again (the absorbed plan's original design). (2) The console
  showed "Live stream" while idle (it always seeds the run's opening message);
  it now says Idle / Audit log / Live stream from the real state, with a hint
  when there is nothing to show. (3) Form selects rendered at body size.
- Verified in the real bundle (isolated `--data`, 1440 px retro-dark, 390 px
  retro-light): create → detail → edit (guard, save) → list → monitor →
  recordings → unknown id → delete; no overflow, no page errors.

Workspaces PR as built:
- `/workspaces` is a list page (`AgentIde/workspace/WorkspaceList.vue`: table,
  create with the backend's name rule checked first, delete via `ConfirmDialog`,
  external-access tag). A created workspace opens only once creation succeeded.
- `/workspaces/:ws/…` has `WorkspaceHeader.vue` (workspace switcher that keeps the
  section; section **links** Files · Assistant · Memory · Security · Playbooks with
  `aria-current`; running-chat marker). An unknown `:ws` shows an in-page error.
- **Files:** `WorkspaceFiles.vue` (filter — `FileTree` gained `expandAll` so
  matches inside closed folders show — new file with `isSafeRelativePath`, delete
  via `ConfirmDialog`) beside an editor `Panel` (file name kept in its case —
  `Panel` / `SectionHeading` gained `preserveCase` — dashed "Unsaved", Save
  enabled only when dirty, Ctrl/Cmd+S, close). `FileEditor.vue` is now just the
  editor surface. Mobile: tree, then the editor with "← Files".
- **`WorkspaceExplorer.vue` deleted** (replaced by the list + files panel); the
  accordion-era `chatActive` / `memoryActive` left `useViewManager`, which gained
  a `playbooks` main view (the page test found Playbooks falling into Files).
- **Playbooks:** `TemplateLibrary.vue` is the section page (was a modal): search,
  category, "New file", and "Append to <open file>" (disabled without one),
  loading / empty / error.
- **Memory:** `MemoryPanel` / `MemoryDetail` rebuilt on tokens and primitives
  (search form, type `SegmentedControl`, confirmed delete / clear, named icon
  buttons); **no `dark:` utilities remain in the app**. Stored zone-less
  timestamps are read as UTC via `utils/format/time.ts → asUtc`.
- Characterised first (D22): `WorkspaceActivity`, `MemoryPanel`, `MemoryDetail`.
  **Assistant** and **Security** keep their content (the "Assistant chrome" and
  "Settings (workspace)" rows); the workspace **Settings** section still waits
  for its row.

Activity PR as built:
- `views/ActivityView.vue` replaces `components/logs/Logs.vue` (deleted): one page
  with a **Runs / App log / Process log** switch. Runs = the global run ledger in a
  `DataTable` (finished, automation + error line, workspace, status, duration,
  model, `IdChip`), filtered by **status / workspace / search in the route query**
  (`?status=&workspace=&q=`, search debounced 250 ms; pure `utils/runs/filterRuns.ts`).
  A row opens `HistoricalRunDetails` in `ContextDrawer` (new `wide` option);
  deleting there refreshes the list. Clearing a log goes through `ConfirmDialog`.
- **`kind`, `from`, `to` are not offered:** the ledger holds finished automation
  runs only (V1 — no assistant or per-request records), so a kind filter would
  have one real value; they stay in `ActivityFilters` for when the data exists.
- `SystemPulseDashboard` characterised first (D22); it remains the workspace /
  automation overview pane until those rows.
- New primitives: `common/forms/` `SegmentedControl` (radio group, arrow keys),
  `SearchInput`, `SelectInput`; `common/display/LogViewer` (follows new lines only
  at the bottom). Log level uses `SegmentedControl` here; `LogLevelPanel` stays
  for Settings until its row.
- **Fixed:** `useLogs` kept polling while the kept-alive Activity page was in the
  background (now paused on deactivate, resumed on return); Enter on a control
  inside an activatable `DataTable` row no longer opens the row; the log panel and
  its log region had the same accessible name (now "… log lines").
- **Known, left to the Automations row:** `HistoricalRunDetails` (reused in the
  drawer) still has its own close button and inline number formatting.

Overview PR as built:
- `views/OverviewView.vue`: Health (four `StatCard`s — throughput and CPU with a
  `Sparkline`, memory and VRAM with a `Meter`; the active model with Stop, or
  "No model loaded" → Models), **Running now** and **Recent runs** (last 5 from the
  run ledger, `DataTable`, D24 formatters, "All automations →" until Activity
  merges run history).
- `StatCard`, `Sparkline`, `Meter` extracted to `common/display/` (second use:
  `/design`, and the run pill / Settings rows later) and shown on `/design`.
- **V8 history:** `useMetrics().history`, a 30-sample ring buffer filled by the
  existing poll, captioned "since page load".
- **Lane limits are not in the API.** `GET /admin/api/active-runs` returns holders
  and queued runs but no per-lane limits (and holders no lane key), so the
  mockup's slot bars cannot be drawn without inventing capacity. Overview shows
  **Running now** (holders + queued) instead; `SlotBar` is not extracted. The
  **Run pill** row needs the same data: exposing `runlane.LaneState`
  (lane, limit, holders) is an additive backend change to agree first.
- `SystemStatus.vue` deleted (replaced by the Health panel); six formatters left
  without callers (`formatPercent`, `formatMemory`, `memPercent`,
  `formatTokenRate`, `gpuTempClass`, `clampPercent`) removed.
- **Found and fixed:** a GPU that reports no temperature (Apple silicon) showed
  "NaN °C"; it now shows "—" with "Temperature not reported".

Models PR as built:
- Characterised against `Dashboard.vue` first (D22), then the test was updated to
  the redesigned contract. `views/ModelsView.vue`: Local and Cloud `DataTable`s
  (status Active / Loading / Idle; context via `formatTokenCount`; port; icon-only
  Start / Stop named per model, Start blocked while another local engine is
  active; routing Primary / Fallback from the config; credential `IdChip`;
  per-model Settings link), empty / loading / error states. **`Dashboard.vue`
  deleted** (the split is complete).
- `useModels` gained an `error` ref (the last catalogue read's failure), so the
  page can offer Retry instead of loading forever.
- Verified in the real bundle with fixture catalogue / lanes / ledger at 1440 px
  (retro-dark) and 390 px (retro-paper): no overflow, no page errors; stacked
  tables on mobile; true empty states without fixtures.
Primitives PR as built (deviations are deliberate and recorded here):
- **Scope follows the measured Phase 0 inventory (D7)**, which is later than the
  table below: `StatCard`, `Sparkline`, `Meter` / `SlotBar` stay **inline until a
  second screen uses them**; they are extracted in the Overview / Run pill PRs.
  Form controls (field, select, text/search input, toggle, segmented control) are
  extracted with the first screen that needs them (Automations edit / Settings).
- **Delivered:** `common/display/` `MicroLabel`, `StatusTag` (state → token map
  in `constants/status.ts`), `IdChip` (copy + polite feedback), `DataTable`
  (generic; `cell-<key>` slots, `max-sm:` stacked cards — breakpoint from the
  shared `screens`, sticky header, loading / error / empty, `activatable` rows);
  `common/layout/` `Panel`, `SectionHeading` (`01 ──` via a CSS counter reset on
  the shell's content element), `PageHeader`; `common/feedback/` `EmptyState`,
  `ErrorState`, `LoadingState` (dithered skeleton + caret); `BaseButton` rebuilt
  on tokens (primary / secondary / ghost / danger × sm / md / lg; loading is
  `aria-busy`; unused `glass` variant removed; **icon-only requires `label`**,
  enforced by a source-scanning test — the six existing icon-only buttons moved
  from `title` to `label`); `ConfirmDialog` rebuilt (alert dialog, focus on
  Cancel, Escape cancels, focus returns, warning/error confirm is `danger`).
- **D24 formatters:** `utils/format/units.ts` (`formatTokenCount`, `formatCost`)
  and `utils/format/time.ts` (`formatDuration` — the old unused `"N ms"` version
  replaced — `formatRelativeTime`, `formatAbsoluteTime`). Screens migrate to them
  in their own PRs.
- **`/design` → 05 Primitives** (`views/design/PrimitivesGallery.vue`): every
  primitive and state with fixture data; visual baselines re-recorded after review.
- **Found and fixed:** Tailwind reads `shadow-[var(...)_…]` as a shadow *colour*,
  so the primary brand offset never rendered (also on the Phase 2 not-found
  link) — it is now the `.offset-brand` component class in `style.css`; and
  `--dither-size` existed only in the mockup CSS, so dithered tracks drew
  nothing — now defined on `:root` in `style.css` (fixed geometry, not a theme token).

**Delivery:** **one PR per row**, in table order, each its own acceptance gate.
Every PR: tokens only (semantic classes, no bridge reliance); loading / empty /
error / disabled states; mobile verified at 390px; characterisation tests first
where the screen has none; its directory added to the D21 `error` overrides list;
`/design` updated with any new primitive; visual regression baselines updated
(D23). The **final PR** deletes `legacyPaletteBridge.ts`, turns the lint rule to
`error` globally, and removes the overrides list.

**Primitives** (first PR; D7 — each shown in ≥2 signed-off screens):

| Primitive | Responsibility |
|---|---|
| `Panel` | hairline border, optional numbered `01 ──` title and actions slot (supersedes a plain `BaseCard`) |
| `MicroLabel` | mono, uppercase, tracked, muted |
| `StatCard` | `MicroLabel` + large tabular numeral + unit + optional `Sparkline` and caption |
| `Sparkline` | dot-matrix bars, latest value in full colour, `role="img"` with a text summary |
| `Meter` / `SlotBar` | ruler meter (dithered track, tick scale) and one-block-per-slot bar; `role="meter"`, value text |
| `StatusTag` | borderless tinted tag with square dot, text always present; state → token map in one place |
| `IdChip` | truncated mono ID, full value in tooltip, copy on click with feedback |
| `DataTable` | column definitions, tabular numerals, sticky header, **stacked card layout below `sm`**, empty/loading/error slots, row keyboard focus |
| `EmptyState` | icon, one-line statement, primary action |
| `Skeleton` | shape-matched placeholders (no layout shift on load) |
| `BaseButton` | variants: primary / secondary / ghost / danger; sizes; loading; icon-only with required accessible name |
| `ConfirmDialog` | the single destructive-action confirmation (delete file, delete theme, delete automation) |

All formatting through `utils/format/` (D24).

**Screens** (ordered by daily-use value):

| Screen | Key change | Tests |
|---|---|---|
| **Overview** (D9) | Health + recent activity + quick actions; `StatCard` row with `Sparkline`s (history source per V8 — client ring buffer labelled "since page load" if the backend has none); `SegmentedBar` meters. Move/refactor `SystemStatus.vue` from `Dashboard.vue`; retain only if it stays a cohesive reusable component | new `Overview.component.test.ts` |
| **Models** (D9) | The model catalogue/ops that `Dashboard.vue` is today ("Models Configuration" / "Local Engines"), on its own route. **`Dashboard.vue` is split and deleted** | new `Models.component.test.ts` (**missing today**) |
| **Activity** (D9) | Merges `SystemPulseDashboard.vue` run timeline with `Logs.vue` behind one searchable `DataTable`; filters in the route query (D18). **If V1 confirms per-request usage data**, a **Requests** view mirrors the reference ledger: time, input/output tokens, latency, model, status, cost, trace `IdChip`. Otherwise recorded as a follow-up plan | characterisation for `SystemPulseDashboard` first; new `Activity.component.test.ts`; update Logs tests |
| Workspaces / Explorer | Workspace detail with Files · Assistant · Memory · Security · Playbooks (from `TemplateLibrary.vue`) · Settings as route sections; tree from Phase 3; `MemoryPanel` / `MemoryDetail` migrated (25 of the 36 `dark:*` utilities) | update `WorkspaceExplorer`, `TemplateLibrary`; new `WorkspaceDetail`; characterisation for `WorkspaceActivity` |
| Automations + edit | List as first-class `DataTable`; edit is a focused route grouped Basics / Model & access / Schedule / Review, with the unsaved-change guard; Recordings (`RecordingsPanel.vue`) at `/automations/recordings`. **Absorbs `automation-edit-form-reactivity.md`** | characterisation for `AutomationForm`, `AutomationDetails`, `useAutomationForm`, `useLiveConsole`, `useAutomationRunner` first; update `AutomationsPanel` |
| Settings (global) | Category nav as route sections; long forms split into scannable sections; mono for paths/args; visible save/restart; unsaved-change guard | characterisation for `Settings.vue`, `GlobalSettings`, `SecuritySettings`, `ModelTuningFields`, `CommunicationSettings` first; update `SearchSettings` |
| **Settings · Appearance** (D2/D3) | Preset picker with live preview; **create / duplicate / import / export / edit / rename / delete** a custom theme; editor derived from the token registry; contrast failures listed per pair with acknowledgement (D2); invalid JSON or a bad value shows an actionable error and changes nothing; delete via `ConfirmDialog`; unsaved-change guard | new `AppearanceSettings.component.test.ts`, `ThemeEditor.component.test.ts` |
| Settings (workspace) | At `/workspaces/:ws/settings`; explicit inherited vs overridden vs exception; reset distinct from save | characterisation for `WorkspaceSettings`, `GuardrailForm` first |
| Run pill | Retro treatment; `SlotBar` lane occupancy; mobile bottom sheet instead of a cramped popover; expands only while running (thin strip at rest, D14) | update `RunActivityPill.component.test.ts` |
| Assistant chrome | Restyle to tokens; `useMessageBuilder` + `ChatMessages` remain the sole consumer. **Adopt (D14):** chat as the canvas; session list (`ChatSessionList.vue`) — **client-side search** over loaded titles, **pins** persisted via `usePersistedState`, grouped Today / Yesterday / This week / Older; thin status strip; empty state naming the plan/guardrail behaviour. **Composer model + reasoning-effort selectors only if V6 confirms API support** — otherwise out of scope and recorded as a follow-up. **Reconcile with `overhaul-chat-history-layout.md` — do not fork it** | characterisation for `useAssistant`, `useAssistantSSE` first (`useTurnInset` already has a test); update `ChatBubble`; new `ChatSessionList`, `ChatComposer` |
| Buttons | Migrate the remaining raw-`<button>` files to `BaseButton` (screens above migrate their own; this row sweeps the rest) | per-primitive tests already exist |
| Close-out | Delete `legacyPaletteBridge.ts`; lint `error` globally; remove overrides list | lint + build + visual regression green |

**Acceptance:** no file references a raw palette class, enforced by D21 at `error`
globally; the bridge is deleted; every screen has all four states; every table is
usable at 360px; every destructive action goes through `ConfirmDialog`; every
user-visible number goes through `utils/format/`.

**Verify (per PR):** `cd frontend && npm test && npm run lint && npm run build`
(+ `npx playwright test` if D23 is approved)

---

## Phase 6 — Hardening

The test register now lives in each phase's **Characterisation first** list (D22).
This phase covers what no screen PR owns:

- Remaining untouched high-risk modules: `useMetrics`, `useLogs` (polling leaks),
  `MetricsExpanded` / `RightPane` successors in `ContextDrawer`.
- `prefers-reduced-motion`, focus-visible and keyboard traversal across the whole
  shell, run against `/design` and each destination.
- **Leak audit:** every timer/listener has a documented owner + teardown; keep-alive
  views release polling on deactivation (`onDeactivated`).
- **Performance, measured not guessed:** tree render on a workspace at the entry cap;
  notification detector over a long idle session; route chunk sizes recorded in the
  PR.
- Contrast re-check of any custom-theme presets shipped as examples.

**Verify:** `cd frontend && npm test && npm run lint && npm run build` ·
`cd backend && go test ./...`

Phase 6 as built (2026-09-30):
- **Leak audit — two leaks fixed.** `useRunningActivity` started a raw interval
  that nothing stopped; it now runs on `usePolling` (pauses on keep-alive
  deactivation, stops on unmount). `useMetrics` polled for as long as any view
  had ever mounted it; consumers now `acquire`/`release` on mount / activate /
  deactivate / unmount, and polling stops at zero consumers
  (`__TESTS__/composables/assistant/pollerLifecycle.component.test.ts`).
  Every other timer/listener was read and has an owner + teardown (`useLogs`
  unsubscribes on deactivate); lists are bounded (live events 1000,
  run-notification seen ids 200, tray capped).
- **Reduced motion.** Motion tokens were already 0 ms; a CSS safety net in
  `style.css` stops fixed-duration animations/transitions, and
  `utils/motion.ts` (`prefersReducedMotion`, `motionScroll`) makes the chat
  auto-scroll and the model list scroll instant. `ArcOrbitLoader` lost its
  hard-coded colour prop (now `--accent-info`, `--radius-md`, motion-token fade).
- **Keyboard.** New `SkipLink` is the first tab stop and focuses
  `main#content` (`tabindex="-1"`). A real-bundle sweep of 16 destinations
  (Tab to wrap-around, reduced motion on) found every stop visible with a focus
  indicator; no page errors.
- **Tree performance at the 5,000-entry cap (measured, Chrome, 1× / 4× CPU):**
  first render 109 / 293 ms (collapsed tree, unchanged). Filtering expanded
  every match — 611 / 2,490 ms with a 261 / 1,021 ms longest task and 4,938
  rows. `filterTreeEntries` now caps matches at `MAX_FILTER_MATCHES = 200` with
  a "showing N of M" status: 65 / 184 ms, longest task 0 / 63 ms, 222 rows.
- **Notification detector:** 5,000 synthetic poll ticks in ~13 ms, no ledger
  fetches while idle (`useRunNotifications.test.ts` long-session case).
- **Route chunks** (min / gzip kB): index 186.48 / 68.15 · SettingsView
  110.00 / 32.36 · WorkspacesView 61.87 / 19.86 · AutomationsView 61.14 / 18.75
  · ContextDrawer 52.20 / 16.69 · MonitorPanel 31.66 · OverviewView 9.53 ·
  ActivityView 9.09 · ModelsView 5.13 · NotFoundView 1.65 · index CSS
  62.59 / 11.18.
- **Carried-over sweeps.** Four states: every data-loading view/component has
  loading, empty and error handling (settings forms have no empty state by
  nature; the chat's empty state lives in `ChatMessages`). Formatting:
  theme-editor contrast ratios use the new `formatContrastRatio`
  (`utils/format/units.ts`); the metrics panel's average latency uses
  `formatDuration`. `/design` keeps its bare `4.50 ≥ 4.5` pair on purpose (the
  `:1` suffix wrapped the mobile cell — caught by the visual baseline).
- **Contrast:** no custom example themes ship; `check-contrast.cjs` (then under `docs/design/retro/`, now `frontend/scripts/`) passes all
  presets (tightest: retro-paper `text-faint` on `surface-hover` 4.72:1).

---

## Phase 7 — Documentation close-out

- **Amend SPEC-003.** §IV mandates a *"glassmorphic Glass Deck"* aesthetic with
  pulse animations — contradicted by this direction. §III's component hierarchy
  (`DiscoveryPanel.vue`, `ModelGrid.vue`, `ToolManifest.vue`, `WorkspaceList.vue`)
  is **stale**: none of those files exist. Rewrite both to match reality.
- Update `docs/architecture.md`: frontend layout, route table and typed builders,
  token contract (channel format, registry, boot script), SPA fallback, tree
  contract and containment approach, notification detector rules, pitfalls.
- Add to `.agents/rules/frontend-vue-engineer.md`: semantic tokens only; no raw
  palette classes (lint-enforced); formatting via `utils/format/`; navigation via
  typed route builders; persistence via `usePersistedState`; destructive actions via
  `ConfirmDialog`.
- Register this plan in `docs/PLANS/README.md` (Active + Remaining Work) and `docs/INDEX.md`.
- Update the `assistant-ui-patterns` skill if shell conventions change.
- Catalog `docs/design/retro/` with a README marking it the frozen, non-normative
  Phase 0 record, superseded by `/design` (D6).
- Record follow-ups not taken here: per-request usage ledger (if V1 negative),
  composer selectors (if V6 negative), global assistant terminal-status source, push
  channel for run lifecycle, cross-device theme sync.

**Verify:** `docs/INDEX.md` cross-references resolve.

---

## Risks

| Risk | Mitigation |
|---|---|
| Design not right until seen | Phase 0 mockups gate all Vue work; tokens make iteration one-file; `/design` keeps it cheap afterwards |
| Half-themed app between Phase 1 and Phase 5 | D16 bridge themes every screen in Phase 1; deleted in the final Phase 5 PR |
| Theme regressions across 153 files | Migrate per screen, one PR per row; D21 lint ratchet; visual regression (D23) |
| Opacity modifiers silently broken by token format | D15 channel tokens + `withAlpha()`; Phase 1 acceptance asserts `/50` works |
| Deep links 404 on refresh | D18 SPA fallback with a backend test |
| Lost drafts / state on navigation | D19 route-meta keep-alive + single unsaved-change guard |
| Tree traversal / symlink escape | `isUnsafeFileParam` is syntactic only; Phase 3 enforces containment through one helper (`os.Root` where available); traversal, `%2F` and symlink tests before shipping |
| Silent or misleading truncation of a large tree | Breadth-first cap, `truncated` surfaced, heavy dirs listed collapsed |
| Notifications suppressed for repeat runs | D20 dedupe on run identity, not lane key |
| False "ended" storm after a failed poll | D20 error ticks ignored; baseline on first ok tick |
| Automation failure reported as "ended" | D20 ledger match + one retry on the next tick |
| Notifications mis-report success | Assistant runs labelled "ended" only; failure only from `AutomationRun.error` |
| Duplicate run polling | D5 — no new interval; asserted by test; paused on hidden tab |
| Accent/border colours fail contrast | Derived tokens (`--border-control`, `--accent-info-text`, readable faint) verified on both bases; validator enforces pairs |
| Light theme reuses dark accents | `retro-paper` has its own accent set, proven by the preset tests |
| Font flash or offline font failure | Self-hosted woff2, preload, `swap` with metric-compatible fallback |
| Pre-paint snippet blocked by CSP or drifting from `apply.ts` | External same-origin boot script; contract test against `apply.ts` |
| Malicious custom theme | D2 — registry names only, per-kind grammar, opaque colours, normalised before storage; boot script re-checks grammar |
| Custom themes lost on a browser data wipe | D3 — export/import is mandatory |
| Conflict with active Assistant UI plan | D8 — ownership split; reconcile before the Assistant chrome PR |
| Scope creep into backend via Hermes ideas | Composer selectors gated on V6; per-request ledger gated on V1 |
| New runtime dependency (`vue-router`) | Single official dependency; lazy per-route chunks (D11) |
| New dev dependency (Playwright) | Needs approval (D23); manual `/design` checklist if declined |
| Dead code left behind | Phase 2 deletes legacy nav, `AppTab`, `MobileTabBar`, `SidebarNavTabs`; Phase 5 deletes `Dashboard.vue`, the bridge and the lint overrides list |
| Features orphaned by the new IA | Playbooks → Workspaces; Recordings → Automations; workspace settings → Workspaces |
| Scope explosion | Phases independently shippable; Phase 5 split into one PR per row |

## File change checklist (new frontend subsystem)

1. `frontend/src/types/*.ts` — types (never exported from `utils/`/`composables/`/`theme/`)
2. `frontend/src/theme/` or `frontend/src/utils/` — pure logic, no Vue imports
3. `frontend/src/composables/` — singleton state, `mountCount` if polling; persistence
   only through `usePersistedState`
4. `frontend/src/services/` — API only (if backend involved); paths via `utils/workspace/filePath.ts`
5. `frontend/src/router/` — routes and typed builders; components never build paths by hand
6. `frontend/src/components/common/<domain>/` — reusable UI; states: loading / empty / error / disabled
7. `frontend/src/__TESTS__/…` — mirroring test path 1:1; characterisation first for existing modules
8. `/design` — every new primitive and state rendered with fixtures
9. Backend (tree, SPA fallback, optional `startedAt`): `models/` → handler → `routes.go` → service → mocks

## Remaining Work

**Phase 0 — signed off 2026-09-28.** V1–V10 answered ([findings](#phase-0-findings-2026-09-27));
palette audit, mockup harness, contrast gate and measured primitive inventory in
`docs/design/retro/`. Wordmark: split disc.

**Decided at sign-off (2026-09-28):** D13 warm ink `#0c0b0a` confirmed; D15–D25
confirmed. Font pair **Inter + JetBrains Mono** (D17). **Playwright approved** as a
dev dependency (D23). `retro-paper` accent set accepted as in `tokens.css`. V10
amendments accepted: D16 bridge maps **per utility**; a `--text-on-accent` token
replaces `text-white` on the 29 solid accent fills; D21 adds a dependency-free
`frontend/scripts/check-palette.mjs` (templates, `@apply`, TS literals, shrinking
allowlist) run by `npm run lint`, alongside `vue/no-restricted-class`.
V1 and V6 keep the per-request usage ledger and composer model/effort selectors
out of scope as follow-ups.

**Phase 1 — delivered 2026-09-28** (see the Phase 1 status notes).

**Phase 2 — delivered 2026-09-28** (see the Phase 2 status notes). Follow-ups
it leaves to later phases: workspace `settings` section and the unsaved-change
guard in `AutomationForm` / `Settings` / `ThemeEditor` (Phase 5); the missing
`power.svg` icon (pre-existing).

**Phase 3 — delivered 2026-09-28** (see the Phase 3 status notes).

**Phase 4 — delivered 2026-09-28** (see the Phase 4 status notes). Phases 5–7 open.
Phase 5 complete (2026-09-30), closed out with the bridge deleted; the missing
`power.svg` follow-up is closed.

**Phase 6 — delivered 2026-09-30** (see the Phase 6 as-built notes).

**Phase 7 — delivered 2026-09-30.** SPEC-003 rewritten as v2.0 (Admin UI: the
shipped destinations, route table, token contract, SPA fallback, tree,
notification rules, accessibility and performance budgets; the stale
component list and "Glass Deck" style withdrawn). `docs/architecture.md`
gained the frontend entry points, the SPA-fallback pointer, corrected polling
guidance and pitfalls 37–40; `.agents/rules/frontend-vue-engineer.md` gained
"Admin UI conventions"; `docs/design/retro/README.md` is marked frozen and
non-normative (its contrast gate stays live).

**Design folder removed (2026-09-30).** Once the redesign shipped, `docs/design/`
(the Phase 0 mockup harness, palette audit and README) was deleted; every
reference above to `docs/design/retro/` is historical. The only live piece, the
contrast gate, moved to `frontend/scripts/check-contrast.cjs` (folded together
with its helper, extended with the accent-fill pairs so it matches
`theme/contrastPairs.ts`) and now runs in `npm run lint`.

**Post-delivery change (2026-09-30):** the user found the near-black ink tiring,
so a fourth preset, `retro-dark-soft` (canvas `#22201d`, a faded black with
lightened muted/faint/queued/error/info-text tones so all 58 contrast pairs still
pass), became the default; `retro-dark` stays selectable and remains the cascade
base (`BASE_PRESET_ID` vs `DEFAULT_PRESET_ID` in `theme/presets.ts`). SPEC-003
v2.1.

**Post-delivery change (2026-09-30):** host stats returned to the header as an
expandable strip on every destination (`components/layout/HostStats.vue`,
`domain/hostStats.ts`; SPEC-003 v2.2), because the redesign had left them on
the Overview only. The popover mechanics were extracted on their second use
(`PopoverPanel.vue`, `useDismissable.ts`; the run pill now uses both).

**Follow-ups not taken (out of scope; each needs its own plan or approval):**
- Per-request usage ledger and an Activity · Requests view (V1: no ledger).
- Composer model / reasoning-effort selectors (V6: the send API has no
  per-turn fields).
- A global assistant terminal-status source — assistant runs are reported
  as "ended", never succeeded/failed (D5).
- A push channel for run lifecycle — detection latency is one poll interval.
- Cross-device theme sync (D3: backend as source, `localStorage` as the
  pre-paint cache).
- DOMPurify behind `renderMarkdown` as defence in depth (new dependency —
  needs approval; the escaping renderer already closes the XSS).
- Theme "Export all" (deliberately not built; per-theme export exists).
- The backend SSE-bleed fix stays tracked in
  `assistant-ui/overhaul-chat-history-layout.md` Phase 5.

Revision history is kept in git, not in this document.