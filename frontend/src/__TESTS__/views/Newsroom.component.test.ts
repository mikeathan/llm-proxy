import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import type { NewsroomDay, NewsroomWatch } from '../../types/newsroom'

const { watchesMock, feedMock, createMock, removeMock, markReadMock, clearMock, runMock, confirmMock } = vi.hoisted(() => ({
  watchesMock: vi.fn(), feedMock: vi.fn(), createMock: vi.fn(), removeMock: vi.fn(), markReadMock: vi.fn(), clearMock: vi.fn(), runMock: vi.fn(), confirmMock: vi.fn(),
}))
vi.mock('../../services/newsroom/newsroomService', () => ({
  NewsroomService: { watches: watchesMock, feed: feedMock, create: createMock, save: vi.fn(), remove: removeMock, markRead: markReadMock, clear: clearMock, run: runMock },
}))
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('../../composables/models/useModels', () => ({
  useModels: () => ({ state: ref({ config: { communication: { connectors: { 'my-telegram': { type: 'telegram', enabled: true, settings: {} } } } } }) }),
}))

import NewsroomView from '../../views/NewsroomView.vue'

const watch = (over: Partial<NewsroomWatch> = {}): NewsroomWatch => ({
  id: 1, name: 'Go releases', interval: '1h0m0s', enabled: true, sources: [{ kind: 'feed', target: 'https://go.dev/blog/feed.atom' }], last_status: 'in_feed', ...over,
})
const entry = (id: number, title: string, read = false) => ({ id, watch_id: 1, at: '2026-11-20T10:00:00Z', source: 'go.dev', title, link: 'https://go.dev/x', read })
const day = (): NewsroomDay => ({
  day: '2026-11-20', label: 'Today', updates: 2, unread: 2,
  watches: [{ watch_id: 1, watch: 'Go releases', updates: 2, unread: 2, last_at: '2026-11-20T10:00:00Z', current: entry(2, 'Go 1.26 released'), earlier: [entry(1, 'Go 1.26 RC2')] }],
})

async function mountView(path = '/newsroom', watches: NewsroomWatch[] = [watch()], feed: NewsroomDay[] = [day()]) {
  watchesMock.mockResolvedValue(watches)
  feedMock.mockResolvedValue(feed)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/newsroom/:tab(watches|settings)?', name: ROUTE_NAMES.newsroom, component: { template: '<div />' } }] })
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
  createMock.mockResolvedValue(watch())
  markReadMock.mockResolvedValue(undefined)
})

