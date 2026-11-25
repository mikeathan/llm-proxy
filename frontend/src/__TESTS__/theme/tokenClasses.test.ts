import { describe, expect, it } from 'vitest'
import { TOKEN_REGISTRY } from '../../theme/tokenRegistry'

// A colour class that names a state or accent token that does not exist (text-state-warning) compiles to nothing and the text draws in the default colour,
// with no error from the build. This test reads every component and composable and refuses such a class.
const sources = import.meta.glob(['/src/**/*.vue', '/src/**/*.ts', '!/src/__TESTS__/**'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const CLASS = /(?:^|[\s"'`:\[])(?:text|bg|border|ring|divide|fill|stroke|outline|decoration|placeholder)-((?:state|accent)-[a-z]+(?:-[a-z]+)*)(?=[\s"'`/\]]|$)/g

describe('colour classes', () => {
  it('name only state and accent tokens that exist', () => {
    const known = new Set(Object.keys(TOKEN_REGISTRY))
    const unknown: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      for (const m of text.matchAll(CLASS)) {
        const name = m[1] ?? ''
        if (!known.has(name)) unknown.push(`${path}: ${name}`)
      }
    }
    expect(unknown).toEqual([])
  })
})
