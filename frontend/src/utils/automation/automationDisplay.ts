import type { Automation } from '../../types/dispatcher'
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
