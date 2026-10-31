import { toSettings } from '../../router/routes'
import type { NetworkAlertItem, NetworkAlertKind, NotificationItem, NotificationRow, NotificationTone, ReminderAlertItem, ReminderEventKind, RunEndItem } from '../../types/notifications'

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

const NETWORK_LABEL: Record<NetworkAlertKind, string> = {
  decoy: 'decoy',
  presence: 'devices',
  summary: 'summary',
}

const NETWORK_TONE: Record<NetworkAlertKind, NotificationTone> = {
  decoy: 'error',
  presence: 'warning',
  summary: 'muted',
}

const NETWORK_DETAIL_SEPARATOR = ' · '
const NOT_SENT_NO_CONNECTOR = 'Not sent to your connectors: none is set up.'
const NOT_SENT_FAILED = 'Not sent to your connectors: sending failed.'
const FIX_NO_CONNECTOR = 'Set up a connector'
const FIX_FAILED = 'Check your connectors'

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

function summarizeNetwork(items: NetworkAlertItem[]): string {
  return items.length === 1 ? items[0]!.message : `${items.length} network alerts`
}

const isReminder = (item: NotificationItem): item is ReminderAlertItem => item.kind === 'reminder'
const isNetwork = (item: NotificationItem): item is NetworkAlertItem => item.kind === 'network'
const isRun = (item: NotificationItem): item is RunEndItem => !isReminder(item) && !isNetwork(item)

/** The line for one notification (a batch of runs, reminders or network alerts): what a toast announces and a bell entry heads with. */
export function summarizeBatch(items: NotificationItem[]): string {
  const runs = items.filter(isRun)
  if (runs.length) return summarizeRuns(runs)
  const network = items.filter(isNetwork)
  return network.length ? summarizeNetwork(network) : summarizeReminders(items.filter(isReminder))
}

function describeNetwork(item: NetworkAlertItem): NotificationRow {
  const notes: string[] = []
  if (item.count > 1) notes.push(`Raised ${item.count} times`)
  const row: NotificationRow = { label: NETWORK_LABEL[item.alertKind], tone: NETWORK_TONE[item.alertKind], text: item.message }
  if (item.delivery === 'no_connector' || item.delivery === 'failed') {
    notes.push(item.delivery === 'no_connector' ? NOT_SENT_NO_CONNECTOR : NOT_SENT_FAILED)
    row.fix = { label: item.delivery === 'no_connector' ? FIX_NO_CONNECTOR : FIX_FAILED, target: toSettings('communication') }
  }
  if (notes.length) row.detail = notes.join(NETWORK_DETAIL_SEPARATOR)
  return row
}

export function describeNotification(item: NotificationItem): NotificationRow {
  if (item.kind === 'network') return describeNetwork(item)
  if (item.kind === 'reminder') return { label: REMINDER_LABEL[item.event], tone: REMINDER_TONE[item.event], text: item.message }
  return {
    label: item.outcome,
    tone: RUN_TONE[item.outcome],
    text: item.kind === 'assistant' ? `assistant · ${item.workspace}` : item.label,
    ...(item.error ? { detail: item.error } : {}),
  }
}
