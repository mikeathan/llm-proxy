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

export type NotificationTone = 'success' | 'warning' | 'error' | 'muted'

/** One bell entry in words (utils/notifications/present.ts). */
export interface NotificationRow {
  /** The outcome in words. */
  label: string
  tone: NotificationTone
  text: string
  /** A failed run's error, shown under the row. */
  detail?: string
}

/** Everything the bell lists: a run that ended, or a reminder that fired. */
export type NotificationItem = RunEndItem | ReminderAlertItem

/** Items that arrived in the same poll tick, shown as one notification. */
export interface RunNotification {
  id: string
  createdAt: number
  items: NotificationItem[]
}
