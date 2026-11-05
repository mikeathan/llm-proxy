import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DeviceInventory, NetworkDevice, PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), ipv6Discovery: vi.fn(), setIpv6Discovery: vi.fn(), nameDevice: vi.fn(), markImportant: vi.fn(), approveDevice: vi.fn(), dismissSuggestion: vi.fn(), forgetDevice: vi.fn(), confirmGateway: vi.fn(), deviceNames: vi.fn(), setDeviceNames: vi.fn(),
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

const row = (w: Awaited<ReturnType<typeof mountCard>>, mac: string) => w.get(`[data-test="device-${mac}"]`)
const open = async (w: Awaited<ReturnType<typeof mountCard>>, mac: string) => {
  const r = row(w, mac)
  if (r.find('[data-test="device-toggle"]').attributes('aria-expanded') !== 'true') await r.get('[data-test="device-toggle"]').trigger('click')
  return r
}
const press = async (r: ReturnType<typeof row>, label: string) => r.findAll('button').find((b) => b.text() === label)!.trigger('click')
const group = (w: Awaited<ReturnType<typeof mountCard>>, status: string) => w.get(`[data-test="group-${status}"]`)
const folded = (w: Awaited<ReturnType<typeof mountCard>>, status: string) => (group(w, status).get('[role="region"]').attributes('style') ?? '').includes('display: none')
const chip = (w: Awaited<ReturnType<typeof mountCard>>, label: string) => w.findAll('[role="radio"]').find((b) => b.text().startsWith(label))!

