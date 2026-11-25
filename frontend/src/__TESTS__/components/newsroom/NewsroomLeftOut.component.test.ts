import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomLeftOut from '../../../components/newsroom/NewsroomLeftOut.vue'
import type { NewsroomDay } from '../../../types/newsroom'

const day: NewsroomDay = {
  day: '2026-11-25', label: 'Today', updates: 1, unread: 0,
  watches: [{ watch_id: 1, watch: 'AI news', updates: 1, unread: 0, last_at: '2026-11-25T10:00:00Z', current: { id: 9, watch_id: 1, at: '2026-11-25T10:00:00Z', source: 'telegram aipost', title: 'A meme about GPUs', why: 'a joke, not a release', left_out: true } }],
}

describe('NewsroomLeftOut', () => {
  it('lists what was left out with the reason and puts one back', async () => {
    const w = mount(NewsroomLeftOut, { props: { days: [day], watches: [], busy: false }, global: { stubs: { Icon: true } } })
    expect(w.text()).toContain('A meme about GPUs')
    expect(w.text()).toContain('Left out: a joke, not a release')
    await w.get('[data-test="keep-9"]').trigger('click')
    expect(w.emitted('keep')).toEqual([[9]])
  })

  it('says when nothing was left out', () => {
    const w = mount(NewsroomLeftOut, { props: { days: [], watches: [], busy: false } })
    expect(w.text()).toContain('Nothing was left out')
  })
})
