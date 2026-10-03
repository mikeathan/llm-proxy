import type { AutomationFormData, NotifyConfig } from '../../types/automation'

/** Days a reported item is remembered when the field is left empty (backend default). */
export const DEFAULT_DEDUP_DAYS = 60
/** The longest retention the backend accepts (models.MaxDedupDays). */
export const MAX_DEDUP_DAYS = 3650

const DELIVERY_OFF_LABEL = 'Not sent'
const WAITS_LABEL = 'Waits its turn'
const SKIPS_LABEL = 'Skips the run'

/**
 * The retention typed into the form: undefined when empty (use the default),
 * null when it is not a whole number from 1 to MAX_DEDUP_DAYS.
 */
export function parseDedupDays(text: string): number | null | undefined {
  const trimmed = text.trim()
  if (trimmed === '') return undefined
  if (!/^\d+$/.test(trimmed)) return null
  const days = Number(trimmed)
  return days > 0 && days <= MAX_DEDUP_DAYS ? days : null
}

/** The notify block the form describes; null when results are not delivered. */
export function notifyFromForm(data: Pick<AutomationFormData, 'notifyConnector' | 'notifyDedup' | 'notifyDedupDays' | 'notifySendEmpty'>): NotifyConfig | null {
  if (!data.notifyConnector) return null
  const days = data.notifyDedup ? parseDedupDays(data.notifyDedupDays) : undefined
  return {
    connector: data.notifyConnector,
    dedup: data.notifyDedup,
    dedup_days: typeof days === 'number' ? days : 0,
    send_empty: data.notifySendEmpty,
  }
}

/** One line describing where results go, for the review and details panels. */
export function deliveryLabel(notify: NotifyConfig | null | undefined): string {
  if (!notify?.connector) return DELIVERY_OFF_LABEL
  const parts = [notify.connector]
  if (notify.dedup) parts.push('skips repeats')
  if (notify.send_empty) parts.push('reports quiet runs')
  return parts.join(' · ')
}

/** What a scheduled run does when the model is busy. */
export function busyLabel(skipIfBusy: boolean | undefined): string {
  return skipIfBusy ? SKIPS_LABEL : WAITS_LABEL
}
