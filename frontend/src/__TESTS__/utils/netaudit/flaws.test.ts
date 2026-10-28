import { describe, expect, it } from 'vitest'
import { flawsOf, nvdUrl } from '../../../utils/netaudit/flaws'

describe('flawsOf', () => {
  it('lists the exploited first, then the highest score, then the id', () => {
    const flaws = flawsOf({
      cves: ['CVE-2024-0001', 'CVE-2024-0002', 'CVE-2024-0003', 'CVE-2024-0004'],
      exploited: ['CVE-2024-0003'],
      cve_scores: { 'CVE-2024-0001': 5, 'CVE-2024-0002': 9.8, 'CVE-2024-0003': 4.3, 'CVE-2024-0004': 9.8 },
    })
    expect(flaws.map((f) => f.id)).toEqual(['CVE-2024-0003', 'CVE-2024-0002', 'CVE-2024-0004', 'CVE-2024-0001'])
    expect(flaws[0]).toMatchObject({ exploited: true, score: 4.3 })
    expect(flaws[1]).toMatchObject({ exploited: false, score: 9.8 })
  })

  it('still lists a flaw whose score is not known, last of its group', () => {
    const flaws = flawsOf({ cves: ['CVE-2024-0001', 'CVE-2024-0002'], cve_scores: { 'CVE-2024-0002': 3 } })
    expect(flaws.map((f) => [f.id, f.score])).toEqual([['CVE-2024-0002', 3], ['CVE-2024-0001', null]])
  })

  it('is empty for a finding without flaws', () => {
    expect(flawsOf({})).toEqual([])
  })

  it('drops anything that is not a CVE id, so nothing odd becomes a link', () => {
    expect(flawsOf({ cves: ['CVE-2024-0001', 'javascript:alert(1)', 'CVE-2024-1'] }).map((f) => f.id)).toEqual(['CVE-2024-0001'])
  })
})

describe('nvdUrl', () => {
  it('points at the flaw on the NVD site', () => {
    expect(nvdUrl('CVE-2024-6387')).toBe('https://nvd.nist.gov/vuln/detail/CVE-2024-6387')
  })
})
