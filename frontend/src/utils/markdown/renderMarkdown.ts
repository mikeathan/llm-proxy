import { Marked, type Tokens } from 'marked'
import { escapeHtml } from '../format/format'

// The one markdown → HTML path for model-written text (assistant answers,
// reasoning, automation output). That text may echo hostile markup from a web
// page or a file, so the output must be safe for v-html without a sanitizer:
// raw HTML in the markdown is shown as text, and link / image URLs keep only
// safe schemes. Every tag left is produced by marked's own renderer, which
// escapes text content.

const SAFE_SCHEMES = new Set(['http:', 'https:', 'mailto:'])
const BLOCKED_URL = '#'
// Parsing base for relative URLs (./x, #top, /path): they keep no scheme of their own.
const RELATIVE_BASE = 'https://relative.invalid'

/** The URL when its scheme is safe (or it is relative), otherwise a harmless anchor. */
function safeUrl(href: string): string {
  try {
    const url = new URL(href.trim(), RELATIVE_BASE)
    return SAFE_SCHEMES.has(url.protocol) ? href : BLOCKED_URL
  } catch {
    return BLOCKED_URL
  }
}

const markdown = new Marked(
  { gfm: true, breaks: true },
  {
    renderer: {
      html: ({ text }: Tokens.HTML | Tokens.Tag) => escapeHtml(text),
    },
    walkTokens(token) {
      if (token.type === 'link' || token.type === 'image') {
        const t = token as Tokens.Link | Tokens.Image
        t.href = safeUrl(t.href)
      }
    },
  },
)

// A wide table must scroll inside its own box instead of widening the card (or
// the page) it sits in. Safe to do on the output string: text and raw HTML are
// escaped above, so a literal <table> tag can only come from marked's renderer.
// tabindex lets keyboard users scroll the region.
function wrapTables(html: string): string {
  return html
    .replace(/<table>/g, '<div class="md-table-scroll" tabindex="0"><table>')
    .replace(/<\/table>/g, '</table></div>')
}

export function renderMarkdown(source: string): string {
  return wrapTables(markdown.parse(source, { async: false }))
}
