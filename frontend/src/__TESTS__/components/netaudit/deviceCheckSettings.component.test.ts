import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), ipv6Discovery: vi.fn(), deviceNames: vi.fn(), followUpScan: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import DeviceCheckSettings from '../../../components/netaudit/DeviceCheckSettings.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const presence = (over: Partial<PresenceState> = {}): PresenceState => ({ enabled: true, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30], ...over })

async function mountSettings() {
  const w = mount(DeviceCheckSettings)
  await flushPromises()
  return w
}

describe('DeviceCheckSettings', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.devices.mockResolvedValue({ devices: [], gateway: { changed: false }, baselined: true, updated_at: '' })
    service.presence.mockResolvedValue(presence())
    service.ipv6Discovery.mockResolvedValue({ enabled: false })
    service.deviceNames.mockResolvedValue({ enabled: true })
    service.followUpScan.mockResolvedValue({ enabled: false })
    useNetauditViewMode().set('simple')
  })

  it('has no switch of its own: each part says whether it is on and points to the Modules tab', async () => {
    service.presence.mockResolvedValue(presence({ enabled: false }))
    const w = await mountSettings()
    expect(w.find('[role="switch"]').exists()).toBe(false)
    expect(w.get('[data-test="presence-line"]').text()).toBe('Off.')
    const states = w.findAll('[data-test="module-switch-state"]').map((n) => n.text().split('.')[0])
    expect(states).toEqual(['Switched off', 'Switched off', 'Switched on', 'Switched off'])
  })

  it('offers how often the check looks in Advanced only, and only while it is on', async () => {
    const simple = await mountSettings()
    expect(simple.find('select').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    const w = await mountSettings()
    expect(w.find('select[aria-label="How often to look"]').exists()).toBe(true)
  })

  it('changes how often the check looks, keeping the check on', async () => {
    useNetauditViewMode().set('advanced')
    service.setPresence.mockResolvedValue(presence({ interval_minutes: 30 }))
    const w = await mountSettings()
    await w.get('select[aria-label="How often to look"]').setValue('30')
    await flushPromises()
    expect(service.setPresence).toHaveBeenCalledWith({ enabled: true, interval_minutes: 30 })
  })

  it('says in plain words what to change when the computer refuses, and shows the exact command', async () => {
    service.ipv6Discovery.mockResolvedValue({ enabled: true, problem: 'not_permitted' })
    const w = await mountSettings()
    const note = w.get('[data-test="ipv6-problem"]')
    expect(note.attributes('role')).toBe('alert')
    expect(note.text()).toContain('ping_group_range')
    expect(note.text()).toContain('sudo sysctl')
  })

  it('shows no IPv6 problem while the switch is off', async () => {
    service.ipv6Discovery.mockResolvedValue({ enabled: false, problem: 'not_permitted' })
    const w = await mountSettings()
    expect(w.find('[data-test="ipv6-problem"]').exists()).toBe(false)
  })

  it('says why the device names setting could not be read', async () => {
    service.deviceNames.mockRejectedValue(new Error('disk full'))
    const w = await mountSettings()
    expect(w.get('[data-test="device-names"] [role="alert"]').text()).toContain('disk full')
  })

  it('shows what failed when nothing can be read', async () => {
    service.presence.mockRejectedValue(new Error('down'))
    service.devices.mockRejectedValue(new Error('down'))
    service.ipv6Discovery.mockRejectedValue(new Error('down'))
    const w = await mountSettings()
    expect(w.text()).toContain('down')
  })
})
