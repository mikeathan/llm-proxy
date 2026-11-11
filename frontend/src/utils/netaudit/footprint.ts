import { FOOTPRINT_NOT_LOOKED, FOOTPRINT_STALE } from '../../constants/netaudit'
import type { FootprintResult } from '../../types/netaudit'

const MS_PER_DAY = 86_400_000

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`

/** The words for one check's latest answer, from its typed facts and the server's fixed reason: nothing an outside service said is shown. */
export function footprintResultText(r: FootprintResult | undefined, nowMs: number = Date.now()): string {
  if (!r) return FOOTPRINT_NOT_LOOKED
  if (r.status !== 'looked' && !r.looked_at) return r.reason_text ?? FOOTPRINT_NOT_LOOKED
  const facts = factsOf(r, nowMs)
  if (r.status !== 'looked') return `${r.reason_text ?? ''} Last known: ${facts}`.trim()
  return r.stale ? `${FOOTPRINT_STALE} ${facts}`.trim() : facts
}

function factsOf(r: FootprintResult, nowMs: number): string {
  switch (r.check) {
    case 'certificates':
      return r.new_certs ? `${plural(r.new_certs, 'new certificate', 'new certificates')} since the last look.` : 'No new certificates since the last look.'
    case 'expiry': {
      if (!r.expires_at) return FOOTPRINT_NOT_LOOKED
      const days = Math.floor((new Date(r.expires_at).getTime() - nowMs) / MS_PER_DAY)
      return days < 0 ? 'The registration has ended.' : `Registered for ${plural(days, 'more day', 'more days')}.`
    }
    case 'mail_records':
      return `SPF: ${r.spf ?? 'unknown'}. DMARC: ${r.dmarc ?? 'unknown'}.`
    case 'breaches':
      return r.breaches ? `Found in ${plural(r.breaches, 'known breach', 'known breaches')}.` : 'Not found in any known breach.'
  }
}
