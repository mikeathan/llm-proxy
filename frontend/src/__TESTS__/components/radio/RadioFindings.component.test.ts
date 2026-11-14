import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioDigest } from '../../../types/radio'

const service = vi.hoisted(() => ({ digest: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioFindings from '../../../components/radio/RadioFindings.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const digest = (over: Partial<RadioDigest> = {}): RadioDigest => ({
  listening: { cycles: 30, min_cycles: 12, window_days: 7, cycles_last_24h: 30 },
  own: { devices: 2, access_points: 3, confirmed_access_points: 2 },
  others: [],
  findings: [{ id: 'radio_rogue_twin', tier: 'seen', count: 1, claim: 'An access point that you have not confirmed is broadcasting the name of one of your own networks.' }],
  could_not_see: [{ id: 'radio_hidden_ap_near', reason: 'A beacon’s signal strength is not kept per access point.' }],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.digest.mockResolvedValue(digest())
})

async function show() {
  const w = mount(RadioFindings)
  await flushPromises()
  return w
}

describe('RadioFindings', () => {
  it('shows each finding in the stack’s own words with how it is known', async () => {
    const w = await show()
    const f = w.get('[data-test="radio-finding"]')
    expect(f.text()).toContain('An access point that you have not confirmed')
    expect(f.get('[data-test="radio-finding-tier"]').text()).toBe('Seen')
    expect(f.get('[data-test="radio-finding-count"]').text()).toBe('1 case')
  })

  it('says a quiet list is not a clean bill when checks cannot run, and names them in Advanced', async () => {
    let w = await show()
    expect(w.get('[data-test="radio-unseen"]').text()).toBe('1 check cannot run yet, so a quiet list does not mean they are clear.')
    expect(w.find('[data-test="radio-unseen-reason"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    w = await show()
    expect(w.get('[data-test="radio-unseen-reason"]').text()).toContain('not kept per access point')
  })

  it('says nothing was found, and that little has been heard when listening is thin', async () => {
    service.digest.mockResolvedValue(digest({ findings: [] }))
    let w = await show()
    expect(w.get('[data-test="radio-findings-none"]').text()).toContain('Nothing to look at')
    service.digest.mockResolvedValue(digest({ findings: [], listening: { cycles: 2, min_cycles: 12, window_days: 7, cycles_last_24h: 2 } }))
    w = await show()
    expect(w.get('[data-test="radio-findings-thin"]').text()).toContain('Keep listening')
  })

  it('shows a failed read with its reason', async () => {
    service.digest.mockRejectedValue(new Error('down'))
    const w = await show()
    expect(w.get('[data-test="radio-findings-error"]').text()).toBe('down')
  })
})
