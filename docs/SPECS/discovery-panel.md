---
id: SPEC-003
title: Admin UI (formerly Discovery Panel)
version: "2.5"
status: stable
last_updated: 2026-10-05
constitution_references: []
related_specs: [SPEC-006, SPEC-007, SPEC-008]
supersedes:
---

# SPEC: Admin UI

## Changelog

- **2.5 (2026-10-05)** — Saving from chats (SPEC-004 §II.8). A turn that saved an explicit "remember …" shows
  **Saved to memory** under its answer (from the turn's run record, so it survives a reload). The assistant header gains
  **Review for memories** (enabled for a saved, idle conversation): a drawer lists the model's proposed facts with where
  each applies and how it is recalled, already-saved ones disabled, and saves only the ticked ones.
- **2.4 (2026-10-04)** — Heartbeat and memory controls. Workspaces gain a **Heartbeat** section
  (`/workspaces/:ws/heartbeat`); the automation form's Heartbeat preset panel is gone. Memory defaults
  live in Settings → Local Engine and are overridden per automation (form) and per workspace
  assistant (Memory section) through one `InheritField` (Default (on|off) / On / Off). **New file** from a
  playbook asks before replacing an existing file. Markdown tables in model output scroll inside their own
  box instead of widening the page.

- **2.3 (2026-10-01)** — Navigation shortcuts and bulk file delete. The
  sidebar's Workspaces row gains an assistant shortcut (a hover-revealed chat
  icon) that opens the assistant of the last-opened workspace; it is not a
  seventh destination.
  Finished runs are links: the Activity drawer is addressed by `?run=<id>`,
  Overview's recent runs open it, and the run's automation and workspace link
  to their pages. The file tree deletes folders (with their content), a
  selection, or every file (`DELETE …/files/{path}?recursive=true`). The
  assistant message box grows with its text and can be expanded for a long
  paste. Assistant and automation output reads as a document (unboxed answer
  in a reading column) with a one-line activity summary that opens a step
  timeline; agent-internal control messages never show as the operator's.
- **2.2 (2026-09-30)** — The top strip carries host stats again (CPU, memory,
  GPU core, token throughput), on every destination, expandable to the detail
  card. The redesign had moved them to the Overview only, so they were missing
  on the assistant and automations pages. The metrics poll is now app-wide
  (owned by the strip) and pauses on a hidden tab.
- **2.1 (2026-09-30)** — Added the `retro-dark-soft` preset (faded black,
  canvas `#22201d`) and made it the default for first visits and for a dark
  OS; `retro-dark` (the original near-black ink) stays selectable and is
  still the cascade base in `tokens.css`. Behaviour change only for browsers
  with no stored theme choice.
- **2.0 (2026-09-30)** — Rewritten to match the shipped admin UI after the
  retro redesign (`docs/PLANS/cross-cutting/frontend-redesign-retro.md`).
  v1.0 described a single "Discovery Panel" (`DiscoveryPanel.vue`,
  `ModelGrid.vue`, `ToolManifest.vue`, `WorkspaceList.vue`) that never
  existed, a glassmorphic "Glass Deck" style with pulse animations, and a
  credential "Unlock" action. All three are withdrawn: the UI is a routed
  shell with six destinations, a token-based theme system with no
  shadows or glass, and masked-by-default secrets with no unlock step.
  The file keeps its name so existing references resolve.
- **1.0 (2026-05-30)** — Initial Discovery Panel spec.

## I. Intent

The admin UI (`frontend/`, served at `/admin`) is the single operator's
console for the proxy: it shows what the system is doing now, lets the
operator configure models, providers, guardrails and automations, and gives
each workspace a file explorer and an assistant. It runs on a LAN, has no
account model, and makes no network request outside the proxy's own origin.

## II. Functional Requirements

### 1. Shell and navigation
- Six destinations in a persistent, collapsible left sidebar: **Overview,
  Workspaces, Automations, Models, Activity, Settings**. The collapse state
  persists; below the mobile breakpoint the sidebar becomes a drawer.
- The sidebar's Workspaces row carries an **assistant shortcut**: a chat icon
  at the row's end linking to `/workspaces/<ws>/assistant` for the workspace
  last opened (remembered through `usePersistedState`, validated as a
  workspace name). When only one workspace exists, that one is remembered as
  soon as the list loads. A remembered workspace that no longer exists is
  forgotten. The icon is revealed on hover or keyboard focus. It stays shown
  while that assistant is open, and always in the mobile drawer, which has no
  hover. It is absent until a workspace is known and in the collapsed rail.
  It is a shortcut, not a destination.
