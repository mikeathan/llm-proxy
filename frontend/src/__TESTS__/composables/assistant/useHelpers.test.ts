import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { HelpersStatus } from '../../../types/helpers'

const service = vi.hoisted(() => ({ status: vi.fn() }))
vi.mock('../../../services/helpers/helpersService', () => ({ HelpersService: service }))

async function fresh() {
  vi.resetModules()
  return (await import('../../../composables/assistant/useHelpers')).useHelpers()
}

describe('useHelpers', () => {
  beforeEach(() => service.status.mockReset())

  it('starts with helpers unavailable and reads what the server says on refresh', async () => {
    service.status.mockResolvedValue({ available: true, model: 'deepseek-v4.1-flash' })
    const helpers = await fresh()
    expect(helpers.status.value).toEqual({ available: false, model: '' })
    expect(service.status).not.toHaveBeenCalled()
    await helpers.refresh()
    expect(helpers.status.value).toEqual({ available: true, model: 'deepseek-v4.1-flash' })
  })

  it('reads again on every refresh, so a Helper model changed in Settings shows when the chat reopens', async () => {
    service.status.mockResolvedValueOnce({ available: true, model: 'a' }).mockResolvedValueOnce({ available: false, model: '' })
    const helpers = await fresh()
    await helpers.refresh()
    await helpers.refresh()
    expect(helpers.status.value.available).toBe(false)
  })

  it('treats an unreachable server as unavailable, and recovers on the next refresh', async () => {
    service.status.mockRejectedValueOnce(new Error('down'))
    const helpers = await fresh()
    await helpers.refresh()
    expect(helpers.status.value).toEqual({ available: false, model: '' })
    service.status.mockResolvedValue({ available: true, model: 'a' })
    await helpers.refresh()
    expect(helpers.status.value.available).toBe(true)
  })

  it('joins a refresh that is already running instead of sending another request', async () => {
    let resolve!: (v: HelpersStatus) => void
    service.status.mockReturnValue(new Promise<HelpersStatus>((r) => (resolve = r)))
    const helpers = await fresh()
    void helpers.refresh()
    void helpers.refresh()
    expect(service.status).toHaveBeenCalledTimes(1)
    resolve({ available: true, model: 'a' })
    await vi.waitFor(() => expect(helpers.status.value.available).toBe(true))
  })
})
