import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeviceInventory, IPv6DiscoveryState, PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), ipv6Discovery: vi.fn(), setIpv6Discovery: vi.fn(), nameDevice: vi.fn(), approveDevice: vi.fn(), forgetDevice: vi.fn(), confirmGateway: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useDevices } from '../../../composables/netaudit/useDevices'

const inv = (over: Partial<DeviceInventory> = {}): DeviceInventory => ({ devices: [], gateway: { changed: false }, baselined: true, updated_at: '2026-10-30T00:00:00Z', ...over })
const state = (over: Partial<PresenceState> = {}): PresenceState => ({ enabled: false, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30], ...over })

describe('useDevices', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.devices.mockResolvedValue(inv())
    service.presence.mockResolvedValue(state())
    service.ipv6Discovery.mockResolvedValue({ enabled: false } satisfies IPv6DiscoveryState)
  })

  it('loads the list and the check together', async () => {
    const d = useDevices()
    await d.load()
    expect(d.inventory.value?.baselined).toBe(true)
    expect(d.presence.value?.enabled).toBe(false)
    expect(d.loadError.value).toBe('')
  })

  it('keeps the check readable when the list fails, and says so', async () => {
    service.devices.mockRejectedValue(new Error('boom'))
    const d = useDevices()
    await d.load()
    expect(d.presence.value).not.toBeNull()
    expect(d.loadError.value).not.toBe('')
  })

  it('replaces the list with the server\'s answer to a change', async () => {
    const after = inv({ devices: [{ mac: 'aa:bb:cc:00:00:01', ip: '192.168.1.5', name: 'Printer', first_seen: '', last_seen: '', approved: true, randomized: false, new: false }] })
    service.nameDevice.mockResolvedValue(after)
    const d = useDevices()
    expect(await d.name('aa:bb:cc:00:00:01', 'Printer')).toBe(true)
    expect(d.inventory.value).toEqual(after)
    expect(service.nameDevice).toHaveBeenCalledWith('aa:bb:cc:00:00:01', 'Printer')
  })

  it('keeps the old list and says what failed when a change is refused', async () => {
    service.forgetDevice.mockRejectedValue(new Error('no such device in the inventory'))
    const d = useDevices()
    await d.load()
    expect(await d.forget('aa:bb:cc:00:00:01')).toBe(false)
    expect(d.inventory.value?.baselined).toBe(true)
    expect(d.actionError.value).toContain('no such device')
  })

  it('turns the check on and shows the saved state', async () => {
    service.setPresence.mockResolvedValue(state({ enabled: true, interval_minutes: 15 }))
    const d = useDevices()
    await d.setPresence(true, 15)
    expect(service.setPresence).toHaveBeenCalledWith({ enabled: true, interval_minutes: 15 })
    expect(d.presence.value?.enabled).toBe(true)
  })

  it('reports a refused router confirmation', async () => {
    service.confirmGateway.mockRejectedValue(new Error('step-up required'))
    const d = useDevices()
    expect(await d.confirmGateway()).toBe(false)
    expect(d.actionError.value).not.toBe('')
  })

  it('loads the IPv6 switch with the rest and replaces it with the server\'s answer to a change', async () => {
    service.ipv6Discovery.mockResolvedValue({ enabled: false })
    service.setIpv6Discovery.mockResolvedValue({ enabled: true, problem: 'not_permitted' })
    const d = useDevices()
    await d.load()
    expect(d.ipv6.value).toEqual({ enabled: false })
    await d.setIPv6(true)
    expect(service.setIpv6Discovery).toHaveBeenCalledWith(true)
    expect(d.ipv6.value).toEqual({ enabled: true, problem: 'not_permitted' })
  })

  it('keeps the list and the check readable when the IPv6 switch cannot be read', async () => {
    service.ipv6Discovery.mockRejectedValue(new Error('boom'))
    const d = useDevices()
    await d.load()
    expect(d.inventory.value).not.toBeNull()
    expect(d.ipv6.value).toBeNull()
  })

  it('keeps the old IPv6 state and says what failed when the change is refused', async () => {
    service.setIpv6Discovery.mockRejectedValue(new Error('disk full'))
    const d = useDevices()
    await d.load()
    await d.setIPv6(true)
    expect(d.ipv6.value).toEqual({ enabled: false })
    expect(d.actionError.value).not.toBe('')
  })
})