describe('DevicesCard', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-10-30T12:00:00Z'))
    for (const fn of Object.values(service)) fn.mockReset()
    confirm.mockReset()
    service.devices.mockResolvedValue(inventory())
    service.presence.mockResolvedValue(presence())
    service.ipv6Discovery.mockResolvedValue({ enabled: false })
    service.deviceNames.mockResolvedValue({ enabled: true })
    useNetauditViewMode().set('simple')
  })
  afterEach(() => vi.useRealTimers())

  describe('the list', () => {
    it('marks a device important from its open row without approving it, and says so once it is', async () => {
      service.markImportant.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', approved: false, new: true, important: true })] }))
      const w = await mountCard()
      const r = await open(w, MAC_NEW)
      await press(r, 'Mark as important')
      await flushPromises()
      expect(service.markImportant).toHaveBeenCalledWith(MAC_NEW, true)
      expect(service.approveDevice).not.toHaveBeenCalled()
      expect(row(w, MAC_NEW).get('[data-test="device-important"]').text()).toBe('Marked as important.')
      expect(row(w, MAC_NEW).get('[data-test="device-important-toggle"]').text()).toBe('Not important')
      w.unmount()
    })

    it('shows the maker of the card in the open row, and calls an unnamed device by it', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', approved: false, new: true, vendor: 'Espressif Inc.' })] }))
      const w = await mountCard()
      const r = await open(w, MAC_NEW)
      expect(r.text()).toContain('Espressif Inc. device')
      expect(r.get('[data-test="device-maker"]').text()).toBe('Made by Espressif Inc.')
    })

    it('puts a device nobody approved first, in Needs a look, open, with a way to claim it', async () => {
      const w = await mountCard()
      const groups = w.findAll('[data-test^="group-"]').map((g) => g.attributes('data-test'))
      expect(groups).toEqual(['group-attention', 'group-online'])
      expect(folded(w, 'attention')).toBe(false)
      const fresh = row(w, MAC_NEW)
      expect(fresh.text()).toContain('New')
      expect(fresh.find('[data-test="device-toggle"]').attributes('aria-expanded')).toBe('true')
      expect(fresh.text()).toContain("It's mine")
      expect(group(w, 'online').text()).toContain('Printer')
    })

    it('shows name, address and status on the row, and the rest only when the row is opened', async () => {
      const w = await mountCard()
      const printer = row(w, MAC_OLD)
      expect(printer.get('[data-test="device-toggle"]').attributes('aria-expanded')).toBe('false')
      expect(printer.get('[data-test="device-toggle"]').text()).toContain('Printer')
      expect(printer.get('[data-test="device-line"]').text()).toBe('192.168.1.5')
      expect(printer.get('[data-test="device-status"]').text()).toContain('Online')
      expect(printer.findAll('button').some((b) => b.text() === 'Rename')).toBe(false)
      await printer.get('[data-test="device-toggle"]').trigger('click')
      expect(printer.get('[data-test="device-toggle"]').attributes('aria-expanded')).toBe('true')
      expect(printer.findAll('button').some((b) => b.text() === 'Rename')).toBe(true)
      await printer.get('[data-test="device-toggle"]').trigger('click')
      expect(printer.findAll('button').some((b) => b.text() === 'Rename')).toBe(false)
    })

    it('keeps a row a 44 px target on a phone', async () => {
      const w = await mountCard()
      expect(row(w, MAC_OLD).get('[data-test="device-toggle"]').classes()).toContain('min-h-11')
    })

    it('folds Offline to one line, and says how many are in it', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ name: 'Printer' }), device({ mac: 'aa:bb:cc:00:00:02', name: 'Old laptop', last_seen: '2026-10-01T00:00:00Z' })] }))
      const w = await mountCard()
      expect(folded(w, 'online')).toBe(false)
      expect(folded(w, 'offline')).toBe(true)
      expect(group(w, 'offline').text()).toContain('1 device')
      await group(w, 'offline').get('button[aria-expanded]').trigger('click')
      expect(folded(w, 'offline')).toBe(false)
    })

    it('says what it holds in one line', async () => {
      const w = await mountCard()
      expect(w.get('[data-test="devices-summary"]').text()).toBe('2 devices · 1 new')
    })

    it('lists a device only seen over IPv6 by that address, and every address in Advanced', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ mac: 'aa:bb:cc:00:00:07', ip: undefined, ipv6: ['fe80::7', 'fd00::7'], name: 'Sensor' })] }))
      const w = await mountCard()
      expect(row(w, 'aa:bb:cc:00:00:07').get('[data-test="device-line"]').text()).toBe('fe80::7 and 1 more')
      useNetauditViewMode().set('advanced')
      const r = await open(w, 'aa:bb:cc:00:00:07')
      expect(r.get('[data-test="device-ipv6"]').text()).toBe('fe80::7, fd00::7')
    })
  })

  describe('search and filters', () => {
    const many = () => service.devices.mockResolvedValue(inventory({ devices: [
      device({ mac: MAC_NEW, ip: '192.168.1.77', approved: false, new: true }),
      device({ mac: 'aa:bb:cc:00:00:02', ip: '192.168.1.20', name: 'Living room TV' }),
      device({ mac: 'aa:bb:cc:00:00:03', ip: '192.168.1.21', name: 'Printer' }),
      device({ mac: 'aa:bb:cc:00:00:04', ip: '192.168.1.30', name: 'Old laptop', last_seen: '2026-10-01T00:00:00Z' }),
    ] }))
    const macsShown = (w: Awaited<ReturnType<typeof mountCard>>) => w.findAll('li[data-test^="device-"]').map((li) => li.attributes('data-test')!.replace('device-', ''))

    it('has a labelled search box and a chip for each filter, each with its count', async () => {
      many()
      const w = await mountCard()
      expect(w.find('input[aria-label="Search devices"]').exists()).toBe(true)
      const chips = w.get('[role="radiogroup"][aria-label="Show devices"]').findAll('[role="radio"]').map((b) => b.text())
      expect(chips).toEqual(['All (4)', 'Needs a look (1)', 'Online (2)', 'Offline (1)', 'Unnamed (1)'])
    })

    it('finds a device by name or address as the person types', async () => {
      many()
      const w = await mountCard()
      await w.get('[data-test="devices-search"]').setValue('living')
      expect(macsShown(w)).toEqual(['aa:bb:cc:00:00:02'])
      await w.get('[data-test="devices-search"]').setValue('192.168.1.21')
      expect(macsShown(w)).toEqual(['aa:bb:cc:00:00:03'])
    })

    it('narrows to one filter, and opens the Offline group when that is what was asked for', async () => {
      many()
      const w = await mountCard()
      await chip(w, 'Offline').trigger('click')
      expect(macsShown(w)).toEqual(['aa:bb:cc:00:00:04'])
      expect(folded(w, 'offline')).toBe(false)
      await chip(w, 'Needs a look').trigger('click')
      expect(macsShown(w)).toEqual([MAC_NEW])
    })

    it('says so when nothing matches, and All brings everything back', async () => {
      many()
      const w = await mountCard()
      await w.get('[data-test="devices-search"]').setValue('nothing like this')
      expect(w.get('[data-test="devices-no-match"]').text()).toContain('No device matches')
      await w.get('[data-test="devices-search"]').setValue('')
      expect(macsShown(w)).toHaveLength(4)
    })

    it('never hides a new device under All, wherever the search is empty', async () => {
      many()
      const w = await mountCard()
      expect(macsShown(w)[0]).toBe(MAC_NEW)
    })

    it('explains why naming helps when the Unnamed filter is on', async () => {
      many()
      const w = await mountCard()
      expect(w.find('[data-test="unnamed-hint"]').exists()).toBe(false)
      await chip(w, 'Unnamed').trigger('click')
      expect(w.get('[data-test="unnamed-hint"]').text()).toContain('not been on the network for days')
    })
  })

  describe('deciding about a device', () => {
    it('approves a device with one tap and shows the server\'s new list', async () => {
      service.approveDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, approved: true })] }))
      const w = await mountCard()
      await press(row(w, MAC_NEW), "It's mine")
      await flushPromises()
      expect(service.approveDevice).toHaveBeenCalledWith(MAC_NEW)
      expect(w.text()).not.toContain('New')
    })

    it('names a device, which approves it', async () => {
      service.nameDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, name: 'TV', approved: true })] }))
      const w = await mountCard()
      await press(row(w, MAC_NEW), 'Name it')
      await w.get('input[aria-label="Device name"]').setValue('TV')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(service.nameDevice).toHaveBeenCalledWith(MAC_NEW, 'TV')
      expect(w.text()).toContain('TV')
    })

    it('opens a folded row to name it', async () => {
      const w = await mountCard()
      expect(row(w, MAC_OLD).find('input').exists()).toBe(false)
      await press(await open(w, MAC_OLD), 'Rename')
      expect((w.get('input[aria-label="Device name"]').element as HTMLInputElement).value).toBe('Printer')
    })

    it('keeps the naming form open when the name cannot be saved, and says why', async () => {
      service.nameDevice.mockRejectedValue(new Error('disk full'))
      const w = await mountCard()
      await press(row(w, MAC_NEW), 'Name it')
      await w.get('input[aria-label="Device name"]').setValue('TV')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(w.find('input[aria-label="Device name"]').exists()).toBe(true)
      expect(w.get('[role="alert"]').text()).toContain('disk full')
    })

    it('asks before removing a device and does nothing when declined', async () => {
      confirm.mockResolvedValue(false)
      const w = await mountCard()
      await press(row(w, MAC_NEW), 'Remove')
      await flushPromises()
      expect(confirm).toHaveBeenCalled()
      expect(service.forgetDevice).not.toHaveBeenCalled()
    })
  })

  describe('Simple and Advanced', () => {
    it('shows the hardware address and the open-port list only in Advanced, and a count in Simple', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ name: 'Camera', open_ports: [22, 80], ports_scanned_at: '2026-10-29T12:00:00Z' })] }))
      const w = await mountCard()
      let r = await open(w, MAC_OLD)
      expect(r.text()).not.toContain(MAC_OLD)
      expect(r.get('[data-test="device-ports"]').text()).toContain('2 open ports')
      expect(r.get('[data-test="device-ports"]').text()).not.toContain('22')
      useNetauditViewMode().set('advanced')
      await flushPromises()
      r = row(w, MAC_OLD)
      expect(r.text()).toContain(MAC_OLD)
      expect(r.get('[data-test="device-ports"]').text()).toContain('Open ports: 22, 80')
    })

    it('adds to the page in Advanced and removes nothing from Simple', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ name: 'Camera', ipv6: ['fe80::5'] })] }))
      const w = await mountCard()
      await open(w, MAC_OLD)
      const simple = row(w, MAC_OLD).findAll('button').map((b) => b.text()).filter(Boolean)
      useNetauditViewMode().set('advanced')
      await flushPromises()
      const advanced = row(w, MAC_OLD).findAll('button').map((b) => b.text()).filter(Boolean)
      for (const label of simple) expect(advanced).toContain(label)
      expect(row(w, MAC_OLD).text()).toContain('fe80::5')
    })
  })

  describe('the check and the list around it', () => {
    it('says whether the check is on, and leaves its switch to the Settings tab', async () => {
      service.presence.mockResolvedValue(presence({ enabled: false }))
      const w = await mountCard()
      expect(w.get('[data-test="presence-line"]').text()).toBe('Off.')
      expect(w.get('[data-test="check-off"]').text()).toContain('only as fresh as the last scan')
      expect(w.find('[role="switch"]').exists()).toBe(false)
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
      expect(w.get('[data-test="devices-empty"]').text()).toContain('No devices seen yet')
    })

    it('shows what failed when the list cannot be read', async () => {
      service.devices.mockRejectedValue(new Error('down'))
      service.presence.mockRejectedValue(new Error('down'))
      const w = await mountCard()
      expect(w.text()).toContain('could not be read')
    })
  })

  describe('a name the device gave when asked', () => {
    const suggestionFor = (over: Partial<NetworkDevice>) => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', suggested_name: 'Samsung', ...over })] }))
    }

    it('is offered for one tap on an approved device that has no name', async () => {
      suggestionFor({ approved: true, new: false })
      service.nameDevice.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, name: 'Samsung' })] }))
      const w = await mountCard()
      const r = await open(w, MAC_NEW)
      expect(r.get('[data-test="suggestion"]').text()).toContain('It calls itself “Samsung”.')
      await press(r, 'Use this name')
      await flushPromises()
      expect(service.nameDevice).toHaveBeenCalledWith(MAC_NEW, 'Samsung')
    })

    it('is only information on a new device, because naming a device approves it', async () => {
      suggestionFor({ approved: false, new: true })
      const w = await mountCard()
      const r = row(w, MAC_NEW)
      expect(r.get('[data-test="suggestion"]').text()).toContain('check it is yours first')
      expect(r.findAll('button').some((b) => b.text() === 'Use this name')).toBe(false)
    })

    it('disappears once the person has named the device, and prefills the naming form while it shows', async () => {
      suggestionFor({ approved: true, new: false, name: 'Mum phone' })
      let w = await mountCard()
      await open(w, MAC_NEW)
      expect(w.find('[data-test="suggestion"]').exists()).toBe(false)
      suggestionFor({ approved: true, new: false })
      w = await mountCard()
      await press(await open(w, MAC_NEW), 'Name it')
      expect((w.get('input[aria-label="Device name"]').element as HTMLInputElement).value).toBe('Samsung')
    })

    it('lets the person reject a suggested name, on a new device too', async () => {
      suggestionFor({ approved: false, new: true })
      service.dismissSuggestion.mockResolvedValue(inventory({ devices: [device({ mac: MAC_NEW, ip: '192.168.1.77', approved: false, new: true })] }))
      const w = await mountCard()
      const r = row(w, MAC_NEW)
      expect(r.find('[data-test="suggestion"]').exists()).toBe(true)
      await press(r, 'Not this')
      await flushPromises()
      expect(service.dismissSuggestion).toHaveBeenCalledWith(MAC_NEW)
      expect(w.find('[data-test="suggestion"]').exists()).toBe(false)
    })
  })

  describe('naming the unnamed', () => {
    it('shows only the unnamed devices under Unnamed, and a device leaves them once named', async () => {
      service.devices.mockResolvedValue(inventory({ devices: [device({ mac: 'aa:bb:cc:00:00:01' }), device({ mac: 'aa:bb:cc:00:00:04', ip: '192.168.1.8', name: 'Printer' })] }))
      service.nameDevice.mockResolvedValue(inventory({ devices: [device({ mac: 'aa:bb:cc:00:00:01', name: 'Hallway camera' }), device({ mac: 'aa:bb:cc:00:00:04', ip: '192.168.1.8', name: 'Printer' })] }))
      const w = await mountCard()
      await chip(w, 'Unnamed').trigger('click')
      expect(w.findAll('li[data-test^="device-"]')).toHaveLength(1)
      await press(await open(w, 'aa:bb:cc:00:00:01'), 'Name it')
      await w.get('input[aria-label="Device name"]').setValue('Hallway camera')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(service.nameDevice).toHaveBeenCalledWith('aa:bb:cc:00:00:01', 'Hallway camera')
      expect(w.find('[data-test="devices-no-match"]').exists()).toBe(true)
      await chip(w, 'All').trigger('click')
      expect(w.findAll('li[data-test^="device-"]')).toHaveLength(2)
    })
  })
})
