import { describe, expect, it } from 'vitest'
import { dnsCheckLine } from '../../../utils/netaudit/dnsCheck'
import type { DnsCheckState } from '../../../types/netaudit'

const NEVER = '0001-01-01T00:00:00Z'
const state = (summary: Partial<DnsCheckState['summary']> = {}, problem?: string): DnsCheckState => ({
  enabled: true, interval_minutes: 60, allowed_intervals: [15, 30, 60], names: ['dns.google'], default_names: ['dns.google'],
  resolver_url: 'https://r.example/q', default_resolver_url: 'https://r.example/q',
  status: { last_run: NEVER, problem },
  summary: { comparisons: 0, matches: 0, differs: 0, cannot_compare: 0, overlapping: 0, first: NEVER, last: NEVER, differences: [], ...summary },
})

describe('dnsCheckLine', () => {
  it('says nothing was compared yet, without implying agreement', () => {
    expect(dnsCheckLine(state(), '')).toContain('Nothing compared yet')
  })
  it('says when nothing could be compared, and that this is not agreement', () => {
    const line = dnsCheckLine(state({ cannot_compare: 3 }), '')
    expect(line).toContain('Nothing could be compared')
    expect(line).toContain('does not mean the answers agree')
  })
  it('counts agreements and differences and says it only keeps a record', () => {
    const line = dnsCheckLine(state({ comparisons: 42, matches: 41, differs: 1 }), 'Mon')
    expect(line).toContain('Compared 42 times, starting Mon: 41 agreed, 1 differed.')
    expect(line).toContain('only keeps a record')
  })
  it('counts tries that could not be made apart from the comparisons, with the right plural', () => {
    expect(dnsCheckLine(state({ comparisons: 1, matches: 1, cannot_compare: 1 }), 'Mon')).toContain('Compared 1 time, starting Mon')
    expect(dnsCheckLine(state({ comparisons: 1, matches: 1, cannot_compare: 1 }), 'Mon')).toContain('1 try could not be made.')
    expect(dnsCheckLine(state({ comparisons: 4, matches: 4, cannot_compare: 2 }), 'Mon')).toContain('2 tries could not be made.')
  })
  it('names the guardrail when internet access is off, whatever was recorded before', () => {
    expect(dnsCheckLine(state({ comparisons: 9, matches: 9 }, 'internet_off'), 'Mon')).toContain('Internet access is off')
  })
})