- A header carries the **host stats strip** (§II.8), the run-activity pill
  (global lane state, see §II.5) and the notification bell. A context drawer (Monitor) is available at every
  width.
- Every page is addressable by URL (§III.2); refreshing a deep link renders
  the same page. An unknown path renders a not-found page **inside the
  shell** — never a silent redirect.
- The first tab stop is a "Skip to content" link that moves focus to
  `main#content`.

### 2. Destinations
- **Overview** — health (active model, system metrics), running work with
  lane slot bars, recent runs.
- **Workspaces** — list page; per workspace: **Files** (tree + editor),
  **Assistant** (sessions with search, pins and time groups), **Memory**,
  **Playbooks**, **Heartbeat** (on/off, interval, model, alert connector, last check; warns when
  a check would wake the local model), **Settings** (the workspace guardrail layer, §II.6).
  The Memory section opens with the workspace's assistant-memory override, and Settings → Local
  Engine holds the two global hot-memory defaults. **New file** from a playbook asks before
  replacing a file that has different content (a missing or empty file is simply written).
- **Automations** — list without a workspace requirement, detail with run
  history, create/edit form, recordings.
- **Models** — local runtimes and remote provider catalogues.
- **Activity** — the global run ledger; filters live in the query string so
  a filtered view is shareable, and so does the open run (`?run=<id>`), so
  any finished-run row elsewhere (Overview's recent runs) links straight to
  its details. A linked run no longer in the kept history says so. In the
  ledger and the run details, the automation and workspace names link to
  their pages.
- **Settings** — global configuration by section (`/settings/<section>`),
  including provider keys, security, MCP servers (SPEC-008) and Appearance.

### 3. Workspace file tree
- Fed by `GET /admin/api/dispatcher/workspaces/{workspace}/tree`
  (`docs/api-reference.md`): slash paths, sorted, breadth-first, **capped at
  5,000 entries** (`truncated: true` beyond); dotfiles and symlinks that
  escape the workspace are omitted; dependency directories are listed
  `collapsed` and not descended.
- All workspace file I/O is contained by `os.Root`, so a symlink cannot
  reach outside the workspace.
- The open file's path is part of the URL. Filtering shows at most **200
  matches** with a "showing N of M" status, so a filter over the full cap
  never blocks the main thread.
- Deleting is always confirmed. A file or a folder can be deleted from its row,
  and a folder is deleted with everything inside it. **Select** mode adds a
  checkbox to each row so several files and folders can be deleted together,
  and offers **Delete all files**. That removes every listed top-level entry,
  whatever the filter shows. Dotfiles are not listed, so they are kept.
  Deletes go through `DELETE …/files/{path}?recursive=true`, and the
  workspace root itself is refused. Deleting a folder that holds the open
  file closes the file.

### 4. Run notifications
- Only **terminal** transitions notify: a run ending or an automation
  failing. Starting a run updates the pill, not the bell.
- The lane snapshot cannot tell success from failure, so an assistant run is
  reported as **ended**. Automation failure comes from the activity ledger's
  `error`, read only after an observed transition (coalesced per tick; one
  retry on the next tick if the entry is not there yet), never per tick.
- Correctness rules: dedupe by lane key **plus** run identity; the baseline
  is the first successful tick (runs finished before load never notify);
  failed ticks are ignored (never read as "every run vanished"); several
  endings in one tick coalesce into one summary.
- No new timers: detection rides the existing global activity poll, which
  pauses while the tab is hidden and ticks immediately on return. The tray
  keeps at most 20 notifications and remembers at most 200 seen runs. While
  unread notifications exist, `document.title` is prefixed with the count.

### 5. Run activity
- One global poller reads `GET /admin/api/active-runs` (lane holders, queue,
  and per-lane `limit` / `running` / `waiting` for slot bars; SPEC-007). A
  failed poll is surfaced ("run state unavailable"), never shown as stale
  live counts.

### 6. Settings and guardrails
- One save model per page: edits are a draft, **Save** / **Discard** act on
  the whole section, and one unsaved-change guard covers route leaves and
  tab close.
- A workspace's guardrails are a **layer** over the global configuration,
  merged exactly as the backend's `MergeWith` does (lists union, switches
  only turn on, numbers > 0 replace); the UI shows each effective value's
  source. A shared fixture keeps the TypeScript mirror and the Go merge in
  step (SPEC-006).

