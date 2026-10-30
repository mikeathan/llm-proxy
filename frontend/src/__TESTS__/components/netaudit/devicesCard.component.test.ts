import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DeviceInventory, NetworkDevice, PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), nameDevice: vi.fn(), approveDevice: vi.fn(), forgetDevice: vi.fn(), confirmGateway: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const confirm = vi.hoisted(() => vi.fn())
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import DevicesCard from '../../../components/netaudit/DevicesCard.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const MAC_NEW = 'aa:bb:cc:00:00:09'
const MAC_OLD = 'aa:bb:cc:00:00:01'
const device = (over: Partial<NetworkDevice>): NetworkDevice => ({
  mac: MAC_OLD, ip: '192.168.1.5', first_seen: '2026-10-01T00:00:00Z', last_seen: '2026-10-30T11:55:00Z', approved: true, randomized: false, new: false, ...over,
})
const inventory = (over: Partial<DeviceInventory> = {}): DeviceInventory => ({
  devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', approved: false, new: true }), device({ name: 'Printer' })],
  gateway: { changed: false }, baselined: true, updated_at: '2026-10-30T12:00:00Z', ...over,
})
const presence = (over: Partial<PresenceState> = {}): PresenceState => ({ enabled: true, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30], ...over })

async function mountCard() {
  const w = mount(DevicesCard)
  await flushPromises()
  return w
}

describe('DevicesCard', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    confirm.mockReset()
    service.devices.mockResolvedValue(inventory())
    service.presence.mockResolvedValue(presence())
    useNetauditViewMode().set('simple')
  })

  it('lists the device nobody approved first, marked New, with a way to claim it', async () => {
    const w = await mountCard()
    const items = w.findAll('li[data-test^="device-"]')
    expect(items[0]!.attributes('data-test')).toBe(`device-${MAC_NEW}`)
    expect(items[0]!.text()).toContain('New')
    expect(items[0]!.text()).toContain("It's mine")
    expect(items[1]!.text()).toContain('Printer')
    expect(items[1]!.text()).not.toContain("It's mine")
  })

  it('approves a device with one tap and shows the server\'s new list', async () => {
    service.approveDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, approved: true })] }))
    const w = await mountCard()
    await w.get(`[data-test="device-${MAC_NEW}"]`).findAll('button').find((b) => b.text() === "It's mine")!.trigger('click')
    await flushPromises()
    expect(service.approveDevice).toHaveBeenCalledWith(MAC_NEW)
    expect(w.text()).not.toContain('New')
  })

  it('names a device, which approves it', async () => {
    service.nameDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, name: 'TV', approved: true })] }))
    const w = await mountCard()
    await w.get(`[data-test="device-${MAC_NEW}"]`).findAll('button').find((b) => b.text() === 'Name it')!.trigger('click')
    await w.get('input[aria-label="Device name"]').setValue('TV')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.nameDevice).toHaveBeenCalledWith(MAC_NEW, 'TV')
    expect(w.text()).toContain('TV')
  })

  it('asks before removing a device and does nothing when declined', async () => {
    confirm.mockResolvedValue(false)
    const w = await mountCard()
    await w.get(`[data-test="device-${MAC_NEW}"]`).findAll('button').find((b) => b.text() === 'Remove')!.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalled()
    expect(service.forgetDevice).not.toHaveBeenCalled()
  })

  it('shows the MAC address only in Advanced', async () => {
    const w = await mountCard()
    expect(w.text()).not.toContain(MAC_NEW)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.text()).toContain(MAC_NEW)
  })

  it('turns the check on and off', async () => {
    service.presence.mockResolvedValue(presence({ enabled: false }))
    service.setPresence.mockResolvedValue(presence({ enabled: true }))
    const w = await mountCard()
    expect(w.get('[data-test="presence-line"]').text()).toBe('Off.')
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setPresence).toHaveBeenCalledWith({ enabled: true, interval_minutes: 10 })
  })

  it('says plainly when the check cannot look, never that all is well', async () => {
    service.presence.mockResolvedValue(presence({ problem: 'this computer cannot list the devices on its network (only Linux can)' }))
    const w = await mountCard()
    expect(w.get('[data-test="presence-line"]').text()).toContain('Cannot look right now')
  })

  it('raises the alarm when the router\'s hardware address changed, and confirms on request', async () => {
    service.devices.mockResolvedValue(inventory({ gateway: { ip: '192.168.1.1', mac: MAC_NEW, changed: true } }))
    service.confirmGateway.mockResolvedValue(inventory())
    const w = await mountCard()
    expect(w.get('[data-test="gateway-changed"]').text()).toContain('hardware address changed')
    await w.get('[data-test="gateway-changed"] button').trigger('click')
    await flushPromises()
    expect(service.confirmGateway).toHaveBeenCalled()
    expect(w.find('[data-test="gateway-changed"]').exists()).toBe(false)
  })

  it('explains the baseline on a first look instead of showing an empty page', async () => {
    service.devices.mockResolvedValue(inventory({ devices: [], baselined: false }))
    const w = await mountCard()
    expect(w.text()).toContain('first look is taken as the normal state')
  })

  it('shows what failed when the list cannot be read', async () => {
    service.devices.mockRejectedValue(new Error('down'))
    service.presence.mockRejectedValue(new Error('down'))
    const w = await mountCard()
    expect(w.text()).toContain('could not be read')
  })
})
