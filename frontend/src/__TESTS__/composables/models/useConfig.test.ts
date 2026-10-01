import { describe, it, expect, vi, beforeEach } from 'vitest'
import { AdminApiService } from '../../../services/admin/adminService'
import { DEFAULT_CONFIG, useConfig } from '../../../composables/models/useConfig'
import type { GlobalConfig } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchState: vi.fn(), updateConfig: vi.fn() },
}))

const STORED: GlobalConfig = { ...structuredClone(DEFAULT_CONFIG), primary_model: 'qwen', model_host: '10.0.0.2' }
const { config, isDirty, error, fetchConfig, updateConfig, discardChanges, reconcileModelRefs, ensureProvider } = useConfig()

describe('useConfig unsaved changes', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    vi.mocked(AdminApiService.fetchState).mockResolvedValue({ config: structuredClone(STORED) } as never)
    vi.mocked(AdminApiService.updateConfig).mockResolvedValue()
    await fetchConfig()
  })

  it('is clean once loaded, and dirty after an edit', () => {
    expect(isDirty.value).toBe(false)
    config.value.model_host = '0.0.0.0'
    expect(isDirty.value).toBe(true)
  })

  it('discards edits back to the last loaded values', () => {
    config.value.model_host = '0.0.0.0'
    discardChanges()
    expect(config.value.model_host).toBe('10.0.0.2')
    expect(isDirty.value).toBe(false)
  })

  it('is clean after a successful save, and stays dirty after a failed one', async () => {
    config.value.model_host = '0.0.0.0'
    await updateConfig()
    expect(isDirty.value).toBe(false)

    config.value.model_host = '1.1.1.1'
    vi.mocked(AdminApiService.updateConfig).mockRejectedValue(new Error('disk full'))
    await expect(updateConfig()).rejects.toThrow('disk full')
    expect(isDirty.value).toBe(true)
    expect(error.value).toBe('disk full')
  })

  it('does not count housekeeping as an edit', () => {
    reconcileModelRefs(['other'])
    ensureProvider('gemini')
    expect(config.value.primary_model).toBe('')
    expect(config.value.providers?.gemini).toBeDefined()
    expect(isDirty.value).toBe(false)
  })

  it('reports a failed load and keeps the page clean', async () => {
    vi.mocked(AdminApiService.fetchState).mockRejectedValue(new Error('offline'))
    await fetchConfig()
    expect(error.value).toBe('offline')
    expect(isDirty.value).toBe(false)
  })
})
