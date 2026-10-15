import { PRESET_IDS } from '../types/theme'
import type { PresetId, ResolvedTokens, ThemeDefinition, ThemeTokens } from '../types/theme'
import tokensCss from '../styles/tokens.css?raw'
import { parseTokenSheet } from './tokenSheet'
import { TOKEN_NAMES } from './tokenRegistry'

// Built-in presets, resolved from styles/tokens.css (D6) the way the cascade
// resolves them: theme-independent tokens, then the retro-dark defaults on
// :root, then the preset's own block. No value is restated here.

// What a first visit shows (and what a custom theme falls back to).
export const DEFAULT_PRESET_ID: PresetId = 'retro-dark-soft'
// The cascade base: the :root values in tokens.css every other preset layers
// over. Not the same as the default — the default is a layer on this base.
export const BASE_PRESET_ID: PresetId = 'retro-dark'

const LABELS: Record<PresetId, string> = {
  'retro-dark': 'Retro dark',
  'retro-dark-soft': 'Retro dark · soft',
  'retro-dark-lifted': 'Retro dark · lifted',
  'retro-midnight': 'Retro midnight',
  'retro-forest': 'Retro forest',
  'retro-plum': 'Retro plum',
  'retro-paper': 'Retro paper',
  'retro-paper-cool': 'Retro paper · cool',
  'retro-sand': 'Retro sand',
}

const sheet = parseTokenSheet(tokensCss)

function resolve(id: PresetId): ResolvedTokens {
  const merged: ThemeTokens = { ...sheet.global, ...sheet.themes[BASE_PRESET_ID], ...sheet.themes[id] }
  const missing = TOKEN_NAMES.filter((name) => merged[name] === undefined)
  if (missing.length) throw new Error(`preset ${id} does not define: ${missing.join(', ')}`)
  return merged as ResolvedTokens
}

export const PRESETS: Readonly<Record<PresetId, ThemeDefinition>> = Object.fromEntries(
  PRESET_IDS.map((id) => [
    id,
    { id, label: LABELS[id], scheme: sheet.schemes[id] ?? sheet.schemes[BASE_PRESET_ID] ?? 'dark', tokens: resolve(id) },
  ]),
) as Record<PresetId, ThemeDefinition>

export function isPresetId(id: unknown): id is PresetId {
  return typeof id === 'string' && (PRESET_IDS as readonly string[]).includes(id)
}
