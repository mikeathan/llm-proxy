---
subsystem: admin-ui
tags: [assistant, automation, mobile, model-runtime, providers]
result: reference
status: reference
last_reviewed: 2026-11-26
date: 2026-11-26
---

# Assistant, automations and service review (2026-11-26, v0.144.0 on vertex)

Asked by the owner after v0.144.0 was merged and deployed: "investigate any improvements we can do in the UI, and general
functionality across assistant, automations… currently assistant is bit noisy especially in a mobile" and "investigate for any
bugs in the service". This file is the evidence. The mocks and the proposed plan that follow from Part A are in
[`../PLANS/assistant-ui/assistant-calm-mobile.md`](../PLANS/assistant-ui/assistant-calm-mobile.md).

**How it was checked.** Service: vertex `/admin/api/app-logs/tail` (505 lines, 2026-11-25 19:41 → 2026-11-26 16:10 restart into
v0.144.0 commit `1a5306f7`), `/state`, `/dispatcher/automations`, `/conversation/sessions/workspace-1`, read with the owner's
admin token. UI: the app's own Vite build on this Mac with Playwright (Chrome, 390×844 and 1440×900, retro-dark-soft), the admin API
answered from fixtures: vertex's two automations and two chats as read, plus one made-up chat with three tool steps, a memory save
and vertex's real provider failure. Screenshots were taken at device scale 2; every number below is a CSS-pixel `getBoundingClientRect` / `scrollHeight` read in the page. The chat's event stream was answered with an empty stream for the U1 check (aborting it made no difference).

Labels: **verified** = reproduced or read in code and logs; **likely** = code and evidence agree, not reproduced end to end;
**owner** = a decision or a config fact only the owner can settle.

## Part B — service bugs

