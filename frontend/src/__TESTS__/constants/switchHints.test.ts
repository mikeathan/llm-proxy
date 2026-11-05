import { describe, expect, it } from 'vitest'
import * as labels from '../../constants/labels'
import * as netaudit from '../../constants/netaudit'

// A switch keeps one short sentence visible; whatever more there is to say goes in its `details`, under "More about this".
const SHORT = 90
const SWITCHES: Array<[string, string, string]> = [
  ['PRESENCE', netaudit.PRESENCE_HINT, netaudit.PRESENCE_DETAILS],
  ['IPV6', netaudit.IPV6_HINT, netaudit.IPV6_DETAILS],
  ['NAMES', netaudit.NAMES_HINT, netaudit.NAMES_DETAILS],
  ['FOLLOW_UP', netaudit.FOLLOW_UP_HINT, netaudit.FOLLOW_UP_DETAILS],
  ['DECOYS', netaudit.DECOYS_HINT, netaudit.DECOYS_DETAILS],
  ['ROUTER_LOG', netaudit.ROUTER_LOG_HINT, netaudit.ROUTER_LOG_DETAILS],
  ['OUTSIDE_LOOKUP', netaudit.OUTSIDE_LOOKUP_HINT, netaudit.OUTSIDE_LOOKUP_DETAILS],
  ['SCAN_CHECKS', netaudit.SCAN_CHECKS_HINT, netaudit.SCAN_CHECKS_DETAILS],
  ['SCAN_KNOWN_VULNS', netaudit.SCAN_KNOWN_VULNS_HINT, netaudit.SCAN_KNOWN_VULNS_DETAILS],
  ['SCAN_UNCONFINED', netaudit.SCAN_UNCONFINED_HINT, netaudit.SCAN_UNCONFINED_DETAILS],
  ['AUTOMATION_JOURNAL', labels.AUTOMATION_JOURNAL_HINT, labels.AUTOMATION_JOURNAL_DETAILS],
  ['AUTOMATION_SKIP_BUSY', labels.AUTOMATION_SKIP_BUSY_HINT, labels.AUTOMATION_SKIP_BUSY_DETAILS],
]

describe('switch hints', () => {
  it.each(SWITCHES)('%s keeps its hint to one short sentence and has details to open', (_name, hint, details) => {
    expect(hint.length).toBeLessThanOrEqual(SHORT)
    expect(hint.match(/[.!?](\s|$)/g)?.length ?? 0).toBeLessThanOrEqual(1)
    expect(details.length).toBeGreaterThan(hint.length)
  })
})
