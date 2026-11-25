import { afterEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import NewsroomEdition from '../../../components/newsroom/NewsroomEdition.vue'
import { resetModelUse } from '../../../composables/newsroom/useModelUse'
import { resetBriefChips } from '../../../composables/newsroom/useBriefChips'
import { toNewsroom } from '../../../router/routes'
import type { BriefIssue, BriefSection, BriefStatus, BriefStory } from '../../../types/newsroom'

const { toastSuccess } = vi.hoisted(() => ({ toastSuccess: vi.fn() }))
vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ items: [] }) } }))
vi.mock('../../../composables/useToast', () => ({ useToast: () => ({ success: toastSuccess, error: vi.fn(), info: vi.fn() }) }))

const story = (over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: 1, headline: 'Chip ban widens', outlets: ['BBC', 'Reuters'], ...over })
const section = (over: Partial<BriefSection> = {}): BriefSection => ({ kind: 'news', topic_id: 7, name: 'Technology', stories: [story()], ...over })
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({ id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-22T06:40:00Z', sections: [section()], model_calls: 3, ...over })
const STATUS: BriefStatus = { tone: 'fresh', title: "Today's brief", detail: 'It was written automatically at 06:40.', next: 'The next one is written automatically today at 18:00.' }
const global = { stubs: { RouterLink: RouterLinkStub } }
const mountIt = (i: BriefIssue, status: BriefStatus = STATUS) => mount(NewsroomEdition, { props: { issue: i, status }, global })

afterEach(() => {
  resetModelUse()
  resetBriefChips()
})

describe('NewsroomEdition', () => {
  it('shows a nameplate and one dateline, the lead and a section per topic, with who reported each story and a tick per outlet', () => {
    const w = mountIt(issue({ outlets_read: 38, top: [story({ pool_id: 9, headline: 'Lead story', outlets: ['A', 'B', 'C'] })] }))
    expect(w.get('[data-test="masthead"]').text()).toBe('Morning')
    expect(w.get('[data-test="dateline"]').text()).toContain('3 model calls')
    expect(w.get('[data-test="dateline"]').text()).toContain('read from 38 outlets')
    expect(w.get('[data-test="lead-story"]').text()).toContain('Lead story')
    expect(w.get('[data-test="lead-story"] [data-test="coverage"]').attributes('aria-label')).toBe('Reported by 3 outlets')
    expect(w.text()).toContain('Reported by BBC, Reuters')
  })

  it('leaves the outlet count out of an edition that did not store one', () => {
    expect(mountIt(issue()).get('[data-test="dateline"]').text()).not.toContain('outlets')
  })

  it('says when the Telegram summary went, and why it did not, with the fix for a refusal', () => {
    const sent = mountIt(issue({ sent_at: '2026-11-22T06:45:00Z' }))
    expect(sent.get('[data-test="send-sent"]').text()).toContain('Summary sent to Telegram')
    const refused = mountIt(issue({ send_state: 'refused' }))
    expect(refused.get('[data-test="send-problem"]').text()).toContain('switched on as a connector')
    expect(refused.find('[data-test="send-fix"]').exists()).toBe(true)
    expect(mountIt(issue({ send_state: 'failed' })).find('[data-test="send-fix"]').exists()).toBe(false)
    const none = mountIt(issue({ send_state: 'nothing' }))
    expect(none.find('[data-test="send-problem"]').exists()).toBe(false)
    expect(none.find('[data-test="send-sent"]').exists()).toBe(false)
  })

  it('says loudly that a brief is not today\'s, and quietly when it is', () => {
    const old = mountIt(issue(), { tone: 'old', title: 'This brief is from Monday', detail: "It is not today's.", next: 'Next at 18:00.' })
    expect(old.get('[data-test="brief-status"]').text()).toContain('This brief is from Monday')
    expect(mountIt(issue()).get('[data-test="brief-status"]').text()).not.toContain("Today's brief")
  })

  it('opens every section, indexes them and makes each section and story an anchor a card can link to', () => {
    const sections = [1, 2, 3].map((n) => section({ topic_id: n, name: `Topic ${n}`, stories: [story({ pool_id: n, headline: `Story of ${n}` })] }))
    const w = mountIt(issue({ sections }))
    for (const n of [1, 2, 3]) {
      expect(w.text()).toContain(`Story of ${n}`)
      expect(w.find(`#brief-section-${n}`).exists()).toBe(true)
      expect(w.find(`#brief-story-${n}`).exists()).toBe(true)
    }
    expect(w.find('[data-test="index-rail"]').exists()).toBe(false)
    expect(w.get('[data-test="index-row"]').findAll('a').map((a) => a.attributes('href'))).toEqual(['#brief-section-1', '#brief-section-2', '#brief-section-3'])
    expect(w.find('[data-test="collapse-all"]').exists()).toBe(false)
  })

  it('shares the row of other top stories by how many there are, so one story does not sit in a third of the width', () => {
    const top = (n: number) => Array.from({ length: n }, (_, i) => story({ pool_id: 100 + i, headline: `Top ${i}` }))
    const cls = (n: number) => mountIt(issue({ top: top(n) })).get('[data-test="top-stories"]').classes()
    expect(cls(2).some((c) => c.includes('grid-cols'))).toBe(false)
    expect(cls(3)).toContain('md:grid-cols-2')
    expect(cls(4)).toContain('md:grid-cols-3')
  })

  it('keeps a section to a few stories and lets the reader show the rest', async () => {
    const stories = [1, 2, 3, 4, 5].map((n) => story({ pool_id: n, headline: `Story ${n}` }))
    const w = mountIt(issue({ sections: [section({ stories })] }))
    expect(w.text()).not.toContain('Story 4')
    await w.get('[data-test="show-more"]').trigger('click')
    expect(w.text()).toContain('Story 5')
    expect(w.get('[data-test="show-more"]').text()).toBe('Show fewer')
  })

  it('says first what needs a look, keeps the rest of Home and the alerts beside the news, and always shows model use', () => {
    const w = mountIt(issue({
      home: [{ id: 'radio_open_or_weak_ap', count: 1, words: 'An open access point of yours.', level: 'critical' }, { id: 'radio_off', words: 'Radio is off.', level: 'info' }],
      watches: [{ watch: 'AI news', new: 1, headline: 'A model shipped' }],
    }))
    const side = w.get('[data-test="edition-side"]')
    expect(side.get('[data-test="brief-attention"]').text()).toContain('An open access point of yours.')
    expect(side.get('[data-test="home-block"]').text()).not.toContain('An open access point')
    expect(side.get('[data-test="watches-block"]').text()).toContain('Alerts')
    expect(side.find('[data-test="model-use-block"]').exists()).toBe(true)
    expect(mountIt(issue()).find('[data-test="model-use-block"]').exists()).toBe(true)
  })

  it('puts the side column after the news in the page, so a phone shows it after the top stories', () => {
    const w = mountIt(issue({ watches: [{ watch: 'AI news', new: 1 }] }))
    const [main, side] = [w.get('[data-test="edition-main"]'), w.get('[data-test="edition-side"]')]
    expect(main.element.compareDocumentPosition(side.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('does not repeat a top story in its section', () => {
    const top = story({ pool_id: 1, headline: 'Lead' })
    const w = mountIt(issue({ top: [top], sections: [section({ stories: [top, story({ pool_id: 2, headline: 'Second' })] })] }))
    const sec = w.get('section[aria-label="Technology"]')
    expect(sec.text()).toContain('Second')
    expect(sec.text()).not.toContain('Lead')
    expect(sec.text()).toContain('2 stories · 1 in Top stories')
    expect(mountIt(issue({ top: [top], sections: [section({ stories: [top] })] })).get('section[aria-label="Technology"]').text()).toContain('in the top stories above')
  })

  it('says why a section is thin and links to where its sources are fixed', () => {
    const w = mountIt(issue({ sections: [section({ stories: [], as_of: '2026-11-22T00:00:00Z', note: 'last collected 9 hours ago', problems: ['Feed X: could not be read'] })] }))
    expect(w.get('[data-test="section-note"]').text()).toBe('last collected 9 hours ago')
    expect(w.get('[data-test="section-problems"]').text()).toContain('Feed X: could not be read')
    const fix = w.findAllComponents(RouterLinkStub).find((l) => l.attributes('data-test') === 'problems-open-setup')
    expect(fix?.props('to')).toEqual(toNewsroom('following'))
    expect(w.text()).toContain('Nothing new here since the last edition.')
  })

  it('does not say "nothing new" about a topic that was never read', () => {
    const w = mountIt(issue({ sections: [section({ stories: [], as_of: undefined })] }))
    expect(w.text()).toContain('Nothing has been read for this topic yet')
  })

  it('says once that a section is headlines only instead of tagging every story', () => {
    const only = [1, 2, 3].map((n) => story({ pool_id: n, headline: `H${n}`, headline_only: true, update: n === 1 }))
    const w = mountIt(issue({ sections: [section({ stories: only })] }))
    expect(w.findAll('[data-test="headlines-only"]')).toHaveLength(1)
    expect(w.text()).not.toContain('Headline only')
    expect(w.text()).toContain('Update')
  })

  it('shows text from the internet as text and opens only a real web link', () => {
    const evil = story({ pool_id: 1, headline: '<img src=x onerror=alert(1)>', link: 'javascript:alert(1)', summary: '<b>bold</b>' })
    const ok = story({ pool_id: 2, headline: 'Fine', link: 'https://example.com/a' })
    const w = mountIt(issue({ sections: [section({ stories: [evil, ok] })] }))
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('b').exists()).toBe(false)
    const story2 = w.get('#brief-story-2 a')
    expect(story2.attributes('href')).toBe('https://example.com/a')
    expect(story2.attributes('rel')).toContain('noopener')
    expect(w.get('#brief-story-1').find('a').exists()).toBe(false)
  })

  it('ends with an end mark that says when the next one comes, and copies the brief as text', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const w = mountIt(issue({ top: [story({ pool_id: 9, headline: 'Lead story', link: 'https://example.com/l' })] }))
    expect(w.get('[data-test="end-mark"]').text()).toContain('You are up to date. The next one is written automatically today at 18:00.')
    await w.get('[data-test="copy-text"]').trigger('click')
    await flushPromises()
    expect(writeText.mock.calls[0]?.[0]).toContain('• Lead story\n  https://example.com/l')
    expect(toastSuccess).toHaveBeenCalled()
  })

  it('moves between stories with j and k', async () => {
    const stories = [1, 2].map((n) => story({ pool_id: n, headline: `S${n}`, link: `https://example.com/${n}` }))
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories })] }), status: STATUS }, global, attachTo: document.body })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    expect(document.activeElement?.getAttribute('href')).toBe('https://example.com/1')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    expect(document.activeElement?.getAttribute('href')).toBe('https://example.com/2')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k' }))
    expect(document.activeElement?.getAttribute('href')).toBe('https://example.com/1')
    w.unmount()
  })

  it('tunes a story only where its topic is known: more or less like it with the words picked, or follows it', async () => {
    const plain = mountIt(issue())
    expect(plain.find('[data-test="story-actions"]').exists()).toBe(false)
    const s = story({ pool_id: 3, headline: 'Central bank holds rates as inflation cools' })
    const w = mount(NewsroomEdition, { props: { issue: issue({ sections: [section({ stories: [s] })] }), status: STATUS, topics: { 7: 'Economy' } }, global })
    await w.get('[data-test="story-more"]').trigger('click')
    expect(w.get('[data-test="story-tune"]').text()).toContain('one more search of Economy')
    const words = w.findAll('[data-test="tune-word"]')
    expect(words.map((b) => b.text())).toEqual(['central', 'bank', 'holds', 'rates', 'inflation', 'cools'])
    await words[3]!.trigger('click')
    await w.get('[data-test="tune-apply"]').trigger('click')
    expect(w.emitted('tune')?.[0]).toEqual([7, true, ['central', 'bank', 'rates']])
    await w.get('[data-test="story-less"]').trigger('click')
    expect(w.get('[data-test="story-tune"]').text()).toContain('skipped in Economy')
    await w.get('[data-test="story-follow"]').trigger('click')
    expect(w.emitted('follow')?.[0]).toEqual([['central', 'bank', 'holds']])
  })
})
