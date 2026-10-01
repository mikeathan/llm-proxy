import type { CustomTheme, PresetId, ThemeDraft, ThemeInput, ThemeTokens, TokenName } from '../types/theme'
import { channelsToHex, formatChannels, parseColour } from './colour'
import { isPresetId, PRESETS } from './presets'
import { TOKEN_NAMES, TOKEN_REGISTRY } from './tokenRegistry'
import { validateCustomTheme } from './validate'

// The theme editor's model (Settings · Appearance, D2): a draft holds every
// editable token as the user sees it; saving keeps only what differs from the
// base preset and goes through the one validator. Nothing here writes to the
// document — the preview gets validated values only.

/** Editable tokens in registry order: the editor rows derive from this. */
export const EDITABLE_TOKENS: readonly TokenName[] = TOKEN_NAMES.filter((name) => TOKEN_REGISTRY[name].editable)

const ID_MAX = 40
const FALLBACK_ID = 'theme'
const TOKEN_ERROR = /^token "([^"]+)"/

/** A stored value as the editor shows it: colours as hex, the rest as stored. */
function displayValue(name: TokenName, stored: string): string {
  if (TOKEN_REGISTRY[name].kind !== 'colour') return stored
  const rgb = parseColour(stored)
  return rgb ? channelsToHex(rgb) : stored
}

function valuesOver(base: PresetId, overrides: ThemeTokens): ThemeDraft['values'] {
  const tokens = { ...PRESETS[base].tokens, ...overrides }
  return Object.fromEntries(EDITABLE_TOKENS.map((name) => [name, displayValue(name, tokens[name])]))
}

/** Lowercase, dash-separated, ≤ 40 characters, never a preset id or a taken id. */
export function uniqueThemeId(label: string, taken: readonly string[]): string {
  const slug =
    label
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, ID_MAX)
      .replace(/-+$/, '') || FALLBACK_ID
  const inUse = (id: string) => taken.includes(id) || isPresetId(id)
  if (!inUse(slug)) return slug
  for (let n = 2; ; n++) {
    const suffix = `-${n}`
    const candidate = `${slug.slice(0, ID_MAX - suffix.length)}${suffix}`
    if (!inUse(candidate)) return candidate
  }
}

export function draftFromPreset(base: PresetId, label: string, taken: readonly string[]): ThemeDraft {
  return { id: uniqueThemeId(label, taken), label, base, values: valuesOver(base, {}) }
}

export function draftFromTheme(theme: CustomTheme): ThemeDraft {
  return { id: theme.id, label: theme.label, base: theme.base, values: valuesOver(theme.base, theme.overrides) }
}

/** Whether a typed value means the same as the preset's stored one. */
function sameAsBase(name: TokenName, value: string, base: PresetId): boolean {
  const stored = PRESETS[base].tokens[name]
  if (TOKEN_REGISTRY[name].kind === 'colour') {
    const rgb = parseColour(value)
    return !!rgb && formatChannels(rgb) === stored
  }
  return value.trim() === stored
}

/** The validator input for a draft: only the values that differ from the base. */
export function draftToInput(draft: ThemeDraft): ThemeInput {
  const overrides: Record<string, string> = {}
  for (const name of EDITABLE_TOKENS) {
    const value = draft.values[name]
    if (value !== undefined && !sameAsBase(name, value, draft.base)) overrides[name] = value
  }
  return { id: draft.id, label: draft.label, base: draft.base, overrides }
}

/** Validator errors about one token, keyed by that token. */
export function tokenErrors(errors: readonly string[]): Partial<Record<TokenName, string>> {
  const out: Partial<Record<TokenName, string>> = {}
  for (const message of errors) {
    const name = TOKEN_ERROR.exec(message)?.[1]
    if (name && name in TOKEN_REGISTRY) out[name as TokenName] = message
  }
  return out
}

/**
 * The validated, normalised overrides of a draft input, leaving out any token
 * that does not validate — so the preview shows every valid edit while the
 * user is still fixing another.
 */
export function previewOverrides(input: ThemeInput): ThemeTokens {
  const first = validateCustomTheme(input)
  if (first.ok) return first.theme.overrides
  const invalid = tokenErrors(first.errors)
  const overrides = Object.fromEntries(Object.entries(input.overrides).filter(([name]) => !(name in invalid)))
  // Id / label problems must not hide the colours, so preview under a known-good identity.
  const second = validateCustomTheme({ id: 'preview', label: 'Preview', base: input.base, overrides })
  return second.ok ? second.theme.overrides : {}
}
