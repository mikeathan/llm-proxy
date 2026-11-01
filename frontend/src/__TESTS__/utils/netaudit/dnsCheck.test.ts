import { describe, expect, it } from 'vitest'
import { dnsCheckFreshness, dnsCheckLine, dnsCheckReport, dnsDifferenceRows } from '../../../utils/netaudit/dnsCheck'
import type { DnsCheckState } from '../../../types/netaudit'

const NEVER = '0001-01-01T00:00:00Z'
const state = (summary: Partial<DnsCheckState['summary']> = {}, problem?: string): DnsCheckState => ({
  enabled: true, interval_minutes: 60, allowed_intervals: [15, 30, 60], names: ['dns.google'], default_names: ['dns.google'], server: '',
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
  it('names the guardrail when local network access is off, and says the router was not asked', () => {
    const line = dnsCheckLine(state({ comparisons: 9, matches: 9 }, 'lan_off'), 'Mon')
    expect(line).toContain('Local network access is off')
    expect(line).toContain('Local network (LAN)')
    expect(line).toContain('nothing was sent')
  })
  it('names the guardrail when internet access is off, whatever was recorded before', () => {
    expect(dnsCheckLine(state({ comparisons: 9, matches: 9 }, 'internet_off'), 'Mon')).toContain('Internet access is off')
  })
})

describe('dnsCheckReport', () => {
  it('carries the settings the results were made under, so a paste explains itself', () => {
    const report = dnsCheckReport({ ...state({ comparisons: 3, matches: 3 }, 'internet_off'), server: '192.168.1.53', interval_minutes: 30 }) as Record<string, unknown>
    expect(report).toMatchObject({
      names: ['dns.google'], server: '192.168.1.53', resolver_url: 'https://r.example/q', interval_minutes: 30, problem: 'internet_off',
    })
    expect((report.summary as { comparisons: number }).comparisons).toBe(3)
  })
  it('says the default gateway when no address is set, and an empty problem when there is none', () => {
    expect(dnsCheckReport(state())).toMatchObject({ server: 'default gateway', problem: '' })
  })
})

const NOW = Date.parse('2026-11-01T20:00:00Z')
const at = (minutesAgo: number) => new Date(NOW - minutesAgo * 60_000).toISOString()
const withRun = (lastRun: string, last = NEVER, interval = 60): DnsCheckState => ({ ...state({ last }), interval_minutes: interval, status: { last_run: lastRun } })

describe('dnsCheckFreshness', () => {
  it('says when the last check was made', () => {
    const got = dnsCheckFreshness(withRun(at(12)), NOW)
    expect(got.text).toBe('Last checked 12 minutes ago.')
    expect(got.late).toBe(false)
  })
  it('stays quiet on the due time itself and up to twice the interval', () => {
    expect(dnsCheckFreshness(withRun(at(119)), NOW).late).toBe(false)
  })
  it('warns when no check was made for more than twice the interval, saying how often one is due', () => {
    const got = dnsCheckFreshness(withRun(at(121)), NOW)
    expect(got.late).toBe(true)
    expect(got.text).toContain('every 60 minutes')
    expect(got.text).toContain('may have stopped')
  })
  it('follows the interval that is set', () => {
    expect(dnsCheckFreshness(withRun(at(40), NEVER, 15), NOW).late).toBe(true)
    expect(dnsCheckFreshness(withRun(at(40), NEVER, 30), NOW).late).toBe(false)
  })
  it('says no check has run since the program started, and when the last comparison was, rather than "never"', () => {
    const got = dnsCheckFreshness(withRun(NEVER, at(300)), NOW)
    expect(got.text).toContain('since the program started')
    expect(got.text).toContain('5 hours ago')
    expect(got.late).toBe(false)
  })
  it('says nothing when there has been nothing at all', () => {
    expect(dnsCheckFreshness(withRun(NEVER), NOW)).toEqual({ text: '', late: false })
  })
})

const diff = (name: string, minutesAgo: number, router: string[], outside: string[], overlap = false) => ({
  at: at(minutesAgo), name, outcome: 'differ' as const, overlap, router, outside,
})

describe('dnsDifferenceRows', () => {
  it('lists the newest differences with both address lists, in the order given', () => {
    const { rows, hidden } = dnsDifferenceRows(state({ differs: 2, differences: [diff('dns.google', 5, ['10.0.0.1'], ['8.8.8.8', '8.8.4.4']), diff('dns.quad9.net', 70, ['1.2.3.4'], ['9.9.9.9'], true)] }).summary, NOW)
    expect(hidden).toBe(0)
    expect(rows.map((r) => r.name)).toEqual(['dns.google', 'dns.quad9.net'])
    expect(rows[0]).toMatchObject({ router: '10.0.0.1', outside: '8.8.8.8, 8.8.4.4', partly: false })
    expect(rows[0]!.when).toContain('5 minutes ago')
    expect(rows[1]!.partly).toBe(true)
  })
  it('shows at most five and counts the rest from the total', () => {
    const many = Array.from({ length: 8 }, (_, i) => diff('dns.google', i + 1, ['1.1.1.1'], ['2.2.2.2']))
    const { rows, hidden } = dnsDifferenceRows(state({ differs: 12, differences: many }).summary, NOW)
    expect(rows).toHaveLength(5)
    expect(hidden).toBe(7)
  })
  it('says "no address" for an empty list and shows nothing when there is no difference', () => {
    expect(dnsDifferenceRows(state({ differs: 1, differences: [diff('dns.google', 1, [], ['8.8.8.8'])] }).summary, NOW).rows[0]!.router).toBe('no address')
    expect(dnsDifferenceRows(state().summary, NOW)).toEqual({ rows: [], hidden: 0 })
  })
})
