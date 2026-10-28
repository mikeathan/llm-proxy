import { describe, expect, it } from 'vitest'
import { vulnProgress, vulnSources, vulnSummary } from '../../../utils/netaudit/vulnData'
import type { VulnDataStatus } from '../../../types/netaudit'

const NEVER = '0001-01-01T00:00:00Z'
const NOW = Date.parse('2026-10-28T12:00:00Z')
/** The short date as the reader's locale writes it, so the tests hold in any locale. */
const shown = (n: number) => new Date(NOW - n * 86_400_000).toLocaleDateString([], { month: 'short', day: 'numeric' })
const day = (n: number) => new Date(NOW - n * 86_400_000).toISOString()
const status = (over: Partial<VulnDataStatus> = {}): VulnDataStatus => ({
  state: 'idle', files_done: 0, files_total: 6,
  nvd: { imported_at: NEVER, cves: 0, entries: 0 }, kev: { imported_at: NEVER, cves: 0, entries: 0 }, ...over,
})
const held = (age: number, over: Partial<VulnDataStatus> = {}) =>
  status({ state: 'done', files_done: 6, nvd: { imported_at: day(age), cves: 160000, entries: 900000, years: [2022, 2023, 2024, 2025, 2026] }, kev: { imported_at: day(age), cves: 1758, entries: 0 }, ...over })

describe('vulnSummary', () => {
  it('says nothing is downloaded and offers the download', () => {
    expect(vulnSummary(status(), NOW)).toEqual({ line: 'Not downloaded', action: 'download', old: false })
  })

  it('says how far a stopped download got and offers to resume it', () => {
    expect(vulnSummary(status({ state: 'stopped', files_done: 3, reason: 'The connection dropped.' }), NOW)).toEqual({ line: 'Stopped after 3 of 6 files', action: 'resume', old: false })
  })

  it('offers the download again when it stopped before any file finished', () => {
    expect(vulnSummary(status({ state: 'stopped', reason: 'Internet access is off.' }), NOW)).toMatchObject({ line: 'Not downloaded', action: 'download' })
  })

  it('shows the file being fetched, with Cancel', () => {
    expect(vulnSummary(status({ state: 'running', files_done: 1 }), NOW)).toEqual({ line: 'Downloading file 2 of 6', action: 'cancel', old: false })
  })

  it('is up to date with the date, and offers nothing to do', () => {
    expect(vulnSummary(held(3), NOW)).toEqual({ line: `Up to date, ${shown(3)}`, action: null, old: false })
  })

  it('says today when it was just downloaded', () => {
    expect(vulnSummary(held(0), NOW)).toMatchObject({ line: 'Up to date, today', action: null })
  })

  it('calls data older than 60 days old, and still offers an update', () => {
    expect(vulnSummary(held(61), NOW)).toMatchObject({ line: `Old, downloaded ${shown(61)}`, action: 'update', old: true })
  })
})

describe('vulnProgress', () => {
  it('gives the fraction and the megabytes of the current file', () => {
    expect(vulnProgress(status({ state: 'running', files_done: 1, bytes_done: 10 * 1024 * 1024, bytes_total: 40 * 1024 * 1024 }))).toEqual({ fraction: 0.25, text: '10 MB of 40 MB' })
  })

  it('shows only the bytes done while the size is not known', () => {
    expect(vulnProgress(status({ state: 'running', bytes_done: 2 * 1024 * 1024 }))).toEqual({ fraction: 0, text: '2 MB so far' })
  })

  it('is empty before the first byte', () => {
    expect(vulnProgress(status({ state: 'running' }))).toEqual({ fraction: 0, text: 'Starting' })
  })
})

describe('vulnSources', () => {
  it('lists the exploited list and each NVD year with its date and counts, and skips what was never imported', () => {
    const rows = vulnSources(held(3), NOW)
    expect(rows.map((r) => r.name)).toEqual(['CISA exploited list', 'NVD 2022–2026'])
    expect(rows[1]).toMatchObject({ detail: `${shown(3)} · 160,000 CVEs` })
    expect(vulnSources(status(), NOW)).toEqual([])
  })

  it('counts what the last NVD file left out', () => {
    const s = held(3)
    s.nvd = { ...s.nvd, no_config: 44242, rejected: 1093 }
    expect(vulnSources(s, NOW)[1]?.skipped).toBe('Left out of the last file: 44,242 not yet analysed, 1,093 rejected')
  })
})
