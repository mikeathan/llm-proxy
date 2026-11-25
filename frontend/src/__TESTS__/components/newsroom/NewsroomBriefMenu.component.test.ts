import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import NewsroomBriefMenu from '../../../components/newsroom/NewsroomBriefMenu.vue'

let w: VueWrapper | null = null
afterEach(() => {
  w?.unmount()
  w = null
})
const show = (olderCount: number, writing = false) =>
  (w = mount(NewsroomBriefMenu, { props: { olderCount, editionName: 'Morning', writing }, attachTo: document.body, global: { stubs: { Icon: true } } }))

describe('NewsroomBriefMenu', () => {
  it('holds the rare actions of a brief: write a new one, delete this one, delete the older ones', async () => {
    const m = show(3)
    expect(m.get('[data-test="brief-more"]').attributes('aria-label')).toBe('More for this brief')
    await m.get('[data-test="brief-more"]').trigger('click')
    expect(m.text()).toContain('Write a new one now')
    expect(m.text()).toContain('Delete this brief')
    expect(m.text()).toContain('Delete every older Morning brief')
    await m.get('[data-test="write-now"]').trigger('click')
    await m.get('[data-test="brief-more"]').trigger('click')
    await m.get('[data-test="brief-delete-older"]').trigger('click')
    await m.get('[data-test="brief-more"]').trigger('click')
    await m.get('[data-test="brief-delete-this"]').trigger('click')
    expect(m.emitted('write')).toHaveLength(1)
    expect(m.emitted('delete')).toEqual([[true], [false]])
  })

  it('offers no older ones when there are none, and no second write while one runs', async () => {
    const m = show(0, true)
    await m.get('[data-test="brief-more"]').trigger('click')
    expect(m.find('[data-test="brief-delete-older"]').exists()).toBe(false)
    expect(m.get('[data-test="write-now"]').attributes('disabled')).toBeDefined()
  })
})
