import type { NotificationItem, NotificationRow, NotificationTone, ReminderAlertItem, ReminderEventKind, RunEndItem } from '../../types/notifications'

// How the bell speaks about what it lists: the toast that announces a batch, and one row's label, tone and text. Pure, so
// the wording is tested without a component. The outcome is always spelled out; colour only reinforces it.

const REMINDER_LABEL: Record<ReminderEventKind, string> = {
  due: 'fired',
  late: 'late',
  partial: 'partly sent',
  failed: 'failed',
  missed: 'missed',
}

const REMINDER_TONE: Record<ReminderEventKind, NotificationTone> = {
  due: 'success',
  late: 'success',
  partial: 'warning',
  failed: 'error',
  missed: 'error',
}

const RUN_TONE: Record<RunEndItem['outcome'], NotificationTone> = {
  failed: 'error',
  completed: 'success',
  ended: 'muted',
}

const NOT_DELIVERED: ReadonlySet<ReminderEventKind> = new Set(['failed', 'missed'])

function summarizeReminder(item: ReminderAlertItem): string {
  switch (item.event) {
    case 'due':
      return `Reminder: ${item.message}`
    case 'late':
      return `Reminder (late): ${item.message}`
    case 'partial':
      return `Reminder: ${item.message} (not sent to every channel)`
    case 'failed':
      return `Reminder could not be sent: ${item.message}`
    case 'missed':
      return `Reminder missed: ${item.message}`
  }
}

function summarizeReminders(items: ReminderAlertItem[]): string {
  if (items.length === 1) return summarizeReminder(items[0]!)
  const lost = items.filter((item) => NOT_DELIVERED.has(item.event)).length
  return lost ? `${items.length} reminders, ${lost} not delivered` : `${items.length} reminders fired`
}

export function summarizeRuns(items: RunEndItem[]): string {
  if (items.length === 1) {
    const [item] = items as [RunEndItem]
    const what = item.kind === 'assistant' ? `Assistant run in ${item.workspace}` : `Automation ${item.label}`
    return `${what} ${item.outcome}`
  }
  const failed = items.filter((item) => item.outcome === 'failed').length
  return failed ? `${items.length} runs ended, ${failed} failed` : `${items.length} runs ended`
}

/** The line for one notification (a batch of runs, or of reminders): what a toast announces and a bell entry heads with. */
export function summarizeBatch(items: NotificationItem[]): string {
  const reminders = items.filter((item): item is ReminderAlertItem => item.kind === 'reminder')
  const runs = items.filter((item): item is RunEndItem => item.kind !== 'reminder')
  return runs.length ? summarizeRuns(runs) : summarizeReminders(reminders)
}

export function describeNotification(item: NotificationItem): NotificationRow {
  if (item.kind === 'reminder') return { label: REMINDER_LABEL[item.event], tone: REMINDER_TONE[item.event], text: item.message }
  return {
    label: item.outcome,
    tone: RUN_TONE[item.outcome],
    text: item.kind === 'assistant' ? `assistant · ${item.workspace}` : item.label,
    ...(item.error ? { detail: item.error } : {}),
  }
}
