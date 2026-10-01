import type { Channels } from '../types/theme'

// Pure colour maths for the theme validator and contrast checks (D2, D15).
// Only opaque colours parse: any alpha form returns null, so translucency can
// never become a token value.

const CHANNEL_MAX = 255
const SEP = String.raw`\s*(?:,\s*|\s+)`
const CHANNEL_TRIPLE = /^(\d{1,3})\s+(\d{1,3})\s+(\d{1,3})$/
const HEX = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i
const RGB = new RegExp(String.raw`^rgb\(\s*(\d{1,3})${SEP}(\d{1,3})${SEP}(\d{1,3})\s*\)$`, 'i')
const NUM = String.raw`(\d+(?:\.\d+)?)`
const HSL = new RegExp(String.raw`^hsl\(\s*${NUM}(?:deg)?${SEP}${NUM}%${SEP}${NUM}%\s*\)$`, 'i')

function channels([r, g, b]: number[]): Channels | null {
  if (r === undefined || g === undefined || b === undefined) return null
  if ([r, g, b].some((v) => !Number.isInteger(v) || v < 0 || v > CHANNEL_MAX)) return null
  return [r, g, b]
}

function fromHex(hex: string): Channels {
  const full = hex.length === 3 ? [...hex].map((c) => c + c).join('') : hex
  return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16)) as unknown as Channels
}

function fromHsl(hue: number, saturation: number, lightness: number): Channels | null {
  if (saturation > 100 || lightness > 100) return null
  const s = saturation / 100
  const l = lightness / 100
  const k = (n: number) => (n + hue / 30) % 12
  const a = s * Math.min(l, 1 - l)
  const f = (n: number) => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1))
  return channels([f(0), f(8), f(4)].map((v) => Math.round(v * CHANNEL_MAX)))
}

/** Parses hex, rgb(), hsl() or a bare "R G B" triple; null for anything else. */
export function parseColour(input: string): Channels | null {
  const value = input.trim()
  let m = CHANNEL_TRIPLE.exec(value) ?? RGB.exec(value)
  if (m) return channels([m[1], m[2], m[3]].map(Number))
  m = HEX.exec(value)
  if (m?.[1]) return fromHex(m[1])
  m = HSL.exec(value)
  if (m) return fromHsl(Number(m[1]) % 360, Number(m[2]), Number(m[3]))
  return null
}

/** The canonical stored form (D15): "R G B". */
export function formatChannels([r, g, b]: Channels): string {
  return `${r} ${g} ${b}`
}

/** Hex for readable export (D15). */
export function channelsToHex(rgb: Channels): string {
  return `#${rgb.map((c) => c.toString(16).padStart(2, '0')).join('')}`
}

function linearise(channel: number): number {
  const c = channel / CHANNEL_MAX
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

function relativeLuminance([r, g, b]: Channels): number {
  return 0.2126 * linearise(r) + 0.7152 * linearise(g) + 0.0722 * linearise(b)
}

/** WCAG 2.x contrast ratio between two opaque colours. */
export function contrastRatio(a: Channels, b: Channels): number {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}
