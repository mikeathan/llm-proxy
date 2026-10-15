import { onBeforeUnmount, onMounted, ref, type Ref } from "vue"

// A horizontally scrolling strip (the Settings tabs on a phone) has more to show
// when an end is not in view; the caller fades that edge so the cut-off reads as
// "keep scrolling" and not as a clipped label. Listeners and the observer are
// removed with the component.
const EDGE_TOLERANCE_PX = 4

export function useScrollEdges(target: Ref<HTMLElement | null>) {
  const moreBefore = ref(false)
  const moreAfter = ref(false)
  let observer: ResizeObserver | null = null

  function update() {
    const el = target.value
    moreBefore.value = !!el && el.scrollLeft > EDGE_TOLERANCE_PX
    moreAfter.value = !!el && el.scrollLeft + el.clientWidth < el.scrollWidth - EDGE_TOLERANCE_PX
  }

  onMounted(() => {
    const el = target.value
    update()
    if (!el) return
    el.addEventListener("scroll", update, { passive: true })
    if (typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver(update)
      observer.observe(el)
    }
  })

  onBeforeUnmount(() => {
    target.value?.removeEventListener("scroll", update)
    observer?.disconnect()
  })

  return { moreBefore, moreAfter, update }
}
