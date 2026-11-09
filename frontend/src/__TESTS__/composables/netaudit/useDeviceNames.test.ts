import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ devices: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useDeviceNames } from '../../../composables/netaudit/useDeviceNames'

describe('useDeviceNames', () => {
  beforeEach(() => {
    service.devices.mockReset()
  })

  it('names devices by address from the inventory', async () => {
    service.devices.mockResolvedValue({ devices: [{ mac: 'a', ip: '192.168.1.5', name: 'Pixel 8' }] })
    const d = useDeviceNames()
    await d.load()
    expect(d.names.value).toEqual({ '192.168.1.5': 'Pixel 8' })
  })

  it('leaves no names when the devices cannot be read', async () => {
    service.devices.mockRejectedValue(new Error('down'))
    const d = useDeviceNames()
    await d.load()
    expect(d.names.value).toEqual({})
  })
})
