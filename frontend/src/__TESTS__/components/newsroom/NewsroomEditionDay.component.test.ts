import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import NewsroomEditionDay from '../../../components/newsroom/NewsroomEditionDay.vue'

const mountDay = (day: object) => mount(NewsroomEditionDay, { props: { day: day as never }, global: { stubs: { RouterLink: RouterLinkStub } } })

describe('NewsroomEditionDay', () => {
  it("lists the owner's reminders as a time and their own words, as text", () => {
    const w = mountDay({ items: [{ clock: '08:30', title: 'Pay the <b>bill</b>', repeats: true }, { clock: '14:00', title: 'Call the dentist' }] })
    const rows = w.findAll('li')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('08:30')
    expect(rows[0]!.text()).toContain('Pay the <b>bill</b>')
    expect(rows[0]!.find('b').exists()).toBe(false)
    expect(rows[0]!.text()).toContain('repeats')
    expect(rows[1]!.text()).not.toContain('repeats')
    expect(w.text()).toContain('Today')
  })

  it('says Tomorrow for an evening brief', () => {
    expect(mountDay({ tomorrow: true, items: [{ clock: '07:15', title: 'Gym' }] }).text()).toContain('Tomorrow')
  })

  it('tells a quiet day from Reminders being off', () => {
    const quiet = mountDay({ items: [] })
    expect(quiet.find('[data-test="day-none"]').exists()).toBe(true)
    expect(quiet.find('[data-test="day-unavailable"]').exists()).toBe(false)
    const off = mountDay({ unavailable: true, items: [] })
    expect(off.get('[data-test="day-unavailable"]').text()).toContain('Reminders is off')
    expect(off.find('[data-test="day-none"]').exists()).toBe(false)
  })

  it('links to Reminders', () => {
    expect(mountDay({ items: [] }).getComponent(RouterLinkStub).props('to')).toMatchObject({ name: 'reminders' })
  })
})
