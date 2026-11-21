import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { NewsroomSource, SourcePreview } from '../../../types/newsroom'

const { lookupMock, suggestMock } = vi.hoisted(() => ({ lookupMock: vi.fn(), suggestMock: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: { lookup: lookupMock, suggest: suggestMock } }))

import NewsroomSourceLookup from '../../../components/newsroom/NewsroomSourceLookup.vue'

const found = (over: Partial<SourcePreview> = {}): SourcePreview => ({
  found: true, target: 'aipost', title: 'AI Post', sample: [{ title: 'Hello world', link: 'https://t.me/aipost/9', at: '2026-11-20T10:00:00Z' }], choices: [], ...over,
})

function mountLookup(source: NewsroomSource) {
  return mount(NewsroomSourceLookup, { props: { source }, global: { stubs: { Icon: true } } })
}
const run = (w: ReturnType<typeof mount>) => w.get('[data-test="lookup-run"]')

enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  suggestMock.mockResolvedValue({ choices: [] })
})

describe('NewsroomSourceLookup', () => {
  it('cannot look up an empty or invalid value, and asks the server nothing', async () => {
    const w = mountLookup({ kind: 'telegram', target: '' })
    expect(run(w).attributes('disabled')).toBeDefined()
    await w.setProps({ source: { kind: 'telegram', target: 'a b' } })
    expect(run(w).attributes('disabled')).toBeDefined()
    expect(lookupMock).not.toHaveBeenCalled()
  })

  it('shows the name and latest items of what it found, and asks to keep the cleaned-up value', async () => {
    lookupMock.mockResolvedValue(found())
    const w = mountLookup({ kind: 'telegram', target: 'https://t.me/s/aipost' })
    await run(w).trigger('click')
    await flushPromises()
    expect(lookupMock.mock.calls[0]![0]).toMatchObject({ kind: 'telegram', target: 'https://t.me/s/aipost' })
    const card = w.get('[data-test="lookup-found"]')
    expect(card.text()).toContain('AI Post')
    expect(card.text()).toContain('Hello world')
    expect(card.text()).toContain('Will be saved as aipost')
    expect(w.emitted('pick')![0]![0]).toEqual({ target: 'aipost' })
  })

  it('says why nothing was found, in the server\'s words', async () => {
    lookupMock.mockResolvedValue({ found: false, sample: [], choices: [], problem: 'No public preview was found for that channel.' })
    const w = mountLookup({ kind: 'telegram', target: 'nobodyhere' })
    await run(w).trigger('click')
    await flushPromises()
    expect(w.get('[data-test="lookup-problem"]').text()).toContain('No public preview')
    expect(w.emitted('pick')).toBeUndefined()
  })

  it('offers the feeds a page advertises, and picking one fills the address', async () => {
    lookupMock.mockResolvedValue({ found: false, sample: [], choices: [{ value: 'https://example.com/feed.xml', label: 'All posts' }], problem: 'That address is not a feed itself.' })
    const w = mountLookup({ kind: 'feed', target: 'https://example.com/' })
    await run(w).trigger('click')
    await flushPromises()
    await w.get('[data-test="lookup-choice"]').trigger('click')
    expect(w.emitted('pick')![0]![0]).toEqual({ target: 'https://example.com/feed.xml' })
    expect(w.find('[data-test="lookup-problem"]').exists()).toBe(false)
  })

  it('shows a request that failed as words, not a crash', async () => {
    lookupMock.mockRejectedValue(new Error('Watches is switched off: switch it on under Stacks'))
    const w = mountLookup({ kind: 'hackernews', target: 'rust' })
    await run(w).trigger('click')
    await flushPromises()
    expect(w.get('[data-test="lookup-problem"]').text()).toContain('switched off')
  })

  it('shows a link only when it is http or https, and never as markup', async () => {
    lookupMock.mockResolvedValue(found({ sample: [{ title: '<b>bold</b>', link: 'javascript:alert(1)' }, { title: 'ok', link: 'https://example.com/a' }] }))
    const w = mountLookup({ kind: 'hackernews', target: 'rust' })
    await run(w).trigger('click')
    await flushPromises()
    const items = w.findAll('[data-test="lookup-sample"]')
    expect(items[0]!.find('a').exists()).toBe(false)
    expect(items[0]!.text()).toContain('<b>bold</b>')
    expect(items[0]!.find('b').exists()).toBe(false)
    expect(items[1]!.get('a').attributes('href')).toBe('https://example.com/a')
  })

  it('forgets a preview when the value is edited', async () => {
    lookupMock.mockResolvedValue(found({ target: 'rust' }))
    const w = mountLookup({ kind: 'hackernews', target: 'rust' })
    await run(w).trigger('click')
    await flushPromises()
    expect(w.find('[data-test="lookup-found"]').exists()).toBe(true)
    await w.setProps({ source: { kind: 'hackernews', target: 'rust async' } })
    expect(w.find('[data-test="lookup-found"]').exists()).toBe(false)
  })

  it('does not ask for Mastodon tags until a server is named, then lists them after a pause and fills the tag on click', async () => {
    vi.useFakeTimers()
    suggestMock.mockResolvedValue({ choices: [{ value: 'golang', label: '#golang', note: '7 posts this week' }] })
    const w = mountLookup({ kind: 'mastodon', target: '', instance: '' })
    await vi.advanceTimersByTimeAsync(600)
    expect(suggestMock).not.toHaveBeenCalled()
    await w.setProps({ source: { kind: 'mastodon', target: '', instance: 'mastodon.social' } })
    await vi.advanceTimersByTimeAsync(600)
    await flushPromises()
    expect(suggestMock).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="lookup-suggestions"]').text()).toContain('Trending on mastodon.social')
    await w.get('[data-test="lookup-suggestion"]').trigger('click')
    expect(w.emitted('pick')![0]![0]).toEqual({ target: 'golang' })
  })

  it('asks for no suggestions for a kind that has none', async () => {
    vi.useFakeTimers()
    mountLookup({ kind: 'telegram', target: 'aipost' })
    await vi.advanceTimersByTimeAsync(1000)
    expect(suggestMock).not.toHaveBeenCalled()
  })
})
