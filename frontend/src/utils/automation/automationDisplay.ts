import cronstrue from 'cronstrue'
import type { Automation } from '../../types/dispatcher'
import type { StatusState } from '../../types/ui'

const DEFAULT_QUEUE_POSITION = 1

/** A Go duration without the zero units it prints ("2h0m0s" → "2h"). */
function shortDuration(value: string): string {
  return value.replace(/(\D)0m/, '$1').replace(/(\D)0s$/, '$1')
}

/** A human description of when an automation runs. */
export function triggerLabel(auto: Pick<Automation, 'trigger' | 'trigger_value'>): string {
  const value = auto.trigger_value ?? ''
  switch (auto.trigger) {
    case 'cron':
      try {
        return cronstrue.toString(value)
      } catch {
        return `Invalid schedule: ${value}`
      }
    case 'interval':
      return `Every ${shortDuration(value)}`
    case 'manual':
      return 'Manual only'
    default:
      return auto.trigger
  }
}

/** The status tag for an automation: running, queued, last run failed, or idle. */
export function automationStatus(auto: Automation): { state: StatusState; label: string } {
  if (auto.is_running) return { state: 'running', label: 'Running' }
  if (auto.queued) return { state: 'queued', label: `Queued #${auto.queue_position ?? DEFAULT_QUEUE_POSITION}` }
  if (auto.last_error) return { state: 'error', label: 'Last run failed' }
  return { state: 'neutral', label: 'Idle' }
}
