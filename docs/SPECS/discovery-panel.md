---
id: SPEC-003
title: Admin UI (formerly Discovery Panel)
version: "2.20"
status: stable
last_updated: 2026-10-30
constitution_references: []
related_specs: [SPEC-006, SPEC-007, SPEC-008]
supersedes:
---

# SPEC: Admin UI

## Changelog

- **2.20 (2026-10-30)** — **Deep check** (network-audit plan 13.J, Constitution II.3 "Intrusive runs"). §II.6: in the Advanced
  view each device has a **Deep check** button. It opens a question that says exactly what will be sent (about 60 TLS
  connections per TLS port; one SSH login with a made-up user name that the device logs; no password guessed), then runs
  one device, shows it as checking and shows the result under the device: problems with their fix and steps, or "No TLS or
  SSH service answered, so there was nothing to check. This is not the same as nothing wrong." A device is held back for ten
  minutes after a check and only one runs at a time. It is a step-up action (§II.9), and Help has a part for it.
- **2.19 (2026-10-30)** — **Sign-in is compulsory** (0.22.0). Until a passphrase exists every page is the setup page (it
  shows the `auth setup-code` command with or without an open code, so the guard cannot loop); the "not set up" banner is
  removed, and a 401 `setup_required` (or `auth reset` under an open page) sends the person to setup, not to a login. The
  status field `required` is now `set_up`.
- **2.18 (2026-10-30)** — **Sign-in** (plan `admin-api-authentication`). §I: "has no account model" becomes **one operator
  credential** (a passphrase) plus named script tokens; there are still no users or roles. New §II.9 describes the
  pages, the 401 handling and the Sign-in settings; §III.2 lists `/login` and `/setup`; §VI says the webhook secret is
  write-only too. The composer draft is kept across the redirect to the sign-in page.
- **2.17 (2026-10-23)** — The chat sends its workspace with every command (`workspace_id` on
  `/admin/api/commands/run` and `/preview`), so **`/run <automation>`** works in the web chat: the preview reads
  "Runs 'x' now", and sending it starts the automation and shows its start line as a toast.
- **2.16 (2026-10-23)** — **Turn commands in the composer** (SPEC-012 1.6). Sending `/research <question>` or
  `/agent <agent> <question>` asks `POST /admin/api/commands/run`; when it answers with a `turn`, the composer sends
  `turn.message` with `turn.agent` and `turn.use_helpers` for that message only (the model and the conversation's agent
  stay as they are), after a warning toast when the reply says why helpers are off. A queued message goes through the
  same path. The command preview under the box says how it would run. Known limit: **Retry** on such an answer reruns
  the stored question under the conversation's own agent, without helpers (the turn's agent is not stored).
- **2.15 (2026-10-23)** — **Settings → Agents** and the rename of "profile" to "agent" in the interface.
  - A General section, **Agents** (`AgentsSettings.vue`), between Runs and Memory & reminders: the list (the user's own
    first, then the built-in ones, each with its summary, what it is the default for and its helper count) beside the
    open agent on a wide screen, one at a time on a phone with a way back. The form is drawn from
    `GET /admin/api/profile-schema`; the server's refusal appears beside the setting it names.
  - Built-in agents are read-only with **Duplicate**; the user's own can be saved, discarded and deleted ("Restore
    built-in" when a file replaces one). Leaving an edited agent asks first; the section reports its unsaved edits to
    the page's guard. Saves go straight to the server (each agent is a file), so it has no shared Save.
  - The picker in Run settings reads **Agent**, the workspace field **Agent for new chats**, the automation form and
    details **Agent**. The API paths, field names and files still say profile (SPEC-012).
