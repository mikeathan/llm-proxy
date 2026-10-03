import type { ChoiceOption } from '../../types/ui'
import type { HeartbeatResult } from '../../types/heartbeat'
import type { TriggerType } from '../../types/automation'

/** The workspace file that holds the checks (models.HeartbeatFilename). */
export const HEARTBEAT_TASK_FILE = 'heartbeat.md'
/** The reserved name of the heartbeat's automation (models.HeartbeatAutomationName). */
export const HEARTBEAT_NAME = 'heartbeat'
export const HEARTBEAT_DEFAULT_EVERY = '30m'

const HEARTBEAT_EVERY_PRESETS: ChoiceOption[] = [
  { value: '5m', label: '5 minutes' },
  { value: '15m', label: '15 minutes' },
  { value: '30m', label: '30 minutes' },
  { value: '1h', label: '1 hour' },
  { value: '2h', label: '2 hours' },
  { value: '6h', label: '6 hours' },
]

/** The interval presets, in order; a saved value that is not one of them stays selectable. */
export function heartbeatEveryOptions(current: string): ChoiceOption[] {
  if (!current || HEARTBEAT_EVERY_PRESETS.some((o) => o.value === current)) return HEARTBEAT_EVERY_PRESETS
  const options = [...HEARTBEAT_EVERY_PRESETS, { value: current, label: current }]
  return options.sort((a, b) => durationMinutes(a.value) - durationMinutes(b.value))
}

const UNIT_MINUTES: Record<string, number> = { m: 1, h: 60 }
function durationMinutes(value: string): number {
  const match = /^(\d+)([mh])$/.exec(value)
  return match ? Number(match[1]) * UNIT_MINUTES[match[2]!]! : Number.MAX_SAFE_INTEGER
}

const RESULT_TEXT: Record<HeartbeatResult, string> = {
  quiet: 'nothing to report',
  alert: 'an alert was sent',
  skipped_no_checks: 'skipped, no checks defined',
  skipped_busy: 'skipped, the model was busy',
  error: 'failed',
}

/** How a check ended, in words. */
export function heartbeatStatusText(result: HeartbeatResult): string {
  return RESULT_TEXT[result]
}

const LOCAL_MODEL_NOTICE =
  'Every scheduled run will start the local model. Choose a cloud connection to keep the local model asleep.'
const DEFAULT_MODEL_NOTICE =
  'The workspace default may be a local model, which every scheduled run would start. Choose a cloud connection to keep the local model asleep.'
const LOCAL_CONNECTION = 'local'

/**
 * A note when a scheduled run would start the local model. providerKey is the
 * form's connection: '' = workspace default, 'local', or a cloud 'provider/key'.
 */
export function modelWakeNotice(triggerType: TriggerType, providerKey: string): string {
  if (triggerType === 'manual') return ''
  if (providerKey === LOCAL_CONNECTION) return LOCAL_MODEL_NOTICE
  return providerKey === '' ? DEFAULT_MODEL_NOTICE : ''
}
