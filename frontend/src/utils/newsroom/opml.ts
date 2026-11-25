import { isFeedAddress } from '../../constants/newsroom'
import type { BriefSource, OpmlFeed } from '../../types/newsroom'

const OPML_NAME_MAX = 80

/**
 * The feeds of an OPML file (what feed readers export): every outline with an http or https xmlUrl, named by its title or text, each address once. The file
 * is someone else's, so it is read as data only (the browser's XML parser runs no script) and anything that is not a feed address is left out.
 */
export function parseOpml(text: string): OpmlFeed[] {
  const doc = new DOMParser().parseFromString(text, 'text/xml')
  if (doc.getElementsByTagName('parsererror').length) return []
  const seen = new Set<string>()
  const out: OpmlFeed[] = []
  for (const el of Array.from(doc.getElementsByTagName('outline'))) {
    const target = (el.getAttribute('xmlUrl') ?? '').trim()
    if (!isFeedAddress(target) || seen.has(target)) continue
    seen.add(target)
    const label = (el.getAttribute('title') || el.getAttribute('text') || new URL(target).hostname).trim().slice(0, OPML_NAME_MAX)
    out.push({ label, target })
  }
  return out
}

const escape = (s: string): string => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

/** The owner's own sources as an OPML file a feed reader can import. */
export function opmlOf(sources: readonly BriefSource[]): string {
  const lines = sources.filter((s) => !s.builtin && s.kind === 'feed').map((s) => `    <outline type="rss" text="${escape(s.label)}" title="${escape(s.label)}" xmlUrl="${escape(s.target)}" />`)
  return ['<?xml version="1.0" encoding="UTF-8"?>', '<opml version="2.0">', '  <head><title>My sources</title></head>', '  <body>', ...lines, '  </body>', '</opml>', ''].join('\n')
}
