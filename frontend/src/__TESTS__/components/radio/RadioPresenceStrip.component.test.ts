import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioPresenceStrip from '../../../components/radio/RadioPresenceStrip.vue'
import type { RadioPresenceDevice } from '../../../types/radio'

const device = (hours: RadioPresenceDevice['hours']): RadioPresenceDevice => ({ id: 'ab', label: 'my phone', state: 'here', joined_own: true, hours })
const at = (i: number) => new Date(Date.UTC(2026, 10, 14, i)).toISOString()

describe('RadioPresenceStrip', () => {
  it('draws a distance bar only for the hours that were heard with a signal, and leaves a gap for the rest', () => {
    const w = mount(RadioPresenceStrip, { props: { device: device([
      { hour: at(0), state: 'here', dbm: -50, near_m: 1.4, far_m: 3.2 },
      { hour: at(1), state: 'away' },
      { hour: at(2), state: 'not_listening' },
      { hour: at(3), state: 'here', dbm: -70, near_m: 6, far_m: 30 },
    ]) } })
    expect(w.findAll('[data-test="radio-presence-hour"]')).toHaveLength(4)
    const bars = w.findAll('[data-test="radio-presence-bar"]')
    expect(bars).toHaveLength(2)
    expect(bars[0]!.get('title').text()).toContain('1.4–3.2 m')
    expect(Number(bars[1]!.attributes('y'))).toBeGreaterThan(Number(bars[0]!.attributes('y')))
  })

  it('shows no distance strip when nothing was measured', () => {
    const w = mount(RadioPresenceStrip, { props: { device: device([{ hour: at(0), state: 'here' }, { hour: at(1), state: 'away' }]) } })
    expect(w.find('[data-test="radio-presence-distance"]').exists()).toBe(false)
  })
})
