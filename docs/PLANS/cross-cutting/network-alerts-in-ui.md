---
status: partial
date: 2026-10-31
last_reviewed: 2026-10-31
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.1, I.2]
evidence: Recon of 2026-10-31 (§2), read from the code; built the same day (v0.30.0), tests and visual specs pass; no live check yet.
---

# Network alerts in the web UI, and clearing the decoy list

**Status:** partial (2026-10-31): built in v0.30.0 and covered by tests; the owner has not yet seen it live. The SPEC-003 change (approved) is recorded as SPEC-003 2.22.
**Related plans:** [`network-guard-module.md`](network-guard-module.md), [`network-detection.md`](network-detection.md).
**Origin:** the owner (2026-10-31), after a decoy alert reached Telegram and the decoy list showed two hits but nothing appeared in the web UI: "I should have an alert or notification in the UI too". Asked in the same message whether the decoy list can be cleared or stays for good. Answers: cover every network alert (decoy touches and presence alerts); write the plan. Later the same day the owner also asked for items 3.6 to 3.9.

## 1. Goal and non-goals

**Goal.** Every alert the Network module sends to connectors also appears on the UI bell, whether or not a connector exists; and the operator can clear the decoy list.

**Non-goals.** New alert kinds beyond the weekly summary (3.6); new wording (the existing fixed messages are reused, so no device-typed text enters an alert, and the operator's device names stay out of them).

## 2. What exists (verified 2026-10-31)

- Both alerts go to connectors only: presence in `Presence.alert` (`core/netaudit/presence.go:131`, message `PresenceMessage`), decoys in `Manager.alert` (`core/netaudit/decoy/decoy.go:309`, message `netaudit.DecoyMessage`); both reach `comm.NotifyAll` (`app/netaudit.go:309`, `:379`). With no connector they fail with "no connector is set up to send alerts" and stay pending; nothing reaches the UI.
- The bell is fed by run events and reminder events only: `useReminderAlerts` → `publishReminders` in `composables/assistant/useRunNotifications.ts`, reading `GET /admin/api/reminders/events?after=<ms>` (`reminder_handlers.go:98`, route `app/routes.go:387`), cursor kept in the browser. No network source exists (searched `utils/notifications` and `composables`).
- The decoy list (`persistence/decoytouches.go`, `decoy.go:280-305`): one entry per source and port, newest first, **capped at 200 entries (the oldest touched is dropped), no age limit, no clear action, no UI button.** It is removed only by a factory reset (`AfterFactoryReset`, `app/netaudit.go:249`). So today a touch stays until 200 newer source-port pairs push it out.
- The decoy alert is sent at most once per 5 minutes (`alertEvery`), covering every source not yet told of.

## 3. Design

**3.1 A bounded network alert log.** One small file store (same shape as `DecoyTouchStore`; factory reset removes it) of the last 50 alerts: `{id, kind (decoy | presence), message, at}`. `id` increases strictly (as `nextEventTime` does for reminders) so a cursor never skips. The message is the exact text sent to connectors, so wording stays fixed.

**3.2 Written at the moment of alerting, independent of connectors.** Both alert paths call one `AlertLog.Add` when they decide to alert, before the connector send. A missing connector therefore no longer hides the alert from the UI. The pending/told logic is unchanged (an alert that could not be sent stays pending and is re-sent), and the log must not gain a second entry for the same pending alert on each retry: add only when the alert is first formed for a given pending set (tested).

**3.3 Read endpoint.** `GET /admin/api/netaudit/alerts?after=<id>&limit=20`, newest-bounded like reminders. Under the existing admin sign-in; no new route family.

**3.4 Bell.** `useNetworkAlerts` mirrors `useReminderAlerts` (rides `useGlobalRunActivity.lastTick`, cursor in `usePersistedState`), publishes through `createRunNotifications`; the row text is the alert message and it links to the Network page (decoys card for decoy alerts). Row wording lives in `utils/notifications/present.ts`.

**3.5 Clear the decoy list.** `DELETE /admin/api/netaudit/decoys/touches` clears all entries (a "Clear list" button on `DecoysCard.vue` with the app's confirm dialog; 44 px target on a phone). Clearing also resets the 5-minute throttle's pending state, since the untold set is gone. Per-entry removal and an age limit are not planned (§6).

**3.6 The weekly summary is a third kind.** It reaches connectors through the same `notifyConnectors` path (`app/netaudit.go:301`), so with no connector it appears nowhere. It joins the log as kind `summary`, with its existing fixed text, written when the summary is formed (once per week; no duplicate on retry).

**3.7 A "not sent to your connectors" marker.** Each log entry records whether the connector send succeeded, was skipped because none is set up, or failed (`sent | no_connector | failed`), updated when the send returns. The bell row says so in plain words and links to the connector settings, in line with the rule that a problem is shown with its fix. The failure reason is not stored beyond that word.

**3.8 One row with a count for a repeating decoy source.** The decoy alert repeats at most every 5 minutes while a source keeps connecting (`alertEvery`). Connectors keep that behaviour unchanged. In the log, a decoy entry whose source set equals the previous decoy entry's, within the last hour, updates that entry (count and last time, the id advances so the cursor sees it) instead of adding a row, so the bell does not fill up.

**3.9 Check whether finished audit runs already reach the bell, and add them only if not.** Phase 1 starts by reading what the run-notification path publishes for an automation run that ends with new high-severity findings (`useRunNotifications.ts`, `utils/notifications/present.ts`, the run events). If a row already exists, this item is closed with a note in the learnings ledger. If not, the run reporter's existing finding counts become one more kind, `findings`, with fixed words.

**3.10 Browser notifications for a hidden tab (last phase, opt-in).** When the tab is hidden and the person has allowed it, a new network alert also raises a browser notification with the same fixed text. The permission is asked for only by an explicit "Notify me in this browser" switch (per browser, kept in browser storage as a per-viewer convenience), never on load. It needs no dependency (the browser's `Notification` API). It works while a tab is open, hidden or in the background; it does not work when the browser is closed (no push service is added), and the page says so. Reminders get no change here; if it works well, extending it to them is a separate decision.

## 4. Strain and safety

No new probe, listener or timer. One small file write per alert; the log is capped at 50 and the decoy list at 200. The endpoint is read-only and signed-in. The log holds only text already sent to connectors.

## 5. Phases

1. **Alert log + write path.** First the 3.9 check (read, no code). Red tests first: presence, decoy and summary paths add one entry; no duplicate on retry; entry exists with no connector; `sent`/`no_connector`/`failed` recorded; a repeated decoy source updates one entry (3.8); cap of 50; ids strictly increase. *Accept:* `go test ./...`, complexity.
2. **Endpoint + clear action.** Handler tests for `after`/`limit`, sign-in required, clear empties the list and the throttle state. *Accept:* handler tests.
3. **Frontend.** Composable, bell row (with the not-sent marker and the count), Clear button with confirm; visual spec for the card change. *Accept:* `npm test`, `npm run build`, `npm run test:visual`.
4. **Browser notifications (3.10).** Switch, permission flow, hidden-tab test with a fake `Notification`; real-browser check by the owner. *Accept:* `npm test`, `npm run build`, `npm run test:visual`, and the owner sees one on a hidden tab.
5. **Docs.** SPEC-003 (approved), network-audit hub, learnings, plan Remaining Work; `./scripts/check-agent-harness.sh`.

## 6. Decisions (owner, 2026-10-31)

- The SPEC-003 change (a new notification source and a new Clear action) is **approved**.
- **No age expiry**: the list changes only by the 200 cap and the Clear button.
- **Clear is all-at-once** for now; per-source removal is not planned.
- A touch from a device the operator has named **still alerts** and appears on the bell (unchanged).

## As built (2026-10-31, v0.30.0)

- **3.9 closed by reading the code:** a finished audit run is an automation run, so `createRunNotifications` already puts "Automation <label> completed | failed | ended" on the bell. It carries no findings count, so no `findings` kind was added (ledger: `learnings/network-audit.md`).
- **3.1–3.3, 3.6–3.8** are `core/netaudit/alertlog.go` (`AlertLog.Send`), written by the presence, decoy and weekly-summary paths through `AppServices.alertNotifier`. Differences from the text above: the delivery has a fourth, transient value `sending` (an entry is written before the send; it is not served until the outcome is known), and an outcome that differs from the stored one gives the entry a new id so a client whose cursor passed it sees the change. A retry of an unsent alert reuses its entry, whatever its kind; only a decoy alert repeating after a successful send updates the entry with a count.
- **3.8, in practice:** `decoy.record` leaves a told source and port told when it connects again, so the count rises only when an already-told source touches a different decoy port (a repeat on the same port is not re-alerted at all). The code handles both; the plan's premise (an alert every 5 minutes while a source keeps connecting) is weaker than written.
- **Clear list leaves the alert log alone:** `network-alerts.json` keeps its (at most 50) messages, source addresses included, and existing bell rows stay until dismissed; only the decoy list and the alert throttle are reset. Clear is an ordinary signed-in request, not step-up (a choice of this build; the decoy switch itself is step-up under I.6).
- **A late success announces again:** an alert that finally sends after a connector is added takes a new id, so it replaces its bell row but toasts (and raises a browser notice with the same tag) a second time.
- **3.5** `DELETE /admin/api/netaudit/decoys/touches` (`decoy.Manager.Clear`), an ordinary signed-in request (not step-up: it removes a list, opens nothing).
- **3.4** `useNetworkAlerts` → `publishNetworkAlerts`; a newer row with the same kind and text replaces the earlier one on the bell, which is how the count shows as one row. A network row is never hidden because the Network page is open.
- **3.10** `useBrowserAlerts` and the **Notices in this browser** panel on Network · Overview (not on the bell).

## Remaining Work

- **Owner check (open):** that a hidden tab shows one browser notice on the owner's browsers (3.10), and that a decoy touch with no connector set up shows on the bell with the "Not sent" link (3.2, 3.7). Neither is verified live; the unit and visual tests use a fake `Notification` and mocked endpoints.
- Not planned (§6): an age limit on the decoy list, per-source removal, browser notices for reminders.
