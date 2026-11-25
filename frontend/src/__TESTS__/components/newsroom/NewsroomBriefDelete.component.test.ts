import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import NewsroomBriefDelete from '../../../components/newsroom/NewsroomBriefDelete.vue'

let w: VueWrapper | null = null
afterEach(() => {
  w?.unmount()
  w = null
})
const show = (olderCount: number) => (w = mount(NewsroomBriefDelete, { props: { olderCount, editionName: 'Morning' }, attachTo: document.body, global: { stubs: { Icon: true } } }))

describe('NewsroomBriefDelete', () => {
  it('offers to delete this brief and the older ones of its edition', async () => {
    const m = show(3)
    await m.get('[data-test="brief-delete"]').trigger('click')
    expect(m.text()).toContain('Delete this brief')
    expect(m.text()).toContain('Delete every older Morning brief')
    await m.get('[data-test="brief-delete-older"]').trigger('click')
    await m.get('[data-test="brief-delete"]').trigger('click')
    await m.get('[data-test="brief-delete-this"]').trigger('click')
    expect(m.emitted('delete')).toEqual([[true], [false]])
  })

  it('offers no older ones when there are none', async () => {
    const m = show(0)
    await m.get('[data-test="brief-delete"]').trigger('click')
    expect(m.find('[data-test="brief-delete-older"]').exists()).toBe(false)
  })
})
