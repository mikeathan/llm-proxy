import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import type { NewsroomDay, NewsroomWatch } from '../../types/newsroom'

const { countsMock, watchesMock, feedMock, createMock, removeMock, markReadMock, clearMock, runMock, confirmMock, briefTopicsMock, deleteAllMock, createTopicMock, saveEditionMock, editionsMock, lookupMock } = vi.hoisted(() => ({
  countsMock: vi.fn(), briefTopicsMock: vi.fn(), deleteAllMock: vi.fn(), createTopicMock: vi.fn(), saveEditionMock: vi.fn(), editionsMock: vi.fn(), lookupMock: vi.fn(), watchesMock: vi.fn(), feedMock: vi.fn(), createMock: vi.fn(), removeMock: vi.fn(), markReadMock: vi.fn(), clearMock: vi.fn(), runMock: vi.fn(), confirmMock: vi.fn(),
}))
vi.mock('../../services/newsroom/newsroomService', () => ({
  NewsroomService: { watches: watchesMock, feed: feedMock, create: createMock, save: vi.fn(), remove: removeMock, markRead: markReadMock, clear: clearMock, run: runMock, lookup: lookupMock, keep: vi.fn().mockResolvedValue(undefined), alertCounts: countsMock },
}))
vi.mock('../../services/newsroom/briefService', () => ({
  BriefService: { editions: editionsMock, createTopic: createTopicMock, saveEdition: saveEditionMock, topics: briefTopicsMock, deleteAll: deleteAllMock, issues: vi.fn().mockResolvedValue([]), jobs: vi.fn().mockResolvedValue([]), latest: vi.fn().mockResolvedValue([]), sources: vi.fn().mockResolvedValue([]), settings: vi.fn().mockResolvedValue({ model_calls_per_day: 0, default: 200, max: 2000 }) },
}))
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
// The brief's side column reads Model use and the chips; quiet answers keep the test off the network.
vi.mock('../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))
vi.mock('../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ items: [] }) } }))
vi.mock('../../composables/models/useModels', () => ({
  useModels: () => ({ state: ref({ config: { communication: { connectors: { 'my-telegram': { type: 'telegram', enabled: true, settings: {} } } } } }) }),
}))

import NewsroomView from '../../views/NewsroomView.vue'
import { resetAlertsUnread } from '../../composables/newsroom/useAlertsUnread'

const watch = (over: Partial<NewsroomWatch> = {}): NewsroomWatch => ({
  id: 1, name: 'Go releases', interval: '1h0m0s', max_age: '72h0m0s', enabled: true, sources: [{ kind: 'feed', target: 'https://go.dev/blog/feed.atom' }], last_status: 'in_feed', ...over,
})
const entry = (id: number, title: string, read = false) => ({ id, watch_id: 1, at: '2026-11-20T10:00:00Z', source: 'go.dev', title, link: 'https://go.dev/x', read })
const day = (): NewsroomDay => ({
  day: '2026-11-20', label: 'Today', updates: 2, unread: 2,
  watches: [{ watch_id: 1, watch: 'Go releases', updates: 2, unread: 2, last_at: '2026-11-20T10:00:00Z', current: entry(2, 'Go 1.26 released'), earlier: [entry(1, 'Go 1.26 RC2')] }],
})

async function mountView(path = '/newsroom/alerts', watches: NewsroomWatch[] = [watch()], feed: NewsroomDay[] = [day()], counts = { unread: 0, leftOut: 0 }) {
  countsMock.mockResolvedValue(counts)
  watchesMock.mockResolvedValue(watches)
  feedMock.mockResolvedValue(feed)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/newsroom/:tab(alerts|following|settings)?', name: ROUTE_NAMES.newsroom, component: { template: '<div />' } }] })
  await router.push(path)
  const w = mount(NewsroomView, { global: { plugins: [router], stubs: { Icon: true, BrandMark: true, StackModules: { props: ['stack'], template: '<div data-test="stub-modules" :data-stack="stack" />' } } } })
  await flushPromises()
  return { w, router }
}
// The full watch form is reached from the Follow form, for every option.
async function openWatchForm(w: ReturnType<typeof mount>) {
  await w.get('[data-test="follow-open"]').trigger('click')
  await flushPromises()
  await w.get('[data-test="follow-advanced"]').trigger('click')
  await flushPromises()
}
// The watches are on Following, under Alerts.
async function mountWatches(watches: NewsroomWatch[] = [watch()]) {
  return mountView('/newsroom/following?kind=alerts', watches)
}
const buttonByName = (w: ReturnType<typeof mount>, name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? b.text()))

enableAutoUnmount(afterEach)
// The Alerts counts are one module-level poller: each test starts from none. Reset before, not after: the auto-unmount runs after an afterEach reset and
// would count its consumer off a poller that was already reset.
beforeEach(() => resetAlertsUnread())
beforeEach(() => {
  vi.clearAllMocks()
  confirmMock.mockResolvedValue(true)
  briefTopicsMock.mockResolvedValue([])
  editionsMock.mockResolvedValue([])
  lookupMock.mockResolvedValue({ found: true, sample: [{ title: 'A model shipped' }], choices: [] })
  deleteAllMock.mockResolvedValue(undefined)
  createMock.mockResolvedValue(watch())
  markReadMock.mockResolvedValue(undefined)
})

describe('NewsroomView', () => {
  it('is a stack page that opens on the brief: Brief, Alerts, Following and a Settings tab that shows the stack switchboard', async () => {
    const { w } = await mountView('/newsroom')
    expect(w.findAll('a.stack-tab').map((a) => a.text())).toEqual(['Brief', 'Alerts', 'Following', 'Settings'])
    expect(w.text()).toContain('No editions yet')
    const m = await mountView('/newsroom/settings')
    expect(m.w.get('[data-test="stub-modules"]').attributes('data-stack')).toBe('newsroom')
  })

  it('sets the brief up on Following, beside the watches, and the switch between them is in the address', async () => {
    const { w, router } = await mountView('/newsroom/following')
    expect(w.text()).toContain('New section')
    expect(w.text()).toContain('Add an edition')
    await w.findAll('[role="radio"]').find((b) => b.text() === 'Alerts')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.kind).toBe('alerts')
    expect(w.text()).toContain('Go releases')
  })

  it('offers "Delete all briefs" only when there is something to delete, and only after a confirmation', async () => {
    const empty = (await mountView('/newsroom/following')).w
    expect(empty.find('[data-test="delete-all-briefs"]').exists()).toBe(false)
    briefTopicsMock.mockResolvedValue([{ id: 1, name: 'Go' }])
    expect((await mountView('/newsroom')).w.find('[data-test="delete-all-briefs"]').exists(), 'not on the reading view').toBe(false)
    const { w } = await mountView('/newsroom/following')
    confirmMock.mockResolvedValueOnce(false)
    await w.get('[data-test="delete-all-briefs"]').trigger('click')
    await flushPromises()
    expect(deleteAllMock).not.toHaveBeenCalled()
    await w.get('[data-test="delete-all-briefs"]').trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenLastCalledWith(expect.objectContaining({ title: 'Delete all briefs?', confirmText: 'Delete all briefs' }))
    expect(deleteAllMock).toHaveBeenCalledTimes(1)
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
    const { w } = await mountView('/newsroom/alerts', [watch()], [day(), older])
    expect(w.text()).toContain('Yesterday')
    expect(w.text()).not.toContain('Go 1.25.5 released')
    await w.get('section[aria-label="Yesterday"] header button').trigger('click')
    expect(w.text()).toContain('Go 1.25.5 released')
  })

  it('stays quiet while every watch works, and names the one that needs a look', async () => {
    const quiet = await mountView()
    expect(quiet.w.find('[data-test="alert-problems"]').exists()).toBe(false)
    quiet.w.unmount()
    const { w } = await mountView('/newsroom/alerts', [watch(), watch({ id: 2, name: 'Rust', last_status: 'failing' })])
    expect(w.get('[data-test="alert-problems"]').text()).toContain('Rust: source failing')
  })

  it('shows how many items the model left out in the switch, and lists them with a way back', async () => {
    const { w } = await mountView('/newsroom/alerts', [watch({ look_for: 'new releases' })], [day()], { unread: 2, leftOut: 6 })
    expect(w.text()).toContain('Left out by the model · 6')
    const alertsTab = w.findAll('a.stack-tab').find((a) => a.text().startsWith('Alerts'))
    expect(alertsTab!.find('[data-test="tab-dot"]').exists()).toBe(true)
    expect(alertsTab!.text()).toContain('2 unread alerts')
    feedMock.mockResolvedValue([])
    const leftOut = w.findAll('button, [role="radio"], label').find((b) => b.text().startsWith('Left out by the model'))
    await leftOut!.trigger('click')
    await flushPromises()
    expect(feedMock).toHaveBeenLastCalledWith({ watch: null, unreadOnly: false, days: null, leftOut: true }, expect.any(String))
    expect(w.find('[data-test="left-out"]').exists()).toBe(true)
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

  const pickPeriod = async (w: ReturnType<typeof mount>, value: string) => {
    const labels: Record<string, string> = { '': 'All', '1': 'Today', '7': '7 days', '14': '14 days' }
    await w.findAll('[role="radio"]').find((b) => b.text() === labels[value])!.trigger('click')
    await flushPromises()
  }

  it('asks the server for the chosen period', async () => {
    const { w } = await mountView()
    await pickPeriod(w, '7')
    expect(feedMock.mock.calls[feedMock.mock.calls.length - 1]![0]).toMatchObject({ days: 7 })
    await pickPeriod(w, '')
    expect(feedMock.mock.calls[feedMock.mock.calls.length - 1]![0]).toMatchObject({ days: null })
  })

  it('chooses a watch with a chip, and asks the server for that watch only', async () => {
    const { w } = await mountView('/newsroom/alerts', [watch(), watch({ id: 2, name: 'Security' })])
    await w.findAll('[data-test="watch-chips"] button').find((b) => b.text() === 'Security')!.trigger('click')
    await flushPromises()
    expect(feedMock.mock.calls[feedMock.mock.calls.length - 1]![0]).toMatchObject({ watch: 2 })
  })

  it('says so when a filter leaves nothing to show', async () => {
    const { w } = await mountView('/newsroom/alerts', [watch(), watch({ id: 2, name: 'Security' })])
    feedMock.mockResolvedValue([])
    await pickPeriod(w, '1')
    expect(w.get('[data-test="feed-filter-empty"]').text()).toContain('Nothing here')
  })

  it('with a period chosen, Clear all deletes only the entries on screen, never the whole feed', async () => {
    clearMock.mockResolvedValue(2)
    const { w } = await mountView()
    await pickPeriod(w, '1')
    await w.findAll('button').find((b) => b.text() === 'Clear all')!.trigger('click')
    await flushPromises()
    expect(clearMock).toHaveBeenCalledWith({ ids: [2, 1] })
  })

  it('asks to add a first watch when none is followed', async () => {
    const { w } = await mountView('/newsroom/alerts', [], [])
    expect(w.text()).toContain('You follow nothing yet')
  })

  it('adds a feed-only watch from the form', async () => {
    const { w } = await mountWatches()
    await openWatchForm(w)
    await w.get('form input[type="text"]').setValue('LLM news')
    const inputs = w.findAll('form input[type="text"]')
    await inputs[1]!.setValue('https://example.com/feed.xml')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).toHaveBeenCalledWith({
      name: 'LLM news', interval: '1h', max_age: '72h', channel: '', enabled: true, look_for: '', keywords: [], skip: [], sources: [{ kind: 'feed', target: 'https://example.com/feed.xml' }],
    })
  })

  it('starts from a suggestion for what to look for and sends the keywords as lists', async () => {
    const { w } = await mountWatches()
    await openWatchForm(w)
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
    const { w } = await mountView('/newsroom/alerts', [watch({ look_for: 'new models' })], [{ ...day(), watches: [{ watch_id: 1, watch: 'Go releases', updates: 1, unread: 1, last_at: '2026-11-20T10:00:00Z', current: found }] }])
    await w.get('article button[aria-expanded]').trigger('click')
    expect(w.text()).toContain('Why: a new model release')
    expect(w.text()).toContain('First check')
    await mountWatches([watch({ look_for: 'new models' })]).then(({ w: p }) => expect(p.text()).toContain('Looking for: new models'))
  })

  it('folds the existing sources of a watch that has several, and opens one on a click', async () => {
    const many = watch({ sources: [{ kind: 'feed', target: 'https://a.example/feed' }, { kind: 'telegram', target: 'aipost' }, { kind: 'hackernews', target: 'rust' }] })
    const { w } = await mountWatches([many])
    await buttonByName(w, /Edit/)!.trigger('click')
    await flushPromises()
    const folded = () => w.findAll('form fieldset > div').map((d) => d.find('div.flex-col').attributes('style') ?? '')
    expect(folded().every((s) => s.includes('display: none'))).toBe(true)
    expect(w.text()).toContain('Telegram public channel: aipost')
    await w.findAll('[data-test="source-toggle"]')[1]!.trigger('click')
    expect(folded().map((s) => s.includes('display: none'))).toEqual([true, false, true])
  })

  it('does not send a watch with no name or no source', async () => {
    const { w } = await mountWatches()
    await openWatchForm(w)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(createMock).not.toHaveBeenCalled()
  })

  it('lists the watches with their last result and checks one now', async () => {
    runMock.mockResolvedValue({ watch_id: 1, status: 'nothing_new', detail: '', new: 0 })
    const { w } = await mountWatches([watch({ last_detail: 'source failing: timeout' })])
    expect(w.text()).toContain('Go releases')
    expect(w.text()).toContain('Every hour')
    expect(w.text()).toContain('Feed only')
    expect(w.text()).toContain('source failing: timeout')
    await buttonByName(w, /Check now/)!.trigger('click')
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith(1)
  })

  it('says the check is running on that watch while it works, then goes back', async () => {
    let finish: (v: unknown) => void = () => {}
    runMock.mockReturnValue(new Promise((r) => { finish = r }))
    const { w } = await mountWatches([watch()])
    await buttonByName(w, /Check now/)!.trigger('click')
    await flushPromises()
    expect(buttonByName(w, /Checking/)).toBeTruthy()
    expect(w.text()).toContain('Reading the sources')
    finish({ watch_id: 1, status: 'nothing_new', detail: '', new: 0 })
    await flushPromises()
    expect(buttonByName(w, /Check now/)).toBeTruthy()
    expect(w.text()).not.toContain('Reading the sources')
  })

  it('asks before it deletes a watch, and does nothing when declined', async () => {
    confirmMock.mockResolvedValue(false)
    const { w } = await mountWatches()
    await buttonByName(w, /Delete/)!.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalled()
    expect(removeMock).not.toHaveBeenCalled()
  })


  it('follows something from one form: in the brief and as an alert, joining the brief that reads its section', async () => {
    const edition = { id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', enabled: true, sections: [{ kind: 'news', topic_id: 1 }], stories_per_section: 5, top_stories: 4, repeat_after: '24h0m0s', max_story_age: '48h0m0s' }
    const ai = { id: 1, name: 'AI', category: 'Technology', queries: ['AI'] }
    const added = { id: 9, name: 'open weights', category: 'Technology', queries: ['open weights'] }
    briefTopicsMock.mockResolvedValue([ai])
    editionsMock.mockResolvedValue([edition])
    createTopicMock.mockImplementation(async () => {
      briefTopicsMock.mockResolvedValue([ai, added])
      return added
    })
    saveEditionMock.mockResolvedValue(edition)
    const { w, router } = await mountView('/newsroom/following')
    await w.get('[data-test="follow-open"]').trigger('click')
    await flushPromises()
    await w.get('[data-test="follow-text"]').setValue('open weights')
    await w.get('[data-test="follow-preview"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="follow-preview-list"]').text()).toContain('A model shipped')
    await w.get('[data-mode="both"]').trigger('click')
    await w.get('[data-test="follow-section"]').setValue('Technology')
    await w.get('[data-test="follow-form"]').trigger('submit')
    await flushPromises()
    expect(createTopicMock).toHaveBeenCalledWith(expect.objectContaining({ name: 'open weights', category: 'Technology', queries: ['open weights'] }))
    expect(saveEditionMock).toHaveBeenCalledWith(2, expect.objectContaining({ sections: [{ kind: 'news', topic_id: 1 }, { kind: 'news', topic_id: 9 }] }))
    expect(createMock).toHaveBeenCalledWith(expect.objectContaining({ name: 'open weights', sources: [{ kind: 'newssearch', target: 'open weights' }] }))
    expect(router.currentRoute.value.query.follow).toBeUndefined()
  })

  it('opens the Follow form with a story\'s words, as an alert', async () => {
    const { w } = await mountView('/newsroom/following?kind=alerts&follow=fed%20rates')
    expect((w.get('[data-test="follow-text"]').element as HTMLInputElement).value).toBe('fed rates')
    expect(w.get('[data-mode="alerts"]').attributes('aria-checked')).toBe('true')
    expect(lookupMock).toHaveBeenCalledWith({ kind: 'newssearch', target: 'fed rates' }, expect.anything())
  })
})
