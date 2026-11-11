import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import StandingCard from '../../../components/netaudit/StandingCard.vue'
import type { NetworkStanding } from '../../../types/netaudit'

const standing = (over: Partial<NetworkStanding> = {}): NetworkStanding => ({
  tracked: 3, oldest_days: 12, median_days: 4, over_month: 0, came_back: 0, week: { opened: 1, resolved: 2 }, previous: { opened: 2, resolved: 0 },
  trend: 'better', history_since: '2026-10-20T00:00:00Z', enough: true, ...over,
})
const show = (s: NetworkStanding) => mount(StandingCard, { props: { standing: s }, global: { plugins: [createAppRouter(createMemoryHistory())] } })

describe('StandingCard link', () => {
  it('leads to the open problems when some are tracked', () => {
    expect(show(standing()).get('[data-test="standing-link"]').attributes('href')).toBe('/network/findings')
  })

  it('offers no link when nothing is tracked or there is no history yet', () => {
    expect(show(standing({ tracked: 0 })).find('[data-test="standing-link"]').exists()).toBe(false)
    expect(show(standing({ history_since: undefined })).find('[data-test="standing-link"]').exists()).toBe(false)
  })
})
