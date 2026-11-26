import type { Automation, AutomationRun } from '../../types/dispatcher'
import type { StatusState } from '../../types/ui'
import { describeSchedule, fromTrigger } from './schedule'
import { formatRunTime } from './runTime'

const DEFAULT_QUEUE_POSITION = 1

/** A human description of when an automation runs. */
export function triggerLabel(auto: Pick<Automation, 'trigger' | 'trigger_value'>): string {
  switch (auto.trigger) {
    case 'cron':
    case 'interval':
    case 'manual':
      return describeSchedule(fromTrigger(auto.trigger, auto.trigger_value ?? ''))
    default:
      return auto.trigger
  }
}

/** When the scheduler will next run it, on the server's clock ("Mon 12 Oct, 07:30"); empty for a manual automation. */
export function nextRunLabel(auto: Pick<Automation, 'next_run_at'>): string {
  return auto.next_run_at ? formatRunTime(auto.next_run_at) : ''
}

/** The status tag for an automation: running, queued, last run failed, or idle. */
export function automationStatus(auto: Automation): { state: StatusState; label: string } {
  if (auto.is_running) return { state: 'running', label: 'Running' }
  if (auto.queued) return { state: 'queued', label: `Queued #${auto.queue_position ?? DEFAULT_QUEUE_POSITION}` }
  if (auto.last_error) return { state: 'error', label: 'Last run failed' }
  return { state: 'neutral', label: 'Idle' }
}


/** The most recent run in an automation's history, or null when it never ran. */
export function latestRun(auto: Pick<Automation, 'history'>): AutomationRun | null {
  let latest: AutomationRun | null = null
  for (const run of auto.history ?? []) {
    if (!latest || Date.parse(run.timestamp) > Date.parse(latest.timestamp)) latest = run
  }
  return latest
}

// Log lines the runner writes ahead of a run's answer ("[2026-11-18 06:58:21] ▶ Executing automation …",
// "Reading task file: …", "⏱ **Duration:** 1m 9s", the "### Final Report" heading). The page shows when, how long and
// where the report was saved itself, so a result opens on the report.
const PREAMBLE_LINES = [/^\[\d{4}-\d{2}-\d{2} [\d:]+\] ▶ Executing automation/, /^Reading task file:/, /^⏱ \*\*Duration:\*\*/, /^#{1,4} Final Report\s*$/i]

/** An automation's output without the runner's preamble; text without one is returned unchanged. */
export function reportBody(output: string): string {
  const lines = output.split('\n')
  let start = 0
  const isPreamble = (line: string) => line === '' || PREAMBLE_LINES.some((p) => p.test(line))
  while (start < lines.length && isPreamble((lines[start] ?? '').trim())) start++
  return lines.slice(start).join('\n')
}

/** One line on how an automation last went, for a list read at a glance: "worked 14 h ago in 4m 7s". */
export function lastRunSummary(auto: Automation, relative: (iso: string) => string, duration: (ms: number) => string): { state: StatusState; text: string } {
  if (auto.is_running) return { state: 'running', text: 'running now' }
  if (auto.queued) return { state: 'queued', text: `queued #${auto.queue_position ?? DEFAULT_QUEUE_POSITION}` }
  const run = latestRun(auto)
  if (!run) return { state: 'neutral', text: 'not run yet' }
  const took = run.duration_ms ? ` in ${duration(run.duration_ms)}` : ''
  if (run.error) return { state: 'error', text: `failed ${relative(run.timestamp)}` }
  if (run.warnings?.length) return { state: 'warning', text: `worked with warnings ${relative(run.timestamp)}${took}` }
  return { state: 'success', text: `worked ${relative(run.timestamp)}${took}` }
}
