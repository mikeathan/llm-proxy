import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { PresenceState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  devices: vi.fn(), presence: vi.fn(), setPresence: vi.fn(), ipv6Discovery: vi.fn(), setIpv6Discovery: vi.fn(), deviceNames: vi.fn(), setDeviceNames: vi.fn(),
  followUpScan: vi.fn(), setFollowUpScan: vi.fn(),
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

  it('turns the new-device check on and off', async () => {
    service.presence.mockResolvedValue(presence({ enabled: false }))
    service.setPresence.mockResolvedValue(presence({ enabled: true }))
    const w = await mountSettings()
    expect(w.get('[data-test="presence-line"]').text()).toBe('Off.')
    await w.get('[aria-label="New-device check"] [role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setPresence).toHaveBeenCalledWith({ enabled: true, interval_minutes: 10 })
  })

  it('offers how often the check looks in Advanced only, and only while it is on', async () => {
    const simple = await mountSettings()
    expect(simple.find('select').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    const w = await mountSettings()
    expect(w.find('select[aria-label="How often to look"]').exists()).toBe(true)
  })

  it('offers the scan on arrival off by default and switches it on with one tap', async () => {
    service.setFollowUpScan.mockResolvedValue({ enabled: true })
    const w = await mountSettings()
    const toggle = w.get('[data-test="follow-up-scan"] [role="switch"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(false)
    await toggle.setValue(true)
    await flushPromises()
    expect(service.setFollowUpScan).toHaveBeenCalledWith(true)
  })

  it('switches IPv6 discovery on with one tap', async () => {
    service.setIpv6Discovery.mockResolvedValue({ enabled: true })
    const w = await mountSettings()
    await w.get('[data-test="ipv6-discovery"] [role="switch"]').setValue(true)
    await flushPromises()
    expect(service.setIpv6Discovery).toHaveBeenCalledWith(true)
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

  describe('the switch for asking devices their names', () => {
    it('shows the setting as the server has it and saves a change at once', async () => {
      service.setDeviceNames.mockResolvedValue({ enabled: false })
      const w = await mountSettings()
      const toggle = w.get('[data-test="device-names"] [role="switch"]')
      expect((toggle.element as HTMLInputElement).checked).toBe(true)
      await toggle.setValue(false)
      await flushPromises()
      expect(service.setDeviceNames).toHaveBeenCalledWith(false)
    })

    it('says why it could not be saved', async () => {
      service.setDeviceNames.mockRejectedValue(new Error('disk full'))
      const w = await mountSettings()
      await w.get('[data-test="device-names"] [role="switch"]').setValue(false)
      await flushPromises()
      expect(w.get('[data-test="device-names"] [role="alert"]').text()).toContain('disk full')
    })
  })

  it('shows what failed when nothing can be read', async () => {
    service.presence.mockRejectedValue(new Error('down'))
    service.devices.mockRejectedValue(new Error('down'))
    service.ipv6Discovery.mockRejectedValue(new Error('down'))
    const w = await mountSettings()
    expect(w.text()).toContain('down')
  })
})
