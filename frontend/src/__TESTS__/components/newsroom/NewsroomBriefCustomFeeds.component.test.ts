import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { SourcePreview } from '../../../types/newsroom'

const { lookupMock } = vi.hoisted(() => ({ lookupMock: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: { lookup: lookupMock } }))

import NewsroomBriefCustomFeeds from '../../../components/newsroom/NewsroomBriefCustomFeeds.vue'

const preview = (over: Partial<SourcePreview> = {}): SourcePreview => ({ found: true, title: 'Daily Blog', sample: [{ title: 'One' }, { title: 'Two' }], choices: [], ...over })
const URL1 = 'https://blog.example/feed.xml'
const mountIt = (extra: Record<string, unknown> = {}) => mount(NewsroomBriefCustomFeeds, { props: { feeds: [URL1], saving: false, save: vi.fn().mockResolvedValue(true), ...extra } })
const button = (w: ReturnType<typeof mountIt>, text: string) => w.findAll('button').find((b) => b.text() === text)!

beforeEach(() => lookupMock.mockReset())

describe('NewsroomBriefCustomFeeds', () => {
  it('reads a pasted feed once and says what it gives', async () => {
    lookupMock.mockResolvedValue(preview())
    const w = mountIt()
    expect(w.text()).toContain('Checking…')
    await flushPromises()
    expect(lookupMock).toHaveBeenCalledWith({ kind: 'feed', target: URL1 }, expect.anything())
    expect(w.text()).toContain('Reads fine: Daily Blog · 2 recent stories')
    expect(w.text()).toContain('the topic still saves')
  })

  it('keeps the feed in My sources under the name the feed gives itself, else its host', async () => {
    lookupMock.mockResolvedValue(preview({ title: ' Daily Blog ' }))
    const save = vi.fn().mockResolvedValue(true)
    const w = mountIt({ save })
    await flushPromises()
    await button(w, 'Save to my sources').trigger('click')
    expect(save).toHaveBeenCalledWith('Daily Blog', URL1)
    lookupMock.mockResolvedValue(preview({ title: undefined }))
    const w2 = mountIt({ save })
    await flushPromises()
    await button(w2, 'Save to my sources').trigger('click')
    expect(save).toHaveBeenLastCalledWith('blog.example', URL1)
  })

  it('names the problem in words, offers no save, and offers the feeds a page advertises', async () => {
    lookupMock.mockResolvedValue(preview({ found: false, problem: 'That page is not a feed.', choices: [{ value: 'https://blog.example/rss', label: 'Blog RSS' }] }))
    const w = mountIt({ feeds: ['https://blog.example/'] })
    await flushPromises()
    expect(w.text()).toContain('That page is not a feed.')
    expect(w.text()).toContain('This page offers these feeds:')
    expect(w.findAll('button').some((b) => b.text() === 'Save to my sources')).toBe(false)
    await button(w, 'Blog RSS').trigger('click')
    expect(w.emitted('replace')![0]).toEqual(['https://blog.example/', 'https://blog.example/rss'])
  })

  it('shows a failed look-up as words, and shows text from the internet as text', async () => {
    lookupMock.mockRejectedValue(new Error('the look-up is off'))
    const w = mountIt()
    await flushPromises()
    expect(w.text()).toContain('the look-up is off')
    lookupMock.mockResolvedValue(preview({ title: '<img src=x onerror=alert(1)>' }))
    const w2 = mountIt()
    await flushPromises()
    expect(w2.find('img').exists()).toBe(false)
    expect(w2.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('checks a link once while it stays, forgets one taken out, and checks it again when it returns', async () => {
    lookupMock.mockResolvedValue(preview())
    const w = mountIt()
    await flushPromises()
    await w.setProps({ feeds: [URL1, 'https://other.example/f'] })
    await flushPromises()
    expect(lookupMock).toHaveBeenCalledTimes(2)
    await w.setProps({ feeds: ['https://other.example/f'] })
    await w.setProps({ feeds: [URL1, 'https://other.example/f'] })
    await flushPromises()
    expect(lookupMock).toHaveBeenCalledTimes(3)
  })

  it('offers no save button when the page gave none', async () => {
    lookupMock.mockResolvedValue(preview())
    const w = mountIt({ save: undefined })
    await flushPromises()
    expect(w.findAll('button')).toHaveLength(0)
  })
})
