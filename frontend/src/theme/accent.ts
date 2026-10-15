import type { AccentChoice, AccentId, Channels, ResolvedTokens, ThemeTokens, TokenName } from '../types/theme'
import { contrastRatio, formatChannels, parseColour } from './colour'

// An accent over any preset (plan frontend-redesign-v2, Phase 1b): the brand colour of
// the logo, the current-page marker, focus and the primary button's offset. A swatch
// is a starting colour, not a promise: it is moved toward white (dark presets) or
// black (light ones) until it reads on every surface and the preset's label colour
// reads on it, so no preset × accent pair can fail the contrast gate. Only the two
// tokens that carry the accent are overridden; run state keeps its own colours.

export const ACCENTS: Readonly<Record<AccentId, { label: string; rgb: Channels }>> = {
  persimmon: { label: 'Persimmon', rgb: [255, 106, 61] },
  raspberry: { label: 'Raspberry', rgb: [240, 98, 146] },
  orchid: { label: 'Orchid', rgb: [210, 120, 235] },
  violet: { label: 'Violet', rgb: [170, 140, 255] },
  indigo: { label: 'Indigo', rgb: [120, 130, 255] },
  sky: { label: 'Sky', rgb: [96, 178, 240] },
  teal: { label: 'Teal', rgb: [44, 190, 196] },
  sand: { label: 'Sand', rgb: [226, 178, 112] },
}

const SURFACES: readonly TokenName[] = ['canvas', 'surface', 'surface-raised', 'surface-hover']
const BODY_TEXT_MIN = 4.5
const STEP = 0.05
const MAX_STEPS = 40
const BLACK: Channels = [0, 0, 0]
const WHITE: Channels = [255, 255, 255]

/** The colour a choice stands for; null for a custom colour that does not parse. */
export function accentChannels(choice: AccentChoice): Channels | null {
  return choice.kind === 'named' ? ACCENTS[choice.id].rgb : parseColour(choice.hex)
}

function towards(from: Channels, to: Channels, amount: number): Channels {
  return from.map((v, i) => Math.round(v + ((to[i] ?? v) - v) * amount)) as unknown as Channels
}

function channelsOf(tokens: ResolvedTokens, name: TokenName): Channels {
  const rgb = parseColour(tokens[name])
  if (!rgb) throw new Error(`--${name} is not an opaque colour: "${tokens[name]}"`)
  return rgb
}

/** The token overrides that put `base` on a preset, adjusted until the preset's pairs hold. */
export function accentOverrides(tokens: ResolvedTokens, base: Channels): ThemeTokens {
  const surfaces = SURFACES.map((name) => channelsOf(tokens, name))
  const label = channelsOf(tokens, 'text-on-accent')
  const canvas = channelsOf(tokens, 'canvas')
  const toward = contrastRatio(canvas, WHITE) > contrastRatio(canvas, BLACK) ? WHITE : BLACK
  const reads = (accent: Channels) =>
    surfaces.every((surface) => contrastRatio(accent, surface) >= BODY_TEXT_MIN) && contrastRatio(label, accent) >= BODY_TEXT_MIN

  let accent = base
  for (let step = 0; step < MAX_STEPS && !reads(accent); step++) accent = towards(accent, toward, STEP)
  const value = formatChannels(accent)
  return { 'accent-brand': value, 'focus-ring': value }
}
