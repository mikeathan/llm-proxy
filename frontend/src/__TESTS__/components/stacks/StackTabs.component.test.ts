import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import StackTabs from '../../../components/stacks/StackTabs.vue'

const to = { name: 'newsroom' }
const mountTabs = (badge?: { count: number; words: string }) =>
  mount(StackTabs, { props: { label: 'Newsroom', active: 'feed', tabs: [{ id: 'feed', label: 'Feed', to }, { id: 'briefs', label: 'Briefs', to, badge }] }, global: { stubs: { RouterLink: RouterLinkStub } } })

describe('StackTabs', () => {
  it('shows a dot on a tab with something new, and says so to a screen reader', () => {
    const w = mountTabs({ count: 2, words: '2 new briefs' })
    expect(w.findAll('[data-test="tab-dot"]')).toHaveLength(1)
    expect(w.text()).toContain('2 new briefs')
  })

  it('shows no dot when nothing is new', () => {
    expect(mountTabs({ count: 0, words: '' }).find('[data-test="tab-dot"]').exists()).toBe(false)
    expect(mountTabs().find('[data-test="tab-dot"]').exists()).toBe(false)
  })
})
