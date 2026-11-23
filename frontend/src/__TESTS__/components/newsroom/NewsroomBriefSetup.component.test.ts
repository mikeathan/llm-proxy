import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
const { lookupMock } = vi.hoisted(() => ({ lookupMock: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: { lookup: lookupMock } }))

import NewsroomBriefSetup from '../../../components/newsroom/NewsroomBriefSetup.vue'
import type { BriefEdition, BriefSource, BriefTopic } from '../../../types/newsroom'

const topic: BriefTopic = { id: 1, name: 'Technology', queries: ['AI chips'] }
const edition: BriefEdition = { id: 2, name: 'Morning', ready_by: '07:00', days: [1, 2, 3, 4, 5], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [{ kind: 'news', topic_id: 1 }], enabled: true }

const catalog: BriefSource[] = [
  { label: 'BBC News', category: 'News', kind: 'feed', target: 'https://bbc.example/rss.xml', builtin: true },
  { label: 'Ars Technica', category: 'Technology', kind: 'feed', target: 'https://ars.example/feed', builtin: true },
  { label: 'Reuters', category: 'Search a publisher', kind: 'newssearch', target: 'site:reuters.com', builtin: true },
  { id: 9, label: 'My blog', category: 'My sources', kind: 'feed', target: 'https://blog.example/feed.xml', builtin: false },
]

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

  it('will not offer an edition before there is a topic to put in it', () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    expect(w.text()).toContain('Add a topic first')
    expect(w.findAll('button').find((b) => b.text() === 'Add an edition')?.attributes('disabled')).toBeDefined()
  })

  it('saves a new topic from the form and closes it only when the save worked', async () => {
    const p = props()
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('World')
    const chips = w.get('[data-test="chip-input"] input')
    await chips.setValue('Middle East')
    await chips.trigger('keydown', { key: 'Enter' })
    await chips.setValue('Ukraine')
    await chips.trigger('keydown', { key: 'Enter' })
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'World', queries: ['Middle East', 'Ukraine'] }))
    expect(w.find('[data-test="chip-input"]').exists()).toBe(false)
  })

  it('keeps the form open when the save is refused', async () => {
    const p = props({ saveTopic: vi.fn().mockResolvedValue(false), error: 'a topic needs a name' })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('World')
    const chips = w.get('[data-test="chip-input"] input')
    await chips.setValue('Ukraine')
    await chips.trigger('keydown', { key: 'Enter' })
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.find('[data-test="chip-input"]').exists()).toBe(true)
    expect(w.get('[role="alert"]').text()).toContain('a topic needs a name')
  })

  it('does not save a topic with nothing to read', async () => {
    const p = props()
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Empty')
    await w.get('form').trigger('submit')
    expect(p.saveTopic).not.toHaveBeenCalled()
    expect(w.get('[data-test="topic-needs"]').text()).toContain('at least one search or one feed')
  })

  it('saves a topic from one search alone: the name comes from the search and nothing else is asked', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    const chips = w.get('[data-test="chip-input"] input')
    await chips.setValue('ai chips')
    await chips.trigger('keydown', { key: 'Enter' })
    expect(w.get('[data-test="topic-summary"]').text()).toContain('Called Ai chips, in a section of its own, reading 1 search and 0 sites')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Ai chips', category: '', queries: ['ai chips'] }))
  })

  it('fills the name, section, searches and sites from a tapped suggestion, and takes them back out on a second tap', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    const tap = () => w.findAll('[data-test="topic-suggestions"] button').find((b) => b.text().startsWith('AI and chips'))!.trigger('click')
    await tap()
    expect(w.get('[data-test="topic-summary"]').text()).toContain('Called AI and chips, in Technology, reading 2 searches and 1 site')
    await tap()
    expect(w.find('[data-test="topic-summary"]').exists()).toBe(false)
    await tap()
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'AI and chips', category: 'Technology', queries: ['AI chips', 'Nvidia'], sources: [{ kind: 'feed', label: 'Ars Technica', target: 'https://ars.example/feed' }] }))
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

  it('opens the form from "Make my own" and keeps Fine-tune open when a topic is edited', async () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    await w.get('[data-test="starter-gallery"]').findAll('button').find((b) => b.text() === 'Make my own')!.trigger('click')
    expect(w.find('[data-test="starter-gallery"]').exists()).toBe(false)
    expect(w.get('[data-test="topic-fine-tune"]').attributes('open')).toBeUndefined()
    const edit = mount(NewsroomBriefSetup, { props: props() })
    await edit.get('button[aria-label="Edit Technology"]').trigger('click')
    expect(edit.get('[data-test="topic-fine-tune"]').attributes('open')).toBeDefined()
    expect(edit.find('[data-test="topic-suggestions"]').exists()).toBe(false)
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

  it('saves an edition with the topics ticked, in whole hours', async () => {
    const p = props({ editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add an edition')!.trigger('click')
    await w.get('form input[type="text"]').setValue('Evening')
    const topicBox = w.get('form input[type="checkbox"]')
    await topicBox.setValue(true)
    await w.get('[data-test="day-pills"] button').trigger('click')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveEdition).toHaveBeenCalledWith(null, expect.objectContaining({ name: 'Evening', collect_every: '2h', collect_for: '12h', sections: [{ kind: 'news', topic_id: 1 }] }))
    expect((p.saveEdition as ReturnType<typeof vi.fn>).mock.calls[0]![1].days.sort()).toEqual([0, 2, 3, 4, 5, 6])
  })

  it('follows a well-known site picked while typing, by its name, and sends the feed with that name', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Tech')
    const feeds = w.get('input[aria-label="Follow a website (optional)"]')
    await feeds.setValue('ars')
    expect(w.findAll('[data-test="chip-suggestions"] [role="option"]').map((o) => o.text())).toEqual(['Ars TechnicaTechnology'])
    await w.get('[data-test="chip-suggestions"] [role="option"]').trigger('mousedown')
    expect(w.text()).toContain('Ars Technica')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ sources: [{ kind: 'feed', label: 'Ars Technica', target: 'https://ars.example/feed' }] }))
  })

  it('does not turn a half-typed word into a feed', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Tech')
    const feeds = w.get('input[aria-label="Follow a website (optional)"]')
    await feeds.setValue('zzz')
    await feeds.trigger('keydown', { key: 'Enter' })
    expect(w.text()).toContain('Pick a site from the list')
    expect(w.find('[data-test="topic-needs"]').exists()).toBe(true)
  })

  it('browses the well-known sources by group, the group that matches the topic first, and adds a feed or a publisher search with one press', async () => {
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.get('[data-test="topic-name"]').setValue('Technology')
    const groups = w.findAll('[data-test="source-browser"] section').map((s) => s.attributes('aria-label'))
    expect(groups).toEqual(['Technology', 'News', 'Search a publisher', 'My sources'])
    const press = async (name: string) => w.findAll('[data-test="source-browser"] button').find((b) => b.text().startsWith(name))!.trigger('click')
    await press('Ars Technica')
    await press('Reuters')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(p.saveTopic).toHaveBeenCalledWith(null, expect.objectContaining({ queries: ['site:reuters.com'], sources: [{ kind: 'feed', label: 'Ars Technica', target: 'https://ars.example/feed' }] }))
  })

  it('takes a picked source back with a second press', async () => {
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
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
    const p = props({ topics: [], editions: [] })
    const w = mount(NewsroomBriefSetup, { props: p })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
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
    const w = mount(NewsroomBriefSetup, { props: props({ topics: [], editions: [] }) })
    await w.findAll('button').find((b) => b.text() === 'Add a topic')!.trigger('click')
    await w.findAll('[data-test="source-browser"] button').find((b) => b.text().startsWith('BBC News'))!.trigger('click')
    await flushPromises()
    expect(w.find('[data-test="custom-feed"]').exists()).toBe(false)
    expect(lookupMock).not.toHaveBeenCalled()
  })
})
