import type { SessionBrief, SessionSection } from '../../types/assistant'
import { sourceLabel } from './source'

// The conversation list's groups (plan D14): pinned conversations first, then
// manual ones by recency (Today / Yesterday / This week / Older), then webhook
// conversations in their own folder. Search narrows every group by title.

const DAY_MS = 86_400_000
const WEEK_DAYS = 7
const WEBHOOK = 'webhook'
const RECENCY = [
  { key: 'today', label: 'Today', maxDaysAgo: 0 },
  { key: 'yesterday', label: 'Yesterday', maxDaysAgo: 1 },
  { key: 'week', label: 'This week', maxDaysAgo: WEEK_DAYS - 1 },
  { key: 'older', label: 'Older', maxDaysAgo: Infinity },
] as const

const startOfDay = (ms: number) => {
  const d = new Date(ms)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

/** Whole calendar days between a timestamp and today (local time); Infinity when unknown. */
function daysAgo(iso: string, now: number): number {
  const at = Date.parse(iso)
  if (Number.isNaN(at)) return Infinity
  return Math.max(0, Math.round((startOfDay(now) - startOfDay(at)) / DAY_MS))
}

const isWebhook = (s: SessionBrief) => !!s.source?.startsWith(WEBHOOK)

export function buildSessionSections(
  sessions: readonly SessionBrief[],
  options: { pinned: ReadonlySet<string>; query: string; now: number },
): SessionSection[] {
  const query = options.query.trim().toLowerCase()
  const shown = query ? sessions.filter((s) => s.snippet.toLowerCase().includes(query)) : [...sessions]
  const pinned = shown.filter((s) => options.pinned.has(s.id))
  const rest = shown.filter((s) => !options.pinned.has(s.id))

  const sections: SessionSection[] = [{ key: 'pinned', label: 'Pinned', kind: 'pinned', sessions: pinned }]
  const manual = rest.filter((s) => !isWebhook(s))
  let lower = 0
  for (const band of RECENCY) {
    sections.push({
      key: band.key,
      label: band.label,
      kind: 'recency',
      sessions: manual.filter((s) => {
        const days = daysAgo(s.updated_at, options.now)
        return days >= lower && days <= band.maxDaysAgo
      }),
    })
    lower = band.maxDaysAgo + 1
  }
  sections.push({ key: WEBHOOK, label: sourceLabel(WEBHOOK), kind: 'source', sessions: rest.filter(isWebhook) })
  return sections.filter((section) => section.sessions.length > 0)
}
