import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomEntryItem from '../../../components/newsroom/NewsroomEntryItem.vue'
import type { NewsroomEntry } from '../../../types/newsroom'

const entry = (over: Partial<NewsroomEntry> = {}): NewsroomEntry => ({
  id: 1, watch_id: 1, at: '2026-11-20T10:00:00Z', source: 'go.dev', title: 'Go 1.26 released', link: 'https://go.dev/blog/go1.26', ...over,
})
const show = (e: NewsroomEntry) => mount(NewsroomEntryItem, { props: { entry: e }, global: { stubs: { Icon: true } } })

describe('NewsroomEntryItem', () => {
  it('opens the source in a new tab without handing it the page', () => {
    const a = show(entry()).get('a')
    expect(a.attributes('href')).toBe('https://go.dev/blog/go1.26')
    expect(a.attributes('target')).toBe('_blank')
    expect(a.attributes('rel')).toBe('noopener noreferrer')
  })

  it('never makes a link of an address that is not http or https', () => {
    const w = show(entry({ link: 'javascript:alert(1)', also_seen: [{ source: 'evil', link: 'data:text/html,x', at: '2026-11-20T11:00:00Z' }] }))
    expect(w.find('a').exists()).toBe(false)
    expect(w.text()).toContain('Go 1.26 released')
    expect(w.text()).toContain('evil')
  })

  it('shows text from the internet as text, never as markup', () => {
    const w = show(entry({ title: '<img src=x onerror=alert(1)>', text: '<script>alert(1)</script>', link: undefined }))
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('script').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('says a finding is new, or what changed, and that it is unread', () => {
    const fresh = show(entry())
    expect(fresh.text()).toContain('New')
    expect(fresh.text()).toContain('Unread')
    const changed = show(entry({ was: 'beta', now: 'stable', read: true }))
    expect(changed.text()).toContain('Changed')
    expect(changed.text()).toContain('beta → stable')
    expect(changed.text()).not.toContain('Unread')
  })
})