### 7. Theming
- Built-in presets: `retro-dark-soft` (default; a faded black, easier on the
  eyes), `retro-dark` (the original near-black ink), `retro-dark-lifted`,
  `retro-paper` (light). With no stored choice the UI follows the system
  colour scheme: a dark OS gets the default, a light OS gets `retro-paper`.
  The picker is Settings · Appearance. Adding a preset means a block in
  `styles/tokens.css`, the id in `types/theme.ts`, `theme/presets.ts`,
  `theme/tokenSheet.ts`, `public/theme-boot.js` and
  `frontend/scripts/check-contrast.cjs` (every pair must pass).
- Custom themes: duplicate a preset or import JSON; both paths run one
  validator. Only known token names are accepted; each value must match its
  kind's grammar (opaque colour, length, duration). Fonts and easing are not
  editable. A contrast failure blocks saving until the operator
  acknowledges it; a grammar failure always rejects. Themes export to JSON
  (they are device-local and would otherwise be lost with browser data).

### 8. Host stats
- The header strip shows CPU load, memory %, GPU core % and tokens/s from
  `GET /admin/api/metrics` on every destination; the GPU figure is omitted
  when there is no GPU. Below `md` only throughput stays in the strip.
- Clicking it opens the detail panel (a popover from `sm`, a bottom sheet
  below): CPU and memory meters, the GPU's VRAM, core utilization and
  temperature (or "Not reported"), throughput with a since-page-load
  sparkline, and a link to the Overview. Escape or an outside press closes it.
- Load levels colour the number, never replace it: CPU and GPU core turn
  amber from 50% and red from 80%; memory from 75% and 90% (used memory
  includes the OS cache, so ~70% is normal). The figures are not announced as
  a live region.
- Cost: one request per poll interval (10 s; the server samples the GPU on
  its own 10 s timer and caches host stats for 2 s), and a few text nodes per
  poll — measured at ~33 ms of main-thread time over 41 s on the assistant page.

## III. Technical Architecture

### 1. Layout (`frontend/src/`)
- `views/` — one component per destination (lazy route chunks).
- `components/layout/` — shell (sidebar, header, context drawer, skip link);
  `components/common/` — shared primitives by domain (`buttons/`,
  `display/`, `feedback/`, `forms/`, `layout/`); `components/ui/` — dialogs
  and toasts; `components/AgentIde/` — workspace/automation feature
  components (historical path, kept); `components/settings/` — settings
  sections.
- `composables/` — module-level singletons by domain; `services/` —
  stateless API clients; `domain/` — pure logic (e.g. guardrail layers);
  `utils/format/` — every user-visible number and time; `theme/` — token
  registry, presets, validator, apply; `router/` — route table and typed
  builders; `types/` — the only place exported types live.
- App-wide lifecycle is wired once in `main.ts` (router, theme boot,
  `startRunNotifications(router)`).

### 2. Routes
History mode under Vite's `base` (`/admin/`). Route names are constants in
`types/routes.ts`; every navigation goes through the typed builders in
`router/routes.ts` (`toWorkspaceFile(ws, path)`, `toAutomation(id)`,
`toSettings(section)`, `toActivity(filters)`, …).

| Path | Name | Notes |
|---|---|---|
| `/` | — | redirect → `/overview` |
| `/overview` | `overview` | |
| `/workspaces` | `workspaces` | list; keep-alive |
| `/workspaces/:ws` | `workspace` | redirect → files |
| `/workspaces/:ws/files/:path(.*)*` | `workspace-files` | nested path in the URL |
| `/workspaces/:ws/assistant/:conversationId?` | `workspace-assistant` | notification target |
| `/workspaces/:ws/:section(memory\|playbooks\|heartbeat\|settings)` | `workspace-section` | `/security` redirects to `settings` |
| `/automations` | `automations` | |
| `/automations/new` | `automation-new` | |
| `/automations/recordings` | `automation-recordings` | static segment ranks above `:id` |
| `/automations/:id` | `automation` | |
| `/automations/:id/edit` | `automation-edit` | |
| `/models` | `models` | |
| `/activity` | `activity` | query: `kind`, `status`, `workspace`, `q`, `from`, `to`; keep-alive |
| `/settings/:section?` | `settings` | |
| `/design` | `design` | **dev builds only** — living token/primitive reference |
| `/:pathMatch(.*)*` | `not-found` | in-shell |

