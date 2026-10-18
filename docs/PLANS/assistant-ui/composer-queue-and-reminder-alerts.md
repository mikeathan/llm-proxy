# Composer message queue and live reminder alerts

**Status:** proposed (2026-10-18) — two findings from the owner's phone testing, planned together as one frontend follow-up; no code yet.
**Related:** SPEC-003 (web UI), SPEC-011 (Reminders), SPEC-001 (Agent Loop, run replacement).

## 1. Problem

| # | Finding | Evidence |
|---|---------|----------|
| 1 | While a run is live the composer takes typing, but nothing sends it: Enter is ignored, the send button is replaced by Stop, and the draft waits. The placeholder says "you can write your next message while it finishes". Nothing tells the user that sending is blocked, and sending would replace (cancel) the running run. | `ChatInput.vue` `onKeydown` (`if (!props.loading) emit('send')`); `AssistantChat.vue` `handleSend` (`if (!text \|\| loading.value) return`); `assistant_handlers.go` run replacement |
| 2 | A reminder that fires gives the web UI **no live signal**, whichever channels it goes to. The "in-app notice" is only the persisted reminder row (served by `GET /admin/api/reminders`) and a log line; the frontend has run notifications (bell, toasts, tab badges, from the global lane poll) but nothing for reminders. Reported with web UI plus Telegram selected: Telegram arrives, the web UI shows nothing. The web-UI-only case has not been tested live, but by the code it behaves the same: the row changes and nobody is told. | `app/reminders.go` `reminderNotice.Notify` (logs only); SPEC-011 §2.4; `App.vue` run notifications; no reminder code in `useRunNotifications.ts` |

## 2. Design

### 2.1 Queue one message while a run is live

- Sending while a run is live puts the message in a **queued** line under the box ("Queued: …", with Remove). One slot: a second send replaces the queued text after a confirmation, so a long queue cannot build up.
- When the run **completes**, the queued message is sent automatically through the normal send path. When it **fails, is stopped or is cancelled**, it is not sent: it returns to the box as a draft with a short note, so a failed run never fires a message the user wrote against a different answer.
- Stop stays the only way to cancel; sending never cancels.
- The placeholder and the button state say what will happen (a send button labelled for queueing while a run is live; Stop stays visible beside it).
- State lives in the existing conversation composable, per conversation, cleared on conversation switch; the queued text is also kept as the unsent draft across a reload (existing draft storage, try/catch).

### 2.2 Tell the web UI when a reminder fires

Choose one source of truth for "a reminder fired", recommended first:

1. **Poll a small cursor endpoint (recommended).** `GET /admin/api/reminders/fired?after=<id or time>` returns reminders delivered since the cursor (id, message, time, late flag, channels). The shell polls it on the same cadence as the existing global lane poll (no new timer), shows a toast and a badge on the Reminders tab, and keeps the cursor in memory plus `localStorage` so a reload does not replay old ones. Simple, works behind any proxy, no new long-lived connection.
2. **Push on the event bus.** Publish a `reminder_fired` event from `reminderNotice.Notify` and add a global live channel. Faster, but the frontend only has per-workspace live channels today, so it adds a new connection and replay rules (the replay-buffer eviction bug is recorded in memory).

**Where it shows:** the top-right bell, the shell's existing notification point (`RunNotifications` in the header's actions slot, fed by `useRunNotifications`; it lists run endings with dismiss and clear). A fired reminder becomes a second kind of item in that same list, so the bell's count, its dropdown, dismiss and clear work for both, with a toast for a new one and a badge on the Reminders tab. Opening or dismissing an item clears its part of the badge. The run-activity pill beside it keeps meaning "runs in progress" and is not used for reminders.

Both: the notice is raised for **every** delivery outcome that reaches the user (including `(late)` and a partial delivery), regardless of connectors, because the web UI is always a recipient (SPEC-011 §2.4, "the web UI list always has every reminder"). A reminder skipped as `missed` is shown as missed, not as a notification. Optionally (separate switch, off by default) a browser `Notification` when the tab is hidden, asked for permission only after the user turns it on.

## 3. Acceptance

- Queue: with a run live, sending shows a queued line and does not cancel; on completion it sends once; on failure or stop it returns to the box; Remove works; reload keeps the draft. Unit tests for the queue state machine (completed / failed / stopped / replaced / switched conversation) and a component test for the composer states.
- Reminders: a reminder delivered to the web UI only, and one delivered to web UI plus Telegram, each raise one toast and the tab badge within one poll; a reload does not replay them; a late and a partial delivery are labelled; opening the Reminders tab clears the badge. Backend test for the cursor endpoint (ordering, cursor, bounded page); frontend test for the poll composable.
- Visual check at 320 to 390 px that the queued line and toast do not widen the page (the sideways-scroll fix in PR #89 must keep holding).
- SPEC-003 and SPEC-011 updated; this plan's row moved to complete.

## 4. Open questions for the owner

- Poll (recommended) or push for reminder alerts?
- Browser notifications for a hidden tab: wanted now, or later?
- Queue depth: one message (recommended) or several?

## Remaining Work

All of it.
