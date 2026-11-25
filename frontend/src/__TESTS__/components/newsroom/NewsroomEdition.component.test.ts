import { describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import NewsroomEdition from '../../../components/newsroom/NewsroomEdition.vue'
import type { BriefIssue, BriefSection, BriefStory } from '../../../types/newsroom'

vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))

const story = (over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: 1, headline: 'Chip ban widens', outlets: ['BBC', 'Reuters'], ...over })
const section = (over: Partial<BriefSection> = {}): BriefSection => ({ kind: 'news', topic_id: 7, name: 'Technology', stories: [story()], ...over })
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({ id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-22T06:40:00Z', sections: [section()], model_calls: 3, ...over })

describe('NewsroomEdition', () => {
  it('shows the top stories and a section per topic, with who reported each story', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ top: [story({ pool_id: 9, headline: 'Lead story' })] }) } })
    expect(w.text()).toContain('Top story')
    expect(w.text()).toContain('Lead story')
    expect(w.text()).toContain('Technology')
    expect(w.text()).toContain('Reported by BBC, Reuters')
    expect(w.text()).toContain('3 model calls')
  })

  it('says how many outlets it read, and what to do about a source that could not be read', async () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ outlets_read: 38, sections: [section({ problems: ['News: Greece: the server did not answer with HTTP 200'] })] }) } })
    expect(w.text()).toContain('read from 38 outlets')
    expect(w.get('[data-test="problems-fix"]').text()).toContain('Check the address')
    await w.get('[data-test="problems-open-setup"]').trigger('click')
    expect(w.emitted('open-setup')).toHaveLength(1)
  })

  it('says when the Telegram summary went, and why it did not, with the fix for a refusal', () => {
    const global = { stubs: { RouterLink: RouterLinkStub } }
    const sent = mount(NewsroomEdition, { props: { issue: issue({ sent_at: '2026-11-22T06:45:00Z' }) }, global })
    expect(sent.get('[data-test="send-sent"]').text()).toContain('Summary sent to Telegram')
    expect(sent.find('[data-test="send-problem"]').exists()).toBe(false)
    const refused = mount(NewsroomEdition, { props: { issue: issue({ send_state: 'refused' }) }, global })
    expect(refused.get('[data-test="send-problem"]').text()).toContain('switched on as a connector')
    expect(refused.getComponent(RouterLinkStub).props('to')).toMatchObject({ params: { section: 'communication' } })
    const failed = mount(NewsroomEdition, { props: { issue: issue({ send_state: 'failed' }) }, global })
    expect(failed.get('[data-test="send-problem"]').text()).toContain('tried again')
    expect(failed.find('[data-test="send-fix"]').exists()).toBe(false)
    const none = mount(NewsroomEdition, { props: { issue: issue({ send_state: 'nothing' }) }, global })
    expect(none.find('[data-test="send-problem"]').exists()).toBe(false)
    expect(none.find('[data-test="send-sent"]').exists()).toBe(false)
    expect(mount(NewsroomEdition, { props: { issue: issue() }, global }).find('[data-test="send-problem"]').exists()).toBe(false)
  })

  it('leaves the outlet count out of an edition that did not store one', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue() } })
    expect(w.text()).not.toContain('outlets')
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

  it('keeps a card to a few stories and lets the reader show the rest', async () => {
    const stories = [1, 2, 3, 4, 5].map((n) => story({ pool_id: n, headline: `Story ${n}` }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories })] }) } })
    expect(w.text()).toContain('Story 3')
    expect(w.text()).not.toContain('Story 4')
    await w.get('[data-test="show-more"]').trigger('click')
    expect(w.text()).toContain('Story 5')
    expect(w.get('[data-test="show-more"]').text()).toBe('Show fewer')
  })

  it('collapses and expands every card at once', async () => {
    const sections = [1, 2, 3].map((n) => section({ topic_id: n, name: `Topic ${n}`, stories: [story({ pool_id: n, headline: `Story of ${n}` })] }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections }) } })
    await w.get('[data-test="collapse-all"]').trigger('click')
    expect(w.text()).not.toContain('Story of 1')
    await w.get('[data-test="collapse-all"]').trigger('click')
    expect(w.text()).toContain('Story of 3')
  })

  it('shows the first top story as the lead, with the masthead', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ top: [story({ pool_id: 8, headline: 'First' }), story({ pool_id: 9, headline: 'Second' })] }) } })
    expect(w.get('[data-test="lead-story"]').text()).toContain('First')
    expect(w.get('[data-test="lead-story"]').text()).not.toContain('Second')
    expect(w.get('[data-test="masthead"]').text()).toBe('Morning')
  })

  it('shows what the watches found, marks an unread source, and shows nothing when the edition has no block', () => {
    const lines = [
      { watch: 'AI news', new: 2, headline: 'A model shipped', summary: 'It is faster.', link: 'https://example.com/a' },
      { watch: 'Prices', new: 0, problems: ['Channel X has no public preview'] },
    ]
    const w = mount(NewsroomEdition, { props: { issue: issue({ watches: lines }) } })
    const block = w.get('[data-test="watches-block"]')
    expect(block.text()).toContain('AI news')
    expect(block.text()).toContain('2 new')
    expect(block.text()).toContain('A model shipped')
    expect(block.text()).toContain('Source not read')
    expect(block.text()).toContain('Channel X has no public preview')
    expect(block.get('a').attributes('href')).toBe('https://example.com/a')
    expect(mount(NewsroomEdition, { props: { issue: issue() } }).find('[data-test="watches-block"]').exists()).toBe(false)
  })

  it('puts Home and Your watches in a side column after the news, so a phone shows them under it', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ home: [{ id: 'radio_off', words: 'Radio is off.' }], watches: [{ watch: 'AI news', new: 1 }] }) }, global: { stubs: { RouterLink: RouterLinkStub } } })
    const body = w.get('[data-test="edition-body"]')
    const [main, side] = [body.get('[data-test="edition-main"]'), body.get('[data-test="edition-side"]')]
    expect(main.element.compareDocumentPosition(side.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(main.text()).toContain('Technology')
    expect(main.find('[data-test="home-block"]').exists()).toBe(false)
    expect(side.find('[data-test="home-block"]').exists()).toBe(true)
    expect(side.find('[data-test="watches-block"]').exists()).toBe(true)
    expect(mount(NewsroomEdition, { props: { issue: issue() } }).find('[data-test="edition-side"]').exists()).toBe(false)
  })

  it('does not repeat a top story in its card', () => {
    const top = story({ pool_id: 1, headline: 'Lead' })
    const w = mount(NewsroomEdition, { props: { issue: issue({ top: [top], sections: [section({ stories: [top, story({ pool_id: 2, headline: 'Second' })] })] }) } })
    expect(w.get('section[aria-label="Technology"]').text()).toContain('Second')
    expect(w.get('section[aria-label="Technology"]').text()).not.toContain('Lead')
    expect(w.get('section[aria-label="Technology"]').text()).toContain('2 stories · 1 in Top stories')
    const only = mount(NewsroomEdition, { props: { issue: issue({ top: [top], sections: [section({ stories: [top] })] }) } })
    expect(only.get('section[aria-label="Technology"]').text()).toContain('in the top stories above')
  })

  it('says why a section is thin: its note and each source that could not be read', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [], as_of: '2026-11-22T00:00:00Z', note: 'last collected 9 hours ago', problems: ['Feed X: could not be read'] })] }) } })
    expect(w.get('[data-test="section-note"]').text()).toBe('last collected 9 hours ago')
    expect(w.get('[data-test="section-problems"]').text()).toContain('Feed X: could not be read')
    expect(w.text()).toContain('Nothing new here since the last edition.')
  })

  it('does not say "nothing new" about a topic that was never read', () => {
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [], as_of: undefined, note: 'not collected yet' })] }) } })
    expect(w.text()).toContain('Nothing has been read for this topic yet')
    expect(w.text()).not.toContain('Nothing new here')
  })

