---
id: SPEC-011
title: Reminders
version: "1.4"
status: stable
last_updated: 2026-10-13
constitution_references: [II.2, II.14, V]
related_specs: [SPEC-009, SPEC-001]
supersedes:
---

# SPEC: Reminders

## Changelog

- **1.4 (2026-10-13)** — Creating a reminder no longer needs the model to do calendar arithmetic or a person to state
  their timezone. (a) **Local clock on every turn:** a context note appended to the turn's user message, for the model only and never
  stored, carries `Now (<zone>): Mon 12 Oct 2026 20:29 (UTC+01:00)` and the next 14 days, so "Sunday the 18th" is read off
  it and the date is right however long the conversation has been open (the system prompt stays byte-identical). (b) **One chat zone:** a conversation reports
  `reminders.ChatZone(owner, browser)` — the owner setting, else the zone the browser reported, else UTC (connector
  messages have no browser zone) — and `schedule_reminder` reads a local `when` in that zone unless the user named one
  (`models.WithChatTimezone`). (c) **`when` takes a local date and time** (`2026-10-18 20:00`, no offset);
  RFC 3339 with an offset still works and wins. (d) **`/remind <when> <message>`** is a slash command answered in code
  with no model (`core/commands`, grammar in §2.11); the same registry serves the web chat (`GET /admin/api/commands`,
  `POST /admin/api/commands/run|preview`) and every connector through the inbound webhook, where a reminder is
  answered on the channel it came from. The chat box shows a command menu, usage and a live preview.

