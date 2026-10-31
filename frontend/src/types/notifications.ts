import type { RouteLocationNamedRaw } from 'vue-router'
import type { Destination } from './routes'

// Run notifications (plan Phase 4, D5/D20): terminal transitions only.
// An assistant run can only be known to have ended — the lane snapshot has no
// status — so it is never "completed" or "failed". An automation's outcome
// comes from its run record (AutomationRun.error); without one it also "ended".
export type RunEndOutcome = 'ended' | 'completed' | 'failed'
export type RunEndKind = 'automation' | 'assistant'

export interface RunEndItem {
  /** Run identity (lane key + start time). */
  id: string
  kind: RunEndKind
  label: string
  workspace: string
  /** Set for automation runs (the lane key is the automation id). */
  automationId?: string
  outcome: RunEndOutcome
  /** The run's error text, for a failed automation. */
  error?: string
  target: RouteLocationNamedRaw
  destination: Destination
}

// What a reminder's last delivery did: reached the owner (on time, late, or to only some of its channels), could not be
// sent, or was found too late to send. Mirrors the backend events.
export type ReminderEventKind = 'due' | 'late' | 'partial' | 'failed' | 'missed'

/** One outcome as GET /admin/api/reminders/events reports it. */
export interface ReminderEvent {
  id: number
  message: string
  event: ReminderEventKind
  /** When the outcome was saved, unix milliseconds: the cursor to ask from next. */
  at: number
  /** Where the reminder went: a connector type, a list of them, or "in-app". */
  channel: string
}

export interface ReminderEvents {
  /** The newest event time the server has; send it back as `after`. */
  cursor: number
  /** Newest first. Empty on a call with no cursor. */
  items: ReminderEvent[]
}

/** A reminder that fired (or failed to), as an entry of the bell. */
export interface ReminderAlertItem {
  /** Reminder id and event time, so one outcome is announced at most once. */
  id: string
  kind: 'reminder'
  message: string
  event: ReminderEventKind
  channel: string
  target: RouteLocationNamedRaw
  destination: Destination
}

// What the Network module alerted about, as GET /admin/api/netaudit/alerts reports it. The text is the exact message sent to
// the operator's connectors; `delivery` says whether it got there.
export type NetworkAlertKind = 'decoy' | 'presence' | 'summary'
export type NetworkAlertDelivery = 'sending' | 'sent' | 'no_connector' | 'failed'

export interface NetworkAlertEvent {
  /** Rises strictly; an entry that is updated (a repeating decoy source, a changed delivery) takes a new one. */
  id: number
  kind: NetworkAlertKind
  message: string
  delivery: NetworkAlertDelivery
  /** How many times a repeating decoy alert was raised for the same sources. */
  count: number
  /** Unix milliseconds. */
  at: number
}

export interface NetworkAlertEvents {
  /** The newest alert id the server has; send it back as `after`. */
  cursor: number
  /** Newest first. Empty on a call with no cursor. */
  items: NetworkAlertEvent[]
}

/** A Network alert, as an entry of the bell. */
export interface NetworkAlertItem {
  id: string
  kind: 'network'
  alertKind: NetworkAlertKind
  message: string
  delivery: NetworkAlertDelivery
  count: number
  /** Same kind and text: a newer entry replaces the earlier row, so a repeating alert is one row. */
  repeatKey: string
  target: RouteLocationNamedRaw
  destination: Destination
}

/** What the browser offers for notices, so the logic can be tested without a browser (composables/netaudit/useBrowserAlerts.ts). */
export interface BrowserAlertsEnv {
  supported: boolean
  permission: () => NotificationPermission
  requestPermission: () => Promise<NotificationPermission>
  raise: (title: string, options: NotificationOptions) => void
  /** Whether the tab is hidden, the only time a notice is raised. */
  hidden: () => boolean
}

export type NotificationTone = 'success' | 'warning' | 'error' | 'muted'

/** One bell entry in words (utils/notifications/present.ts). */
export interface NotificationRow {
  /** The outcome in words. */
  label: string
  tone: NotificationTone
  text: string
  /** A failed run's error, or what became of an alert's send, shown under the row. */
  detail?: string
  /** A way to put right what the row says went wrong. */
  fix?: { label: string; target: RouteLocationNamedRaw }
}

/** Everything the bell lists: a run that ended, a reminder that fired, or a Network alert. */
export type NotificationItem = RunEndItem | ReminderAlertItem | NetworkAlertItem

/** Items that arrived in the same poll tick, shown as one notification. */
export interface RunNotification {
  id: string
  createdAt: number
  items: NotificationItem[]
}
