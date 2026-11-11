import { describe, expect, it } from 'vitest'
import { footprintResultText } from '../../../utils/netaudit/footprint'
import type { FootprintResult } from '../../../types/netaudit'

const NOW = Date.parse('2026-11-10T12:00:00Z')
const result = (over: Partial<FootprintResult>): FootprintResult => ({ check: 'certificates', status: 'looked', at: '2026-11-10T11:00:00Z', looked_at: '2026-11-10T11:00:00Z', ...over })

describe('footprintResultText', () => {
  it('says each check in plain words from its typed facts', () => {
    expect(footprintResultText(result({ new_certs: 2 }), NOW)).toBe('2 new certificates since the last look.')
    expect(footprintResultText(result({}), NOW)).toBe('No new certificates since the last look.')
    expect(footprintResultText(result({ check: 'expiry', expires_at: '2026-11-20T12:00:00Z' }), NOW)).toBe('Registered for 10 more days.')
    expect(footprintResultText(result({ check: 'expiry', expires_at: '2026-11-01T12:00:00Z' }), NOW)).toBe('The registration has ended.')
    expect(footprintResultText(result({ check: 'mail_records', spf: 'weak', dmarc: 'none' }), NOW)).toBe('SPF: weak. DMARC: none.')
    expect(footprintResultText(result({ check: 'breaches', breaches: 1 }), NOW)).toBe('Found in 1 known breach.')
    expect(footprintResultText(result({ check: 'breaches' }), NOW)).toBe('Not found in any known breach.')
  })

  // A look that could not be made is never "fine": the reason is shown, and what was known before it, marked as last known.
  it('says why a look could not be made and never reads it as clean', () => {
    const failed = result({ status: 'could_not_look', reason_text: 'The service did not answer.', new_certs: 0 })
    expect(footprintResultText(failed, NOW)).toBe('The service did not answer. Last known: No new certificates since the last look.')
    expect(footprintResultText({ ...failed, looked_at: undefined }, NOW)).toBe('The service did not answer.')
    expect(footprintResultText(undefined, NOW)).toBe('Not looked at yet.')
  })

  it('marks an old look as no longer known', () => {
    expect(footprintResultText(result({ stale: true }), NOW)).toContain('not known any more')
  })
})
