// Session source values are produced by the backend (models.SessionSource):
// "webhook-<platform>" (e.g. "webhook-telegram") or "manual". The frontend only
// maps that string to an icon/label — it never parses the session ID.

import type { SessionBrief, SourceSection } from '../../types/assistant'

// SourceSection is a display section in the session list. Webhook sessions are
// grouped under a headed "Webhook" section; manual sessions render flat
// (grouped: false) with no header. The type lives in types/assistant.ts.

// groupSessionsBySource splits sessions into a webhook section (first, headed)
// and a manual section (flat). Manual sessions keep the same row markup — they
// are simply not wrapped in a group header.
export function groupSessionsBySource(sessions: SessionBrief[]): SourceSection[] {
  const webhook = sessions.filter((s) => s.source?.startsWith('webhook'))
  const manual = sessions.filter((s) => !s.source?.startsWith('webhook'))
  const sections: SourceSection[] = []
  if (webhook.length) sections.push({ source: 'webhook', sessions: webhook, grouped: true })
  if (manual.length) sections.push({ source: 'manual', sessions: manual, grouped: false })
  return sections
}

// sourceIcon returns the icon name for a session, or null for manual sessions.
export function sourceIcon(source?: string): string | null {
  if (source?.startsWith('webhook')) return 'radio'
  return null
}

// sourceLabel returns the group heading for a session source. The coarse
// "webhook" key renders "Webhook"; a full "webhook-<platform>" value renders
// "Webhook — <Platform>". Manual sessions have no heading.
export function sourceLabel(source?: string): string {
  if (!source?.startsWith('webhook')) return ''
  if (source === 'webhook') return 'Webhook'
  const platform = source.slice('webhook-'.length)
  return `Webhook — ${platform.charAt(0).toUpperCase()}${platform.slice(1)}`
}
