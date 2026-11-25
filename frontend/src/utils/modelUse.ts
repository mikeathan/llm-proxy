import { MODEL_USE_NO_VALUE, MODEL_USE_SPEED_UNIT } from '../constants/modelStats'
import type { ChartColumn, ModelStatsDay, ModelStatsHour, ModelStatsReport, ModelUse } from '../types/modelStats'

const PERCENT = 100

/** All calls today, local and cloud. */
export const callsToday = (report: ModelStatsReport): number => report.today.reduce((n, m) => n + m.calls, 0)

/** The calls of each day, for the small trend beside the header (oldest first). */
export const trendOf = (days: ModelStatsDay[]): number[] => days.map((d) => d.local + d.cloud)

/** The columns of the week's chart; the tallest day fills the chart and the others are drawn in proportion. */
export function chartColumns(days: ModelStatsDay[]): ChartColumn[] {
  const peak = Math.max(0, ...days.map((d) => d.local + d.cloud))
  return days.map((d) => {
    const total = d.local + d.cloud
    return {
      date: d.date,
      total,
      heightPercent: peak > 0 ? (total / peak) * PERCENT : 0,
      localPercent: total > 0 ? (d.local / total) * PERCENT : 0,
      cloudPercent: total > 0 ? (d.cloud / total) * PERCENT : 0,
    }
  })
}

/** The columns of today's chart by hour: the busiest hour fills the chart; the column's `date` is the hour number. */
export const hourColumns = (hours: ModelStatsHour[]): ChartColumn[] => chartColumns(hours.map((h, i) => ({ date: String(i), local: h.local, cloud: h.cloud })))

/** The short weekday of a YYYY-MM-DD day (a day is read as a day in this browser's zone, never as a moment, so it cannot slip to the day before). */
export function weekdayOf(date: string): string {
  const [y, m, d] = date.split('-').map(Number)
  if (!y || !m || !d) return date
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { weekday: 'short' })
}

/** A model's tokens in and out. */
export const tokensOf = (m: ModelUse): number => m.prompt_tokens + m.completion_tokens

/** A local server's own speed as "41 tok/s", or the no-value mark for a model that reports none. */
export const speedOf = (m: ModelUse): string => (m.tokens_per_sec && m.tokens_per_sec > 0 ? `${Math.round(m.tokens_per_sec)} ${MODEL_USE_SPEED_UNIT}` : MODEL_USE_NO_VALUE)

/** The week's local and cloud totals, for the chart's text summary. */
export function weekTotals(days: ModelStatsDay[]): { local: number; cloud: number } {
  return { local: days.reduce((n, d) => n + d.local, 0), cloud: days.reduce((n, d) => n + d.cloud, 0) }
}
