import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function memoryStorage(): Storage {
  const data = new Map<string, string>()
  return {
    get length() { return data.size },
    clear: () => data.clear(),
    getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => void data.delete(k),
    setItem: (k, v) => void data.set(k, v),
  }
}

describe('useProblemLayout', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('localStorage', memoryStorage())
  })
  afterEach(() => vi.unstubAllGlobals())

  it('starts by kind of problem and keeps one choice for every screen', async () => {
    const { useProblemLayout } = await import('../../../composables/netaudit/useProblemLayout')
    const a = useProblemLayout()
    expect(a.layout.value).toBe('check')
    a.set('device')
    expect(useProblemLayout().layout.value).toBe('device')
  })
})