- **2.14 (2026-10-22)** — The Helpers row in Run settings links to **Settings → Runs**, where the Helper model is
  chosen, and says what is wrong when helpers cannot run, by `GET /admin/api/helpers`'s `problem`: no model chosen
  (`unset`), the chosen one **removed from the models** (`missing`), or one that is not a cloud model (`not_cloud`).
  The link text follows ("Pick another in Settings → Runs"). In Settings, a helper model that is no longer available
  still shows, flagged "(not available)", instead of a blank dropdown. Nothing else changes when the model is gone:
  helpers switch themselves off, the delegate tool is not offered and the switch is ignored.

- **2.13 (2026-10-22)** — The composer's Run settings gain **Use helpers** (`RunSettings.vue`).
  - **When it shows:** a row under Profile and Model, shown when the selected profile names `delegates`. It is enabled
    when `GET /admin/api/helpers` says helpers are available; otherwise it is shown disabled with "Pick a Helper model
    in Settings → Runs". It is hidden for a profile that names none.
  - **State:** the switch is off for a new conversation, stays on through follow-ups in the open one, and is off for a
    conversation opened from the list (`useAssistant.helpersOn`). When on, the Run settings button reads
    "Research · helpers".
  - **Send:** `use_helpers` is sent only when the switch is on, the profile names helpers and they are available.
  - **Settings:** it relies on Settings → Runs → Helper model (cloud models only) and Helpers at once.
  - **Reads:** the chat re-reads the helpers status whenever it opens (`useHelpers().refresh`).

- **2.12 (2026-10-18)** — One bell for runs and reminders, and a message queue in the composer. (a) The bell
  (`NotificationBell`, formerly `RunNotifications`) lists runs that ended **and reminders that fired or failed**
  (`NotificationItem` = `RunEndItem | ReminderAlertItem`); a reminder also gets a toast and a badge on the Reminders tab.
  Reminders ride the same global run poll (`useReminderAlerts`, no new timer): the first good poll only learns where "now"
  is, the cursor (the newest event time seen) is kept in `usePersistedState` so what fired while the app was closed is
  announced on return and a reload replays nothing; a poll that fails keeps the cursor. On a phone the panel spans the
  screen under the header (anchored to the bell it was cut off at the left edge). (b) While a chat run is live the composer
  accepts a message and **queues** it (one at a time, with Remove; a second asks before replacing). It is sent once, only
  when that run ended as **completed** (`useAssistant.runEnd`, per run token: an error is `failed` even though a completed
  event follows, a stop is recorded when asked, `loading` is not used because fetching sessions toggles it) and the server
  shows no live chat run (checked up to 10 times over 5 s, since it records the end a moment later); in every other case
  (failed, stopped, another conversation or workspace, the chat was not on screen) the text returns to the box as a draft
  with a toast, and is never sent late. Enter and a Queue button beside Stop do this; sending never cancels the run.

