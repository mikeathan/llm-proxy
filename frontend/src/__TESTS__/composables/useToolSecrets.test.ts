import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref } from 'vue'
import { useToolSecrets } from '../../composables/useToolSecrets'
import { AdminApiService } from '../../services/admin/adminService'

vi.mock('../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchToolSecret: vi.fn(),
    saveToolSecret: vi.fn(),
    deleteToolSecret: vi.fn(),
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

  it('persists dirty tokens under the category and adopts the server mask', async () => {
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('secr...-key')
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('brave')
    mgr.tokens.value['brave']!.dirty = 'secret-key'
    const err = ref('')

    const ok = await mgr.saveDirty(err)

    expect(ok).toBe(true)
    expect(AdminApiService.saveToolSecret).toHaveBeenCalledWith('search', 'brave', 'secret-key')
    expect(mgr.tokens.value['brave']!.dirty).toBeNull()
    expect(mgr.tokens.value['brave']!.masked).toBe('secr...-key')
    expect(err.value).toBe('')
  })

  it('never fabricates a shortened mask that could be re-saved as the key', async () => {
    // Regression: the composable used to set masked = dirty.slice(0, 4) + "...".
    // Because the input is bound to `masked`, clicking Save again sent that
    // truncated string back as the real credential, clobbering the stored key
    // (a 5-char "illy " got persisted as the Tavily key and 401'd).
    vi.mocked(AdminApiService.saveToolSecret).mockResolvedValue('tvly...9f2c')
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')
    mgr.tokens.value['tavily']!.dirty = 'tvly-dev-REALKEYVALUE9f2c'

    await mgr.saveDirty(ref(''))

    expect(mgr.tokens.value['tavily']!.masked).toBe('tvly...9f2c')
    expect(mgr.tokens.value['tavily']!.masked).not.toBe('tvly...')
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

  it('clears a token server-side and drops the local mask', async () => {
    vi.mocked(AdminApiService.deleteToolSecret).mockResolvedValue('')
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')
    mgr.tokens.value['tavily'] = { masked: 'tvly...9f2c', dirty: null }

    const err = await mgr.clear('tavily')

    expect(err).toBe('')
    expect(AdminApiService.deleteToolSecret).toHaveBeenCalledWith('search', 'tavily')
    expect(mgr.tokens.value['tavily']).toEqual({ masked: '', dirty: null })
  })

  it('discards a pending dirty value when clearing', async () => {
    // A queued dirty edit must not survive the clear, or the next save would
    // write the secret straight back.
    vi.mocked(AdminApiService.deleteToolSecret).mockResolvedValue('')
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')
    mgr.tokens.value['tavily']!.dirty = 'tvly-pending'

    await mgr.clear('tavily')
    await mgr.saveDirty(ref(''))

    expect(mgr.tokens.value['tavily']!.dirty).toBeNull()
    expect(AdminApiService.saveToolSecret).not.toHaveBeenCalled()
  })

  it('reports the failure and keeps the mask when clearing fails', async () => {
    vi.mocked(AdminApiService.deleteToolSecret).mockRejectedValue(new Error('boom'))
    const mgr = useToolSecrets('search')
    mgr.ensureTracked('tavily')
    mgr.tokens.value['tavily'] = { masked: 'tvly...9f2c', dirty: null }

    const err = await mgr.clear('tavily')

    expect(err).toContain('tavily')
    expect(mgr.tokens.value['tavily']!.masked).toBe('tvly...9f2c')
  })
})
