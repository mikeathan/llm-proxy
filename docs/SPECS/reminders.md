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

1. **Creation.** The agent tools (attended runs only — hidden from and refused to unattended automations, which may be
   steered by content they read) `schedule_reminder`, `list_reminders` and `cancel_reminder` (category `reminders`) are the
   only creators. `schedule_reminder` takes `message` and exactly one of `when` (RFC 3339, with offset) or `cron` (five
   fields or a descriptor — the one grammar of `core/automation/cron.go`, no seconds), plus optional `until`, `timezone`
   (IANA) and `connector` (a connector *type*, e.g. `telegram`).
2. **Echo.** The tool result states the id, absolute local time, timezone and channel; the model must repeat it so a wrong
   guess is visible. Vague phrases map in the tool description: morning 09:00, afternoon 14:00, evening 18:00,
   tonight 20:00, in the reminder's timezone.
3. **Validation (refuse, store nothing).** Blank message; both or neither of `when`/`cron`; a time in the past or more
   than a year ahead; an unknown timezone; a recurrence that fires more often than every 15 minutes or has an `until` in
   the past; a named connector type that is not configured; more than 50 open reminders.
4. **Channel.** A named connector is used as given. Otherwise a request that arrived through a connector (chat app) is
   answered on that connector if it is still configured (`models.OriginConnector`, set by the inbound webhook run). Otherwise
   the default is `reminders.default_connector` if that type is configured, else the only configured connector, else
   **in-app only** — with several connectors and no setting nothing is guessed, and no connector name is hard-coded.
   Every outcome is also recorded for the in-app list.
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
