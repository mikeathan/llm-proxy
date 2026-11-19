import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

const service = vi.hoisted(() => ({ devices: vi.fn(), presence: vi.fn(), ipv6Discovery: vi.fn(), captureWindow: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
// The card asks which audit is the owner's, to link a device to its problems; here there is none.
vi.mock('../../../composables/netaudit/useNetworkAudit', () => ({ useNetworkAudit: () => ({ audit: ref(null), loaded: ref(true) }) }))
vi.mock('../../../composables/netaudit/useCaptureWindow', () => ({
  useCaptureWindow: () => ({ available: ref(false), summary: ref(null), loaded: ref(true), running: ref(false), error: ref(''), load: vi.fn(), listen: vi.fn() }),
}))

import NetworkDevicesCard from '../../../components/dashboard/cards/NetworkDevicesCard.vue'

const now = Date.now()
const ago = (ms: number) => new Date(now - ms).toISOString()
const device = (mac: string, over: Record<string, unknown> = {}) => ({ mac, ip: `192.168.1.${mac.length}`, first_seen: ago(9e8), last_seen: ago(60_000), approved: true, randomized: false, new: false, ...over })
const mountCard = async () => {
  const w = mount(NetworkDevicesCard, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  service.presence.mockResolvedValue({ enabled: true, interval_minutes: 10, allowed_intervals: [5, 10] })
  service.ipv6Discovery.mockResolvedValue({ enabled: false })
})

// The list itself (naming, approving, forgetting, the groups) is tested with DevicesCard; here only that the card carries it.
describe('NetworkDevicesCard', () => {
  it('shows the device list with its decisions, the devices that need a look first', async () => {
    service.devices.mockResolvedValue({
      devices: [device('aa', { name: 'Old TV', last_seen: ago(5 * 86_400_000) }), device('bbb', { name: 'Phone' }), device('cccc', { new: true, name: 'Stranger' })],
      gateway: { changed: false }, baselined: true, updated_at: ago(0),
    })
    const w = await mountCard()
    expect(w.get('[data-test="devices-summary"]').text()).not.toBe('')
    const groups = w.findAll('[data-test^="group-"]').map((g) => g.attributes('data-test'))
    expect(groups).toEqual(['group-attention', 'group-online', 'group-offline'])
    expect(w.get('[data-test="group-attention"]').text()).toContain('Stranger')
  })

  it('says it could not read the devices', async () => {
    service.devices.mockRejectedValue(new Error('down'))
    const w = await mountCard()
    expect(w.get('[role="alert"]').text()).not.toBe('')
  })
})
