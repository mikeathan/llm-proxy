import { describe, it, expect, vi } from 'vitest'

const fetchProviderKeys = vi.fn()
vi.mock('../../../services/admin/adminService', () => ({ AdminApiService: { fetchProviderKeys: (p: string) => fetchProviderKeys(p) } }))
vi.mock('../../../composables/useToast', () => ({ useToast: () => ({ error: vi.fn() }) }))
vi.mock('vue', async (original) => ({ ...(await original<typeof import('vue')>()), onUnmounted: vi.fn() }))

import { useProviderKeys } from '../../../composables/settings/useProviderKeys'

describe('useProviderKeys', () => {
  // The Models page loads every provider's keys at once; each answer must land beside the others, whatever order they arrive in.
  it('keeps every provider\'s keys when several load at once', async () => {
    const answers: Record<string, (v: unknown) => void> = {}
    fetchProviderKeys.mockImplementation((p: string) => new Promise((resolve) => (answers[p] = resolve)))
    const keys = useProviderKeys(async () => {}, () => undefined)
    const loads = [keys.load('openai'), keys.load('nvidia')]
    answers.nvidia!([{ id: 'n', name: 'N', key: '••••' }])
    answers.openai!([{ id: 'o', name: 'O', key: '••••' }])
    await Promise.all(loads)
    expect(keys.keys.value.openai?.map((k) => k.name)).toEqual(['O'])
    expect(keys.keys.value.nvidia?.map((k) => k.name)).toEqual(['N'])
  })
})
