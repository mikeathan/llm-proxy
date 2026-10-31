import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DeviceInventory, NetworkDevice, PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), ipv6Discovery: vi.fn(), setIpv6Discovery: vi.fn(), nameDevice: vi.fn(), approveDevice: vi.fn(), dismissSuggestion: vi.fn(), forgetDevice: vi.fn(), confirmGateway: vi.fn(), deviceNames: vi.fn(), setDeviceNames: vi.fn(),
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
    service.ipv6Discovery.mockResolvedValue({ enabled: false })
    service.deviceNames.mockResolvedValue({ enabled: true })
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

  it('switches IPv6 discovery on with one tap', async () => {
    service.setIpv6Discovery.mockResolvedValue({ enabled: true })
    const w = await mountCard()
    await w.get('[data-test="ipv6-discovery"] [role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setIpv6Discovery).toHaveBeenCalledWith(true)
  })

  it('says in plain words what to change when the computer refuses, and shows the exact command', async () => {
    service.ipv6Discovery.mockResolvedValue({ enabled: true, problem: 'not_permitted' })
    const w = await mountCard()
    const note = w.get('[data-test="ipv6-problem"]')
    expect(note.attributes('role')).toBe('alert')
    expect(note.text()).toContain('ping_group_range')
    expect(note.text()).toContain('sudo sysctl')
  })

  it('shows no IPv6 problem while the switch is off', async () => {
    service.ipv6Discovery.mockResolvedValue({ enabled: false, problem: 'not_permitted' })
    const w = await mountCard()
    expect(w.find('[data-test="ipv6-problem"]').exists()).toBe(false)
  })

  it('folds the list to one summary line when nothing is new', async () => {
    service.devices.mockResolvedValue(inventory({ devices: [device({ name: 'Printer' }), device({ mac: MAC_NEW, ip: '192.168.1.9' })] }))
    const w = await mountCard()
    expect(w.get('[data-test="devices-list"]').text()).toContain('2 devices')
    const folded = () => (w.get('[data-test="devices-list"] [role="region"]').attributes('style') ?? '').includes('display: none')
    expect(folded()).toBe(true)
    await w.get('[data-test="devices-list"] button[aria-expanded]').trigger('click')
    expect(folded()).toBe(false)
  })

  it('opens itself when a device is new, so it is never hidden behind a count', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="devices-list"]').text()).toContain('1 new')
    expect(w.get('[data-test="devices-list"] button[aria-expanded]').attributes('aria-expanded')).toBe('true')
  })

  it('stays open after the last new device is approved', async () => {
    service.approveDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, approved: true })] }))
    const w = await mountCard()
    await w.get(`[data-test="device-${MAC_NEW}"]`).findAll('button').find((b) => b.text() === "It's mine")!.trigger('click')
    await flushPromises()
    expect(w.get('[data-test="devices-list"] button[aria-expanded]').attributes('aria-expanded')).toBe('true')
  })

  it('splits the list into an IPv4 and an IPv6 tab with their counts', async () => {
    const v6only = device({ mac: 'aa:bb:cc:00:00:07', ip: undefined, ipv6: ['fe80::7'], name: 'Sensor' })
    const dual = device({ name: 'Printer', ipv6: ['fe80::5', 'fd00::5'] })
    service.devices.mockResolvedValue(inventory({ devices: [dual, v6only] }))
    const w = await mountCard()
    await w.get('[data-test="devices-list"] button[aria-expanded]').trigger('click')
    const tabs = w.get('[role="radiogroup"][aria-label="Address type"]')
    expect(tabs.text()).toContain('IPv4 (1)')
    expect(tabs.text()).toContain('IPv6 (2)')
    expect(w.text()).toContain('Printer')
    expect(w.text()).not.toContain('Sensor')
    await tabs.findAll('[role="radio"]').find((b) => b.text().startsWith('IPv6'))!.trigger('click')
    expect(w.text()).toContain('Sensor')
    expect(w.text()).toContain('fe80::7')
    expect(w.text()).toContain('fe80::5 and 1 more')
  })

  it('tells the person how to get IPv6 devices when the switch is off and none are listed', async () => {
    service.devices.mockResolvedValue(inventory({ devices: [device({ name: 'Printer' })] }))
    const w = await mountCard()
    await w.get('[data-test="devices-list"] button[aria-expanded]').trigger('click')
    await w.findAll('[role="radio"]').find((b) => b.text().startsWith('IPv6'))!.trigger('click')
    expect(w.get('[data-test="devices-empty-ipv6"]').text()).toContain('Look for IPv6 devices')
  })

  describe('a name the device gave when asked', () => {
    const suggestionFor = (over: Partial<NetworkDevice>) => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', suggested_name: 'Samsung', ...over })] }))
    }

    it('is offered for one tap on an approved device that has no name', async () => {
      suggestionFor({ approved: true, new: false })
      service.nameDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, name: 'Samsung' })] }))
      const w = await mountCard()
      const row = w.get(`[data-test="device-${MAC_NEW}"]`)
      expect(row.get('[data-test="suggestion"]').text()).toContain('It calls itself “Samsung”.')
      await row.findAll('button').find((b) => b.text() === 'Use this name')!.trigger('click')
      await flushPromises()
      expect(service.nameDevice).toHaveBeenCalledWith(MAC_NEW, 'Samsung')
    })

    it('is only information on a new device, because naming a device approves it', async () => {
      suggestionFor({ approved: false, new: true })
      const w = await mountCard()
      const row = w.get(`[data-test="device-${MAC_NEW}"]`)
      expect(row.get('[data-test="suggestion"]').text()).toContain('check it is yours first')
      expect(row.findAll('button').some((b) => b.text() === 'Use this name')).toBe(false)
    })

    it('disappears once the person has named the device, and prefills the naming form while it shows', async () => {
      suggestionFor({ approved: true, new: false, name: 'Mum phone' })
      let w = await mountCard()
      expect(w.find('[data-test="suggestion"]').exists()).toBe(false)
      suggestionFor({ approved: true, new: false })
      w = await mountCard()
      await w.get(`[data-test="device-${MAC_NEW}"]`).findAll('button').find((b) => b.text() === 'Name it')!.trigger('click')
      expect((w.get('input[aria-label="Device name"]').element as HTMLInputElement).value).toBe('Samsung')
    })
  })

  describe('the switch for asking devices their names', () => {
    it('shows the setting as the server has it and saves a change at once', async () => {
      service.deviceNames.mockResolvedValue({ enabled: true })
      service.setDeviceNames.mockResolvedValue({ enabled: false })
      const w = await mountCard()
      const toggle = w.get('[data-test="device-names"] input[type="checkbox"], [data-test="device-names"] [role="switch"]')
      expect((toggle.element as HTMLInputElement).checked).toBe(true)
      await toggle.setValue(false)
      await flushPromises()
      expect(service.setDeviceNames).toHaveBeenCalledWith(false)
    })

    it('says why it could not be saved', async () => {
      service.setDeviceNames.mockRejectedValue(new Error('disk full'))
      const w = await mountCard()
      await w.get('[data-test="device-names"] input[type="checkbox"], [data-test="device-names"] [role="switch"]').setValue(false)
      await flushPromises()
      expect(w.get('[data-test="device-names"] [role="alert"]').text()).toContain('disk full')
    })
  })

  describe('suggestions, hints and the naming list', () => {
    const unnamed = (mac: string, ip: string, over: Partial<NetworkDevice> = {}) => device({ mac, ip, ...over })

    it('lets the person reject a suggested name, on a new device too', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [unnamed(MAC_NEW, '192.168.1.77', { approved: false, new: true, suggested_name: 'Samsung' })] }))
      service.dismissSuggestion.mockResolvedValue(inventory({ devices: [unnamed(MAC_NEW, '192.168.1.77', { approved: false, new: true })] }))
      const w = await mountCard()
      const row = w.get(`[data-test="device-${MAC_NEW}"]`)
      expect(row.find('[data-test="suggestion"]').exists()).toBe(true)
      await row.findAll('button').find((b) => b.text() === 'Not this')!.trigger('click')
      await flushPromises()
      expect(service.dismissSuggestion).toHaveBeenCalledWith(MAC_NEW)
      expect(w.find('[data-test="suggestion"]').exists()).toBe(false)
    })

    it('shows what helps tell an unnamed device apart: the open ports from the last scan', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [unnamed(MAC_OLD, '192.168.1.5', { open_ports: [22, 80], ports_scanned_at: '2026-10-29T12:00:00Z' })] }))
      const w = await mountCard()
      expect(w.get(`[data-test="device-${MAC_OLD}"]`).text()).toContain('Open ports: 22, 80')
    })

    it('offers a naming list only when there are devices the person can name, and counts only those', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [
        unnamed('aa:bb:cc:00:00:01', '192.168.1.5'),
        unnamed('aa:bb:cc:00:00:02', '192.168.1.6'),
        unnamed('aa:bb:cc:00:00:03', '192.168.1.7', { randomized: true }),
        unnamed('aa:bb:cc:00:00:04', '192.168.1.8', { name: 'Printer' }),
      ] }))
      const w = await mountCard()
      expect(w.get('[data-test="unnamed-prompt"]').text()).toContain('2 devices have no name')
      service.devices.mockResolvedValue(inventory({ devices: [unnamed('aa:bb:cc:00:00:04', '192.168.1.8', { name: 'Printer' })] }))
      const named = await mountCard()
      expect(named.find('[data-test="unnamed-prompt"]').exists()).toBe(false)
    })

    it('shows only the unnamed devices in the naming list, and a device leaves it once named', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [
        unnamed('aa:bb:cc:00:00:01', '192.168.1.5'),
        unnamed('aa:bb:cc:00:00:04', '192.168.1.8', { name: 'Printer' }),
      ] }))
      service.nameDevice.mockResolvedValue(inventory({ devices: [
        unnamed('aa:bb:cc:00:00:01', '192.168.1.5', { name: 'Hallway camera' }),
        unnamed('aa:bb:cc:00:00:04', '192.168.1.8', { name: 'Printer' }),
      ] }))
      const w = await mountCard()
      expect(w.get('[data-test="devices-list"] button[aria-expanded]').attributes('aria-expanded')).toBe('false')
      await w.get('[data-test="unnamed-show"]').trigger('click')
      expect(w.get('[data-test="devices-list"] button[aria-expanded]').attributes('aria-expanded')).toBe('true')
      expect(w.findAll('li[data-test^="device-"]')).toHaveLength(1)
      await w.get('li[data-test^="device-"]').findAll('button').find((b) => b.text() === 'Name it')!.trigger('click')
      await w.get('input[aria-label="Device name"]').setValue('Hallway camera')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(service.nameDevice).toHaveBeenCalledWith('aa:bb:cc:00:00:01', 'Hallway camera')
      expect(w.find('[data-test="unnamed-prompt"]').exists()).toBe(false)
      expect(w.findAll('li[data-test^="device-"]')).toHaveLength(2)
    })
  })
})
