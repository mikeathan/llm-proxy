import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomWatchChips from '../../../components/newsroom/NewsroomWatchChips.vue'
import type { NewsroomWatch } from '../../../types/newsroom'

const watch = (id: number, name: string): NewsroomWatch => ({ id, name, interval: '1h0m0s', max_age: '72h0m0s', enabled: true, sources: [] })
const watches = [watch(1, 'AI news'), watch(2, 'Security')]

describe('NewsroomWatchChips', () => {
  it('shows All watches and one chip per watch, with the chosen one pressed', () => {
    const w = mount(NewsroomWatchChips, { props: { watches, modelValue: 2 } })
    const chips = w.findAll('button')
    expect(chips.map((c) => c.text())).toEqual(['All watches', 'AI news', 'Security'])
    expect(chips.map((c) => c.attributes('aria-pressed'))).toEqual(['false', 'false', 'true'])
    expect(w.get('[role="group"]').attributes('aria-label')).toBe('Show one watch')
  })

  it('asks for a watch by id, and for all of them with null', async () => {
    const w = mount(NewsroomWatchChips, { props: { watches, modelValue: null } })
    await w.findAll('button')[1]!.trigger('click')
    await w.findAll('button')[0]!.trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([[1], [null]])
  })

  it('keeps the whole name in the tooltip, as text', () => {
    const long = 'A very long watch name '.repeat(5) + '<b>x</b>'
    const w = mount(NewsroomWatchChips, { props: { watches: [watch(1, long)], modelValue: null } })
    expect(w.findAll('button')[1]!.attributes('title')).toBe(long)
    expect(w.find('b').exists()).toBe(false)
  })
})
