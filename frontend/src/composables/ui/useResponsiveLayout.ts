import { computed, onMounted, onUnmounted, ref } from "vue"
import type { Breakpoint } from "../../types/ui"
import { BREAKPOINTS, DESKTOP_BREAKPOINT } from "../../theme/breakpoints"

const BASE = "base"
// Widest first, so the first matching query names the current breakpoint.
const DESCENDING = (Object.entries(BREAKPOINTS) as [Breakpoint, number][]).sort((a, b) => b[1] - a[1])

/**
 * Named viewport breakpoint (`base` below `sm`) from the shared table that
 * also drives Tailwind's `screens`. Listens to one media query per breakpoint,
 * so it updates only when a boundary is crossed; listeners are removed on unmount.
 */
export function useResponsiveLayout() {
  const queries = DESCENDING.map(([name, px]) => ({ name, mql: window.matchMedia(`(min-width: ${px}px)`) }))
  const breakpoint = ref<Breakpoint | typeof BASE>(BASE)

  function update() {
    breakpoint.value = queries.find((q) => q.mql.matches)?.name ?? BASE
  }
  update()

  const isMobile = computed(() => {
    const current = breakpoint.value
    return current === BASE || BREAKPOINTS[current] < BREAKPOINTS[DESKTOP_BREAKPOINT]
  })

  onMounted(() => queries.forEach((q) => q.mql.addEventListener("change", update)))
  onUnmounted(() => queries.forEach((q) => q.mql.removeEventListener("change", update)))

  return { breakpoint, isMobile }
}
