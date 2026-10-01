import type { ColourScheme, PresetId, TokenSheet } from '../types/theme'
import { isTokenName } from './tokenRegistry'

// Reads token declarations out of styles/tokens.css — the single source of
// truth (D6) — so presets never restate a value. Only the shapes tokens.css
// uses are recognised: top-level rule blocks, one declaration per `;`.
// At-rule blocks (@media reduced motion) are skipped: they are runtime
// overrides, not preset values.

const PRESET_SELECTOR = /^\[data-theme='([a-z-]+)'\]$/
const ROOT_SELECTOR = ':root'
const COMMENT = /\/\*[\s\S]*?\*\//g
const DECLARATION = /^(--[a-z0-9-]+|color-scheme)\s*:\s*(.+)$/
const PRESETS: readonly PresetId[] = ['retro-dark', 'retro-dark-soft', 'retro-dark-lifted', 'retro-paper']

interface Block {
  selectors: string[]
  body: string
}

function topLevelBlocks(css: string): Block[] {
  const blocks: Block[] = []
  let depth = 0
  let start = 0
  let selector = ''
  for (let i = 0; i < css.length; i++) {
    if (css[i] === '{') {
      if (depth === 0) {
        selector = css.slice(start, i).trim()
        start = i + 1
      }
      depth++
    } else if (css[i] === '}') {
      depth--
      if (depth === 0) {
        if (!selector.startsWith('@')) {
          blocks.push({ selectors: selector.split(',').map((s) => s.trim()), body: css.slice(start, i) })
        }
        start = i + 1
      }
    }
  }
  return blocks
}

function isPreset(id: string): id is PresetId {
  return (PRESETS as readonly string[]).includes(id)
}

export function parseTokenSheet(css: string): TokenSheet {
  const sheet: TokenSheet = {
    global: {},
    themes: { 'retro-dark': {}, 'retro-dark-soft': {}, 'retro-dark-lifted': {}, 'retro-paper': {} },
    schemes: {},
  }
  for (const { selectors, body } of topLevelBlocks(css.replace(COMMENT, ''))) {
    const presets = selectors.map((s) => PRESET_SELECTOR.exec(s)?.[1]).filter((id): id is string => !!id)
    const unknown = presets.find((id) => !isPreset(id))
    if (unknown) throw new Error(`tokens.css: unknown preset "${unknown}"`)
    const isGlobal = presets.length === 0 && selectors.includes(ROOT_SELECTOR)
    if (!isGlobal && presets.length === 0) continue

    for (const statement of body.split(';')) {
      const [, property, rawValue] = DECLARATION.exec(statement.trim()) ?? []
      if (!property || rawValue === undefined) continue
      const value = rawValue.trim()
      if (property === 'color-scheme') {
        for (const id of presets as PresetId[]) sheet.schemes[id] = value as ColourScheme
        continue
      }
      const name = property.slice(2)
      if (!isTokenName(name)) throw new Error(`tokens.css: "--${name}" is not in the token registry`)
      if (isGlobal) sheet.global[name] = value
      for (const id of presets as PresetId[]) sheet.themes[id][name] = value
    }
  }
  return sheet
}
