import { describe, expect, it } from 'vitest'
import { APP_TITLE, PRODUCT_NAME } from '../../config/brand'

// Plan D25: the product name lives only in config/brand.ts so a rename is one line.
// No underscore: LLM_PROXY_HOME is a backend env var, out of D25's scope.
const NAME_PATTERN = /llm[-. ]?proxy/i
const BRAND_FILE = '../../config/brand.ts'

const sources = import.meta.glob<string>(['../../**/*.{vue,ts}', '!../../__TESTS__/**', '../../../index.html'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

describe('product name (D25)', () => {
  it('is defined once', () => {
    expect(PRODUCT_NAME).toBe('llm-proxy')
    expect(APP_TITLE).toContain(PRODUCT_NAME)
  })

  it('scans index.html as well as the source tree', () => {
    expect(Object.keys(sources)).toContain('../../../index.html')
  })

  it('appears literally nowhere but config/brand.ts', () => {
    const offenders = Object.entries(sources)
      .filter(([file, text]) => file !== BRAND_FILE && NAME_PATTERN.test(text))
      .map(([file]) => file)
    expect(offenders).toEqual([])
  })
})
