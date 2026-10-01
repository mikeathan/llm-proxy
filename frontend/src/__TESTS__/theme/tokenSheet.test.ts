import { describe, expect, it } from 'vitest'
import { parseTokenSheet } from '../../theme/tokenSheet'

const SHEET = `
/* header comment with --fake: 1 2 3; */
:root,
[data-theme='retro-dark'],
[data-theme='retro-dark-lifted'] {
  color-scheme: dark;
  --canvas: 12 11 10;
  --text-primary: 242 237 227;
}
[data-theme='retro-dark-lifted'] { --canvas: 21 20 18; }
[data-theme='retro-paper'] { color-scheme: light; --canvas: 243 239 228; }
:root { --radius-sm: 2px; --motion-fast: 120ms; }
@media (prefers-reduced-motion: reduce) {
  :root { --motion-fast: 0ms; }
}
`

describe('parseTokenSheet', () => {
  const sheet = parseTokenSheet(SHEET)

  it('reads each preset block, the dark defaults counting as retro-dark', () => {
    expect(sheet.themes['retro-dark']).toEqual({ canvas: '12 11 10', 'text-primary': '242 237 227' })
    expect(sheet.themes['retro-dark-lifted']).toEqual({ canvas: '21 20 18', 'text-primary': '242 237 227' })
    expect(sheet.themes['retro-paper']).toEqual({ canvas: '243 239 228' })
  })

  it('reads theme-independent tokens from the plain :root block only', () => {
    expect(sheet.global).toEqual({ 'radius-sm': '2px', 'motion-fast': '120ms' })
  })

  it('ignores comments and at-rule blocks (reduced motion is not the default)', () => {
    expect(sheet.global['motion-fast']).toBe('120ms')
    expect(JSON.stringify(sheet)).not.toContain('fake')
  })

  it('records the colour scheme each preset declares', () => {
    expect(sheet.schemes).toEqual({ 'retro-dark': 'dark', 'retro-dark-lifted': 'dark', 'retro-paper': 'light' })
  })

  it('rejects an unknown token name so the registry cannot drift', () => {
    expect(() => parseTokenSheet(':root { --not-a-token: 1px; }')).toThrow(/not-a-token/)
  })
})