Keep-alive is opt-in by route meta (Workspaces, Activity); there is no global
`<KeepAlive>`.

### 3. SPA fallback (backend)
`handlers/admin_ui.go` serves existing files from the embedded bundle as-is.
Any other `GET` under `/admin` is a client route and receives `index.html` —
except paths that can only be files (`/admin/api/*`, Vite's `assets/`, and
root-level files), which **404** so a stale bundle or bad API call fails
loudly instead of receiving HTML.

### 4. Theme token contract
- `theme/tokenRegistry.ts` is the one list of token names and kinds;
  `styles/tokens.css`, the validator, the Tailwind map and the theme editor
  derive from it.
- Colour tokens are stored as opaque `R G B` channel triples so Tailwind's
  opacity modifiers work (`rgb(var(--x) / <alpha-value>)`); users author hex,
  `rgb()` or `hsl()` and the validator normalises. Translucency is a usage
  decision, never a token property.
- A theme is applied to `<html>`: `data-theme` selects the preset block, the
  `dark` class tracks the scheme, and a custom theme's validated overrides
  are inline custom properties (`theme/apply.ts`).
- **Pre-paint boot:** `public/theme-boot.js` is a blocking same-origin script
  in `<head>` that applies the stored, already-validated theme before first
  paint (no flash, no CSP change). It re-checks each value against a cheap
  grammar and falls back to the default preset on any error. A contract test
  proves it produces the same `<html>` state as `apply.ts`.
- Browser persistence goes through `usePersistedState` only (namespaced,
  versioned keys; corrupt data falls back): theme selection and custom
  themes, sidebar collapse, pinned sessions.

### 5. Data orchestration
- Polling goes through `usePolling`: it pauses when a keep-alive view is
  deactivated and stops on unmount. `useMetrics` counts its consumers and
  stops at zero; the header strip is a permanent consumer, so the poll runs
  for the app's lifetime and pauses while the tab is hidden (refreshing at
  once on return). `useGlobalRunActivity` runs app-wide (notifications depend
  on it) and pauses while the tab is hidden.
- Assistant runs stream over SSE (SPEC-001 events); the assistant is one
  module-level singleton.

## IV. Design Language
- Warm ink canvas with a cream light preset; persimmon is the brand accent and
  never a state colour; mint / cobalt / citrine / red carry success / info /
  running / error.
- Flat surfaces: **no shadows**, except the primary button's hard brand
  offset. No glass, no blur.
- Components use semantic token classes only; raw Tailwind palette classes
  fail `npm run lint`.
- Typography (Inter, JetBrains Mono) is self-hosted; motion durations are
  tokens.

## V. Accessibility and Performance
- Every preset passes the contrast gate
  (`frontend/scripts/check-contrast.cjs`, run by `npm run lint`; WCAG 4.5:1 for text roles); the
  preset test fails `npm test` on a regression.
- Every control is keyboard reachable with a visible focus indicator;
  icon-only buttons carry a label; toasts are announced through a live
  region.
- `prefers-reduced-motion` zeroes motion tokens, stops fixed-duration
  animations, and makes scripted scrolling instant.
- Every data view has loading, empty and error states.
- Budgets (measured 2026-09-30, see the plan's Phase 6): the tree at the
  5,000-entry cap renders collapsed in ~110 ms; filtering stays under ~70 ms
  with no long task at 1× CPU.

## VI. Security
- The UI respects `CONSTITUTION.md`; it talks only to its own origin.
- Model and user markdown is rendered only through
  `utils/markdown/renderMarkdown.ts`: raw HTML is escaped and only
  `http`, `https`, `mailto` and relative URLs become links. Each table is wrapped in a
  horizontally scrollable box (`.md-table-scroll`, keyboard-focusable) so a wide table never widens
  the page; columns keep a readable width and long URLs wrap.
- Secrets (provider keys) are write-only in the UI: shown masked, replaced,
  never read back in clear.
- Guardrail approvals from the UI go through the same `GuardrailEngine`
  decision as agent calls (SPEC-006); one approval posts once.
