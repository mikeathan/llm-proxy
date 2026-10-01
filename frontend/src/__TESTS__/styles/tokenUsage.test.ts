import { describe, expect, it } from 'vitest'

// Source-level guards for the token migration (plan Phase 1 acceptance).
const sources = import.meta.glob<string>(['../../**/*.{vue,ts,css}', '!../../__TESTS__/**'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

describe('token usage', () => {
  it('scans the source tree', () => {
    expect(Object.keys(sources).length).toBeGreaterThan(100)
  })

  it('no --color-live remains — assistant-run styling reads --state-live', () => {
    const offenders = Object.entries(sources).filter(([, text]) => text.includes('--color-live'))
    expect(offenders.map(([file]) => file)).toEqual([])
  })

  it('the retired styles/theme.css is gone and nothing imports it', () => {
    expect(Object.keys(sources).some((f) => f.endsWith('styles/theme.css'))).toBe(false)
    expect(Object.entries(sources).filter(([, t]) => t.includes('styles/theme.css')).map(([f]) => f)).toEqual([])
  })
})
