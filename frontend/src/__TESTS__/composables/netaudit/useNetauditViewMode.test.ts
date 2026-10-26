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

describe('useNetauditViewMode', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('localStorage', memoryStorage())
  })
  afterEach(() => vi.unstubAllGlobals())

  it('starts in the simple view', async () => {
    const { useNetauditViewMode } = await import('../../../composables/netaudit/useNetauditViewMode')
    const view = useNetauditViewMode()
    expect(view.mode.value).toBe('simple')
    expect(view.advanced.value).toBe(false)
  })

  it('shares one choice between screens and remembers it', async () => {
    const { useNetauditViewMode } = await import('../../../composables/netaudit/useNetauditViewMode')
    useNetauditViewMode().set('advanced')
    expect(useNetauditViewMode().advanced.value).toBe(true)
    await Promise.resolve()
    vi.resetModules()
    const again = await import('../../../composables/netaudit/useNetauditViewMode')
    expect(again.useNetauditViewMode().mode.value).toBe('advanced')
  })

  it('ignores a stored value it does not know', async () => {
    localStorage.setItem('admin-ui:netaudit-view', JSON.stringify({ version: 1, data: 'loud' }))
    const { useNetauditViewMode } = await import('../../../composables/netaudit/useNetauditViewMode')
    expect(useNetauditViewMode().mode.value).toBe('simple')
  })
})
