import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { useScrollEdges } from '../../../composables/ui/useScrollEdges'

// A strip that is 100px wide showing 300px of content.
function strip(scrollLeft: number) {
  const el = document.createElement('div')
  Object.defineProperty(el, 'clientWidth', { value: 100 })
  Object.defineProperty(el, 'scrollWidth', { value: 300 })
  el.scrollLeft = scrollLeft
  return el
}

function harness(el: HTMLElement) {
  let edges!: ReturnType<typeof useScrollEdges>
  const Host = defineComponent({
    setup() {
      const target = ref<HTMLElement | null>(el)
      edges = useScrollEdges(target)
      return () => h('div')
    },
  })
  const w = mount(Host)
  return { w, edges: () => edges }
}

describe('useScrollEdges', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reports more content past the end while the strip is not scrolled to it', () => {
    const { edges } = harness(strip(0))
    expect(edges().moreAfter.value).toBe(true)
  })

  it('reports none once scrolled to the end, and content before the start', () => {
    const el = strip(200)
    const { edges } = harness(el)
    expect(edges().moreAfter.value).toBe(false)
    expect(edges().moreBefore.value).toBe(true)
  })

  it('reports nothing before the start while the strip is at its start', () => {
    const { edges } = harness(strip(0))
    expect(edges().moreBefore.value).toBe(false)
  })

  it('updates as the strip scrolls', () => {
    const el = strip(0)
    const { edges } = harness(el)
    el.scrollLeft = 200
    el.dispatchEvent(new Event('scroll'))
    expect(edges().moreAfter.value).toBe(false)
  })

  it('stops listening and observing when the component unmounts', () => {
    const disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class { observe = vi.fn(); disconnect = disconnect })
    const el = strip(0)
    const remove = vi.spyOn(el, 'removeEventListener')
    const { w } = harness(el)
    w.unmount()
    expect(remove).toHaveBeenCalledWith('scroll', expect.any(Function))
    expect(disconnect).toHaveBeenCalled()
  })
})