- **2.11 (2026-10-14)** — Redesign v2 (plan `cross-cutting/frontend-redesign-v2.md`). The shell is assistant-first:
  **Assistant** leads the navigation, **Workspaces** leaves the sidebar for a single **Workspace** entry (Files, Memory,
  Playbooks, Heartbeat and the workspace's Settings), and a **workspace chip** in the header names and switches the
  workspace the shell opens (the one last used; Settings says "All workspaces"). A compact **Ask** field opens the
  assistant as a column beside any page (a full-screen dialog on a phone); the app opens on the assistant of the workspace
  last used, else the Overview. On a phone a **bottom tab bar** (Assistant, Workspace, Automations, Reminders, More)
  replaces the drawer. **Settings** has six tabs (General, Models, Safety, Connectors, Appearance, System) over the
  unchanged `/settings/<section>` routes. The assistant's message box starts at three lines (16 px on touch) with a small toolbar under it (Commands, which types the `/` that opens the command menu, and Memory, the review of the conversation for memories); a long
  draft opens a full-screen editor on a phone, history and New are one control (the list's open state persists), and
  an answer reads with deliberate spacing. **Look:** areas are separated by tone, not borders, and corners are 6/8 px.
  **Themes:** five more presets (three dark, two light) and an accent picker over any preset. Changes §II.1 (seven destinations, drawer), the retro design
  language of §IV (2 px corners, hairline panel borders) and the preset registration steps of §II.7. One chat is ever
  live; Workspaces closes its chat when left.

- **2.10 (2026-10-12)** — Reminders page management. **New reminder** (page header and empty state) opens the form
  with the time set to 09:00 tomorrow in the browser's timezone. The form has a **Repeat** switch, so a one-off can
  become repeating and the other way round. Finished reminders (sent, missed, failed) in History have **Reschedule**,
  which opens the same form. A repeating upcoming reminder has **Pause** / **Resume** and **Skip next occurrence**; a
  paused one shows a Paused tag. The Overview summary uses the same row, read-only. Refusals from pause, resume and
  skip appear on the page; refusals from a form appear in the form.

- **2.9 (2026-10-11)** — Reminders page. A seventh destination, **Reminders** (`/reminders`), between Automations and
  Models, lists what the assistant has been asked to remind the user of and lets them edit or delete it (SPEC-011).
  *Upcoming* rows show the time in the reminder's own timezone, the channel and, for a repeating reminder, how often
  and until when; an inline form edits the message, the time (or, for a repeating one, the repeat and its end date)
  and the channel, and shows the server's reason when it refuses. *History* lists sent, missed and failed reminders,
  each deletable. Deleting asks first. Reminders are created by asking the assistant, not on this page. The Overview
  Reminders panel links here.

- **2.8 (2026-10-11)** — Automations from playbooks, next run, test run. Each playbook in a workspace's Playbooks
  section has **Create automation**: it opens `/automations/new?workspace=<ws>&playbook=<id>`, where the form opens
  with that workspace chosen, **New task** selected, the playbook's text filled and the automation named after the
  playbook (all editable). The automation list gains a **Next run** column and the detail page a **Next run** row
  (`<time> (server time)`), from `next_run_at` (SPEC-007 §II.1). After a successful create the new automation opens
  and a prompt offers **Run now** to run it once (the same run a schedule would start) or **Not now**. The form's
  Create button is held back while the server says the chosen schedule is not valid. A playbook card shows **Append**
  only when a file is open to append to.
- **2.7 (2026-10-11)** — Writing an automation's task in the form. When creating, the form's Basics panel offers
  **Existing file** or **New task**. A new task is a text box, optionally started from a playbook (the library's
  playbooks, grouped by category; picking one fills the box and asks before replacing text the user wrote), saved
  as `<automation name>.md` in the workspace when the automation is created. An existing file of that name is
  replaced only after asking (identical text is left alone); declining or a failed write creates nothing. Before
  anything is written the form refuses a name already used in that workspace, and a name whose file is another
  automation's task (shown on the Name field), so a failed create cannot leave another automation running new text. Editing an
  automation keeps the file picker. The playbook library and the form share one "write a file, ask before replacing"
  rule (`useWorkspaceFileWriter`).
- **2.6 (2026-10-11)** — Automation schedule picker. The automation form's Schedule panel is one **Runs** choice
  (Manual only · Every few minutes or hours · Every day · Every weekday · Certain days of the week · Once a month ·
  Custom (cron)) with a time, weekday chips or day-of-month where they apply, so nobody has to write cron. It
  reads the schedule back in plain English and lists the next three runs in the server's time zone (stated on the
  panel), from `POST /admin/api/dispatcher/schedule/preview`. A schedule with no preset (a hand-written cron, a
  `30s` interval) stays under Custom, untouched. A new automation starts as Manual. The list and review use the
  same wording ("Every weekday at 07:00").
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
  retro redesign (`docs/PLANS/ARCHIVE/cross-cutting/frontend-redesign-retro.md`).
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
each workspace a file explorer and an assistant. It runs on a LAN, has one
operator credential (a passphrase; no users or roles; scripts use named tokens),
and makes no network request outside the proxy's own origin.

## II. Functional Requirements

### 1. Shell and navigation
- Eight entries in a persistent, collapsible left sidebar: **Assistant, Overview, Workspace, Automations, Reminders,
  Models, Activity, Settings**. The collapse state persists. Assistant and Workspace open the workspace last used
  (`/workspaces/<ws>/assistant`, `/workspaces/<ws>/files`), or the workspace list while none is known; the current entry
  comes from the route (an assistant route is Assistant, any other workspace route is Workspace). Run-end badges go to
  Assistant for an assistant run and otherwise to the owning destination.
- The app opens (`/`) on the assistant of the workspace last used, else the Overview; the brand links there. The last
  workspace is remembered through `usePersistedState`; one place (`useActiveWorkspace`, started from `App.vue`) keeps it
  valid: a workspace that no longer exists is forgotten, and when none is remembered the first is used.
- Below the desktop breakpoint the sidebar is replaced by a **bottom tab bar**: Assistant, Workspace, Automations,
  Reminders, and **More**, a sheet holding Overview, Models, Activity and Settings. It respects the safe area
  (`viewport-fit=cover`); the content is padded clear of it, and the assistant fills the space between the top strip
  and the bar so its message box stays in reach.
- A header carries the **workspace chip** (the workspace the shell opens; a menu switches it, keeping the section on a
  workspace page, and links to Manage workspaces; Settings, which is about everything, says "All workspaces"), the
  **host stats strip** (§II.8), a compact **Ask** field, the run-activity pill (global lane state, §II.5) and the
  notification bell. A context drawer (Monitor) is available at every width.
- The **Ask** field opens the assistant of the workspace last used as a column beside the page (a full-screen dialog on
  a phone). It resumes the conversation that is open, so a run started on the Assistant page carries on there; it is
  absent on the Assistant page, where the panel is never open, and while no workspace is known. Escape closes it. Only one
  `AssistantChat` is ever live (each owns a live connection): the panel and the Assistant page never coexist, and the
  Workspaces view closes its chat when it is left (the run continues on the server and replays when a chat opens).
- Every page is addressable by URL (§III.2); refreshing a deep link renders
  the same page. An unknown path renders a not-found page **inside the shell** — never a silent redirect.
- The first tab stop is a "Skip to content" link that moves focus to `main#content`.

### 2. Destinations
- **Overview** — health (active model, system metrics), running work with
  lane slot bars, recent runs.
- **Workspace** — the entry for the active workspace (the **Workspaces** list, with create and delete, is reached from
  the chip's "Manage workspaces"); per workspace: **Files** (tree + editor),
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
- **Settings** — global configuration by section (`/settings/<section>`), under six tabs: **General** (four short sections —
  Routing & engine, Runs, Memory & reminders, Storage & logging — each its own `/settings/<section>` page over one form; `/settings/local` is the first), **Models** (local models and each cloud provider's keys and models, as a second
  row), **Safety** (Sandboxing, Limits & reset — the two halves of the security form, which stays one draft and one Save — and Agent Guardrails), **Connectors** (Communication, Search, MCP servers — SPEC-008),
  **Appearance** and **System** (processes). A tab opens its first section; a tab with several shows them as a row that
  scrolls sideways on a phone. Providers discovered at runtime appear under Models.

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
- The bell also lists reminders that fired or failed (SPEC-011 §Changelog 1.6; Changelog 2.12 here); the rules below are
  for runs.
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
- **Network scan** (Settings → Safety → Network scan, and an automation whose agent is `security-auditor`): every screen has a
  **Simple** view (default; plain sentences, one decision at a time) and an **Advanced** view (ports, versions, the full
  list, ranges, the unconfined switch), switched at its top right, remembered per person and shared by all of them, over
  the same data. A **Help** button on each screen opens one side panel; a warning's "Learn more" opens the right part.
  The scope saves on every action (no Save bar); the readiness banner names each missing step and its one fix.
  Accepting a finding ("This is fine") asks how long (30 days, 90 days, 1 year) and takes an optional note.
  What an audit scans (how thoroughly, whether the safe checks run, and under Advanced the networks and ports) is set in the form's "What the audit scans" panel and shown as one line on the audit's page; the server applies it, not the agent. Creating an audit is the ordinary new-automation form started from the **Network audit** playbook (the playbook library lists
  what each playbook sets up); the form and the automation page show what the computer is missing and whether the
  automation's network access cuts the scan tool off, and a failed audit run is explained in plain words with the fix, the
  original message behind "Show details". Settings → Network scan lists the audits that already exist (name, status, schedule, last run, **Open**) instead of offering to set one up; **Add another audit** is Advanced-only. The report also shows the problems the audit's checks found (SMBv1, anonymous FTP, a VNC or Redis with no password, expired certificates and a few more): worst first in Simple, with a severity tag in Advanced, each acceptable like an open port. A device the scan ran out of time on is flagged with an **Allow more time** shortcut to the Advanced "Time allowed per device" setting, and the report reads itself again when a run finishes.

### 9. Sign-in
- Sign-in is compulsory. Until a passphrase exists the admin API answers only the sign-in routes and every page is the
  setup page. The passphrase is set once, on `/setup`, with a one-time code made on the server by
  `llm-proxy auth setup-code`; the UI never makes or shows that code, and the page tells the person which command to run.
- Once a passphrase exists, the router sends every visit without a session to `/login?next=…` and returns to `next`
  (an address inside the app only) after signing in. A 401 from any request does the same; an event stream that fails
  without a reason asks `/auth/status` instead of retrying for ever. The message box's draft is kept in memory per
  workspace across that trip and comes back with the chat.
- A request the server answers `reauth_required` opens the **confirm it's you** dialog, then repeats once the passphrase is
  right. These are the destructive actions (wipe, reset, sandbox switches, secrets, MCP servers, new tokens, a longer
  session, a deep check of a device) and a settings save that changes what the model server runs. A script token cannot confirm a passphrase.
- **Settings → Safety → Sign-in & access** holds: the **Exposure** card (where the server listens, sign-in, encryption,
  inference key, webhook secrets n of m, sandbox; each item that needs attention says what to do), the passphrase, how
  long a sign-in lasts, where you are signed in (sign out one, or everywhere), script tokens (shown once) and the
  recent sign-in log. Nothing there shows a secret except a new token, once.

### 7. Theming
- Built-in presets: `retro-dark-soft` (default; a faded black, easier on the eyes), `retro-dark` (the original
  near-black ink), `retro-dark-lifted`, `retro-midnight`, `retro-forest`, `retro-plum` (more dark themes, each with its
  own accent), `retro-paper` (light), `retro-paper-cool` and `retro-sand` (more light themes). With no stored choice the UI
  follows the system colour scheme: a dark OS gets the default, a light OS gets `retro-paper`. The picker is Settings ·
  Appearance. Adding a preset means a block in `styles/tokens.css`, the id in `PRESET_IDS` (`types/theme.ts`, the one
  TypeScript list), a label in `theme/presets.ts` and the scheme in `public/theme-boot.js` (plain ES5; the boot contract
  test holds it to the list); `frontend/scripts/check-contrast.cjs` reads the ids from `tokens.css`, so the new block is gated at
  once and every pair must pass. A test fails if `tokens.css` and `PRESET_IDS` disagree.
- **Accent**: over any built-in theme the operator can pick one of eight swatches or any colour (Settings · Appearance, stored through `usePersistedState`, applied as overrides of `--accent-brand` and `--focus-ring`). The colour is moved toward white (dark presets) or black (light ones) until it reads on every surface and the preset's label colour reads on it (`theme/accent.ts`), so no preset × accent pair can fail the contrast gate; a unit test proves every preset × swatch and awkward colours. Choosing one while following the OS pins the theme that is showing; a custom theme keeps its own colours and the picker is disabled.
- Custom themes: duplicate a preset or import JSON; both paths run one validator. Only known token names are accepted;
  each value must match its kind's grammar (opaque colour, length, duration). Fonts and easing are not editable. A
  contrast failure blocks saving until the operator acknowledges it; a grammar failure always rejects. Themes export to
  JSON (they are device-local and would otherwise be lost with browser data).

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
- `components/layout/` — shell (sidebar, bottom tab bar, header, workspace chip, ask field and panel, context drawer, skip
  link);
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
| `/` | — | redirect → the assistant of the workspace last used, else `/overview` |
| `/login`, `/setup` | `login`, `setup` | bare (no shell), public: the only routes open to someone not signed in |
| `/overview` | `overview` | |
| `/workspaces` | `workspaces` | list; keep-alive |
| `/workspaces/:ws` | `workspace` | redirect → files |
| `/workspaces/:ws/files/:path(.*)*` | `workspace-files` | nested path in the URL |
| `/workspaces/:ws/assistant/:conversationId?` | `workspace-assistant` | notification target; the Assistant entry |
| `/workspaces/:ws/:section(memory\|playbooks\|heartbeat\|settings)` | `workspace-section` | `/security` redirects to `settings` |
| `/automations` | `automations` | |
| `/automations/new` | `automation-new` | |
| `/automations/recordings` | `automation-recordings` | static segment ranks above `:id` |
| `/automations/:id` | `automation` | |
| `/automations/:id/edit` | `automation-edit` | |
| `/models` | `models` | |
| `/activity` | `activity` | query: `kind`, `status`, `workspace`, `q`, `from`, `to`; keep-alive |
| `/settings/:section?` | `settings` | `workspaceScope: 'all'` (the header chip says "All workspaces") |
| `/design` | `design` | **dev builds only** — living token/primitive reference |
| `/:pathMatch(.*)*` | `not-found` | in-shell |

Keep-alive is opt-in by route meta (Workspaces, Activity); there is no global
`<KeepAlive>`. The kept-alive Workspaces view closes its assistant chat when it is deactivated.

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
- A warm ink canvas with a cream light preset; the brand accent (persimmon by default) is never a state colour; mint /
  cobalt / citrine / red carry success / info / running / error.
- **Tone, not lines.** Panels, the header, the sidebar and the chat are set apart by surface tone (raised on canvas); a
  hairline border stays only where it says "this is an input" (fields, the composer), under the title of a flush panel
  and on a drawer's edge. Corners are 6 px (`--radius-sm`) and 8 px (`--radius-md`).
- The accent appears on the primary button (its hard offset), the current-page marker, focus and the logo.
- **No shadows**, except the primary button's hard brand offset. No glass, no blur.
- Components use semantic token classes only; raw Tailwind palette classes fail `npm run lint`.
- Typography (Inter, JetBrains Mono) is self-hosted; motion durations are tokens. Mono micro-labels and numerals are
  the retro signature.

## V. Accessibility and Performance
- Every preset passes the contrast gate
  (`frontend/scripts/check-contrast.cjs`, run by `npm run lint`; WCAG 4.5:1 for text roles); the
  preset test fails `npm test` on a regression.
- On a touch screen the message box uses 16 px text (a smaller field makes the browser zoom) and send, stop and expand are
  44 px targets; the bottom bar clears the home indicator.
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
- Secrets (provider keys, bot tokens, connector webhook secrets) are write-only in the UI: shown masked, replaced,
  never read back in clear; a masked value sent back means "keep what is stored". The Copy button works on a plain-HTTP
  page (no clipboard API) through a selected hidden box, and says so when nothing can copy; a new token is then selected
  for the person to copy by hand.
- Guardrail approvals from the UI go through the same `GuardrailEngine`
  decision as agent calls (SPEC-006); one approval posts once.