describe('NewsroomView', () => {
  it('is a stack page: Feed, Watches and a Settings tab that shows the stack switchboard', async () => {
    const { w } = await mountView()
    expect(w.findAll('a.stack-tab').map((a) => a.text())).toEqual(['Feed', 'Watches', 'Settings'])
    const m = await mountView('/newsroom/settings')
    expect(m.w.get('[data-test="stub-modules"]').attributes('data-stack')).toBe('newsroom')
  })

  it("asks for the feed in the reader's zone, and shows the day's findings as one list, newest first", async () => {
    const { w } = await mountView()
    expect(feedMock.mock.calls[0]![1]).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
    expect(w.text()).toContain('Today')
    expect(w.text()).toContain('2 updates')
    expect(w.text()).toContain('Go 1.26 released')
    expect(w.text()).toContain('Go 1.26 RC2')
    expect(w.text()).not.toContain('Earlier today')
  })

  it('marks every unread finding of a day read, and one finding when its link is opened', async () => {
    const { w } = await mountView()
    const button = w.findAll('section[aria-label="Today"] header button').find((b) => /Mark read/.test(b.text()))!
    await button.trigger('click')
    await flushPromises()
    expect(markReadMock).toHaveBeenCalledWith([2, 1], true)
    markReadMock.mockClear()
    await w.get('section[aria-label="Today"] ul a').trigger('click')
    await flushPromises()
    expect(markReadMock).toHaveBeenCalledWith([2], true)
  })

  it('folds older days to a header and opens one on a click', async () => {
    const older: NewsroomDay = { day: '2026-11-19', label: 'Yesterday', updates: 1, unread: 0, watches: [{ watch_id: 1, watch: 'Go releases', updates: 1, unread: 0, last_at: '2026-11-19T10:00:00Z', current: { ...entry(9, 'Go 1.25.5 released', true), at: '2026-11-19T10:00:00Z' } }] }
    const { w } = await mountView('/newsroom', [watch()], [day(), older])
    expect(w.text()).toContain('Yesterday')
    expect(w.text()).not.toContain('Go 1.25.5 released')
    await w.get('section[aria-label="Yesterday"] header button').trigger('click')
    expect(w.text()).toContain('Go 1.25.5 released')
  })

  it('selects entries, asks before deleting them for good, then clears only those', async () => {
    clearMock.mockResolvedValue(1)
    confirmMock.mockResolvedValue(true)
    const { w } = await mountView()
    await w.findAll('button').find((b) => b.text() === 'Select')!.trigger('click')
    await w.findAll('section[aria-label="Today"] input[type=checkbox]')[0]!.setValue(true)
    expect(w.text()).toContain('1 selected')
    await w.findAll('button').find((b) => b.text() === 'Clear')!.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalledOnce()
    expect(confirmMock.mock.calls[0]![0].message).toMatch(/1 entry .*deleted for good/)
    expect(clearMock).toHaveBeenCalledWith({ ids: [2] })
  })

  it('clears nothing when the owner says no, and clears everything after a yes', async () => {
    clearMock.mockResolvedValue(2)
    const { w } = await mountView()
    const clearAll = () => w.findAll('button').find((b) => b.text() === 'Clear all')!
    confirmMock.mockResolvedValue(false)
    await clearAll().trigger('click')
    await flushPromises()
    expect(clearMock).not.toHaveBeenCalled()
    confirmMock.mockResolvedValue(true)
    await clearAll().trigger('click')
    await flushPromises()
    expect(clearMock).toHaveBeenCalledWith({ scope: 'all' })
  })

  it('asks to add a first watch when none is followed', async () => {
    const { w } = await mountView('/newsroom', [], [])
    expect(w.text()).toContain('You follow nothing yet')
  })

  it('adds a feed-only watch from the form', async () => {
    const { w } = await mountView('/newsroom/watches')
    await buttonByName(w, /New watch/)!.trigger('click')
    await flushPromises()
    await w.get('form input[type="text"]').setValue('LLM news')
    const inputs = w.findAll('form input[type="text"]')
    await inputs[1]!.setValue('https://example.com/feed.xml')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).toHaveBeenCalledWith({
      name: 'LLM news', interval: '1h', channel: '', enabled: true, look_for: '', keywords: [], skip: [], sources: [{ kind: 'feed', target: 'https://example.com/feed.xml' }],
    })
  })

  it('starts from a suggestion for what to look for and sends the keywords as lists', async () => {
    const { w } = await mountView('/newsroom/watches')
    await buttonByName(w, /New watch/)!.trigger('click')
    await flushPromises()
    await buttonByName(w, /Security problems/)!.trigger('click')
    const inputs = w.findAll('form input[type="text"]')
    expect((inputs[0]!.element as HTMLInputElement).value).toBe('Security')
    await inputs[1]!.setValue('https://example.com/feed.xml')
    await inputs[2]!.setValue('CVE, patch ,')
    await inputs[3]!.setValue('sponsored')
    await w.get('form').trigger('submit')
    await flushPromises()
    const sent = createMock.mock.calls[0]![0]
    expect(sent.look_for).toContain('Security vulnerabilities')
    expect(sent.keywords).toEqual(['CVE', 'patch'])
    expect(sent.skip).toEqual(['sponsored'])
  })

  it('shows what a watch looks for, and the reason and first-check mark on a finding', async () => {
    const found = { ...entry(1, 'Qwen4 released'), why: 'a new model release', first: true }
    const { w } = await mountView('/newsroom', [watch({ look_for: 'new models' })], [{ ...day(), watches: [{ watch_id: 1, watch: 'Go releases', updates: 1, unread: 1, last_at: '2026-11-20T10:00:00Z', current: found }] }])
    await w.get('article button[aria-expanded]').trigger('click')
    expect(w.text()).toContain('Why: a new model release')
    expect(w.text()).toContain('First check')
    await mountView('/newsroom/watches', [watch({ look_for: 'new models' })]).then(({ w: p }) => expect(p.text()).toContain('Looking for: new models'))
  })

  it('folds the existing sources of a watch that has several, and opens one on a click', async () => {
    const many = watch({ sources: [{ kind: 'feed', target: 'https://a.example/feed' }, { kind: 'telegram', target: 'aipost' }, { kind: 'hackernews', target: 'rust' }] })
    const { w } = await mountView('/newsroom/watches', [many])
    await buttonByName(w, /Edit/)!.trigger('click')
    await flushPromises()
    const folded = () => w.findAll('form fieldset > div').map((d) => d.find('div.flex-col').attributes('style') ?? '')
    expect(folded().every((s) => s.includes('display: none'))).toBe(true)
    expect(w.text()).toContain('Telegram public channel: aipost')
    await w.findAll('[data-test="source-toggle"]')[1]!.trigger('click')
    expect(folded().map((s) => s.includes('display: none'))).toEqual([true, false, true])
  })

  it('does not send a watch with no name or no source', async () => {
    const { w } = await mountView('/newsroom/watches')
    await buttonByName(w, /New watch/)!.trigger('click')
    await flushPromises()
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).not.toHaveBeenCalled()
  })

  it('lists the watches with their last result and checks one now', async () => {
    runMock.mockResolvedValue({ watch_id: 1, status: 'nothing_new', detail: '', new: 0 })
    const { w } = await mountView('/newsroom/watches', [watch({ last_detail: 'source failing: timeout' })])
    expect(w.text()).toContain('Go releases')
    expect(w.text()).toContain('Every hour')
    expect(w.text()).toContain('Feed only')
    expect(w.text()).toContain('source failing: timeout')
    await buttonByName(w, /Check now/)!.trigger('click')
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith(1)
  })

  it('asks before it deletes a watch, and does nothing when declined', async () => {
    confirmMock.mockResolvedValue(false)
    const { w } = await mountView('/newsroom/watches')
    await buttonByName(w, /Delete/)!.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalled()
    expect(removeMock).not.toHaveBeenCalled()
  })

})