| # | Finding | Evidence | Label |
|---|---|---|---|
| B1 | **A stop releases the lock with `activeModel` already nil, so any start on that port while the old server is still exiting collides.** `signalStopLocked()` sets `activeModel = nil` before the process has exited (`backend/internal/core/llm/lifecycle.go:32-90`). In `Get`'s swap path the caller then unlocks to wait (`backend/internal/core/llm/manager.go:463-470`), and a second caller for the same model takes the lock, sees nothing active and starts on the same port; `reclaim` (`backend/internal/core/llm/providers/local_provider.go:167-197`) then fails it and calls the dying server a foreign process. `UpdateModel` / `RemoveModel` (`manager.go:786-813`) open the same window with `go waiter()`. Fixing only those two would not fix what vertex hit. | vertex log 2026-11-26T15:36:11, lines in order: Qwen3.6 starts → the Inkling caller stops it → a second Qwen3.6 caller starts on :8081 → `ERROR Cannot start local model … port 8081 is held by a process that is not a llama-server started on it` → Qwen3.6 `exited gracefully` (the Inkling caller's wait returns) → Inkling starts. No config change in that minute. | verified (log order + code) |
| B2 | **Changing the log level restarts the running local model** (and so does any other System config save). The System `OnChange` handler pushes the environment to every model through `UpdateModel` (`backend/internal/app/app_context.go:122-133`), which stops the active model whenever its name matches, without comparing the old and new config (`manager.go:786-801`). | vertex log 2026-11-26T10:22:15: `log level updated | level DEBUG` → `System config change detected` → `Signaling stop to local model | model Qwen3.6 35B A3B`; again at 10:22:16 (back to INFO), then a fresh start at 10:22:25. Four log-level clicks, two reloads of a 35B model. | verified |
| B3 | **Two callers that want different local models undo each other's start.** `Get` stops whatever is active and starts its own model, returning `ErrModelStarting`; the other caller retries and does the same (`manager.go:451-482`). Nothing arbitrates, so neither becomes ready until one caller gives up. | vertex log 15:35:01–15:36:25: 13 starts alternating Inkling 2 Mini (:8089) and Qwen3.6 (:8081), most stopped in the same second they started, ending in B1's error. This window overlaps the previous session's live check of the small-window work (`docs/PLANS/agent-loop/small-window-tools-handoff-2026-11-26.md`), so the *traffic* was probably test traffic; the missing arbitration is not. The run lanes (`runlane`) admit runs, but a stack model call or a `/v1` caller for another local model can still trigger the swap. | likely (cause in code; callers not identified from the log) |
| B4 | **The two opencode-hosted cloud models fail every request.** `glm-5.3-flash` and `deepseek-v4.1-flash` (provider label `nvidia`, URL `https://opencode.ai/inference/go/openai/v1/…`) answer `400 MissingSessionID: Request is missing x-opencode-session`. The OpenAI-compatible provider sends only auth headers (`provider_openai_compatible.go:350`, `setAuthHeaders`); there is no way to add a header a provider requires. Primary is Qwen3.6, fallback LFM2.5 (both local), so chat still works while a local model can run. In session `conv_20261121161931` the owner tried glm (21:02:45) and then deepseek (21:03:06); both failed. (That chat's first turn, 2026-11-21 on Inkling 2 Mini, failed with "prompt too large for the model's context window", which the v0.144.0 small-window work addresses.) | vertex log; the turns' stored `run.model` and `error`; `/state` | verified (failure); **owner**: is opencode meant to sit under the `nvidia` entry, and should providers take extra headers (a provider manifest field) |
| B5 | **Model overrides for removed models are kept forever.** Every start logs `ApplyModelOverrides: model not found` for six names, including `zz-add-check-1` and `zz-add-check-2` (test leftovers written to the live install) and `Agents A1.5`, `Sage-A1.5-Q4_K_M`, `nanbeige4.2-3b`, `nemotron-3-ultra-550b-a55b:free`. Removing a model does not remove its override. | vertex log 16:10:20 | verified; minor; **owner**: clean once |
| B6 | Serving-context reconcile warned on every model start before v0.144.0 (`stale metadata corrected`, 4 times for Qwen3.6). The 16:10 start of v0.144.0 already loads `max_tokens 5461 context_budget 43692` from `--ctx-size`, so this should stop. | vertex log | fixed by v0.144.0 — check the next model start logs no WARN |

## Part A — assistant and automations UX

### Bugs in the UI

| # | Finding | Evidence | Label |
|---|---|---|---|
| U1 | **Opening a chat does not land on the latest message.** After load the pane sits 334 px (phone) / 288 px (desktop) above the bottom, still there after 8 s, with the jump button showing. `settleLoadedSession` → `scrollToLatest` does one *smooth* scroll after a single `nextTick` (`AssistantChat.vue:156-162, 334-346`); the Markdown, tables and the memory block lay out after it, and the smooth scroll's own scroll events mark the pane "not at bottom", so the follow-up `maybeScroll` does not follow (`ChatMessages.vue:70-100`, `useAutoScroll.ts:59`). | measured `scrollHeight - scrollTop - clientHeight` | verified (symptom); cause likely |
| U2 | **A failed turn hides its error.** The turn shows only "● Worked 1s" (a red dot); the error is inside the folded step list. The stored text is already a sentence ("The model server rejected the request (HTTP 400): Request is missing x-opencode-session and cannot be routed efficiently…"), but it is folded away, says nothing about what to do, and offers no action. The owner's rule is a plain sentence plus the fix, raw text behind a toggle (`feedback-ux-no-hidden-knowledge`). "Worked" is also the wrong verb for a turn that failed. | `now-phone-failed.png` (its fixture used the log's raw line, so the opened text there is rawer than vertex stores) | verified (folded, no fix) |
| U3 | **The model label under a failed turn loses its first letter** ("lm-5.3-flash"). With no answer there is no Copy button, but `.turn-footer` keeps `-ml-1.5` (`ChatBubble.vue:383`), so the label starts 6 px left of the reading column (measured left edge 6 px, column 12 px) and its first glyph is not drawn. | element crop | verified (symptom); exact clipping ancestor not pinned |
| U4 | **The Send icon reads as a warning sign.** The `send` icon is an upward paper plane (`m12 19 9 2-9-18-9 18zm0 0v-8`), which at 16 px on the primary button is a triangle with a bar: ⚠. | `ChatInput.vue:202` | verified |

