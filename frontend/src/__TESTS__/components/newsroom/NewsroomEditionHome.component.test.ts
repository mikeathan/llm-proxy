import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import NewsroomEdition from '../../../components/newsroom/NewsroomEdition.vue'
import type { BriefHomeLine, BriefIssue } from '../../../types/newsroom'

const issue = (home?: BriefHomeLine[]): BriefIssue => ({ id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-22T06:40:00Z', sections: [], model_calls: 0, home })
const mountIt = (home?: BriefHomeLine[]) => mount(NewsroomEdition, { props: { issue: issue(home) }, global: { stubs: { RouterLink: RouterLinkStub } } })

describe('the Home block of an edition', () => {
  it('shows each count with the core\'s sentence, says it holds counts only, and links to Network and Radio', () => {
    const w = mountIt([{ id: 'network_online', count: 5, words: 'Devices the network sees right now.' }, { id: 'radio_off', words: 'Radio is off, so nothing is said about the air.' }])
    const block = w.get('[data-test="home-block"]')
    expect(block.text()).toContain('5')
    expect(block.text()).toContain('Devices the network sees right now.')
    expect(block.text()).toContain('Radio is off')
    expect(block.text()).toContain('Counts only')
    expect(w.findAllComponents(RouterLinkStub).map((l) => l.props('to'))).toEqual([{ name: 'network' }, { name: 'radio' }])
  })

  it('shows its text as text and is absent when the edition did not ask', () => {
    const w = mountIt([{ id: 'x', words: '<img src=x onerror=alert(1)>' }])
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
    expect(mountIt(undefined).find('[data-test="home-block"]').exists()).toBe(false)
  })
})
