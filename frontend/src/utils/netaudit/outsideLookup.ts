import {
  OUTSIDE_LOOKUP_FAILED, OUTSIDE_LOOKUP_INTERNET_OFF, OUTSIDE_LOOKUP_LAST_LOOKED, OUTSIDE_LOOKUP_LISTS, OUTSIDE_LOOKUP_NO_PORTS, OUTSIDE_LOOKUP_NOT_YET, OUTSIDE_LOOKUP_ON_ADDRESS,
  OUTSIDE_LOOKUP_PORTS_SEEN, OUTSIDE_LOOKUP_REASON_OTHER, OUTSIDE_LOOKUP_REASONS, OUTSIDE_LOOKUP_UNKNOWN,
} from '../../constants/netaudit'
import type { OutsideLookupLast } from '../../types/netaudit'

/**
 * The one line under the switch: what the last look did, in words. A look that could not be made says so and never reads as
 * "nothing is open". `when` is the look's time already written for a person ("4 minutes ago").
 */
export function outsideLookupLine(last: OutsideLookupLast, when: string): string {
  switch (last.outcome) {
    case 'looked':
      return last.ports === 0
        ? `${OUTSIDE_LOOKUP_LAST_LOOKED} ${when}: ${OUTSIDE_LOOKUP_LISTS} ${OUTSIDE_LOOKUP_NO_PORTS} ${OUTSIDE_LOOKUP_ON_ADDRESS}`
        : `${OUTSIDE_LOOKUP_LAST_LOOKED} ${when}: ${OUTSIDE_LOOKUP_LISTS} ${last.ports} open ${last.ports === 1 ? 'port' : 'ports'} ${OUTSIDE_LOOKUP_ON_ADDRESS} ${OUTSIDE_LOOKUP_PORTS_SEEN}`
    case 'cannot_look':
      return `${OUTSIDE_LOOKUP_REASONS[last.reason ?? ''] ?? OUTSIDE_LOOKUP_REASON_OTHER} ${OUTSIDE_LOOKUP_UNKNOWN}`
    case 'failed':
      return `${last.reason === OUTSIDE_LOOKUP_INTERNET_OFF ? OUTSIDE_LOOKUP_REASONS[OUTSIDE_LOOKUP_INTERNET_OFF] : OUTSIDE_LOOKUP_FAILED} ${OUTSIDE_LOOKUP_UNKNOWN}`
    default:
      return OUTSIDE_LOOKUP_NOT_YET
  }
}