### Noise (phone first)

| # | Finding | Where else the same primitive is used |
|---|---|---|
| N1 | **Chrome takes 237 of 844 px (28%) on a phone; the messages get 607.** App header (logo, workspace chip, tokens/s pill, bell, run dot) 48 px + chat strip (☰ +, title, ✕) 44 px + composer and its Commands/Memory row 89 px + bottom tabs 56 px. Each row is reasonable alone; together, with a second row of icons in each bar, it reads as busy. | App header and `BottomTabs` are on every page. |
| N2 | **The tokens/s pill and CPU/MEM show at idle** ("0.0 T/S") on every page; on a phone it takes a header slot the chat icon then lacks. | `HostStats.vue` in `AdminHeader` |
| N3 | **Every finished turn carries a footer** (copy icon · model · tokens) in mono; three turns = three footers. "Saved to memory · remove it in the Memory panel" is an uppercase mono label. | `ChatBubble` is the renderer for chat *and* live automation runs (`ChatMessages` in `AutomationDetails.vue`). |
| N4 | **Thoughts are full paragraphs in the step list** ("THOUGHT" + the model's reasoning text between every tool step), so opening the steps doubles the turn. | Same timeline in automation live runs. |
| N5 | **The conversation drawer shows three icons on every row** (pin, rename, delete), a delete-all icon in its header and another on the Webhook group: seven bins and pencils on one screen. | `ChatSessionList.vue` only. |
| N6 | The empty state shows keyboard hints (Enter, Shift+Enter, Ctrl/⌘ K) on a phone, inside a dashed box. | — |
| N7 | Wide Markdown tables scroll sideways (`.md-table-scroll`, `overflow-x: auto`) with no sign that they do: the Rain column is just cut. | `MarkdownViewer` (4 consumers: chat, reports, newsroom, memory). |
| N8 | Three doors to the assistant on desktop: the Assistant menu entry, the header "Ask the agent…" field (hidden on the Assistant page) and the phone header chat icon; plus a "Monitor" button on the Assistant tab that opens another drawer of runs. | Header, `WorkspacesView`, `AutomationsView`. |

### Automations

| # | Finding | Where else |
|---|---|---|
| M1 | **Two detail layouts.** network-audit has Results · Live run · History · Setup tabs with Simple/Advanced; every other automation is one 2,984 px column: 01 Configuration (12 fields) → 02 Last result → 03 Saved reports → 04 Console → 05 Runs. The result is below the configuration. | `AutomationDetails.vue`; the open plan `assistant-automation-workbench-layout.md` (proposed, gated) already names the long column and the second renderer (`ExecutionAuditTrail`). |
| M2 | **The list on a phone is a label/value card per automation** (AUTOMATION, SCHEDULE, NEXT RUN (SERVER TIME), MODEL, STATUS, ACTIONS) and does not say whether the last run worked or when it ran. | `DataTable` stacks every row this way below `sm` (7 consumers: AutomationList, AutomationDetails runs, RecentRunsCard, ModelsCatalog, WorkspaceList, AppearanceSettings, the design gallery). |
| M3 | **Runs show raw ids** ("d19005ea…") and a red "Clear all runs" button as loud as Run now. | `DataTable` in `AutomationDetails.vue`. |
| M4 | **The last result opens with log lines** (`[2026-11-18 06:58:21] ▶ Executing automation …`, `Reading task file: …`, `⏱ Duration`) before "Final Report". These come from the backend's `last_output`. | Same text in the Results tab and the dashboard's last-result views. |
| M5 | The Console section explains its own storage ("keeps the latest run's steps in memory only, so they are gone after a restart") when nothing is running. | — |
| M6 | Numbered section headings (`01 ── TITLE`) on detail pages that are not a sequence. | `SectionHeading` via `Panel` (76 files): changing it changes every page, so the mocks keep it and only drop the title where a tab already names the content. |

Not findings (fixture artefacts, recorded so they are not re-reported): "Not scanned yet" on network-audit and "Saved reports:
loading" (the fixtures did not answer those endpoints); the 379 px phone width (pitfall 76, already in the ledger).
