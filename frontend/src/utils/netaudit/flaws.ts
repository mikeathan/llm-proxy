import type { ScanFinding, VulnFlaw } from '../../types/netaudit'

// The flaws of a known_vulns finding, ready to list. The server sends the ids and the exploited ones as sorted sets and
// each score beside them; the order a person wants (exploited first, then the most serious) is made here.

const CVE_ID = /^CVE-\d{4}-\d{4,}$/
const NVD_DETAIL = 'https://nvd.nist.gov/vuln/detail/'

export const nvdUrl = (id: string): string => `${NVD_DETAIL}${id}`

export function flawsOf(f: Pick<ScanFinding, 'cves' | 'exploited' | 'cve_scores'>): VulnFlaw[] {
  const exploited = new Set(f.exploited ?? [])
  const flaws = (f.cves ?? []).filter((id) => CVE_ID.test(id)).map((id) => ({ id, score: f.cve_scores?.[id] ?? null, exploited: exploited.has(id) }))
  return flaws.sort((a, b) => Number(b.exploited) - Number(a.exploited) || (b.score ?? -1) - (a.score ?? -1) || a.id.localeCompare(b.id))
}

/** One short line for a place: how many known flaws, and how many of them attackers are known to use. Empty without a flaw list. */
export function flawSummary(f: Pick<ScanFinding, 'cves' | 'exploited' | 'cve_scores'>): string {
  const flaws = flawsOf(f)
  if (!flaws.length) return ''
  const exploited = flaws.filter((x) => x.exploited).length
  return `${flaws.length} known ${flaws.length === 1 ? 'flaw' : 'flaws'}${exploited ? ` · ${exploited} exploited by attackers` : ''}`
}
