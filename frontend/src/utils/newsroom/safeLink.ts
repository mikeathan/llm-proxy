const WEB_PROTOCOLS = new Set(['http:', 'https:'])

/**
 * The link as an absolute http or https address, or null. A feed's link is text from the open internet: a javascript: or data: address must never
 * become an href, and a relative one would point into this app.
 */
export function safeWebLink(raw: string | undefined): string | null {
  if (!raw) return null
  try {
    const url = new URL(raw)
    return WEB_PROTOCOLS.has(url.protocol) ? url.href : null
  } catch {
    return null
  }
}
