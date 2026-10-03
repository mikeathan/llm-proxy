import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import MemoryReviewDialog from '../../../../components/AgentIde/memory/MemoryReviewDialog.vue'
import type { ReviewItem } from '../../../../types/memory'

const item = (content: string, over: Partial<ReviewItem> = {}): ReviewItem =>
  ({ content, scope: 'workspace', mode: 'on_demand', duplicate: false, selected: true, failed: false, ...over })

const mounted: VueWrapper[] = []
function mountDialog(props: Record<string, unknown> = {}) {
  const w = mount(MemoryReviewDialog, {
    props: { open: true, loading: false, saving: false, error: '', items: [], selectedCount: 0, ...props },
    global: { stubs: { Icon: true } },
    attachTo: document.body,
  })
  mounted.push(w)
  return w
}
const button = (w: VueWrapper, text: RegExp) => w.findAll('button').find((b) => text.test(b.text()))!

describe('MemoryReviewDialog', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('says nothing is saved until the operator presses Save', () => {
    expect(mountDialog().text()).toContain('Nothing is saved until you press Save selected')
  })

  it('shows loading, an error with a way to retry, and an empty result', async () => {
    expect(mountDialog({ loading: true }).get('[role="status"]').text()).toContain('Reading this conversation')

    const failed = mountDialog({ error: 'The model is busy right now. Try again in a moment.' })
    expect(failed.get('[role="alert"]').text()).toContain('The model is busy right now')
    await button(failed, /Try again/).trigger('click')
    expect(failed.emitted('retry')).toHaveLength(1)

    expect(mountDialog().text()).toContain('No suggestions')
  })

  it('lists each fact with where it applies and how it is recalled, and marks what is already saved', () => {
    const w = mountDialog({
      items: [item('We deploy through vertex'), item('Answer briefly', { scope: 'user', mode: 'always' }), item('Already known', { duplicate: true, selected: false })],
      selectedCount: 2,
    })
    const rows = w.findAll('[data-test="review-list"] li')
    expect(rows).toHaveLength(3)
    expect(rows[0]!.text()).toContain('This workspace')
    expect(rows[1]!.text()).toContain('Me, in every workspace')
    expect(rows[1]!.text()).toContain('always in context')
    expect(rows[2]!.text()).toContain('Already saved')
    expect(rows[2]!.get('input').attributes('disabled')).toBeDefined()
  })

  it('reports ticks by position, and saves only through the button, which counts what is ticked', async () => {
    const w = mountDialog({ items: [item('A fact'), item('Another fact', { selected: false })], selectedCount: 1 })
    await w.findAll('input')[1]!.setValue(true)
    expect(w.emitted('toggle')).toEqual([[1]])
    const save = button(w, /Save selected \(1\)/)
    await save.trigger('click')
    expect(w.emitted('save')).toHaveLength(1)
  })

  it('cannot save with nothing ticked, and shows progress while saving', () => {
    const none = mountDialog({ items: [item('A fact', { selected: false })], selectedCount: 0 })
    expect(button(none, /Save selected \(0\)/).attributes('disabled')).toBeDefined()
    const saving = mountDialog({ items: [item('A fact')], selectedCount: 1, saving: true })
    expect(button(saving, /Saving/).attributes('disabled')).toBeDefined()
  })

  it('marks a fact that failed to save and keeps the error visible', () => {
    const w = mountDialog({ items: [item('A fact', { failed: true })], selectedCount: 1, error: 'Some memories could not be saved.' })
    expect(w.text()).toContain('Could not be saved')
    expect(w.get('[role="alert"]').text()).toContain('could not be saved')
  })

  it('closes from Cancel', async () => {
    const w = mountDialog({ items: [item('A fact')], selectedCount: 1 })
    await button(w, /^Cancel$/).trigger('click')
    expect(w.emitted('update:open')).toEqual([[false]])
  })
})
