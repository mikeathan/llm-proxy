import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomEntryItem from '../../../components/newsroom/NewsroomEntryItem.vue'
import type { NewsroomEntry } from '../../../types/newsroom'

const entry = (over: Partial<NewsroomEntry> = {}): NewsroomEntry => ({
  id: 1, watch_id: 1, at: '2026-11-20T10:00:00Z', source: 'go.dev', title: 'Go 1.26 released', link: 'https://go.dev/blog/go1.26', ...over,
})
const show = (e: NewsroomEntry, props: Record<string, unknown> = {}) => mount(NewsroomEntryItem, { props: { entry: e, ...props }, global: { stubs: { Icon: true } } })
const openDetails = async (w: ReturnType<typeof show>) => {
  await w.get('button[aria-expanded]').trigger('click')
  return w
}

// The real Telegram entry that defeated the first version of the duplicate-text fix (2026-11-21): the title has its line breaks collapsed and the text keeps them.
const NEURALINK_TEXT = "😎 A Neuralink patient is already playing GTA VI, using only his brain implant\n\nNeuralink posted the two-minute clip on Friday, a day after the game came out."
const NEURALINK_TITLE = '😎 A Neuralink patient is already playing GTA VI, using only his brain implant Neuralink posted the two-minute clip on Friday, a day after the game came out.'

describe('NewsroomEntryItem headline and summary', () => {
  const digested = entry({ title: 'OpenAI GPT-Live-1.1 listens while it talks, a long original post title', headline: 'GPT-Live-1.1 listens while it talks', summary: 'A full-duplex voice model in the API.', why: 'New model: GPT-Live-1.1' })

  it('shows the short headline as the row title and keeps the link', () => {
    const w = show(digested)
    expect(w.get('a').text()).toBe('GPT-Live-1.1 listens while it talks')
    expect(w.get('a').attributes('href')).toBe('https://go.dev/blog/go1.26')
    expect(w.text()).not.toContain('long original post title')
  })

  it('opens to the summary, the reason and the original title', async () => {
    const w = await openDetails(show(digested))
    expect(w.text()).toContain('A full-duplex voice model in the API.')
    expect(w.text()).toContain('Why: New model: GPT-Live-1.1')
    expect(w.text()).toContain('Original: OpenAI GPT-Live-1.1 listens while it talks, a long original post title')
  })

  it('falls back to the original title when there is no headline', () => {
    expect(show(entry()).get('a').text()).toBe('Go 1.26 released')
  })
})

describe('NewsroomEntryItem', () => {
  it('opens the source in a new tab without handing it the page, and counts that as reading it', async () => {
    const w = show(entry())
    const a = w.get('a')
    expect(a.attributes('href')).toBe('https://go.dev/blog/go1.26')
    expect(a.attributes('target')).toBe('_blank')
    expect(a.attributes('rel')).toBe('noopener noreferrer')
    await a.trigger('click')
    expect(w.emitted('open')).toHaveLength(1)
  })

  it('counts opening the details as reading it, once', async () => {
    const w = await openDetails(show(entry({ summary: 'What happened.' })))
    expect(w.emitted('open')).toHaveLength(1)
    await w.get('button[aria-expanded]').trigger('click')
    expect(w.emitted('open')).toHaveLength(1)
  })

  it('says why the model left an item out, on the row itself', () => {
    const w = show(entry({ left_out: true, why: 'a joke, not a release' }))
    expect(w.text()).toContain('Left out: a joke, not a release')
  })

  it('never makes a link of an address that is not http or https', async () => {
    const w = await openDetails(show(entry({ link: 'javascript:alert(1)', also_seen: [{ source: 'evil', link: 'data:text/html,x', at: '2026-11-20T11:00:00Z' }] })))
    expect(w.find('a').exists()).toBe(false)
    expect(w.text()).toContain('Go 1.26 released')
    expect(w.text()).toContain('evil')
  })

  it('shows text from the internet as text, never as markup', async () => {
    const w = await openDetails(show(entry({ title: '<img src=x onerror=alert(1)>', text: '<script>alert(1)</script>', link: undefined })))
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('script').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('does not say a post twice, even when the text has line breaks the title lost', async () => {
    const real = show(entry({ title: NEURALINK_TITLE, text: NEURALINK_TEXT, link: undefined }))
    expect(real.text().split('A Neuralink patient').length - 1).toBe(1)
    // The row shows a short fallback headline, so the rest of the post is behind the button, once.
    const opened = await openDetails(real)
    expect(opened.text()).toContain('a day after the game came out.')
    expect(opened.text().split('Neuralink posted the two-minute clip').length - 1).toBe(1)
    const clipped = await openDetails(show(entry({ title: NEURALINK_TITLE.slice(0, 60) + '…', text: NEURALINK_TEXT, link: undefined })))
    expect(clipped.text()).toContain('Neuralink posted the two-minute clip')
    const other = await openDetails(show(entry({ text: 'A different summary line' })))
    expect(other.text()).toContain('A different summary line')
  })

  it('has no details button when there is nothing more to show, and folds the details until asked', async () => {
    expect(show(entry()).find('button[aria-expanded]').exists()).toBe(false)
    const w = show(entry({ why: 'a new model release' }))
    expect(w.text()).not.toContain('Why: a new model release')
    await openDetails(w)
    expect(w.text()).toContain('Why: a new model release')
  })

  it('marks an unread finding with a dot and no "New" pill, says what changed, and shows nothing unread once read', async () => {
    const fresh = show(entry())
    expect(fresh.text()).toContain('Unread')
    expect(fresh.text()).not.toContain('New')
    const changed = await openDetails(show(entry({ was: 'beta', now: 'stable', read: true })))
    expect(changed.text()).toContain('Changed')
    expect(changed.text()).toContain('beta → stable')
    expect(changed.text()).not.toContain('Unread')
    expect(changed.text()).not.toContain('New')
  })

  it('names the watch when asked, and shows a checkbox only while selecting', async () => {
    const w = show(entry(), { watchName: 'AI news' })
    expect(w.text()).toContain('AI news')
    expect(w.find('input[type=checkbox]').exists()).toBe(false)
    const s = show(entry(), { selectable: true, selected: true })
    const box = s.get('input[type=checkbox]')
    expect((box.element as HTMLInputElement).checked).toBe(true)
    await box.trigger('change')
    expect(s.emitted('toggle-select')).toHaveLength(1)
  })
})

describe('NewsroomEntryItem posted date', () => {
  it('says when the post was published, not when the watch found it', () => {
    const w = show(entry({ at: '2026-11-21T17:10:00Z', posted: '2026-10-17T09:00:00Z' }))
    expect(w.get('time').attributes('datetime')).toBe('2026-10-17T09:00:00Z')
    expect(w.get('time').text()).toContain('posted')
  })

  it('falls back to when it was found when the source gave no date', () => {
    const w = show(entry({ at: '2026-11-21T17:10:00Z' }))
    expect(w.get('time').attributes('datetime')).toBe('2026-11-21T17:10:00Z')
    expect(w.get('time').text()).not.toContain('posted')
  })
})
