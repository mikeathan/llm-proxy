import type { ContrastPair, ContrastResult, ResolvedTokens, TokenName } from '../types/theme'
import { contrastRatio, parseColour } from './colour'

// The explicit contrast pair list (plan: Accessibility gate). Presets must pass
// every pair; custom themes get the failures back as warnings (D2).
// frontend/scripts/check-contrast.cjs (npm run lint) checks the same list from
// tokens.css without the app bundle — keep the two in step.

const BODY_TEXT_MIN = 4.5
const NON_TEXT_MIN = 3

const SURFACES: readonly TokenName[] = ['canvas', 'surface', 'surface-raised', 'surface-hover']
const TEXT_TIERS: readonly TokenName[] = ['text-primary', 'text-secondary', 'text-muted', 'text-faint']
const STATE_TEXT: readonly TokenName[] = [
  'accent-brand',
  'state-success',
  'state-running',
  'state-queued',
  'state-error',
  'accent-info-text',
]
/** Solid fills that carry text-on-accent labels (primary / danger buttons). */
const ACCENT_FILLS: readonly TokenName[] = ['accent-brand', 'accent-info', 'state-success', 'state-running', 'state-error']

function buildPairs(): ContrastPair[] {
  const pairs: ContrastPair[] = []
  for (const bg of SURFACES) {
    for (const fg of TEXT_TIERS) pairs.push({ fg, bg, min: BODY_TEXT_MIN, role: 'text' })
    for (const fg of STATE_TEXT) pairs.push({ fg, bg, min: BODY_TEXT_MIN, role: 'state text' })
    pairs.push({ fg: 'border-control', bg, min: NON_TEXT_MIN, role: 'control boundary' })
    pairs.push({ fg: 'focus-ring', bg, min: NON_TEXT_MIN, role: 'focus indicator' })
    pairs.push({ fg: 'accent-info', bg, min: NON_TEXT_MIN, role: 'fill / large text' })
  }
  pairs.push({ fg: 'text-inverse', bg: 'text-primary', min: BODY_TEXT_MIN, role: 'primary button' })
  for (const bg of ACCENT_FILLS) pairs.push({ fg: 'text-on-accent', bg, min: BODY_TEXT_MIN, role: 'text on accent' })
  return pairs
}

export const CONTRAST_PAIRS: readonly ContrastPair[] = buildPairs()

function channelsOf(tokens: ResolvedTokens, name: TokenName) {
  const rgb = parseColour(tokens[name])
  if (!rgb) throw new Error(`--${name} is not an opaque colour: "${tokens[name]}"`)
  return rgb
}

export function evaluateContrast(tokens: ResolvedTokens): ContrastResult[] {
  return CONTRAST_PAIRS.map((pair) => {
    const ratio = contrastRatio(channelsOf(tokens, pair.fg), channelsOf(tokens, pair.bg))
    return { ...pair, ratio, pass: ratio >= pair.min }
  })
}