it('says once that a card is headlines only instead of tagging every story, and tags the odd one out in a mixed card', () => {
    const only = [1, 2, 3].map((n) => story({ pool_id: n, headline: `H${n}`, headline_only: true, update: n === 1 }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: only })] }) } })
    expect(w.findAll('[data-test="headlines-only"]')).toHaveLength(1)
    expect(w.text()).not.toContain('Headline only')
    expect(w.text()).toContain('Update')
    const mixed = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [story({ pool_id: 1, headline_only: true }), story({ pool_id: 2, summary: 'Text.' })] })] }) } })
    expect(mixed.find('[data-test="headlines-only"]').exists()).toBe(false)
    expect(mixed.text()).toContain('Headline only')
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

  it('counts what the section holds and says how many the top stories already show, and names the section of each top story', () => {
    const stories = [1, 2, 3, 4, 5].map((n) => story({ pool_id: n, headline: `Story ${n}` }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories })], top: stories.slice(0, 4) }) } })
    expect(w.get('section[aria-label="Technology"]').text()).toContain('5 stories · 4 in Top stories')
    expect(w.get('[data-test="lead-story"] [data-test="story-section"]').text()).toBe('Technology')
    expect(mount(NewsroomEdition, { props: { issue: issue() } }).get('section[aria-label="Technology"]').text()).not.toContain('in Top stories')
  })
})
