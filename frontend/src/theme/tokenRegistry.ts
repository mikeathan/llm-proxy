import type { TokenName, TokenSpec } from '../types/theme'

// The one list of token names (plan: tokenRegistry). styles/tokens.css, the
// validator, the Tailwind map and the theme editor all derive from it; the
// Record type makes the compiler reject a TokenName without a spec.
const colour: TokenSpec = { kind: 'colour', scope: 'theme', editable: true }
const length: TokenSpec = { kind: 'length', scope: 'global', editable: true }
const typeLength: TokenSpec = { kind: 'length', scope: 'global', editable: false }
const duration: TokenSpec = { kind: 'duration', scope: 'global', editable: true }
const font: TokenSpec = { kind: 'font', scope: 'global', editable: false }
const easing: TokenSpec = { kind: 'easing', scope: 'global', editable: false }
const number: TokenSpec = { kind: 'number', scope: 'global', editable: false }

export const TOKEN_REGISTRY: Readonly<Record<TokenName, TokenSpec>> = {
  canvas: colour,
  surface: colour,
  'surface-raised': colour,
  'surface-hover': colour,
  'surface-active': colour,
  'border-hairline': colour,
  'border-strong': colour,
  'border-control': colour,
  'text-primary': colour,
  'text-secondary': colour,
  'text-muted': colour,
  'text-faint': colour,
  'text-decorative': colour,
  'text-inverse': colour,
  'text-on-accent': colour,
  'accent-brand': colour,
  'accent-info': colour,
  'accent-info-text': colour,
  'state-success': colour,
  'state-running': colour,
  'state-queued': colour,
  'state-error': colour,
  'state-live': colour,
  'focus-ring': colour,
  'bar-track': colour,
  scrim: colour,
  'font-sans': font,
  'font-mono': font,
  'text-micro': typeLength,
  'tracking-micro': typeLength,
  'text-body': typeLength,
  'text-small': typeLength,
  'text-heading': typeLength,
  'text-display': typeLength,
  'text-numeral': typeLength,
  'leading-body': number,
  'radius-sm': length,
  'radius-md': length,
  'offset-control': length,
  'space-unit': length,
  'motion-fast': duration,
  'motion-base': duration,
  'motion-slow': duration,
  'ease-standard': easing,
  'ease-emphasised': easing,
}

export const TOKEN_NAMES = Object.keys(TOKEN_REGISTRY) as readonly TokenName[]

export function isTokenName(name: string): name is TokenName {
  return Object.prototype.hasOwnProperty.call(TOKEN_REGISTRY, name)
}
