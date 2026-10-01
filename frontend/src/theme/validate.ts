import type { CustomTheme, ThemeTokens, ThemeValidationResult, TokenKind } from '../types/theme'
import { formatChannels, parseColour } from './colour'
import { evaluateContrast } from './contrastPairs'
import { isPresetId, PRESETS } from './presets'
import { isTokenName, TOKEN_REGISTRY } from './tokenRegistry'

// The single validator for custom themes (D2). Every value is checked against a
// strict grammar for its token kind and normalised to the stored form, so
// nothing unvalidated can reach a CSS custom property. Grammar and name errors
// reject; contrast failures are returned for the editor to warn about.

const ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,39}$/
const LABEL_MAX = 60
const LENGTH = /^(\d{1,3}(?:\.\d{1,2})?)(px)?$/
const DURATION = /^(\d{1,4})(ms)?$/
const LENGTH_MAX_PX = 64
const DURATION_MAX_MS = 2000

type Normaliser = (raw: string) => string | null

function bounded(pattern: RegExp, max: number, unit: string): Normaliser {
  return (raw) => {
    const m = pattern.exec(raw.trim())
    if (!m || Number(m[1]) > max) return null
    return `${Number(m[1])}${unit}`
  }
}

// Only editable kinds have a normaliser; the registry marks the rest non-editable.
const NORMALISERS: Partial<Record<TokenKind, Normaliser>> = {
  colour: (raw) => {
    const rgb = parseColour(raw)
    return rgb && formatChannels(rgb)
  },
  length: bounded(LENGTH, LENGTH_MAX_PX, 'px'),
  duration: bounded(DURATION, DURATION_MAX_MS, 'ms'),
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function normaliseOverrides(raw: unknown, errors: string[]): ThemeTokens {
  const overrides: ThemeTokens = {}
  if (!isRecord(raw)) {
    errors.push('overrides must be an object of token → value')
    return overrides
  }
  for (const [name, value] of Object.entries(raw)) {
    if (!isTokenName(name)) {
      errors.push(`unknown token "${name}"`)
      continue
    }
    const spec = TOKEN_REGISTRY[name]
    const normalise = spec.editable ? NORMALISERS[spec.kind] : undefined
    if (!normalise) {
      errors.push(`token "${name}" cannot be customised`)
      continue
    }
    const normalised = typeof value === 'string' || typeof value === 'number' ? normalise(String(value)) : null
    if (normalised === null) errors.push(`token "${name}": "${String(value)}" is not a valid ${spec.kind}`)
    else overrides[name] = normalised
  }
  return overrides
}

export function validateCustomTheme(input: unknown): ThemeValidationResult {
  if (!isRecord(input)) return { ok: false, errors: ['a theme must be a JSON object'] }
  const errors: string[] = []
  const { id, label, base } = input
  if (typeof id !== 'string' || !ID_PATTERN.test(id) || isPresetId(id)) {
    errors.push('id must be 1–40 lowercase letters, digits or dashes, and not a preset id')
  }
  if (typeof label !== 'string' || !label.trim() || label.trim().length > LABEL_MAX) {
    errors.push(`label must be 1–${LABEL_MAX} characters`)
  }
  if (!isPresetId(base)) errors.push('base must be one of the built-in presets')
  const overrides = normaliseOverrides(input.overrides, errors)
  if (errors.length || !isPresetId(base)) return { ok: false, errors }

  const theme: CustomTheme = { id: id as string, label: (label as string).trim(), base, overrides }
  const contrastFailures = evaluateContrast({ ...PRESETS[base].tokens, ...overrides }).filter((r) => !r.pass)
  return { ok: true, theme, contrastFailures }
}
