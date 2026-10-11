---
id: SPEC-011
title: Reminders
version: "1.0"
status: stable
last_updated: 2026-10-11
constitution_references: [II.2, II.14, V]
related_specs: [SPEC-009, SPEC-001]
supersedes:
---

# SPEC: Reminders

## Changelog

- **1.0 (2026-10-11)** — First version. The assistant can store a one-off or recurring reminder and the service delivers
  it verbatim at the right time. This is the reminder half of the proposed Agenda plan
  ([`agenda-reminders-and-followups.md`](../PLANS/autonomy/agenda-reminders-and-followups.md)); self-scheduled follow-ups
  (runs) and `RunReason` remain in that plan and will extend this SPEC.

## 1. Intent

"Remind me tomorrow morning to call the bank." The reminder text is delivered exactly as stored, at the time given, to a
chosen connector or the default one. No model runs when it fires, so it costs nothing and cannot be garbled.

## 2. Functional requirements

1. **Creation.** The agent tools `schedule_reminder`, `list_reminders` and `cancel_reminder` (category `reminders`) are the
   only creators. `schedule_reminder` takes `message` and exactly one of `when` (RFC 3339, with offset) or `cron` (five
   fields or a descriptor — the one grammar of `core/automation/cron.go`, no seconds), plus optional `until`, `timezone`
   (IANA) and `connector` (a connector *type*, e.g. `telegram`).
2. **Echo.** The tool result states the id, absolute local time, timezone and channel; the model must repeat it so a wrong
   guess is visible. Vague phrases map in the tool description: morning 09:00, afternoon 14:00, evening 18:00,
   tonight 20:00, in the reminder's timezone.
3. **Validation (refuse, store nothing).** Blank message; both or neither of `when`/`cron`; a time in the past or more
   than a year ahead; an unknown timezone; a recurrence that fires more often than every 15 minutes or has an `until` in
   the past; a named connector type that is not configured; more than 50 open reminders.
4. **Channel.** A named connector is used as given. Otherwise the default is: `reminders.default_connector` if that type
   is configured, else `telegram` if configured, else the first configured type, else **in-app only**. Every outcome is
   also recorded for the in-app list.
5. **Timezone.** `timezone` argument → `reminders.timezone` setting → server local zone. A connector-originated chat
   reports the owner zone (`reminders.timezone`, else `UTC`) to the agent instead of a hard-coded `UTC`
   (`WebhookHandler.OwnerTimezone`). The zone is stored with the reminder, so recurrence follows that zone across DST.
6. **Delivery.** One timer loop (`reminders.Service.Run`, started by the app lifecycle, tethered to the root context)
   sleeps until the earliest scheduled time and is woken by create/cancel. Text is sent through
   `CommunicationTools.NotifyAll` with a 30 s timeout. A one-off becomes `done`; a recurring reminder advances to its next
   occurrence after now (never replays missed occurrences).
7. **Late and missed.** Overdue by ≤ 2 min is on time. Overdue up to 12 h (service was down) is delivered with a
   `(late) ` prefix. Overdue more than 12 h becomes `missed` (recurring: advances) and is **not** sent to a connector.
8. **Failure.** A failed send is retried after 1 min, then 5 min; after 3 attempts the reminder is `failed` with the error.
   A reminder is never `done` unless a send returned nil (or the channel is in-app only).
9. **Verbatim.** The message is stored and sent as text; it is never interpreted, templated into a prompt, or logged.

## 3. Data model

Table `reminders` in the shared SQLite database (`core/reminders/store.go`): `id, message, fire_at (UTC unix),
cron, until_at, timezone, connector, status (scheduled|done|cancelled|missed|failed), attempts, last_error, created_by,
created_at`. Settings (`settings.yml`, `reminders:`): `default_connector`, `timezone`; both optional, saved whole,
timezone validated at save (400).

## 4. API

`GET /admin/api/reminders` → `{open: [...], recent: [...]}` (see [api-reference](../api-reference.md#reminders-api)). The
Settings page edits `reminders` through `PUT /admin/api/config`; the Overview page lists the result.

## 5. Error handling

Tool errors are returned to the model as errors (the reminder was not saved). A failed store or loop iteration is logged
and retried on the next wake; the loop exits only when its context is cancelled. If the database or connector registry is
unavailable at start-up, reminders are disabled and everything else runs.
