import { HEALTH_WORDS, MODULE_BLOCKED_WORD, MODULE_LAST_RAN_PREFIX, MODULE_OFF_WORD, TRAFFIC_WORDS } from '../../constants/stacks'
import { formatRelativeTime, isRecorded } from '../format/time'
import type { ModuleStatus, StackModule } from '../../types/stacks'

const SEPARATOR = ' · '
const ON_WORD = 'On'

/**
 * The one quiet line under a module: its state, what it sends, and what stops it. A module that is switched on but blocked
 * says Blocked and is never called working. A module with no monitor of its own is simply On. A module that is on also says when it
 * last ran, and shows the stack's own phrase for a problem in amber.
 */
export function moduleStatus(m: StackModule, nowMs: number = Date.now()): ModuleStatus {
  const sends = TRAFFIC_WORDS[m.sends]
  if (!m.enabled) return { text: [MODULE_OFF_WORD, sends, m.blocked].filter(Boolean).join(SEPARATOR), tone: 'muted' }
  if (m.blocked) return { text: [MODULE_BLOCKED_WORD, sends, m.blocked].join(SEPARATOR), tone: 'warning' }
  if (m.health === 'stopped') return { text: [HEALTH_WORDS.stopped, sends].join(SEPARATOR), tone: 'warning' }
  const ran = isRecorded(m.last_run) ? `${MODULE_LAST_RAN_PREFIX} ${formatRelativeTime(m.last_run, nowMs)}` : ''
  const text = [m.health ? HEALTH_WORDS[m.health] : ON_WORD, sends, ran, m.problem].filter(Boolean).join(SEPARATOR)
  return { text, tone: m.problem ? 'warning' : 'success' }
}

/** A layer's number: the bottom layer is 01, and the server lists them top first. */
export function layerNumber(index: number, count: number): string {
  return String(count - index).padStart(2, '0')
}
