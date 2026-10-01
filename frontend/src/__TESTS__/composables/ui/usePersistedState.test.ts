import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { persistedKey, usePersistedState } from '../../../composables/ui/usePersistedState'
import { memoryStorage } from '../../helpers/memoryStorage'

const parseCount = (data: unknown) => (typeof data === 'number' ? data : null)
const envelope = (version: number, data: unknown) => JSON.stringify({ version, data })

describe('usePersistedState', () => {
  it('namespaces keys', () => {
    expect(persistedKey('theme')).toBe('admin-ui:theme')
  })

  it('round-trips a value through storage', async () => {
    const storage = memoryStorage()
    const state = usePersistedState('count', { version: 1, parse: parseCount, fallback: 0, storage })
    state.value = 3
    await nextTick()
    expect(storage.getItem('admin-ui:count')).toBe(envelope(1, 3))
    expect(usePersistedState('count', { version: 1, parse: parseCount, fallback: 0, storage }).value).toBe(3)
  })

  it.each([
    ['absent', {}],
    ['corrupt JSON', { 'admin-ui:count': '{nope' }],
    ['wrong shape', { 'admin-ui:count': envelope(1, 'three') }],
    ['no envelope', { 'admin-ui:count': '3' }],
    ['newer version', { 'admin-ui:count': envelope(2, 3) }],
  ])('falls back when data is %s', (_case, initial) => {
    const state = usePersistedState('count', { version: 1, parse: parseCount, fallback: 7, storage: memoryStorage(initial) })
    expect(state.value).toBe(7)
  })

  it('migrates data stored under an older version', () => {
    const storage = memoryStorage({ 'admin-ui:count': envelope(1, '4') })
    const state = usePersistedState('count', {
      version: 2,
      parse: parseCount,
      fallback: 0,
      migrate: (from, data) => (from === 1 && typeof data === 'string' ? Number(data) : null),
      storage,
    })
    expect(state.value).toBe(4)
  })

  it('keeps working in memory when storage is missing or rejects writes', async () => {
    const noStorage = usePersistedState('count', { version: 1, parse: parseCount, fallback: 1, storage: undefined })
    noStorage.value = 2
    const failing = usePersistedState('count', { version: 1, parse: parseCount, fallback: 1, storage: memoryStorage({}, true) })
    failing.value = 5
    await nextTick()
    expect(noStorage.value).toBe(2)
    expect(failing.value).toBe(5)
  })
})
