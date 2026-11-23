import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
const { lookupMock } = vi.hoisted(() => ({ lookupMock: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: { lookup: lookupMock } }))

import NewsroomBriefSetup from '../../../components/newsroom/NewsroomBriefSetup.vue'
import type { BriefEdition, BriefSource, BriefTopic } from '../../../types/newsroom'

const topic: BriefTopic = { id: 1, name: 'Technology', queries: ['AI chips'] }
const edition: BriefEdition = { id: 2, name: 'Morning', ready_by: '07:00', days: [1, 2, 3, 4, 5], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [{ kind: 'news', topic_id: 1 }], enabled: true }

// A topic with nothing to read yet, to open in the editor.
const fresh: BriefTopic = { id: 9, name: 'Fresh', category: 'News' }

const catalog: BriefSource[] = [
  { label: 'BBC News', category: 'News', kind: 'feed', target: 'https://bbc.example/rss.xml', builtin: true },
  { label: 'Ars Technica', category: 'Technology', kind: 'feed', target: 'https://ars.example/feed', builtin: true },
  { label: 'Reuters', category: 'Search a publisher', kind: 'newssearch', target: 'site:reuters.com', builtin: true },
  { id: 9, label: 'My blog', category: 'My sources', kind: 'feed', target: 'https://blog.example/feed.xml', builtin: false },
]

// The edition form is not the first form on the page: every section has its own add-a-topic form above it.
const editionForm = (w: ReturnType<typeof mount>) => w.findAll('form').find((f) => f.find('[data-test="edition-name"]').exists())!

const props = (over: Record<string, unknown> = {}) => ({
  topics: [topic], editions: [edition], settings: { model_calls_per_day: 0, default: 200, max: 2000 }, sources: catalog, saving: false, error: '',
  saveTopic: vi.fn().mockResolvedValue(true), saveEdition: vi.fn().mockResolvedValue(true), saveLimit: vi.fn().mockResolvedValue(true),
  addSource: vi.fn().mockResolvedValue(true), removeSource: vi.fn().mockResolvedValue(true), ...over,
})

