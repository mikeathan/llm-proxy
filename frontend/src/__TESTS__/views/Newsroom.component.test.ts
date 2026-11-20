import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import type { NewsroomDay, NewsroomTopic } from '../../types/newsroom'

const { topicsMock, feedMock, createMock, removeMock, markReadMock, runMock, confirmMock } = vi.hoisted(() => ({
  topicsMock: vi.fn(), feedMock: vi.fn(), createMock: vi.fn(), removeMock: vi.fn(), markReadMock: vi.fn(), runMock: vi.fn(), confirmMock: vi.fn(),
}))
vi.mock('../../services/newsroom/newsroomService', () => ({
  NewsroomService: { topics: topicsMock, feed: feedMock, create: createMock, save: vi.fn(), remove: removeMock, markRead: markReadMock, run: runMock },
}))
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('../../composables/models/useModels', () => ({
  useModels: () => ({ state: ref({ config: { communication: { connectors: { 'my-telegram': { type: 'telegram', enabled: true, settings: {} } } } } }) }),
}))

import NewsroomView from '../../views/NewsroomView.vue'

const topic = (over: Partial<NewsroomTopic> = {}): NewsroomTopic => ({
  id: 1, name: 'Go releases', interval: '1h0m0s', enabled: true, sources: [{ kind: 'feed', target: 'https://go.dev/blog/feed.atom' }], last_status: 'in_feed', ...over,
})
const entry = (id: number, title: string, read = false) => ({ id, topic_id: 1, at: '2026-11-20T10:00:00Z', source: 'go.dev', title, link: 'https://go.dev/x', read })
const day = (): NewsroomDay => ({
  day: '2026-11-20', label: 'Today', updates: 2, unread: 2,
  topics: [{ topic_id: 1, topic: 'Go releases', updates: 2, unread: 2, last_at: '2026-11-20T10:00:00Z', current: entry(2, 'Go 1.26 released'), earlier: [entry(1, 'Go 1.26 RC2')] }],
})

async function mountView(path = '/newsroom', topics: NewsroomTopic[] = [topic()], feed: NewsroomDay[] = [day()]) {
  topicsMock.mockResolvedValue(topics)
  feedMock.mockResolvedValue(feed)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/newsroom/:tab(topics|sources|settings|help)?', name: ROUTE_NAMES.newsroom, component: { template: '<div />' } }] })
  await router.push(path)
  const w = mount(NewsroomView, { global: { plugins: [router], stubs: { Icon: true, BrandMark: true, StackModules: { props: ['stack'], template: '<div data-test="stub-modules" :data-stack="stack" />' } } } })
  await flushPromises()
  return { w, router }
}
const buttonByName = (w: ReturnType<typeof mount>, name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? b.text()))

enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  confirmMock.mockResolvedValue(true)
  createMock.mockResolvedValue(topic())
  markReadMock.mockResolvedValue(undefined)
})

describe('NewsroomView', () => {
  it('is a stack page: Feed, Topics, Sources and a Settings tab that shows the stack switchboard', async () => {
    const { w } = await mountView()
    expect(w.findAll('a.stack-tab').map((a) => a.text())).toEqual(['Feed', 'Topics', 'Sources', 'Settings', 'Help'])
    const m = await mountView('/newsroom/settings')
    expect(m.w.get('[data-test="stub-modules"]').attributes('data-stack')).toBe('newsroom')
  })

  it("asks for the feed in the reader's zone, and shows the newest finding with the earlier ones of the day still visible", async () => {
    const { w } = await mountView()
    expect(feedMock.mock.calls[0]![1]).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
    expect(w.text()).toContain('Today')
    expect(w.text()).toContain('2 updates')
    expect(w.text()).toContain('Go 1.26 released')
    expect(w.text()).toContain('Go 1.26 RC2')
  })

  it('marks every unread finding of a topic card read', async () => {
    const { w } = await mountView()
    const button = w.findAll('section[aria-label="Go releases"] button').find((b) => /Mark read/.test(b.text()))!
    await button.trigger('click')
    await flushPromises()
    expect(markReadMock).toHaveBeenCalledWith([2, 1], true)
  })

  it('asks to add a first topic when none is followed', async () => {
    const { w } = await mountView('/newsroom', [], [])
    expect(w.text()).toContain('You follow nothing yet')
  })

  it('adds a feed-only topic from the form', async () => {
    const { w } = await mountView('/newsroom/topics')
    await buttonByName(w, /New topic/)!.trigger('click')
    await flushPromises()
    await w.get('form input[type="text"]').setValue('LLM news')
    const inputs = w.findAll('form input[type="text"]')
    await inputs[1]!.setValue('https://example.com/feed.xml')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).toHaveBeenCalledWith({
      name: 'LLM news', interval: '1h', channel: '', enabled: true, sources: [{ kind: 'feed', target: 'https://example.com/feed.xml' }],
    })
  })

  it('does not send a topic with no name or no source', async () => {
    const { w } = await mountView('/newsroom/topics')
    await buttonByName(w, /New topic/)!.trigger('click')
    await flushPromises()
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).not.toHaveBeenCalled()
  })

  it('lists the topics with their last result and checks one now', async () => {
    runMock.mockResolvedValue({ topic_id: 1, status: 'nothing_new', detail: '', new: 0 })
    const { w } = await mountView('/newsroom/topics', [topic({ last_detail: 'source failing: timeout' })])
    expect(w.text()).toContain('Go releases')
    expect(w.text()).toContain('Every hour')
    expect(w.text()).toContain('Feed only')
    expect(w.text()).toContain('source failing: timeout')
    await buttonByName(w, /Check now/)!.trigger('click')
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith(1)
  })

  it('asks before it deletes a topic, and does nothing when declined', async () => {
    confirmMock.mockResolvedValue(false)
    const { w } = await mountView('/newsroom/topics')
    await buttonByName(w, /Delete/)!.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalled()
    expect(removeMock).not.toHaveBeenCalled()
  })

  it('lists each source once with whether it works, and checks the topic that reads it', async () => {
    runMock.mockResolvedValue({ topic_id: 1, status: 'nothing_new', detail: '', new: 0 })
    const { w } = await mountView('/newsroom/sources', [topic({ last_status: 'failing', last_detail: 'timed out' })])
    expect(w.findAll('[data-test="newsroom-source"]')).toHaveLength(1)
    expect(w.text()).toContain('Failing')
    expect(w.text()).toContain('timed out')
    expect(w.text()).toContain('Go releases')
    await buttonByName(w, /Check now/)!.trigger('click')
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith(1)
  })

  it('asks for a first topic on the Sources tab when none is followed', async () => {
    const { w } = await mountView('/newsroom/sources', [], [])
    expect(w.text()).toContain('No sources yet')
  })

  it('shows the Help tab: what a topic is and where to find a Telegram name', async () => {
    const { w } = await mountView('/newsroom/help', [], [])
    const text = w.get('[data-test="newsroom-help"]').text()
    expect(text).toContain('only a label for you')
    expect(text).toContain('t.me/s/NAME')
  })

})
