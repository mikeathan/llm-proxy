import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioPresenceStrip from '../../../components/radio/RadioPresenceStrip.vue'
import type { RadioPresenceDevice } from '../../../types/radio'

const device = (hours: RadioPresenceDevice['hours']): RadioPresenceDevice => ({ id: 'ab', label: 'my phone', state: 'here', joined_own: true, hours })
const at = (i: number) => new Date(Date.UTC(2026, 10, 14, i)).toISOString()

describe('RadioPresenceStrip', () => {
  it('draws a square an hour with its state, an axis from the oldest hour to now, and no distance chart', () => {
    const w = mount(RadioPresenceStrip, { props: { device: device([
      { hour: at(0), state: 'here', dbm: -50, near_m: 1.4, far_m: 3.2 },
      { hour: at(1), state: 'away' },
      { hour: at(2), state: 'not_listening' },
      { hour: at(3), state: 'here' },
    ]) } })
    const cells = w.findAll('[data-test="radio-presence-hour"]')
    expect(cells.map((c) => c.attributes('data-state'))).toEqual(['here', 'away', 'not_listening', 'here'])
    expect(w.get('[role="img"]').attributes('aria-label')).toBe('my phone: heard in 2 of the last 4 hours')
    expect(w.get('[data-test="radio-presence-axis"]').text()).toBe('4 hours agonow')
    expect(w.find('svg').exists()).toBe(false)
  })
})
