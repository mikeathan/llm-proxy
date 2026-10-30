import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DecoyState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ decoys: vi.fn(), setDecoys: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useDecoys } from '../../../composables/netaudit/useDecoys'

const state = (over: Partial<DecoyState> = {}): DecoyState => ({ enabled: false, listening: [], ports: [{ number: 2222, name: 'SSH login' }], touches: [], ...over })

describe('useDecoys', () => {
  beforeEach(() => { service.decoys.mockReset(); service.setDecoys.mockReset() })

  it('loads the state', async () => {
    service.decoys.mockResolvedValue(state())
    const d = useDecoys()
    await d.load()
    expect(d.state.value?.enabled).toBe(false)
    expect(d.error.value).toBe('')
  })

  it('shows the server\'s answer after a switch', async () => {
    service.setDecoys.mockResolvedValue(state({ enabled: true, listening: [2222] }))
    const d = useDecoys()
    await d.set(true)
    expect(service.setDecoys).toHaveBeenCalledWith(true)
    expect(d.state.value?.listening).toEqual([2222])
  })

  it('keeps the old state and says so when the switch is refused', async () => {
    service.decoys.mockResolvedValue(state({ enabled: true }))
    service.setDecoys.mockRejectedValue(new Error('passphrase not confirmed'))
    const d = useDecoys()
    await d.load()
    await d.set(false)
    expect(d.state.value?.enabled).toBe(true)
    expect(d.error.value).toContain('passphrase')
  })

  it('says when the state cannot be read', async () => {
    service.decoys.mockRejectedValue(new Error('down'))
    const d = useDecoys()
    await d.load()
    expect(d.error.value).not.toBe('')
  })
})
