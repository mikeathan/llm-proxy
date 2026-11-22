import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NewsroomWatchRow from '../../../components/newsroom/NewsroomWatchRow.vue'
import type { NewsroomWatch } from '../../../types/newsroom'

const watch = (over: Partial<NewsroomWatch>): NewsroomWatch => ({ id: 1, name: 'AI news', interval: '1h0m0s', max_age: '72h0m0s', enabled: true, sources: [], ...over })

describe('NewsroomWatchRow', () => {
  it('names each source the last check could not read, even when the check itself found nothing new', () => {
    const w = mount(NewsroomWatchRow, { props: { busy: false, watch: watch({ last_status: 'nothing_new', last_problems: ['telegram X: this channel has no public preview, so it cannot be watched'] }) } })
    const warning = w.get('[data-test="watch-problems"]')
    expect(warning.text()).toContain('telegram X: this channel has no public preview')
    expect(warning.attributes('role')).toBe('status')
  })

  it('shows no warning block when every source was read', () => {
    const w = mount(NewsroomWatchRow, { props: { busy: false, watch: watch({ last_status: 'nothing_new' }) } })
    expect(w.find('[data-test="watch-problems"]').exists()).toBe(false)
  })
})
