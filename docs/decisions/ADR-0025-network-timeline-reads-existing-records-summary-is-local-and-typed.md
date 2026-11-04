---
id: ADR-0025
title: The Network timeline reads records the program already keeps, and its model summary is local-only and typed-fields-only
status: accepted
date: 2026-11-04
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0025: The Network timeline reads records the program already keeps, and its model summary is local-only and typed-fields-only

## Context
The owner asked (2026-11-04) for "one timeline" of what happened on the network, with an optional plain-language "explain this week" from the local model (plan `network-attempts-integrity-timeline`, P4). The Overview already listed new devices and address changes; the alert log, the daily history and the last audit's baseline hold the rest. Network data must not leave the machine: a scheduled audit is refused when its model is not local (`auditModelIsLocal`, `ModelNotLocalCode`).

## Decision
1. **The timeline is a read, not a store.** `netaudit.ActivityOf` joins the inventory, the alert log, the daily series and the last audit's baseline into typed rows (`kind`, `group`, `at`, a few numbers). It keeps nothing, starts no scan and sends no packet. A row's words are composed by the screen from its kind and typed fields; the one text in a row is an alert's own message, which the program wrote and the bell already shows.
2. **One correlation rule, stated as timing only.** A new device that appeared less than an hour before a decoy alert gives a `related` row ("appeared within an hour before a decoy service was touched; the timing is all the program can say"). The alert log keeps no typed source address, so a stronger rule would need parsing message text, which is refused.
3. **Rows are bounded and quiet.** Default 20 rows, at most 200; a port row exists only on a day something opened or closed; the last audit is a row only when it found something new or saw something go.
4. **The model summary is a button, nothing automatic.** `POST /admin/api/netaudit/activity/summary` asks the audit workspace's model once. Three rules in code, each with a test and a mutation check: (a) the prompt is built only from the program's fixed words per kind, "how long ago" and numbers (`SummaryPrompt`); a device name, address, hardware address or an alert's text never enters it, and an alert kind outside the known list is left out; (b) the model must be **local** (`modelIsLocal`, the workload classifier), checked after the lease and before the call, and a model that cannot be found is refused; (c) one summary at a time, a time limit, and a reply with no answer is an error, never an empty summary. The answer is cleaned (thinking removed, 1,200 characters) and shown as text. Failures show fixed wording; the model's or provider's own error text is logged and never sent to the page.
5. **Tab name.** The tab is "Timeline". "Activity" is already a main navigation page (the run feed), and two things with one name was found by a test on desktop.

## Consequences
- A cloud workspace model gets a 409 with the reason; the owner changes the workspace's model to use the button.
- Host findings have no timestamps, so the timeline has no "Host" filter; host changes reach it only through the audit row.
- The router's block counts (plan P2) will add a row kind and a filter when they exist; nothing else here changes.
- The summary can be wrong; the screen says so and says what the model was and was not shown.
