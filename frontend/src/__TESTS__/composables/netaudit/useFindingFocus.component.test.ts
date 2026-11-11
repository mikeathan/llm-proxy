import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useFindingFocus } from '../../../composables/netaudit/useFindingFocus'

const scrolled: string[] = []
const rows = ['192.168.50.7|tcp|22|ssh_password_login', '192.168.50.7|tcp|80|http_no_https', '192.168.50.9|tcp|23|telnet_open']

async function harness(query: Record<string, string>, ready = true) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })
  await router.push({ path: '/', query })
  const isReady = ref(ready)
  let result!: ReturnType<typeof useFindingFocus>
  const Probe = defineComponent({
    setup() {
      const root = ref<HTMLElement | null>(null)
      result = useFindingFocus(root, isReady)
      return () => h('div', { ref: root }, rows.map((key) => h('p', { key, 'data-key': key })))
    },
  })
  const w = mount(Probe, { global: { plugins: [router] }, attachTo: document.body })
  await nextTick()
  await nextTick()
  return { w, result, isReady, router }
}

describe('useFindingFocus', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    scrolled.length = 0
    Element.prototype.scrollIntoView = function scroll(this: Element) { scrolled.push(this.getAttribute('data-key') ?? '') }
  })
  afterEach(() => { vi.useRealTimers(); document.body.innerHTML = '' })

  it('scrolls to the one finding a link named and marks only it', async () => {
    const { w, result } = await harness({ finding: rows[1]! })
    expect(scrolled).toEqual([rows[1]])
    expect(w.findAll('[data-flash]').map((e) => e.attributes('data-key'))).toEqual([rows[1]])
    expect(result.missing.value).toBe(false)
  })

  it('marks every finding of a device and scrolls to the first', async () => {
    const { w } = await harness({ device: '192.168.50.7' })
    expect(scrolled).toEqual([rows[0]])
    expect(w.findAll('[data-flash]')).toHaveLength(2)
  })

  it('does not take 192.168.50.7 for 192.168.50.70', async () => {
    const { w, result } = await harness({ device: '192.168.50.70' })
    expect(w.findAll('[data-flash]')).toHaveLength(0)
    expect(result.missing.value).toBe(true)
  })

  it('says a finding is missing when the report has none like it, and scrolls nowhere', async () => {
    const { result } = await harness({ finding: '10.0.0.1|tcp|22|gone' })
    expect(result.missing.value).toBe(true)
    expect(scrolled).toEqual([])
  })

  it('does nothing without a link, and waits for the report to be drawn', async () => {
    const plain = await harness({})
    expect(plain.result.missing.value).toBe(false)
    expect(scrolled).toEqual([])
    const waiting = await harness({ finding: rows[0]! }, false)
    expect(scrolled).toEqual([])
    waiting.isReady.value = true
    await nextTick()
    await nextTick()
    expect(scrolled).toEqual([rows[0]])
  })

  it('takes the mark off after a few seconds', async () => {
    const { w } = await harness({ finding: rows[0]! })
    expect(w.findAll('[data-flash]')).toHaveLength(1)
    vi.advanceTimersByTime(3500)
    expect(w.findAll('[data-flash]')).toHaveLength(0)
  })

  it('moves to the next finding when the link changes on the same page', async () => {
    const { w, router } = await harness({ finding: rows[0]! })
    await router.push({ path: '/', query: { finding: rows[2]! } })
    await nextTick()
    await nextTick()
    expect(scrolled).toEqual([rows[0], rows[2]])
    expect(w.findAll('[data-flash]').map((e) => e.attributes('data-key'))).toEqual([rows[2]])
  })
})
