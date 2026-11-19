import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { routes } from '../../../router'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const service = vi.hoisted(() => ({ ownerDevices: vi.fn(), presence: vi.fn(), live: vi.fn(), addOwnerDevice: vi.fn(), removeOwnerDevice: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import RadioMyDevices from '../../../components/radio/RadioMyDevices.vue'

const A = 'aaaaaaaa'.padEnd(64, '0')
const B = 'bbbbbbbb'.padEnd(64, '0')
const device = (id: string, label: string, cls = 'personal', by = 'rules') => ({ id, label, visits: 3, first_seen: '2026-11-10', last_seen: '2026-11-13', class: cls, class_by: by })
const hours = (n: number, every = 1) => Array.from({ length: n }, (_, i) => ({ hour: new Date(Date.UTC(2026, 10, 13, i)).toISOString(), state: i % every ? 'away' : 'here' }))
const liveView = (listening: boolean, devices: unknown[] = []) => ({ listening, cycles: 1, model: { ref_24_dbm: -40, ref_5_dbm: -40, calibrated: true, provisional: false }, devices, rings: {} })

async function show() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  const w = mount(RadioMyDevices, { global: { plugins: [router] } })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.presence.mockResolvedValue({ devices: [] })
  service.live.mockResolvedValue(liveView(true))
})

describe('RadioMyDevices', () => {
  it('leads with how many are home, puts them first, and gives each row its router box, motion and last day as words', async () => {
    service.ownerDevices.mockResolvedValue([device(A, 'Laptop'), device(B, 'Phone')])
    service.presence.mockResolvedValue({ devices: [
      { id: A, label: 'Laptop', state: 'away', last_heard: new Date().toISOString(), joined_own: true, hours: hours(24, 2) },
      { id: B, label: 'Phone', state: 'here', last_heard: new Date().toISOString(), joined_own: true, hours: hours(24) },
    ] })
    service.live.mockResolvedValue(liveView(true, [{ id: 'bbbbbbbb', mine: true, moving: true, router: { box: 'x', box_name: 'ZenWiFi', band: '5G', dbm: -70, at: 1 } }]))
    const w = await show()
    expect(w.get('[data-test="radio-my-summary"]').text()).toBe('1 of 2 devices home now')
    const rows = w.findAll('[data-test="radio-my-device"]')
    expect(rows.map((r) => r.get('[data-test="radio-my-device-name"]').text())).toEqual(['Phone', 'Laptop'])
    expect(rows[0]!.get('[data-test="radio-my-device-facts"]').text()).toContain('Joined to ZenWiFi · Last heard')
    expect(rows[0]!.get('[data-test="radio-my-device-hours"]').text()).toMatch(/^Heard in 24 of the last 24 hours: since \d/)
    expect(rows[1]!.get('[data-test="radio-my-device-hours"]').text()).toMatch(/^Heard in 12 of the last 24 hours: .+ and .+ \(9 earlier\)$/)
    expect(rows[0]!.get('a').attributes('href')).toContain('bbbbbbbb')
    expect(w.find('[data-test="radio-presence-hour"]').exists()).toBe(false)
    expect(w.find('[data-test="radio-my-device-remove"]').exists()).toBe(false)
  })

  it('shows two devices with one name as a problem with its fix', async () => {
    service.ownerDevices.mockResolvedValue([device(A, 'Phone or tablet'), device(B, 'Phone or tablet')])
    const w = await show()
    expect(w.get('[data-test="radio-my-dups"]').text()).toContain('2 devices are called “Phone or tablet”. Open each one and rename it')
  })

  it('says who is home is not known while not listening, with the way to start', async () => {
    service.ownerDevices.mockResolvedValue([device(A, 'Laptop')])
    service.live.mockResolvedValue(liveView(false))
    const w = await show()
    expect(w.get('[data-test="radio-my-summary"]').text()).toContain('Not listening now, so who is home is not known.')
    expect(w.get('[data-test="radio-my-start"]').text()).toBe('Start listening')
  })

  it('still lists the devices when the presence and the live picture cannot be read', async () => {
    service.ownerDevices.mockResolvedValue([device(A, 'Laptop')])
    service.presence.mockRejectedValue(new Error('down'))
    service.live.mockRejectedValue(new Error('down'))
    const w = await show()
    expect(w.get('[data-test="radio-my-device-name"]').text()).toBe('Laptop')
    expect(w.get('[data-test="radio-my-device"]').attributes('data-state')).toBe('not_listening')
  })

  it('in Advanced, shows the class and never the hour squares, and has no remove button: removing is on the Radio page', async () => {
    useNetauditViewMode().set('advanced')
    service.ownerDevices.mockResolvedValue([device(A, 'Laptop', 'personal', 'owner')])
    service.presence.mockResolvedValue({ devices: [{ id: A, label: 'Laptop', state: 'here', signal: 'strong', joined_own: true, hours: hours(48) }] })
    const w = await show()
    expect(w.get('[data-test="radio-my-device-details"]').text()).toBe('Comes and goes (your word) · Strong signal')
    expect(w.find('[data-test="radio-presence-hour"]').exists()).toBe(false)
    expect(w.get('[data-test="radio-my-device-hours"]').text()).toMatch(/^Heard in 24 of the last 24 hours: since /)
    expect(w.find('[data-test="radio-my-device-remove"]').exists()).toBe(false)
    expect(w.find('[data-test="radio-add-open"]').exists()).toBe(false)
  })
})
