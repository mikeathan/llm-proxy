import { TOKEN_REGISTRY } from '../src/theme/tokenRegistry'
import type { TokenName } from '../src/types/theme'
import { BREAKPOINTS } from '../src/theme/breakpoints'

// Tailwind ↔ token mapping (plan D1, D15), generated from the token registry so
// no colour is hand-mapped. Every colour token is available under its full name
// on every colour utility (`bg-surface-raised`, `ring-focus-ring`), and the text
// and border tiers also get short aliases on their own utility (`text-muted`,
// `border-hairline`). Opacity modifiers work because tokens hold bare channels.

type ColourMap = Record<string, string>

/** `rgb(var(--x) / <alpha-value>)`, optionally scaled (used by the legacy bridge). */
export function withAlpha(token: TokenName, factor = 1): string {
  const alpha = factor === 1 ? '<alpha-value>' : `calc(<alpha-value> * ${factor})`
  return `rgb(var(--${token}) / ${alpha})`
}

const colourTokens = (Object.keys(TOKEN_REGISTRY) as TokenName[]).filter((t) => TOKEN_REGISTRY[t].kind === 'colour')

function aliases(prefix: string): ColourMap {
  return Object.fromEntries(
    colourTokens.filter((t) => t.startsWith(prefix)).map((t) => [t.slice(prefix.length), withAlpha(t)]),
  )
}

/** CSS colour keywords Tailwind keeps next to the tokens (bg-transparent, border-current). */
export const colourKeywords: ColourMap = { transparent: 'transparent', current: 'currentColor', inherit: 'inherit' }

export const semanticColours: ColourMap = Object.fromEntries(colourTokens.map((t) => [t, withAlpha(t)]))
export const semanticTextColours: ColourMap = aliases('text-')
export const semanticBorderColours: ColourMap = aliases('border-')

// Tailwind `screens` from the shared breakpoint table (one source of truth).
export const screens: Record<string, string> = Object.fromEntries(
  Object.entries(BREAKPOINTS).map(([name, px]) => [name, `${px}px`]),
)

export const fontFamily = {
  sans: ['var(--font-sans)'],
  mono: ['var(--font-mono)'],
}

export const transitionDuration = {
  fast: 'var(--motion-fast)',
  base: 'var(--motion-base)',
  slow: 'var(--motion-slow)',
}

export const transitionTimingFunction = {
  standard: 'var(--ease-standard)',
  emphasised: 'var(--ease-emphasised)',
}

// @tailwindcss/typography reads --tw-prose-* (plan D17). Both the normal and the
// `prose-invert` variables point at tokens: tokens already switch per theme, so
// assistant markdown matches whichever preset is active either way.
const PROSE: Record<string, string> = {
  body: 'rgb(var(--text-secondary))',
  headings: 'rgb(var(--text-primary))',
  lead: 'rgb(var(--text-secondary))',
  links: 'rgb(var(--accent-info-text))',
  bold: 'rgb(var(--text-primary))',
  counters: 'rgb(var(--text-muted))',
  bullets: 'rgb(var(--text-faint))',
  hr: 'rgb(var(--border-strong))',
  quotes: 'rgb(var(--text-primary))',
  'quote-borders': 'rgb(var(--border-strong))',
  captions: 'rgb(var(--text-muted))',
  kbd: 'rgb(var(--text-primary))',
  'kbd-shadows': 'var(--text-primary)',
  code: 'rgb(var(--text-primary))',
  'pre-code': 'rgb(var(--text-secondary))',
  'pre-bg': 'rgb(var(--surface-raised))',
  'th-borders': 'rgb(var(--border-strong))',
  'td-borders': 'rgb(var(--border-hairline))',
}

// The plugin wraps inline code in backticks and blockquotes in curly quotes.
// Model text brings its own, so they doubled (`` `name` ``, ""quote"").
const NO_DECORATION = { content: 'none' }
const PROSE_RESETS = {
  'code::before': NO_DECORATION,
  'code::after': NO_DECORATION,
  'blockquote p:first-of-type::before': NO_DECORATION,
  'blockquote p:last-of-type::after': NO_DECORATION,
}

export const typography = {
  DEFAULT: {
    css: {
      ...Object.fromEntries(
        Object.entries(PROSE).flatMap(([key, value]) => [
          [`--tw-prose-${key}`, value],
          [`--tw-prose-invert-${key}`, value],
        ]),
      ),
      ...PROSE_RESETS,
    },
  },
}
