import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref } from 'vue'
import { useToolSecrets } from '../../composables/useToolSecrets'
import { AdminApiService } from '../../services/admin/adminService'

vi.mock('../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchToolSecret: vi.fn(),
    saveToolSecret: vi.fn(),
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useToolSecrets', () => {
  it('loads a masked secret for the given category', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockResolvedValue('abcd...')
    const mgr = useToolSecrets('search')

    await mgr.load('tavily')

    expect(AdminApiService.fetchToolSecret).toHaveBeenCalledWith('search', 'tavily')
    expect(mgr.tokens.value['tavily']).toEqual({ masked: 'abcd...', dirty: null })
  })

  it('falls back to an empty masked value on load failure', async () => {
    vi.mocked(AdminApiService.fetchToolSecret).mockRejectedValue(new Error('nope'))
    const mgr = useToolSecrets('connector')

    await mgr.load('telegram')

    expect(AdminApiService.fetchToolSecret).toHaveBeenCalledWith('connector', 'telegram')
    expect(mgr.tokens.value['telegram']).toEqual({ masked: '', dirty: null })
  })

  it('persists dirty tokens under the category and masks them', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue(undefined)
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('brave')
    mgr.tokens.value['brave']!.dirty = 'secret-key'
    const err = ref('')

    const ok = await mgr.saveDirty(err)

    expect(ok).toBe(true)
    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'brave', 'secret-key')
    expect(mgr.tokens.value['brave']!.dirty).toBeNull()
    expect(mgr.tokens.value['brave']!.masked).toBe('secr...')
    expect(err.value).toBe('')
  })

  it('reports the failure and keeps dirty state when a save fails', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockRejectedValue(new Error('boom'))
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')
    mgr.tokens.value['tavily']!.dirty = 'k'
    const err = ref('')

    const ok = await mgr.saveDirty(err)

    expect(ok).toBe(false)
    expect(err.value).toContain('tavily')
    expect(mgr.tokens.value['tavily']!.dirty).toBe('k')
    expect(mgr.tokens.value['tavily']!.masked).toBe('')
  })

  it('skips clean entries', async () => {
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')

    const ok = await mgr.saveDirty(ref(''))

    expect(ok).toBe(true)
    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
  })
})
