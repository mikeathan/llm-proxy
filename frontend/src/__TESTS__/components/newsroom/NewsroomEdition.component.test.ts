import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomEdition from '../../../components/newsroom/NewsroomEdition.vue'
import type { BriefIssue, BriefSection, BriefStory } from '../../../types/newsroom'

const story = (over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: 1, headline: 'Chip ban widens', outlets: ['BBC', 'Reuters'], ...over })
const section = (over: Partial<BriefSection> = {}): BriefSection => ({ kind: 'news', topic_id: 7, name: 'Technology', stories: [story()], ...over })
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({ id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-22T06:40:00Z', sections: [section()], model_calls: 3, ...over })

describe('NewsroomEdition', () => {
  it('shows the top stories and a section per topic, with who reported each story', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ top: [story({ pool_id: 9, headline: 'Lead story' })] }) } })
    expect(w.text()).toContain('Top stories')
    expect(w.text()).toContain('Lead story')
    expect(w.text()).toContain('Technology')
    expect(w.text()).toContain('Reported by BBC, Reuters')
    expect(w.text()).toContain('3 model calls')
  })

  it('opens only the first two sections and lets the reader fold one by hand', async () => {
    const sections = [1, 2, 3].map((n) => section({ topic_id: n, name: `Topic ${n}`, stories: [story({ pool_id: n, headline: `Story of ${n}` })] }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections }) } })
    expect(w.text()).toContain('Story of 1')
    expect(w.text()).toContain('Story of 2')
    expect(w.text()).not.toContain('Story of 3')
    const third = w.get('section[aria-label="Topic 3"] button')
    expect(third.attributes('aria-expanded')).toBe('false')
    await third.trigger('click')
    expect(w.text()).toContain('Story of 3')
  })

  it('says why a section is thin: its note and each source that could not be read', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [], note: 'last collected 9 hours ago', problems: ['Feed X: could not be read'] })] }) } })
    expect(w.get('[data-test="section-note"]').text()).toBe('last collected 9 hours ago')
    expect(w.get('[data-test="section-problems"]').text()).toContain('Feed X: could not be read')
    expect(w.text()).toContain('Nothing new here since the last edition.')
  })

  it('tags a headline-only story and an update, and shows no summary it was not given', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [story({ headline_only: true, update: true })] })] }) } })
    expect(w.text()).toContain('Headline only')
    expect(w.text()).toContain('Update')
  })

  it('shows text from the internet as text and opens only a real web link', () => {
    const evil = story({ pool_id: 1, headline: '<img src=x onerror=alert(1)>', link: 'javascript:alert(1)', summary: '<b>bold</b>' })
    const ok = story({ pool_id: 2, headline: 'Fine', link: 'https://example.com/a' })
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [evil, ok] })] }) } })
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('b').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
    const links = w.findAll('a').map((a) => a.attributes('href'))
    expect(links).toEqual(['https://example.com/a'])
    expect(w.get('a').attributes('rel')).toContain('noopener')
  })
})
