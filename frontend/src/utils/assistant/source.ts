// Session source values are produced by the backend (models.SessionSource):
// "webhook-<platform>" (e.g. "webhook-telegram") or "manual". The frontend only
// maps that string to an icon/label — it never parses the session ID.

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