- **1.3 (2026-10-12)** — Edit gaps and pause. (a) A finished reminder (sent, missed, failed) is **rescheduled** by editing
  it: it becomes scheduled again with its failure record cleared; a cancelled one stays closed. (b) An edit can turn a
  one-off into a repeating reminder and back. (c) Reminders can be **created** from the UI (`POST /admin/api/reminders`;
  zone = the browser's, else the owner setting, else UTC; creator `ui`). (d) A repeating reminder can be **paused** (new
  status `paused`: listed, counted against the open limit, never fires, still cancellable), **resumed** (next occurrence
  after now; what was silenced is not replayed; refused if its end date has passed) and have its **next occurrence
  skipped**. (e) Concurrency: a reminder carries a revision counter bumped by every user change; the delivery loop writes a
  result back only if the revision is unchanged, so a send that was in flight can no longer overwrite an edit, pause or skip
  made meanwhile (the user's change wins).

- **1.2 (2026-10-12)** — A connector is used only because it was named, chosen as the default, or is the one the
  request came through — never because it happens to be the only one configured. A reminder with no channel and no
  default now goes to the web UI only (previously the single configured connector was picked). The Settings option
  "Automatic" is now **Web UI only**; a reminder's own channel choice "Default channel" follows the setting. Internally,
  where a reminder goes is decided in one place (`ChannelPolicy`, `core/reminders/channel.go`) and what a delivery
  outcome does to a reminder in one pure function (`Policy.Advance`, `transition.go`); behaviour of retries, late and
  missed handling is unchanged.

- **1.1 (2026-10-11)** — Managing reminders. The Reminders page (SPEC-003 2.9) lists, edits and deletes them through
  `PUT` and `DELETE /admin/api/reminders/{id}` (§4). An edit replaces message, time or recurrence, end and channel under
  the same rules as creation, keeps the reminder's timezone, starts delivery afresh (attempts reset) and re-arms the
  loop; a refused edit changes nothing (400) and an edit or delete of an unknown or no-longer-open reminder is a 404.
  `in-app` is a valid explicit channel. Delete removes a reminder in any status; a delete made during a send wins.

- **1.0 (2026-10-11)** — First version. The assistant can store a one-off or recurring reminder and the service delivers
  it verbatim at the right time. This is the reminder half of the proposed Agenda plan
  ([`agenda-reminders-and-followups.md`](../PLANS/autonomy/agenda-reminders-and-followups.md)); self-scheduled follow-ups
  (runs) and `RunReason` remain in that plan and will extend this SPEC.

## 1. Intent

"Remind me tomorrow morning to call the bank." The reminder text is delivered exactly as stored, at the time given, to a
chosen connector or the default one. No model runs when it fires, so it costs nothing and cannot be garbled.

## 2. Functional requirements

1. **Creation.** The agent tools (attended runs only — hidden from and refused to unattended automations, which may be
   steered by content they read) `schedule_reminder`, `list_reminders` and `cancel_reminder` (category `reminders`) are the
   only creators. `schedule_reminder` takes `message` and exactly one of `when` (a local `YYYY-MM-DD HH:MM` in the conversation's zone, or RFC 3339 with offset) or `cron` (five
   fields or a descriptor — the one grammar of `core/automation/cron.go`, no seconds), plus optional `until`, `timezone`
   (IANA) and `connector` (a connector *type*, e.g. `telegram`).
2. **Echo.** The tool result states the id, absolute local time, timezone and channel; the model must repeat it so a wrong
   guess is visible. Vague phrases map in the tool description: morning 09:00, afternoon 14:00, evening 18:00,
   tonight 20:00, in the reminder's timezone.
3. **Validation (refuse, store nothing).** Blank message; both or neither of `when`/`cron`; a time in the past or more
   than a year ahead; an unknown timezone; a recurrence that fires more often than every 15 minutes or has an `until` in
   the past; a named connector type that is not configured; more than 50 open reminders.
4. **Channel.** A named connector is used as given (`in-app` is a valid choice). Otherwise a request that arrived through a
   connector (chat app) is answered on it if it is still configured (`models.OriginConnector`, set by the inbound webhook
   run). Otherwise the reminder follows `reminders.default_connector` if that type is configured, else it is delivered
   **in-app only**. A connector is never chosen just because it is available, and no connector name is hard-coded. The
   choice stored on a reminder is "" (follow the default, resolved at send time), `in-app` or a connector type. Every
   outcome is also recorded for the in-app list.
5. **Timezone.** `timezone` argument → `reminders.timezone` setting → `UTC` (never a bare "Local", which names no zone a
   user or browser can read). A connector-originated chat
   reports the owner zone (`reminders.timezone`, else `UTC`) to the agent instead of a hard-coded `UTC`
   (`WebhookHandler.OwnerTimezone`). The zone is stored with the reminder, so recurrence follows that zone across DST.
6. **Delivery.** One timer loop (`reminders.Service.Run`, started by the app lifecycle, tethered to the root context)
   sleeps until the earliest scheduled time and is woken by create/cancel. Text is sent through
   `CommunicationTools.NotifyAllPlain` with a 30 s timeout: a connector with a `SendPlain` path (Telegram) sends it with
   no markup parsing, so a Markdown link or `*x*` arrives as typed. A one-off becomes `done`; a recurring reminder advances to its next
   occurrence after now (never replays missed occurrences).
7. **Late and missed.** Overdue by ≤ 2 min is on time. Overdue up to 12 h (service was down) is delivered with a
   `(late) ` prefix. Overdue more than 12 h becomes `missed` (recurring: advances) and is **not** sent to a connector.
8. **Failure.** A failed send is retried after 1 min, then 5 min; after 3 attempts a one-off is `failed` with the error and
   a recurring reminder reports the failure and moves on to its next occurrence. A reminder is never `done` unless a send
   returned nil (or the channel is in-app only). Delivery is at-least-once: if a send succeeds but the store cannot save
   the result, the reminder is held back 30 s and may be sent again; a connector that fails after part of the text
   was sent can repeat that part on retry.
9. **Cancel wins.** A state write only applies to a reminder that is still `scheduled`; a cancel made while a send is in
   flight is never reversed.
10. **Verbatim.** The message is stored and sent as text; it is never interpreted, templated into a prompt, or logged.
11. **`/remind` grammar.** `/remind <when> <message>` (also `/remind@bot` in Telegram). `<when>` is read from the left by
    `reminders.ParseCommand` and the first word that is not part of it starts the message, kept as typed. Day: `today`,
    `tomorrow`, `tonight` (20:00), `[next|this] <weekday>` (the next one, never today), `18[th] [of] oct [year]`,
    `oct 18 [year]`, `18/10[/yyyy]` (day first), `2026-10-18`; a month and day already past mean next year. Time:
    `[at] 8pm | 8:30pm | 20:00 | noon | midnight | morning | afternoon | evening` — a bare number is not a time. A time alone (`8pm call mum`) is today if still ahead, else tomorrow. A day with no time
    means 09:00. `in 2h | 30 minutes | 3 days | 1 week`. Recurring: `every day|weekday|weekend|<weekday> [time]` (cron).
    A word ending in `:` closes the time part (`tomorrow 9am: March for charity`). Anything unreadable is refused with
    the word named and the usage; nothing is stored. The same validation as the tools applies (`Service.Plan`).

## 3. Data model

Table `reminders` in the shared SQLite database (`core/reminders/store.go`): `id, message, fire_at (UTC unix),
cron, until_at, timezone, connector, status (scheduled|paused|done|cancelled|missed|failed), rev, attempts, last_error, created_by,
created_at`. Settings (`settings.yml`, `reminders:`): `default_connector`, `timezone`; both optional, saved whole,
timezone validated at save (400).

## 4. API

`GET /admin/api/reminders` → `{open: [...], recent: [...]}` (open includes paused), `POST /admin/api/reminders` (create),
`PUT /admin/api/reminders/{id}` (edit / reschedule), `POST /admin/api/reminders/{id}/pause|resume|skip` and
`DELETE /admin/api/reminders/{id}` (see [api-reference](../api-reference.md#reminders-api)). The Settings page edits
`reminders` through `PUT /admin/api/config`; the Overview page summarises the list and the Reminders page manages it.

## 5. Error handling

Tool errors are returned to the model as errors (the reminder was not saved). A failed store or loop iteration is logged
and retried on the next wake; the loop exits only when its context is cancelled. If the database or connector registry is
unavailable at start-up, reminders are disabled and everything else runs.
