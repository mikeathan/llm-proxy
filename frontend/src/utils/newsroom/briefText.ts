import { safeWebLink } from './safeLink'
import type { BriefIssue } from '../../types/newsroom'

/**
 * The brief as plain text, to paste into a note or a message: the name and the day, the top stories, then each section, every headline with its address when
 * it is a real web address. No home fact, reminder or watch is in it: those stay on the owner's screen (ADR-0083).
 */
export function briefText(issue: BriefIssue, day: string): string {
  const line = (headline: string, link?: string): string => {
    const safe = safeWebLink(link)
    return safe ? `• ${headline}\n  ${safe}` : `• ${headline}`
  }
  const parts = [`${issue.name} · ${day}`]
  const top = issue.top ?? []
  const topIds = new Set(top.map((s) => s.pool_id))
  if (top.length) parts.push(['Top stories', ...top.map((s) => line(s.headline, s.link))].join('\n'))
  for (const sec of issue.sections) {
    const rest = sec.stories.filter((s) => !topIds.has(s.pool_id))
    if (rest.length) parts.push([sec.name, ...rest.map((s) => line(s.headline, s.link))].join('\n'))
  }
  return parts.join('\n\n')
}
