import { describe, it, expect, vi, beforeEach } from 'vitest'
import { nextTick, ref } from 'vue'

const service = vi.hoisted(() => ({ getWorkspaceConfig: vi.fn(), updateWorkspaceConfig: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: service }))

import { useWorkspaceConfigField } from '../../../composables/automation/useWorkspaceConfigField'

const normalize = (raw: unknown): string => (typeof raw === 'string' ? raw : '')

describe('useWorkspaceConfigField', () => {
  beforeEach(() => {
    service.getWorkspaceConfig.mockReset()
    service.updateWorkspaceConfig.mockReset().mockResolvedValue(undefined)
  })

  it('loads the field for the workspace in view, and again when the workspace changes', async () => {
    service.getWorkspaceConfig.mockImplementation(async (ws: string) => ({ assistant_profile: ws === 'a' ? 'research' : 7 }))
    const ws = ref('a')
    const field = useWorkspaceConfigField(ws, 'assistant_profile', normalize, '')
    await vi.waitFor(() => expect(field.value.value).toBe('research'))
    ws.value = 'b'
    await vi.waitFor(() => expect(field.value.value).toBe(''))
    expect(service.getWorkspaceConfig).toHaveBeenCalledWith('b')
  })

  it('saves by reading the config fresh and changing only its own field', async () => {
    service.getWorkspaceConfig.mockResolvedValueOnce({}).mockResolvedValue({ assistant_memory: 'on', guardrails: { x: 1 }, assistant_profile: 'old' })
    const field = useWorkspaceConfigField(ref('a'), 'assistant_profile', normalize, '')
    await nextTick()
    await field.save('builder')
    expect(service.updateWorkspaceConfig).toHaveBeenCalledWith('a', { assistant_memory: 'on', guardrails: { x: 1 }, assistant_profile: 'builder' })
    expect(field.value.value).toBe('builder')
    expect(field.error.value).toBeNull()
  })

  it('says why a load or a save failed, and keeps the shown value when a save fails', async () => {
    service.getWorkspaceConfig.mockRejectedValueOnce(new Error('config unreadable'))
    const field = useWorkspaceConfigField(ref('a'), 'assistant_profile', normalize, 'kept')
    await vi.waitFor(() => expect(field.error.value).toBe('config unreadable'))

    service.getWorkspaceConfig.mockResolvedValue({})
    service.updateWorkspaceConfig.mockRejectedValueOnce(new Error('disk full'))
    await field.save('builder')
    expect(field.error.value).toBe('disk full')
    expect(field.value.value).toBe('kept')
    expect(field.saving.value).toBe(false)
  })
})
