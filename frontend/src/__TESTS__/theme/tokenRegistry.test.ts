import { describe, expect, it } from 'vitest'
import tokensCss from '../../styles/tokens.css?raw'
import { semanticColours } from '../../../theme/tailwindTokens'
import { TOKEN_NAMES, TOKEN_REGISTRY } from '../../theme/tokenRegistry'
import { parseTokenSheet } from '../../theme/tokenSheet'
import { validateCustomTheme } from '../../theme/validate'

// The registry is the one list of token names (plan: tokenRegistry). The CSS
// file, the Tailwind map and the validator must all agree with it.
const colourNames = TOKEN_NAMES.filter((n) => TOKEN_REGISTRY[n].kind === 'colour')

describe('token registry', () => {
  it('matches exactly the tokens styles/tokens.css declares', () => {
    const sheet = parseTokenSheet(tokensCss)
    const declared = new Set([...Object.keys(sheet.global), ...Object.keys(sheet.themes['retro-dark'])])
    expect([...declared].sort()).toEqual([...TOKEN_NAMES].sort())
  })

  it('gives Tailwind a class for every colour token and nothing else', () => {
    expect(Object.keys(semanticColours).sort()).toEqual([...colourNames].sort())
  })

  it('lets the validator (and so the editor) accept exactly the editable tokens', () => {
    const sample: Record<string, string> = { colour: '#808080', length: '4', duration: '100' }
    for (const name of TOKEN_NAMES) {
      const { kind, editable } = TOKEN_REGISTRY[name]
      const result = validateCustomTheme({ id: 'probe', label: 'Probe', base: 'retro-dark', overrides: { [name]: sample[kind] ?? 'x' } })
      expect(result.ok, `${name} (${kind})`).toBe(editable)
    }
  })
})
