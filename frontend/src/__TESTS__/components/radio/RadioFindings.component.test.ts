import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioDigest } from '../../../types/radio'

const service = vi.hoisted(() => ({ digest: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioFindings from '../../../components/radio/RadioFindings.vue'
import { routeLocationKey } from 'vue-router'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const digest = (over: Partial<RadioDigest> = {}): RadioDigest => ({
  listening: { cycles: 30, min_cycles: 12, window_days: 7, cycles_last_24h: 30, now: true, hours_left: 100 },
  classifier: { version: 2, provisional: true },
  own: { devices: 2, access_points: 3, confirmed_access_points: 2 },
  others: [],
  findings: [{ id: 'radio_rogue_twin', tier: 'seen', count: 1, claim: 'Access points that you have not confirmed are broadcasting the name of one of your own networks.', means: 'A neighbour’s network or a repeater can share a name, so these are candidates, not proof.' }],
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
    expect(f.text()).toContain('Access points that you have not confirmed')
    expect(f.get('[data-test="radio-finding-tier"]').text()).toBe('Seen')
    expect(f.get('[data-test="radio-finding-count"]').text()).toBe('1 case')
    expect(f.get('[data-test="radio-finding-means"]').text()).toContain('candidates, not proof')
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
    service.digest.mockResolvedValue(digest({ findings: [], listening: { cycles: 2, min_cycles: 12, window_days: 7, cycles_last_24h: 2, now: false } }))
    w = await show()
    expect(w.get('[data-test="radio-findings-thin"]').text()).toContain('Keep listening')
  })

  it('shows a failed read with its reason', async () => {
    service.digest.mockRejectedValue(new Error('down'))
    const w = await show()
    expect(w.get('[data-test="radio-findings-error"]').text()).toBe('down')
  })
})

describe('RadioFindings: a link from the brief', () => {
  const mountAt = (finding: string) => mount(RadioFindings, { global: { provide: { [routeLocationKey as symbol]: { query: { finding } } } } })

  it('scrolls to and marks the finding the link names', async () => {
    const w = mountAt('radio_rogue_twin')
    await flushPromises()
    expect(w.get('[data-test="radio-finding"]').attributes('data-flash')).toBeDefined()
    expect(w.find('[data-test="radio-focus-missing"]').exists()).toBe(false)
    w.unmount()
  })

  it('says so when the finding is not in the report any more', async () => {
    const w = mountAt('radio_pmf_off')
    await flushPromises()
    expect(w.get('[data-test="radio-focus-missing"]').text()).toContain('not in the latest report')
    w.unmount()
  })
})