describe('NewsroomBriefSetup', () => {
  it('lists topics and editions, and says what a topic could not read', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [{ ...topic, last_problems: ['Feed X: could not be read'] }] }) })
    expect(w.text()).toContain('Technology')
    expect(w.text()).toContain('Ready at 07:00 · Mon–Fri')
    expect(w.text()).toContain('AI chips')
    expect(w.text()).toContain('Gathers every 2 hours, from 12 hours before.')
    expect(w.text()).toContain('Feed X: could not be read')
  })

  it('lets a brief be only your home: news cannot be switched on without a topic, and no topic is needed to save', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add an edition')!.trigger('click')
    const news = w.get('[data-test="block-news"] input[role="switch"]')
    expect(news.attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="block-news"]').text()).toContain('Add a topic first to include news.')
    await w.get('[data-test="edition-name"]').setValue('Home')
    expect(editionForm(w).get('[type="submit"]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="edition-needs"]').text()).toContain('Pick at least one thing')
    await w.findAll('[data-test="edition-blocks"] input[role="switch"]')[2]!.setValue(true)
    await editionForm(w).trigger('submit')
    await flushPromises()
    expect(p.saveEdition).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Home', home: true, watches: false, sections: [] }))
  })

  const news: BriefTopic[] = [{ id: 5, name: 'UK', category: 'News', queries: ['UK news'] }, { id: 6, name: 'Greece', category: ' news ', queries: ['Greece'], sources: [{ kind: 'feed', label: 'BBC UK', target: 'https://bbc.example/uk' }] }, { id: 7, name: 'AI', category: 'Technology', queries: ['AI chips'] }]

  it('lays topics out under their section, spacing and case ignored, with the ones that have none last', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [...news, topic] }) })
    const groups = w.findAll('[data-test="topic-section-group"]')
    expect(groups.map((g) => g.get('h3').text())).toEqual(['News', 'Technology', 'No section'])
    expect(groups[0]!.findAll('[data-test="topic-card"]').map((c) => c.get('span').text())).toEqual(['UK', 'Greece'])
    expect(groups[0]!.text()).toContain('2 topics')
    expect(groups[0]!.text()).toContain('SearchesGreece')
    expect(groups[0]!.text()).toContain('ReadsBBC UK')
    expect(groups[2]!.find('input').exists()).toBe(false)
  })

  it('adds a topic to a section from the one word typed there, and clears the field when it saved', async () => {
    const p = props({ topics: news })
    const w = mount(NewsroomBriefSetup, { props: p })
    const input = w.get('[data-test="topic-section-group"] [data-test="add-topic-input"]')
    await input.setValue(' World ')
    await w.get('[data-test="topic-section-group"] form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, { name: 'World', category: 'News', queries: ['World'], region: '', sources: [], ignore: [] })
    expect((input.element as HTMLInputElement).value).toBe('')
  })

  it('keeps what was typed when the topic is refused', async () => {
    const p = props({ topics: news, saveTopic: vi.fn().mockResolvedValue(false) })
    const w = mount(NewsroomBriefSetup, { props: p })
    const input = w.get('[data-test="add-topic-input"]')
    await input.setValue('World')
    await w.get('[data-test="topic-section-group"] form').trigger('submit')
    await flushPromises()
    expect((input.element as HTMLInputElement).value).toBe('World')
  })

  it('makes a new section from its name and puts the first topic typed there in it', async () => {
    const p = props({ topics: news })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'New section')!.trigger('click')
    await w.get('[data-test="new-section-name"]').setValue('Sport')
    await w.get('[data-test="new-section-form"]').trigger('submit')
    const groups = w.findAll('[data-test="topic-section-group"]')
    expect(groups.map((g) => g.get('h3').text())).toEqual(['News', 'Technology', 'Sport'])
    expect(groups[2]!.text()).toContain('0 topics')
    await groups[2]!.get('[data-test="add-topic-input"]').setValue('Football')
    await groups[2]!.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Football', category: 'Sport', queries: ['Football'] }))
  })

  it('says when a topic is one short word with nothing else', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [{ id: 8, name: 'War', category: 'News', queries: ['war'] }, news[0]!] }) })
    const cards = w.findAll('[data-test="topic-card"]')
    expect(cards[0]!.find('[data-test="thin-topic"]').exists()).toBe(true)
    expect(cards[1]!.find('[data-test="thin-topic"]').exists()).toBe(false)
  })

  it('opens a topic with everything visible and saves it under the same id', async () => {
    const p = props({ topics: news })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit UK"]').trigger('click')
    expect((w.get('[data-test="topic-name"]').element as HTMLInputElement).value).toBe('UK')
    expect((w.get('[data-test="topic-section"]').element as HTMLInputElement).value).toBe('News')
    expect(w.find('[data-test="topic-fine-tune"]').exists()).toBe(false)
    await w.get('[data-test="topic-name"]').setValue('United Kingdom')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(5, expect.objectContaining({ name: 'United Kingdom', category: 'News', queries: ['UK news'] }))
  })

  it('does not save a topic with nothing to read', async () => {
    const p = props({ topics: [fresh] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    await w.get('form').trigger('submit')
    expect(p.saveTopic).not.toHaveBeenCalled()
    expect(w.get('[data-test="topic-needs"]').text()).toContain('at least one search or one feed')
  })

  it('shows the starter gallery on an empty page and saves the topics picked, one after another', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    const gallery = w.get('[data-test="starter-gallery"]')
    const add = gallery.findAll('button').find((b) => b.text().startsWith('Add 0'))!
    expect(add.attributes('disabled')).toBeDefined()
    await gallery.findAll('[aria-label="Starter topics"] button').find((b) => b.text().includes('AI and chips'))!.trigger('click')
    await gallery.findAll('[aria-label="Starter topics"] button').find((b) => b.text().includes('Cybersecurity'))!.trigger('click')
    await gallery.findAll('button').find((b) => b.text() === 'Add 2 topics')!.trigger('click')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledTimes(2)
    expect(p.saveTopic).toHaveBeenNthCalledWith(1, null, expect.objectContaining({ name: 'AI and chips', category: 'Technology' }))
  })

  it('stops saving the picked starters at the first refusal', async () => {
    const p = props({ topics: [], editions: [], saveTopic: vi.fn().mockResolvedValue(false) })
    const w = mount(NewsroomBriefSetup, { props: p })
    const gallery = w.get('[data-test="starter-gallery"]')
    for (const name of ['AI and chips', 'Cybersecurity']) await gallery.findAll('[aria-label="Starter topics"] button').find((b) => b.text().includes(name))!.trigger('click')
    await gallery.findAll('button').find((b) => b.text() === 'Add 2 topics')!.trigger('click')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledTimes(1)
  })

  it('opens the new-section field from "Make my own"', async () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    await w.get('[data-test="starter-gallery"]').findAll('button').find((b) => b.text() === 'Make my own')!.trigger('click')
    expect(w.find('[data-test="starter-gallery"]').exists()).toBe(false)
    expect(w.find('[data-test="new-section-name"]').exists()).toBe(true)
  })

  it('shows the daily limit, saves a number in bounds and refuses one outside them', async () => {
    const p = props({ settings: { model_calls_per_day: 300, default: 200, max: 2000 } })
    const w = mount(NewsroomBriefSetup, { props: p })
    const box = w.get('input[inputmode="numeric"]')
    expect((box.element as HTMLInputElement).value).toBe('300')
    const lastOf = <T>(list: T[]): T => list[list.length - 1]!
    const limitForm = () => lastOf(w.findAll('form'))
    const save = lastOf(w.findAll('button').filter((b) => b.text() === 'Save'))
    expect(save.attributes('disabled')).toBeDefined()
    await box.setValue('5000')
    expect(w.text()).toContain('Use a whole number from 1 to 2000')
    expect(save.attributes('disabled')).toBeDefined()
    await box.setValue('500')
    await limitForm().trigger('submit')
    expect(p.saveLimit).toHaveBeenCalledWith(500)
    await box.setValue('')
    await limitForm().trigger('submit')
    expect(p.saveLimit).toHaveBeenLastCalledWith(0)
  })

  it('saves an edition with the sections kept, as their topics, in whole hours', async () => {
    const p = props({ editions: [], topics: news })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add an edition')!.trigger('click')
    await w.get('[data-test="edition-name"]').setValue('Evening')
    await w.get('[data-test="block-news"] input[role="switch"]').setValue(true)
    expect(w.findAll('[data-test="edition-sections"] > *').map((s) => s.text())).toEqual(['News2×', 'Technology1×'])
    await w.get('[data-test="edition-sections"] button[aria-label="Remove Technology from this brief"]').trigger('click')
    expect(w.get('[data-test="edition-sections"]').text()).toContain('+ Technology')
    await w.get('[data-test="day-pills"] button').trigger('click')
    await editionForm(w).trigger('submit')
    await flushPromises()
    expect(p.saveEdition).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Evening', collect_every: '2h', collect_for: '12h', watches: false, home: false, sections: [{ kind: 'news', topic_id: 5 }, { kind: 'news', topic_id: 6 }] }))
    expect((p.saveEdition as ReturnType<typeof vi.fn>).mock.calls[0]![1].days.sort()).toEqual([0, 2, 3, 4, 5, 6])
  })

  it('shows an edition by what it reads, its sections then watches then home, and refuses more topics than a brief can hold', async () => {
    const many: BriefTopic[] = Array.from({ length: 13 }, (_, i) => ({ id: 20 + i, name: 'T' + i, category: 'Big', queries: ['x' + i] }))
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [...news, ...many], editions: [{ ...edition, sections: [{ kind: 'news', topic_id: 5 }], watches: true, home: true }] }) })
    expect(w.get('[data-test="edition-card"]').text()).toContain('News')
    expect(w.get('[data-test="edition-card"]').text()).toContain('Watches')
    expect(w.get('[data-test="edition-card"]').text()).toContain('Home')
    await w.get('button[aria-label="Edit Morning"]').trigger('click')
    await w.get('[data-test="edition-sections"] button[aria-label="Add Big to this brief"]').trigger('click')
    expect(w.get('[data-test="edition-too-many"]').text()).toContain('at most 12 topics')
    expect(editionForm(w).get('[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('follows a well-known site picked while typing, by its name, and sends the feed with that name', async () => {
    const p = props({ topics: [fresh], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Tech')
    const feeds = w.get('input[aria-label="Follow a website (optional)"]')
    await feeds.setValue('ars')
    expect(w.findAll('[data-test="chip-suggestions"] [role="option"]').map((o) => o.text())).toEqual(['Ars TechnicaTechnology'])
    await w.get('[data-test="chip-suggestions"] [role="option"]').trigger('mousedown')
    expect(w.text()).toContain('Ars Technica')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(9, expect.objectContaining({ sources: [{ kind: 'feed', label: 'Ars Technica', target: 'https://ars.example/feed' }] }))
  })

  it('does not turn a half-typed word into a feed', async () => {
    const p = props({ topics: [fresh], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Tech')
    const feeds = w.get('input[aria-label="Follow a website (optional)"]')
    await feeds.setValue('zzz')
    await feeds.trigger('keydown', { key: 'Enter' })
    expect(w.text()).toContain('Pick a site from the list')
    expect(w.find('[data-test="topic-needs"]').exists()).toBe(true)
  })

  it('browses the well-known sources by group, the group that matches the topic first, and adds a feed or a publisher search with one press', async () => {
    const p = props({ topics: [fresh], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Technology')
    const groups = w.findAll('[data-test="source-browser"] section').map((s) => s.attributes('aria-label'))
    expect(groups).toEqual(['Technology', 'News', 'Search a publisher', 'My sources'])
    const press = async (name: string) => w.findAll('[data-test="source-browser"] button').find((b) => b.text().startsWith(name))!.trigger('click')
    await press('Ars Technica')
    await press('Reuters')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(9, expect.objectContaining({ queries: ['site:reuters.com'], sources: [{ kind: 'feed', label: 'Ars Technica', target: 'https://ars.example/feed' }] }))
  })

  it('takes a picked source back with a second press', async () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [fresh], editions: [] }) })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    const bbc = () => w.findAll('[data-test="source-browser"] button').find((b) => b.text().startsWith('BBC News'))!
    await bbc().trigger('click')
    expect(bbc().attributes('aria-pressed')).toBe('true')
    await bbc().trigger('click')
    expect(bbc().attributes('aria-pressed')).toBe('false')
  })

  it('lists the owner\'s own sources, adds one by name and feed link and removes one', async () => {
    const p = props()
    const w = mount(NewsroomBriefSetup, { props: p })
    const mine = () => w.findAll('[data-test="my-source"]')
    expect(mine().map((m) => m.text())).toEqual(['My bloghttps://blog.example/feed.xml'])
    await w.get('button[aria-label="Remove My blog"]').trigger('click')
    expect(p.removeSource).toHaveBeenCalledWith(9)
    const form = w.findAll('form').find((f) => f.text().includes('Add to my sources'))!
    const add = form.findAll('button').find((b) => b.text() === 'Add to my sources')!
    expect(add.attributes('disabled')).toBeDefined()
    await form.findAll('input')[0]!.setValue('Daily Blog')
    await form.findAll('input')[1]!.setValue('blog.example/rss')
    expect(form.text()).toContain('starts with https://')
    expect(add.attributes('disabled')).toBeDefined()
    await form.findAll('input')[1]!.setValue('https://blog.example/rss')
    await form.trigger('submit')
    await flushPromises()
    expect(p.addSource).toHaveBeenCalledWith('Daily Blog', 'https://blog.example/rss')
    expect((form.findAll('input')[0]!.element as HTMLInputElement).value).toBe('')
  })

  it('keeps what was typed and shows the server\'s reason when a source is refused', async () => {
    const p = props({ addSource: vi.fn().mockResolvedValue(false), error: 'BBC News is already in the built-in list' })
    const w = mount(NewsroomBriefSetup, { props: p })
    const form = w.findAll('form').find((f) => f.text().includes('Add to my sources'))!
    await form.findAll('input')[0]!.setValue('Again')
    await form.findAll('input')[1]!.setValue('https://bbc.example/rss.xml')
    await form.trigger('submit')
    await flushPromises()
    expect(form.text()).toContain('already in the built-in list')
    expect((form.findAll('input')[1]!.element as HTMLInputElement).value).toBe('https://bbc.example/rss.xml')
  })

  it('checks a pasted feed link, shows what it gives and keeps it in My sources from the topic form', async () => {
    lookupMock.mockResolvedValue({ found: true, title: 'Daily Blog', sample: [{ title: 'One' }], choices: [] })
    const p = props({ topics: [fresh], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    const feeds = w.get('input[aria-label="Follow a website (optional)"]')
    await feeds.setValue('https://new.example/feed.xml')
    await feeds.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(w.get('[data-test="custom-feed"]').text()).toContain('Reads fine: Daily Blog')
    await w.findAll('[data-test="custom-feed"] button').find((b) => b.text() === 'Save to my sources')!.trigger('click')
    expect(p.addSource).toHaveBeenCalledWith('Daily Blog', 'https://new.example/feed.xml')
  })

  it('does not check a feed that is in a list', async () => {
    lookupMock.mockClear()
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [fresh], editions: [] }) })
    await w.get('button[aria-label="Edit Fresh"]').trigger('click')
    await w.findAll('[data-test="source-browser"] button').find((b) => b.text().startsWith('BBC News'))!.trigger('click')
    await flushPromises()
    expect(w.find('[data-test="custom-feed"]').exists()).toBe(false)
    expect(lookupMock).not.toHaveBeenCalled()
  })
})
