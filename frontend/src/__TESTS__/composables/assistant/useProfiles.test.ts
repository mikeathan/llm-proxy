import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { PromptProfile } from '../../../types/profile'

const service = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../../services/profile/profileService', () => ({ ProfileService: service }))

const general: PromptProfile = { id: 'general', name: 'General', summary: 's', source: 'builtin', answer_first: true, may_ask: true, memory_save: 'model', reminders: true, default_for: ['assistant'] }

async function fresh() {
  vi.resetModules()
  return (await import('../../../composables/assistant/useProfiles')).useProfiles()
}

describe('useProfiles', () => {
  beforeEach(() => service.list.mockReset())

  it('reads the catalog once and shares it', async () => {
    service.list.mockResolvedValue([general])
    const first = await fresh()
    await vi.waitFor(() => expect(first.profiles.value).toHaveLength(1))
    const { useProfiles } = await import('../../../composables/assistant/useProfiles')
    useProfiles()
    expect(service.list).toHaveBeenCalledTimes(1)
  })

  it('says why the catalog could not be read, and recovers on refresh', async () => {
    service.list.mockRejectedValueOnce(new Error('backend down'))
    const profiles = await fresh()
    await vi.waitFor(() => expect(profiles.error.value).toBe('backend down'))
    service.list.mockResolvedValue([general])
    await profiles.refresh()
    expect(profiles.error.value).toBeNull()
    expect(profiles.profiles.value).toEqual([general])
  })

  it('joins a refresh that is already running instead of sending another request', async () => {
    let resolve!: (v: PromptProfile[]) => void
    service.list.mockReturnValue(new Promise<PromptProfile[]>((r) => (resolve = r)))
    const profiles = await fresh()
    void profiles.refresh()
    void profiles.refresh()
    expect(service.list).toHaveBeenCalledTimes(1)
    resolve([general])
    await vi.waitFor(() => expect(profiles.profiles.value).toHaveLength(1))
  })
})
